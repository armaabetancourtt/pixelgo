package files

import (
	"context"
	"net/url"
	"testing"
	"time"
)

func TestSignedURLRoundTrip(t *testing.T) {
	service := NewService("http://localhost:8080", "test-secret", time.Minute)
	raw, err := service.UploadURL(context.Background(), "tr_123", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")

	if err != nil {
		t.Fatal(err)
	}

	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}

	if err := service.Verify(
		"upload",
		"tr_123",
		u.Query().Get("exp"),
		u.Query().Get("sig"),
	); err != nil {
		t.Fatalf("expected valid signature, got %v", err)
	}
}

func TestSignedURLRejectsTampering(t *testing.T) {
	service := NewService("http://localhost:8080", "test-secret", time.Minute)
	raw, err := service.DownloadURL(context.Background(), "tr_123")
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}

	if err := service.Verify(
		"download",
		"tr_other",
		u.Query().Get("exp"),
		u.Query().Get("sig"),
	); err != ErrInvalidSignature {
		t.Fatalf("expected invalid signature, got %v", err)
	}
}

func TestStoreCopiesPayload(t *testing.T) {
	service := NewService("http://localhost:8080", "test-secret", time.Minute)
	input := []byte("pixel-go")
	service.Put("tr_1", input)
	input[0] = 'X'

	got, err := service.Get("tr_1")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "pixel-go" {
		t.Fatalf("expected immutable stored copy, got %q", got)
	}
}
