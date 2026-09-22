package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"time"
)

var ErrProviderUnavailable = errors.New("push provider unavailable")

type Delivery struct {
	ID         int64
	DeviceID   string
	TransferID string
	EventType  string
	Platform   string
	PushToken  string
	Payload    json.RawMessage
	Attempts   int
}

type Repository interface {
	Claim(context.Context, int) ([]Delivery, error)
	MarkSent(context.Context, int64) error
	Retry(context.Context, int64, string, time.Time, bool) error
}

type Sender interface {
	Send(context.Context, Delivery) error
}

type PermanentError struct {
	Err error
}

func (e PermanentError) Error() string {
	if e.Err == nil {
		return "permanent push delivery error"
	}
	return e.Err.Error()
}

func (e PermanentError) Unwrap() error { return e.Err }

type Worker struct {
	repo       Repository
	sender     Sender
	logger     *slog.Logger
	batchSize  int
	pollEvery  time.Duration
	maxAttempts int
}

func NewWorker(
	repo Repository,
	sender Sender,
	logger *slog.Logger,
) *Worker {
	return &Worker{
		repo:        repo,
		sender:      sender,
		logger:      logger,
		batchSize:   50,
		pollEvery:   time.Second,
		maxAttempts: 8,
	}
}

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.pollEvery)
	defer ticker.Stop()

	w.drain(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.drain(ctx)
		}
	}
}

func (w *Worker) drain(ctx context.Context) {
	for {
		deliveries, err := w.repo.Claim(ctx, w.batchSize)
		if err != nil {
			w.logger.Error("push outbox claim failed", "error", err)
			return
		}
		if len(deliveries) == 0 {
			return
		}

		for _, delivery := range deliveries {
			w.deliver(ctx, delivery)
		}
		if len(deliveries) < w.batchSize {
			return
		}
	}
}

func (w *Worker) deliver(ctx context.Context, delivery Delivery) {
	err := w.sender.Send(ctx, delivery)
	if err == nil {
		if markErr := w.repo.MarkSent(ctx, delivery.ID); markErr != nil {
			w.logger.Error(
				"push outbox mark-sent failed",
				"delivery_id", delivery.ID,
				"error", markErr,
			)
		}
		return
	}

	attempt := delivery.Attempts + 1
	var permanent PermanentError
	final := errors.As(err, &permanent) || attempt >= w.maxAttempts

	backoff := retryBackoff(attempt)
	if retryErr := w.repo.Retry(
		ctx,
		delivery.ID,
		err.Error(),
		time.Now().UTC().Add(backoff),
		final,
	); retryErr != nil {
		w.logger.Error(
			"push outbox retry update failed",
			"delivery_id", delivery.ID,
			"error", retryErr,
		)
		return
	}

	level := slog.LevelWarn
	if final {
		level = slog.LevelError
	}
	w.logger.Log(
		ctx,
		level,
		"push delivery failed",
		"delivery_id", delivery.ID,
		"device_id", delivery.DeviceID,
		"platform", delivery.Platform,
		"attempt", attempt,
		"final", final,
		"error", err,
	)
}

func retryBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	exp := math.Min(float64(attempt-1), 8)
	delay := time.Duration(math.Pow(2, exp)) * time.Second
	if delay > 5*time.Minute {
		return 5 * time.Minute
	}
	return delay
}
