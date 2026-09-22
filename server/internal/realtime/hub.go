package realtime

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/armaabetancourtt/pixelgo/server/internal/presence"
	"github.com/coder/websocket"
)

const defaultPresenceTTL = 45 * time.Second

type Event struct {
	Type       string    `json:"type"`
	OccurredAt time.Time `json:"occurredAt"`
	Payload    any       `json:"payload"`
}

type Option func(*Hub)

func WithBroker(broker Broker) Option {
	return func(h *Hub) {
		h.broker = broker
	}
}

func WithPresence(store presence.Store) Option {
	return func(h *Hub) {
		h.presence = store
	}
}

type Hub struct {
	mu       sync.RWMutex
	clients  map[*websocket.Conn]string
	broker   Broker
	presence presence.Store
	start    sync.Once
}

func NewHub(options ...Option) *Hub {
	h := &Hub{
		clients: make(map[*websocket.Conn]string),
	}
	for _, option := range options {
		option(h)
	}
	return h
}

func (h *Hub) Start(ctx context.Context) {
	if h.broker == nil {
		return
	}
	h.start.Do(func() {
		go func() {
			_ = h.broker.Subscribe(ctx, h.broadcast)
		}()
	})
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

	if h.broker != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := h.broker.Publish(ctx, data)
		cancel()
		if err == nil {
			return
		}
	}

	h.broadcast(data)
}

func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	deviceID := r.URL.Query().Get("deviceId")
	if deviceID == "" {
		http.Error(w, "deviceId query parameter is required", http.StatusBadRequest)
		return
	}

	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		return
	}

	h.mu.Lock()
	h.clients[c] = deviceID
	h.mu.Unlock()

	if h.presence != nil {
		_ = h.presence.Online(r.Context(), deviceID, defaultPresenceTTL)
	}
	h.Publish("device.online", map[string]string{"deviceId": deviceID})

	done := make(chan struct{})
	go h.refreshPresence(c, deviceID, done)

	defer func() {
		close(done)

		h.mu.Lock()
		delete(h.clients, c)
		h.mu.Unlock()

		if h.presence != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			_ = h.presence.Offline(ctx, deviceID)
			cancel()
		}
		h.Publish("device.offline", map[string]string{"deviceId": deviceID})
		_ = c.Close(websocket.StatusNormalClosure, "bye")
	}()

	for {
		if _, _, err := c.Read(r.Context()); err != nil {
			return
		}
	}
}

func (h *Hub) refreshPresence(c *websocket.Conn, deviceID string, done <-chan struct{}) {
	ticker := time.NewTicker(defaultPresenceTTL / 3)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := c.Ping(ctx)
			if err == nil && h.presence != nil {
				_ = h.presence.Online(ctx, deviceID, defaultPresenceTTL)
			}
			cancel()
			if err != nil {
				_ = c.Close(websocket.StatusGoingAway, "presence heartbeat failed")
				return
			}
		}
	}
}

func (h *Hub) broadcast(data []byte) {
	h.mu.RLock()
	clients := make([]*websocket.Conn, 0, len(h.clients))
	for c := range h.clients {
		clients = append(clients, c)
	}
	h.mu.RUnlock()

	for _, c := range clients {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = c.Write(ctx, websocket.MessageText, data)
		cancel()
	}
}
