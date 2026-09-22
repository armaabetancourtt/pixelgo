package devices

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"sync"
	"time"
)

var ErrNotFound = errors.New("device not found")

type Device struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Platform  string    `json:"platform"`
	PushToken string    `json:"pushToken,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

type RegisterInput struct {
	Name      string `json:"name"`
	Platform  string `json:"platform"`
	PushToken string `json:"pushToken"`
}

type Service struct {
	mu    sync.RWMutex
	items map[string]Device
}

func NewService() *Service { return &Service{items: map[string]Device{}} }

func (s *Service) Register(_ context.Context, in RegisterInput) (Device, error) {
	if in.Name == "" || (in.Platform != "ios" && in.Platform != "android") {
		return Device{}, errors.New("invalid device")
	}
	var b [10]byte
	if _, err := rand.Read(b[:]); err != nil { return Device{}, err }
	d := Device{
		ID: "dev_" + hex.EncodeToString(b[:]),
		Name: in.Name, Platform: in.Platform, PushToken: in.PushToken,
		CreatedAt: time.Now().UTC(),
	}
	s.mu.Lock()
	s.items[d.ID] = d
	s.mu.Unlock()
	return d, nil
}

func (s *Service) List(_ context.Context) []Device {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Device, 0, len(s.items))
	for _, d := range s.items { out = append(out, d) }
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

func (s *Service) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[id]; !ok { return ErrNotFound }
	delete(s.items, id)
	return nil
}
