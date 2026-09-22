package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/armaabetancourtt/pixelgo/server/internal/devices"
	"github.com/armaabetancourtt/pixelgo/server/internal/realtime"
	"github.com/armaabetancourtt/pixelgo/server/internal/transfers"
)

type Server struct {
	devices   *devices.Service
	transfers *transfers.Service
	hub       *realtime.Hub
}

func New(devicesService *devices.Service, transferService *transfers.Service, hub *realtime.Hub) http.Handler {
	s := &Server{devices: devicesService, transfers: transferService, hub: hub}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /v1/devices", s.listDevices)
	mux.HandleFunc("POST /v1/devices", s.registerDevice)
	mux.HandleFunc("DELETE /v1/devices/{deviceId}", s.deleteDevice)
	mux.HandleFunc("GET /v1/transfers", s.listTransfers)
	mux.HandleFunc("POST /v1/transfers", s.createTransfer)
	mux.HandleFunc("GET /v1/transfers/{transferId}", s.getTransfer)
	mux.HandleFunc("POST /v1/transfers/{transferId}/uploaded", s.markUploaded)
	mux.HandleFunc("POST /v1/transfers/{transferId}/complete", s.complete)
	mux.Handle("GET /v1/events", hub)

	idempotency := newIdempotencyStore(24 * time.Hour)
	return withJSON(withIdempotency(mux, idempotency))
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) listDevices(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.devices.List(r.Context()))
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
	if err := s.devices.Delete(r.Context(), r.PathValue("deviceId")); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "device not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
	t, err := s.transfers.MarkUploaded(r.Context(), r.PathValue("transferId"))
	s.writeTransferResult(w, t, err)
}

func (s *Server) complete(w http.ResponseWriter, r *http.Request) {
	t, err := s.transfers.Complete(r.Context(), r.PathValue("transferId"))
	s.writeTransferResult(w, t, err)
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
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{"code": code, "message": message})
}
