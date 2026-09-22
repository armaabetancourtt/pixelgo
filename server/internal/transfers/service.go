package transfers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io/fs"
	"regexp"
	"strings"
	"time"
)

var checksumPattern = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)

var (
	ErrNotFound          = errors.New("transfer not found")
	ErrInvalidTransition = errors.New("invalid transfer state transition")
	ErrInvalidInput      = errors.New("invalid transfer input")
	ErrUploadMissing     = errors.New("uploaded object is missing")
	ErrUploadIntegrity   = errors.New("uploaded object failed integrity verification")
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
	UploadURL(
		ctx context.Context,
		transferID string,
		sha256 string,
	) (string, error)
	DownloadURL(
		ctx context.Context,
		transferID string,
	) (string, error)
	InspectUploaded(
		ctx context.Context,
		transferID string,
	) (sizeBytes int64, sha256 string, err error)
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
	if in.SourceDeviceID == "" ||
		in.DestinationDeviceID == "" ||
		in.SizeBytes < 0 ||
		!checksumPattern.MatchString(in.SHA256) {
		return Transfer{}, ErrInvalidInput
	}
	if in.Kind != KindFile &&
		in.Kind != KindPhoto &&
		in.Kind != KindLink &&
		in.Kind != KindText &&
		in.Kind != KindClipboard {
		return Transfer{}, ErrInvalidInput
	}

	now := time.Now().UTC()
	id, err := newID("tr")
	if err != nil {
		return Transfer{}, err
	}

	uploadURL, err := s.urls.UploadURL(ctx, id, in.SHA256)
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
	persisted.UploadURL = uploadURL
	return persisted, nil
}

func (s *Service) List(ctx context.Context) ([]Transfer, error) {
	items, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i], err = s.decorate(ctx, items[i])
		if err != nil {
			return nil, err
		}
	}
	return items, nil
}

func (s *Service) Get(ctx context.Context, id string) (Transfer, error) {
	t, err := s.repo.Get(ctx, id)
	if err != nil {
		return Transfer{}, err
	}
	return s.decorate(ctx, t)
}

func (s *Service) MarkUploaded(ctx context.Context, id string) (Transfer, error) {
	t, err := s.repo.Get(ctx, id)
	if err != nil {
		return Transfer{}, err
	}
	if t.Status != StatusUploading {
		return Transfer{}, ErrInvalidTransition
	}

	sizeBytes, checksum, err := s.urls.InspectUploaded(ctx, id)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return Transfer{}, ErrUploadMissing
	case err != nil:
		return Transfer{}, err
	case sizeBytes != t.SizeBytes:
		return Transfer{}, ErrUploadIntegrity
	case !strings.EqualFold(checksum, t.SHA256):
		return Transfer{}, ErrUploadIntegrity
	}

	downloadURL, err := s.urls.DownloadURL(ctx, id)
	if err != nil {
		return Transfer{}, err
	}

	t.Status = StatusReady
	t.UploadURL = ""
	t.DownloadURL = ""
	t.UpdatedAt = time.Now().UTC()

	t, err = s.repo.Update(ctx, t)
	if err != nil {
		return Transfer{}, err
	}

	t.DownloadURL = downloadURL
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

	downloadURL, err := s.urls.DownloadURL(ctx, id)
	if err != nil {
		return Transfer{}, err
	}

	t.Status = StatusCompleted
	t.UploadURL = ""
	t.DownloadURL = ""
	t.UpdatedAt = time.Now().UTC()

	t, err = s.repo.Update(ctx, t)
	if err != nil {
		return Transfer{}, err
	}

	t.DownloadURL = downloadURL
	s.publisher.Publish("transfer.completed", t)
	return t, nil
}

func (s *Service) decorate(
	ctx context.Context,
	t Transfer,
) (Transfer, error) {
	t.UploadURL = ""
	t.DownloadURL = ""

	switch t.Status {
	case StatusUploading:
		uploadURL, err := s.urls.UploadURL(ctx, t.ID, t.SHA256)
		if err != nil {
			return Transfer{}, err
		}
		t.UploadURL = uploadURL
	case StatusReady, StatusDownloading, StatusCompleted:
		downloadURL, err := s.urls.DownloadURL(ctx, t.ID)
		if err != nil {
			return Transfer{}, err
		}
		t.DownloadURL = downloadURL
	}

	return t, nil
}

func newID(prefix string) (string, error) {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return prefix + "_" + hex.EncodeToString(b[:]), nil
}
