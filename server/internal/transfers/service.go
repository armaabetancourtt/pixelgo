package transfers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"regexp"
	"time"
)

var checksumPattern = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)

var (
	ErrNotFound          = errors.New("transfer not found")
	ErrInvalidTransition = errors.New("invalid transfer state transition")
	ErrInvalidInput      = errors.New("invalid transfer input")
)

type Repository interface {
	Create(context.Context, Transfer) (Transfer, error)
	List(context.Context) ([]Transfer, error)
	Get(context.Context, string) (Transfer, error)
	Update(context.Context, Transfer) (Transfer, error)
}

type Publisher interface {
	Publish(eventType string, payload any)
}

type SignedURLProvider interface {
	UploadURL(transferID string) string
	DownloadURL(transferID string) string
}

type Service struct {
	repo      Repository
	publisher Publisher
	urls      SignedURLProvider
}

func NewService(repo Repository, publisher Publisher, urls SignedURLProvider) *Service {
	return &Service{repo: repo, publisher: publisher, urls: urls}
}

func (s *Service) Create(ctx context.Context, in CreateInput) (Transfer, error) {
	if in.SourceDeviceID == "" || in.DestinationDeviceID == "" || in.SizeBytes < 0 || !checksumPattern.MatchString(in.SHA256) {
		return Transfer{}, ErrInvalidInput
	}
	if in.Kind != KindFile && in.Kind != KindPhoto && in.Kind != KindLink && in.Kind != KindText && in.Kind != KindClipboard {
		return Transfer{}, ErrInvalidInput
	}

	now := time.Now().UTC()
	id, err := newID("tr")
	if err != nil {
		return Transfer{}, err
	}

	persisted, err := s.repo.Create(ctx, Transfer{
		ID:                  id,
		SourceDeviceID:      in.SourceDeviceID,
		DestinationDeviceID: in.DestinationDeviceID,
		Kind:                in.Kind,
		Status:              StatusUploading,
		DisplayName:         in.DisplayName,
		ContentType:         in.ContentType,
		SizeBytes:           in.SizeBytes,
		SHA256:              in.SHA256,
		CreatedAt:           now,
		UpdatedAt:           now,
	})
	if err != nil {
		return Transfer{}, err
	}
	return s.decorate(persisted), nil
}

func (s *Service) List(ctx context.Context) ([]Transfer, error) {
	items, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i] = s.decorate(items[i])
	}
	return items, nil
}

func (s *Service) Get(ctx context.Context, id string) (Transfer, error) {
	t, err := s.repo.Get(ctx, id)
	if err != nil {
		return Transfer{}, err
	}
	return s.decorate(t), nil
}

func (s *Service) MarkUploaded(ctx context.Context, id string) (Transfer, error) {
	t, err := s.repo.Get(ctx, id)
	if err != nil {
		return Transfer{}, err
	}
	if t.Status != StatusUploading {
		return Transfer{}, ErrInvalidTransition
	}

	t.Status = StatusReady
	t.UploadURL = ""
	t.DownloadURL = ""
	t.UpdatedAt = time.Now().UTC()

	t, err = s.repo.Update(ctx, t)
	if err != nil {
		return Transfer{}, err
	}

	t = s.decorate(t)
	s.publisher.Publish("transfer.ready", t)
	return t, nil
}

func (s *Service) Complete(ctx context.Context, id string) (Transfer, error) {
	t, err := s.repo.Get(ctx, id)
	if err != nil {
		return Transfer{}, err
	}
	if t.Status != StatusReady && t.Status != StatusDownloading {
		return Transfer{}, ErrInvalidTransition
	}

	t.Status = StatusCompleted
	t.UploadURL = ""
	t.DownloadURL = ""
	t.UpdatedAt = time.Now().UTC()

	t, err = s.repo.Update(ctx, t)
	if err != nil {
		return Transfer{}, err
	}

	t = s.decorate(t)
	s.publisher.Publish("transfer.completed", t)
	return t, nil
}

func (s *Service) decorate(t Transfer) Transfer {
	t.UploadURL = ""
	t.DownloadURL = ""

	switch t.Status {
	case StatusUploading:
		t.UploadURL = s.urls.UploadURL(t.ID)
	case StatusReady, StatusDownloading, StatusCompleted:
		t.DownloadURL = s.urls.DownloadURL(t.ID)
	}

	return t
}

func newID(prefix string) (string, error) {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return prefix + "_" + hex.EncodeToString(b[:]), nil
}
