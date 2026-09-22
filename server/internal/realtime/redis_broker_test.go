package realtime

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/armaabetancourtt/pixelgo/server/internal/platform/redisdb"
)

func TestRedisBrokerFanout(t *testing.T) {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		t.Skip("REDIS_URL is not configured")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, err := redisdb.Open(ctx, redisURL)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	broker := NewRedisBroker(client)
	received := make(chan string, 1)

	go func() {
		_ = broker.Subscribe(ctx, func(payload []byte) {
			select {
			case received <- string(payload):
			default:
			}
		})
	}()

	deadline := time.After(2 * time.Second)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case got := <-received:
			if got != "pixelgo-distributed-event" {
				t.Fatalf("unexpected payload %q", got)
			}
			return
		case <-ticker.C:
			if err := broker.Publish(ctx, []byte("pixelgo-distributed-event")); err != nil {
				t.Fatal(err)
			}
		case <-deadline:
			t.Fatal("timed out waiting for Redis pubsub fanout")
		}
	}
}
