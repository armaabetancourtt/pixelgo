package transfers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"time"
)

var checksumPattern = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)

var (
	ErrNotFound         = errors.New("transfer not found")
	ErrInvalidTransition = errors.New("invalid transfer state transition")
	ErrInvalidInput     = errors.New("invalid transfer input")
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

type Service struct {
	repo      Repository
	publisher Publisher
	baseURL   string
}

func NewService(repo Repository, publisher Publisher, baseURL string) *Service {
	return &Service{repo: repo, publisher: publisher, baseURL: baseURL}
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
	t := Transfer{
		ID:                  id,
		SourceDeviceID:      in.SourceDeviceID,
		DestinationDeviceID: in.DestinationDeviceID,
		Kind:                in.Kind,
		Status:              StatusUploading,
		DisplayName:         in.DisplayName,
		ContentType:         in.ContentType,
		SizeBytes:           in.SizeBytes,
		SHA256:              in.SHA256,
		UploadURL:           fmt.Sprintf("%s/dev-upload/%s", s.baseURL, id),
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	return s.repo.Create(ctx, t)
}

func (s *Service) List(ctx context.Context) ([]Transfer, error) { return s.repo.List(ctx) }
func (s *Service) Get(ctx context.Context, id string) (Transfer, error) { return s.repo.Get(ctx, id) }

func (s *Service) MarkUploaded(ctx context.Context, id string) (Transfer, error) {
	t, err := s.repo.Get(ctx, id)
	if err != nil { return Transfer{}, err }
	if t.Status != StatusUploading { return Transfer{}, ErrInvalidTransition }
	t.Status = StatusReady
	t.UploadURL = ""
	t.DownloadURL = fmt.Sprintf("%s/dev-download/%s", s.baseURL, id)
	t.UpdatedAt = time.Now().UTC()
	t, err = s.repo.Update(ctx, t)
	if err == nil { s.publisher.Publish("transfer.ready", t) }
	return t, err
}

func (s *Service) Complete(ctx context.Context, id string) (Transfer, error) {
	t, err := s.repo.Get(ctx, id)
	if err != nil { return Transfer{}, err }
	if t.Status != StatusReady && t.Status != StatusDownloading { return Transfer{}, ErrInvalidTransition }
	t.Status = StatusCompleted
	t.UpdatedAt = time.Now().UTC()
	t, err = s.repo.Update(ctx, t)
	if err == nil { s.publisher.Publish("transfer.completed", t) }
	return t, err
}

func newID(prefix string) (string, error) {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil { return "", err }
	return prefix + "_" + hex.EncodeToString(b[:]), nil
}
