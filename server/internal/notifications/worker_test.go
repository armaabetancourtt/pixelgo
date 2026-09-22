package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

type fakeRepo struct {
	mu      sync.Mutex
	items   []Delivery
	sent    []int64
	retried []int64
	final   []bool
}

func (r *fakeRepo) Claim(_ context.Context, _ int) ([]Delivery, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.items) == 0 {
		return nil, nil
	}
	items := append([]Delivery(nil), r.items...)
	r.items = nil
	return items, nil
}

func (r *fakeRepo) MarkSent(_ context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sent = append(r.sent, id)
	return nil
}

func (r *fakeRepo) Retry(
	_ context.Context,
	id int64,
	_ string,
	_ time.Time,
	final bool,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.retried = append(r.retried, id)
	r.final = append(r.final, final)
	return nil
}

type fakeSender struct {
	err error
}

func (s fakeSender) Send(context.Context, Delivery) error {
	return s.err
}

func TestWorkerMarksSuccessfulDeliverySent(t *testing.T) {
	repo := &fakeRepo{
		items: []Delivery{{ID: 7, Platform: "ios", PushToken: "token"}},
	}
	worker := NewWorker(
		repo,
		fakeSender{},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	worker.drain(context.Background())

	if len(repo.sent) != 1 || repo.sent[0] != 7 {
		t.Fatalf("expected delivery 7 sent, got %#v", repo.sent)
	}
	if len(repo.retried) != 0 {
		t.Fatalf("did not expect retries, got %#v", repo.retried)
	}
}

func TestWorkerRetriesTransientFailure(t *testing.T) {
	repo := &fakeRepo{
		items: []Delivery{{ID: 8, Attempts: 1}},
	}
	worker := NewWorker(
		repo,
		fakeSender{err: errors.New("temporary")},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	worker.drain(context.Background())

	if len(repo.retried) != 1 || repo.retried[0] != 8 {
		t.Fatalf("expected retry for delivery 8, got %#v", repo.retried)
	}
	if repo.final[0] {
		t.Fatal("transient failure should not be final")
	}
}

func TestWorkerStopsRetryingPermanentFailure(t *testing.T) {
	repo := &fakeRepo{
		items: []Delivery{{ID: 9}},
	}
	worker := NewWorker(
		repo,
		fakeSender{err: PermanentError{Err: errors.New("unregistered")}},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	worker.drain(context.Background())

	if len(repo.final) != 1 || !repo.final[0] {
		t.Fatal("expected permanent delivery failure")
	}
}

func TestRouterRejectsUnknownPlatform(t *testing.T) {
	router := Router{}
	err := router.Send(context.Background(), Delivery{
		Platform:  "windows",
		PushToken: "token",
		Payload:   json.RawMessage("{\"transferId\":\"tr_1\"}"),
	})
	var permanent PermanentError
	if !errors.As(err, &permanent) {
		t.Fatalf("expected permanent error, got %v", err)
	}
}

func TestRetryBackoffCapsAtFiveMinutes(t *testing.T) {
	if got := retryBackoff(50); got != 5*time.Minute {
		t.Fatalf("expected 5m cap, got %s", got)
	}
}


type fakePresence struct {
	online bool
	err    error
}

func (p fakePresence) IsOnline(context.Context, string) (bool, error) {
	return p.online, p.err
}

type countingSender struct {
	calls int
}

func (s *countingSender) Send(context.Context, Delivery) error {
	s.calls++
	return nil
}

func TestOfflineOnlySenderSkipsPushWhenDeviceIsOnline(t *testing.T) {
	next := &countingSender{}
	sender := OfflineOnlySender{
		Presence: fakePresence{online: true},
		Next:     next,
	}

	if err := sender.Send(context.Background(), Delivery{DeviceID: "dev_1"}); err != nil {
		t.Fatal(err)
	}
	if next.calls != 0 {
		t.Fatalf("expected realtime-online device to skip push, got %d calls", next.calls)
	}
}

func TestOfflineOnlySenderFallsBackToPushWhenPresenceIsUnknown(t *testing.T) {
	next := &countingSender{}
	sender := OfflineOnlySender{
		Presence: fakePresence{err: errors.New("redis unavailable")},
		Next:     next,
	}

	if err := sender.Send(context.Background(), Delivery{DeviceID: "dev_1"}); err != nil {
		t.Fatal(err)
	}
	if next.calls != 1 {
		t.Fatalf("expected push fallback, got %d calls", next.calls)
	}
}


type contextSender struct{}

func (contextSender) Send(ctx context.Context, _ Delivery) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestWorkerReleasesClaimWhenShutdownCancelsProvider(t *testing.T) {
	repo := &fakeRepo{}
	worker := NewWorker(
		repo,
		contextSender{},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	worker.deliver(ctx, Delivery{
		ID:       10,
		Attempts: 0,
	})

	if len(repo.retried) != 1 || repo.retried[0] != 10 {
		t.Fatalf("expected cancelled delivery to be released, got %#v", repo.retried)
	}
	if repo.final[0] {
		t.Fatal("shutdown cancellation must remain retryable")
	}
}
