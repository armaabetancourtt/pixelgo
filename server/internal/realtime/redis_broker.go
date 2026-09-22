package realtime

import (
	"context"

	"github.com/redis/go-redis/v9"
)

type Broker interface {
	Publish(ctx context.Context, payload []byte) error
	Subscribe(ctx context.Context, handler func([]byte)) error
}

type RedisBroker struct {
	client  *redis.Client
	channel string
}

func NewRedisBroker(client *redis.Client) *RedisBroker {
	return &RedisBroker{
		client:  client,
		channel: "pixelgo:events",
	}
}

func (b *RedisBroker) Publish(ctx context.Context, payload []byte) error {
	return b.client.Publish(ctx, b.channel, payload).Err()
}

func (b *RedisBroker) Subscribe(ctx context.Context, handler func([]byte)) error {
	pubsub := b.client.Subscribe(ctx, b.channel)
	defer pubsub.Close()

	if _, err := pubsub.Receive(ctx); err != nil {
		return err
	}

	channel := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case message, ok := <-channel:
			if !ok {
				return nil
			}
			handler([]byte(message.Payload))
		}
	}
}
