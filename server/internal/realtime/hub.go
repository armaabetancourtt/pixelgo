package realtime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/armaabetancourtt/pixelgo/server/internal/presence"
	"github.com/coder/websocket"
)

const (
	presenceTTL      = 45 * time.Second
	heartbeatEvery   = 15 * time.Second
	presenceTimeout  = 2 * time.Second
)

type Event struct {
	Type       string    `json:"type"`
	OccurredAt time.Time `json:"occurredAt"`
	Payload    any       `json:"payload"`
}

type client struct {
	deviceID string
	leaseID  string
}

type Hub struct {
	mu       sync.RWMutex
	clients  map[*websocket.Conn]client
	presence presence.Store
}

func NewHub(stores ...presence.Store) *Hub {
	var store presence.Store = presence.NewMemoryStore()
	if len(stores) > 0 && stores[0] != nil {
		store = stores[0]
	}
	return &Hub{
		clients:  make(map[*websocket.Conn]client),
		presence: store,
	}
}

func (h *Hub) Publish(eventType string, payload any) {
	event := Event{
		Type:       eventType,
		OccurredAt: time.Now().UTC(),
		Payload:    payload,
	}
	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	h.mu.RLock()
	clients := make([]*websocket.Conn, 0, len(h.clients))
	for conn := range h.clients {
		clients = append(clients, conn)
	}
	h.mu.RUnlock()

	for _, conn := range clients {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = conn.Write(ctx, websocket.MessageText, data)
		cancel()
	}
}

func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	deviceID := strings.TrimSpace(r.URL.Query().Get("deviceId"))
	if deviceID == "" {
		http.Error(w, "deviceId is required", http.StatusBadRequest)
		return
	}

	conn, err := websocket.Accept(
		w,
		r,
		&websocket.AcceptOptions{InsecureSkipVerify: true},
	)
	if err != nil {
		return
	}

	leaseID, err := newLeaseID()
	if err != nil {
		_ = conn.Close(websocket.StatusInternalError, "presence lease failed")
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	wasOnline := h.isOnline(ctx, deviceID)
	if err := h.presence.Touch(ctx, deviceID, leaseID, presenceTTL); err != nil {
		_ = conn.Close(websocket.StatusInternalError, "presence unavailable")
		return
	}

	h.mu.Lock()
	h.clients[conn] = client{deviceID: deviceID, leaseID: leaseID}
	h.mu.Unlock()

	if !wasOnline {
		h.Publish("device.online", map[string]string{"deviceId": deviceID})
	}

	go h.heartbeat(ctx, deviceID, leaseID)

	defer func() {
		cancel()

		h.mu.Lock()
		delete(h.clients, conn)
		h.mu.Unlock()

		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), presenceTimeout)
		defer cleanupCancel()

		_ = h.presence.Remove(cleanupCtx, deviceID, leaseID)
		if !h.isOnline(cleanupCtx, deviceID) {
			h.Publish("device.offline", map[string]string{"deviceId": deviceID})
		}

		_ = conn.Close(websocket.StatusNormalClosure, "bye")
	}()

	for {
		if _, _, err := conn.Read(ctx); err != nil {
			return
		}
	}
}

func (h *Hub) heartbeat(ctx context.Context, deviceID, leaseID string) {
	ticker := time.NewTicker(heartbeatEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = h.presence.Touch(ctx, deviceID, leaseID, presenceTTL)
		}
	}
}

func (h *Hub) isOnline(ctx context.Context, deviceID string) bool {
	state, err := h.presence.Online(ctx, []string{deviceID})
	if err != nil {
		return false
	}
	return state[deviceID]
}

func newLeaseID() (string, error) {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "lease_" + hex.EncodeToString(b[:]), nil
}
