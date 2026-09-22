package files

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"sync"
	"time"
)

var (
	ErrInvalidSignature = errors.New("invalid signed URL")
	ErrExpiredSignature = errors.New("signed URL expired")
	ErrNotFound         = errors.New("file not found")
)

type Service struct {
	baseURL string
	secret  []byte
	ttl     time.Duration

	mu    sync.RWMutex
	blobs map[string][]byte
}

func NewService(baseURL, secret string, ttl time.Duration) *Service {
	return &Service{
		baseURL: baseURL,
		secret:  []byte(secret),
		ttl:     ttl,
		blobs:   make(map[string][]byte),
	}
}

func (s *Service) UploadURL(transferID string) string {
	return s.signedURL("upload", transferID, "/dev-upload/"+transferID)
}

func (s *Service) DownloadURL(transferID string) string {
	return s.signedURL("download", transferID, "/dev-download/"+transferID)
}

func (s *Service) Verify(action, transferID, expires, signature string) error {
	expUnix, err := strconv.ParseInt(expires, 10, 64)
	if err != nil {
		return ErrInvalidSignature
	}
	if time.Now().Unix() > expUnix {
		return ErrExpiredSignature
	}

	expected := s.signature(action, transferID, expUnix)
	provided, err := hex.DecodeString(signature)
	if err != nil {
		return ErrInvalidSignature
	}
	expectedBytes, _ := hex.DecodeString(expected)
	if !hmac.Equal(provided, expectedBytes) {
		return ErrInvalidSignature
	}
	return nil
}

func (s *Service) Put(transferID string, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.blobs[transferID] = append([]byte(nil), data...)
}

func (s *Service) Get(transferID string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, ok := s.blobs[transferID]
	if !ok {
		return nil, ErrNotFound
	}
	return append([]byte(nil), data...), nil
}

func (s *Service) Exists(transferID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.blobs[transferID]
	return ok
}

func (s *Service) signedURL(action, transferID, path string) string {
	expires := time.Now().Add(s.ttl).Unix()
	u, err := url.Parse(s.baseURL)
	if err != nil {
		return ""
	}
	u.Path = path
	query := u.Query()
	query.Set("exp", strconv.FormatInt(expires, 10))
	query.Set("sig", s.signature(action, transferID, expires))
	u.RawQuery = query.Encode()
	return u.String()
}

func (s *Service) signature(action, transferID string, expires int64) string {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = fmt.Fprintf(mac, "%s\n%s\n%d", action, transferID, expires)
	return hex.EncodeToString(mac.Sum(nil))
}
