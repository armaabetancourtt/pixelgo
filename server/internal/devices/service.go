package devices

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"
)

var ErrNotFound = errors.New("device not found")

type Device struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Platform  string    `json:"platform"`
	PushToken string    `json:"pushToken,omitempty"`
	Online    bool      `json:"online"`
	CreatedAt time.Time `json:"createdAt"`
}

type RegisterInput struct {
	Name      string `json:"name"`
	Platform  string `json:"platform"`
	PushToken string `json:"pushToken"`
}

type Repository interface {
	Create(context.Context, Device) (Device, error)
	List(context.Context) ([]Device, error)
	Delete(context.Context, string) error
}

type PresenceReader interface {
	Online(context.Context, []string) (map[string]bool, error)
}

type Service struct {
	repo     Repository
	presence PresenceReader
}

func NewService(repo Repository, presence ...PresenceReader) *Service {
	if repo == nil {
		repo = NewMemoryRepository()
	}

	var reader PresenceReader
	if len(presence) > 0 {
		reader = presence[0]
	}

	return &Service{
		repo:     repo,
		presence: reader,
	}
}

func (s *Service) Register(ctx context.Context, in RegisterInput) (Device, error) {
	if in.Name == "" || (in.Platform != "ios" && in.Platform != "android") {
		return Device{}, errors.New("invalid device")
	}

	id, err := newID()
	if err != nil {
		return Device{}, err
	}

	return s.repo.Create(ctx, Device{
		ID:        id,
		Name:      in.Name,
		Platform:  in.Platform,
		PushToken: in.PushToken,
		CreatedAt: time.Now().UTC(),
	})
}

func (s *Service) List(ctx context.Context) ([]Device, error) {
	items, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	if s.presence == nil || len(items) == 0 {
		return items, nil
	}

	ids := make([]string, 0, len(items))
	for _, device := range items {
		ids = append(ids, device.ID)
	}

	online, err := s.presence.Online(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i].Online = online[items[i].ID]
	}
	return items, nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

func newID() (string, error) {
	var b [10]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "dev_" + hex.EncodeToString(b[:]), nil
}
