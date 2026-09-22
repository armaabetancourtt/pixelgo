package realtime

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/armaabetancourtt/pixelgo/server/internal/presence"
	"github.com/coder/websocket"
)

func TestWebSocketConnectionControlsPresenceLease(t *testing.T) {
	store := presence.NewMemoryStore()
	hub := NewHub(store)
	server := httptest.NewServer(hub)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "?deviceId=dev_ios"
	conn, _, err := websocket.Dial(context.Background(), wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}

	waitForPresence(t, store, "dev_ios", true)

	if err := conn.Close(websocket.StatusNormalClosure, "test complete"); err != nil {
		t.Fatal(err)
	}

	waitForPresence(t, store, "dev_ios", false)
}

func TestSecondConnectionKeepsDeviceOnline(t *testing.T) {
	store := presence.NewMemoryStore()
	hub := NewHub(store)
	server := httptest.NewServer(hub)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "?deviceId=dev_android"

	first, _, err := websocket.Dial(context.Background(), wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := websocket.Dial(context.Background(), wsURL, nil)
	if err != nil {
		_ = first.Close(websocket.StatusNormalClosure, "cleanup")
		t.Fatal(err)
	}

	waitForPresence(t, store, "dev_android", true)

	if err := first.Close(websocket.StatusNormalClosure, "first closed"); err != nil {
		t.Fatal(err)
	}
	waitForPresence(t, store, "dev_android", true)

	if err := second.Close(websocket.StatusNormalClosure, "second closed"); err != nil {
		t.Fatal(err)
	}
	waitForPresence(t, store, "dev_android", false)
}

func waitForPresence(t *testing.T, store presence.Store, deviceID string, expected bool) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		state, err := store.Online(context.Background(), []string{deviceID})
		if err != nil {
			t.Fatal(err)
		}
		if state[deviceID] == expected {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("presence for %s did not become %v", deviceID, expected)
}
