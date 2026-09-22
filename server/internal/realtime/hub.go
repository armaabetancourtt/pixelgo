package realtime

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/armaabetancourtt/pixelgo/server/internal/auth"
	"github.com/armaabetancourtt/pixelgo/server/internal/presence"
	"github.com/coder/websocket"
)

const defaultPresenceTTL = 45 * time.Second

type Event struct {
	Type       string    `json:"type"`
	OccurredAt time.Time `json:"occurredAt"`
	Payload    any       `json:"payload"`
}

type routeTargets struct {
	DeviceIDs []string `json:"deviceIds,omitempty"`
	UserIDs   []string `json:"userIds,omitempty"`
}

type brokerEnvelope struct {
	Targets routeTargets `json:"targets"`
	Event   Event        `json:"event"`
}

type clientIdentity struct {
	deviceID string
	userID   string
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
	clients  map[*websocket.Conn]clientIdentity
	broker   Broker
	presence presence.Store
	start    sync.Once
}

func NewHub(options ...Option) *Hub {
	h := &Hub{
		clients: make(map[*websocket.Conn]clientIdentity),
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
			_ = h.broker.Subscribe(ctx, h.handleBrokerMessage)
		}()
	})
}

func (h *Hub) PublishToDevices(
	deviceIDs []string,
	eventType string,
	payload any,
) {
	h.publish(
		routeTargets{DeviceIDs: compactTargets(deviceIDs)},
		eventType,
		payload,
	)
}

func (h *Hub) publishToUsers(
	userIDs []string,
	eventType string,
	payload any,
) {
	h.publish(
		routeTargets{UserIDs: compactTargets(userIDs)},
		eventType,
		payload,
	)
}

func (h *Hub) publish(
	targets routeTargets,
	eventType string,
	payload any,
) {
	if len(targets.DeviceIDs) == 0 && len(targets.UserIDs) == 0 {
		return
	}

	envelope := brokerEnvelope{
		Targets: targets,
		Event: Event{
			Type:       eventType,
			OccurredAt: time.Now().UTC(),
			Payload:    payload,
		},
	}
	data, err := json.Marshal(envelope)
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

	h.route(envelope)
}

func (h *Hub) handleBrokerMessage(data []byte) {
	var envelope brokerEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return
	}
	if envelope.Event.Type == "" {
		return
	}
	h.route(envelope)
}

func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	deviceID := r.URL.Query().Get("deviceId")
	if deviceID == "" {
		http.Error(w, "deviceId query parameter is required", http.StatusBadRequest)
		return
	}

	identity := clientIdentity{
		deviceID: deviceID,
		userID:   auth.UserID(r.Context()),
	}

	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		return
	}

	h.mu.Lock()
	h.clients[c] = identity
	h.mu.Unlock()

	if h.presence != nil {
		_ = h.presence.Online(r.Context(), deviceID, defaultPresenceTTL)
	}
	h.publishPresence(identity, "device.online")

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
		h.publishPresence(identity, "device.offline")
		_ = c.Close(websocket.StatusNormalClosure, "bye")
	}()

	for {
		if _, _, err := c.Read(r.Context()); err != nil {
			return
		}
	}
}

func (h *Hub) publishPresence(identity clientIdentity, eventType string) {
	payload := map[string]string{"deviceId": identity.deviceID}
	if identity.userID != "" {
		h.publishToUsers([]string{identity.userID}, eventType, payload)
		return
	}

	// Authentication can be optional in isolated local development. In that
	// mode, never broadcast presence globally; target only the current device.
	h.PublishToDevices([]string{identity.deviceID}, eventType, payload)
}

func (h *Hub) refreshPresence(
	c *websocket.Conn,
	deviceID string,
	done <-chan struct{},
) {
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
				_ = c.Close(
					websocket.StatusGoingAway,
					"presence heartbeat failed",
				)
				return
			}
		}
	}
}

func (h *Hub) route(envelope brokerEnvelope) {
	h.mu.RLock()
	clients := make([]*websocket.Conn, 0, len(h.clients))
	for connection, identity := range h.clients {
		if targetMatches(identity, envelope.Targets) {
			clients = append(clients, connection)
		}
	}
	h.mu.RUnlock()

	if len(clients) == 0 {
		return
	}

	data, err := json.Marshal(envelope.Event)
	if err != nil {
		return
	}

	for _, connection := range clients {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = connection.Write(ctx, websocket.MessageText, data)
		cancel()
	}
}

func targetMatches(identity clientIdentity, targets routeTargets) bool {
	for _, deviceID := range targets.DeviceIDs {
		if deviceID != "" && deviceID == identity.deviceID {
			return true
		}
	}
	if identity.userID == "" {
		return false
	}
	for _, userID := range targets.UserIDs {
		if userID != "" && userID == identity.userID {
			return true
		}
	}
	return false
}

func compactTargets(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
