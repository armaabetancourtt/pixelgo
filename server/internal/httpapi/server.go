package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/armaabetancourtt/pixelgo/server/internal/auth"
	"github.com/armaabetancourtt/pixelgo/server/internal/devices"
	"github.com/armaabetancourtt/pixelgo/server/internal/files"
	"github.com/armaabetancourtt/pixelgo/server/internal/presence"
	"github.com/armaabetancourtt/pixelgo/server/internal/ratelimit"
	"github.com/armaabetancourtt/pixelgo/server/internal/realtime"
	"github.com/armaabetancourtt/pixelgo/server/internal/transfers"
	"github.com/redis/go-redis/v9"
)

const maxDevBlobBytes int64 = 64 << 20

type Option func(*Server)

func WithPresence(store presence.Store) Option {
	return func(s *Server) {
		s.presence = store
	}
}

func WithRedisIdempotency(client *redis.Client) Option {
	return func(s *Server) {
		s.idempotencyRedis = client
	}
}

func WithRateLimiter(limiter ratelimit.Limiter) Option {
	return func(s *Server) {
		s.rateLimiter = limiter
	}
}

func WithAuth(service *auth.Service, required bool) Option {
	return func(s *Server) {
		s.auth = service
		s.authRequired = required
	}
}

type Server struct {
	devices   *devices.Service
	transfers *transfers.Service
	hub       *realtime.Hub
	files     *files.Service
	presence         presence.Store
	idempotencyRedis *redis.Client
	rateLimiter      ratelimit.Limiter
	auth      *auth.Service
	authRequired     bool
}

func New(
	devicesService *devices.Service,
	transferService *transfers.Service,
	hub *realtime.Hub,
	fileService *files.Service,
	options ...Option,
) http.Handler {
	s := &Server{
		devices:   devicesService,
		transfers: transferService,
		hub:       hub,
		files:     fileService,
	}
	for _, option := range options {
		option(s)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.health)
	if s.auth != nil {
		mux.HandleFunc("POST /v1/auth/register", s.authRegister)
		mux.HandleFunc("POST /v1/auth/login", s.authLogin)
		mux.HandleFunc("POST /v1/auth/refresh", s.authRefresh)
	}
	mux.HandleFunc("GET /v1/devices", s.listDevices)
	mux.HandleFunc("POST /v1/devices", s.registerDevice)
	mux.HandleFunc("DELETE /v1/devices/{deviceId}", s.deleteDevice)
	mux.HandleFunc("GET /v1/transfers", s.listTransfers)
	mux.HandleFunc("POST /v1/transfers", s.createTransfer)
	mux.HandleFunc("GET /v1/transfers/{transferId}", s.getTransfer)
	mux.HandleFunc("POST /v1/transfers/{transferId}/uploaded", s.markUploaded)
	mux.HandleFunc("POST /v1/transfers/{transferId}/complete", s.complete)
	mux.HandleFunc("GET /v1/events", s.events)

	if s.presence != nil {
		mux.HandleFunc("GET /v1/presence/{deviceId}", s.getPresence)
	}

	// Development-only signed blob adapter. Production replaces this boundary
	// with direct object-storage signed URLs.
	mux.HandleFunc("PUT /dev-upload/{transferId}", s.devUpload)
	mux.HandleFunc("GET /dev-download/{transferId}", s.devDownload)

	var handler http.Handler = mux
	if s.rateLimiter != nil {
		handler = withRateLimit(handler, s.rateLimiter)
	}

	if s.idempotencyRedis != nil {
		handler = withRedisIdempotency(handler, s.idempotencyRedis, 24*time.Hour)
	} else {
		idempotency := newIdempotencyStore(24 * time.Hour)
		handler = withIdempotency(handler, idempotency)
	}

	if s.authRequired && s.auth != nil {
		handler = withAuthentication(handler, s.auth)
	}
	return handler
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) listDevices(w http.ResponseWriter, r *http.Request) {
	items, err := s.devices.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "could not list devices")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) registerDevice(w http.ResponseWriter, r *http.Request) {
	var in devices.RegisterInput
	if err := decode(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	d, err := s.devices.Register(r.Context(), in)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_device", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, d)
}

func (s *Server) deleteDevice(w http.ResponseWriter, r *http.Request) {
	err := s.devices.Delete(r.Context(), r.PathValue("deviceId"))
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, devices.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "device not found")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "could not delete device")
	}
}

func (s *Server) getPresence(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("deviceId")
	if _, err := s.devices.Get(r.Context(), deviceID); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "device not found")
		return
	}
	online, err := s.presence.IsOnline(r.Context(), deviceID)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "presence_unavailable", "presence service is unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"deviceId": deviceID,
		"online":   online,
	})
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	deviceID := r.URL.Query().Get("deviceId")
	if deviceID == "" {
		writeError(w, http.StatusBadRequest, "device_required", "deviceId query parameter is required")
		return
	}
	if _, err := s.devices.Get(r.Context(), deviceID); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "device not found")
		return
	}
	s.hub.ServeHTTP(w, r)
}

func (s *Server) listTransfers(w http.ResponseWriter, r *http.Request) {
	items, err := s.transfers.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "could not list transfers")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) createTransfer(w http.ResponseWriter, r *http.Request) {
	var in transfers.CreateInput
	if err := decode(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	t, err := s.transfers.Create(r.Context(), in)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_transfer", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) getTransfer(w http.ResponseWriter, r *http.Request) {
	t, err := s.transfers.Get(r.Context(), r.PathValue("transferId"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "transfer not found")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) markUploaded(w http.ResponseWriter, r *http.Request) {
	transferID := r.PathValue("transferId")
	if !s.files.Exists(transferID) {
		writeError(w, http.StatusConflict, "upload_missing", "payload has not been uploaded")
		return
	}

	t, err := s.transfers.MarkUploaded(r.Context(), transferID)
	s.writeTransferResult(w, t, err)
}

func (s *Server) complete(w http.ResponseWriter, r *http.Request) {
	t, err := s.transfers.Complete(r.Context(), r.PathValue("transferId"))
	s.writeTransferResult(w, t, err)
}

func (s *Server) devUpload(w http.ResponseWriter, r *http.Request) {
	transferID := r.PathValue("transferId")
	if err := s.files.Verify(
		"upload",
		transferID,
		r.URL.Query().Get("exp"),
		r.URL.Query().Get("sig"),
	); err != nil {
		writeError(w, http.StatusForbidden, "invalid_upload_url", err.Error())
		return
	}

	t, err := s.transfers.Get(r.Context(), transferID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "transfer not found")
		return
	}
	if t.Status != transfers.StatusUploading {
		writeError(w, http.StatusConflict, "invalid_transition", "transfer is not accepting uploads")
		return
	}
	if t.SizeBytes > maxDevBlobBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "dev_blob_too_large", "local in-memory adapter is limited to 64 MiB")
		return
	}

	payload, err := io.ReadAll(io.LimitReader(r.Body, t.SizeBytes+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_upload", "could not read upload")
		return
	}
	if int64(len(payload)) != t.SizeBytes {
		writeError(w, http.StatusUnprocessableEntity, "size_mismatch", "uploaded byte count does not match transfer metadata")
		return
	}

	sum := sha256.Sum256(payload)
	actualChecksum := hex.EncodeToString(sum[:])
	if !strings.EqualFold(actualChecksum, t.SHA256) {
		writeError(w, http.StatusUnprocessableEntity, "checksum_mismatch", "uploaded payload failed SHA-256 verification")
		return
	}

	s.files.Put(transferID, payload)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) devDownload(w http.ResponseWriter, r *http.Request) {
	transferID := r.PathValue("transferId")
	if err := s.files.Verify(
		"download",
		transferID,
		r.URL.Query().Get("exp"),
		r.URL.Query().Get("sig"),
	); err != nil {
		writeError(w, http.StatusForbidden, "invalid_download_url", err.Error())
		return
	}

	t, err := s.transfers.Get(r.Context(), transferID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "transfer not found")
		return
	}
	if t.Status != transfers.StatusReady &&
		t.Status != transfers.StatusDownloading &&
		t.Status != transfers.StatusCompleted {
		writeError(w, http.StatusConflict, "invalid_transition", "transfer is not ready for download")
		return
	}

	payload, err := s.files.Get(transferID)
	if err != nil {
		writeError(w, http.StatusNotFound, "payload_not_found", "transfer payload is unavailable")
		return
	}

	contentType := t.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) writeTransferResult(w http.ResponseWriter, t transfers.Transfer, err error) {
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, t)
	case errors.Is(err, transfers.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "transfer not found")
	case errors.Is(err, transfers.ErrInvalidTransition):
		writeError(w, http.StatusConflict, "invalid_transition", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "transfer update failed")
	}
}

func decode(r *http.Request, v any) error {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return errors.New("content-type must be application/json")
	}
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func withJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{"code": code, "message": message})
}
