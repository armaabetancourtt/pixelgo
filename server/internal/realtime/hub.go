package realtime

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
)

type Event struct {
	Type       string    `json:"type"`
	OccurredAt time.Time `json:"occurredAt"`
	Payload    any       `json:"payload"`
}

type Hub struct {
	mu      sync.RWMutex
	clients map[*websocket.Conn]struct{}
}

func NewHub() *Hub { return &Hub{clients: map[*websocket.Conn]struct{}{}} }

func (h *Hub) Publish(eventType string, payload any) {
	event := Event{Type: eventType, OccurredAt: time.Now().UTC(), Payload: payload}
	data, err := json.Marshal(event)
	if err != nil { return }

	h.mu.RLock()
	clients := make([]*websocket.Conn, 0, len(h.clients))
	for c := range h.clients { clients = append(clients, c) }
	h.mu.RUnlock()

	for _, c := range clients {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = c.Write(ctx, websocket.MessageText, data)
		cancel()
	}
}

func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil { return }
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.clients, c)
		h.mu.Unlock()
		_ = c.Close(websocket.StatusNormalClosure, "bye")
	}()

	ctx := r.Context()
	for {
		if _, _, err := c.Read(ctx); err != nil { return }
	}
}
