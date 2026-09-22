package devices

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"
	"unicode/utf8"
)

const (
	maxDeviceNameRunes = 120
	maxPushTokenRunes  = 4096
)

var ErrNotFound = errors.New("device not found")

type Device struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Platform  string    `json:"platform"`
	PushToken string    `json:"-"`
	CreatedAt time.Time `json:"createdAt"`
}

type RegisterInput struct {
	Name      string `json:"name"`
	Platform  string `json:"platform"`
	PushToken string `json:"pushToken"`
}

type PushTokenInput struct {
	PushToken string `json:"pushToken"`
}

type Repository interface {
	Create(context.Context, Device) (Device, error)
	List(context.Context) ([]Device, error)
	Get(context.Context, string) (Device, error)
	UpdatePushToken(context.Context, string, string) (Device, error)
	Delete(context.Context, string) error
}

type Service struct {
	repo Repository
}

func NewService(repositories ...Repository) *Service {
	var repo Repository = NewMemoryRepository()
	if len(repositories) > 0 && repositories[0] != nil {
		repo = repositories[0]
	}
	return &Service{repo: repo}
}

func (s *Service) Register(ctx context.Context, in RegisterInput) (Device, error) {
	if utf8.RuneCountInString(in.Name) < 1 ||
		utf8.RuneCountInString(in.Name) > maxDeviceNameRunes ||
		utf8.RuneCountInString(in.PushToken) > maxPushTokenRunes ||
		(in.Platform != "ios" && in.Platform != "android") {
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
	return s.repo.List(ctx)
}

func (s *Service) Get(ctx context.Context, id string) (Device, error) {
	return s.repo.Get(ctx, id)
}

func (s *Service) UpdatePushToken(
	ctx context.Context,
	id string,
	token string,
) (Device, error) {
	if id == "" || utf8.RuneCountInString(token) > maxPushTokenRunes {
		return Device{}, errors.New("invalid push token")
	}
	return s.repo.UpdatePushToken(ctx, id, token)
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
