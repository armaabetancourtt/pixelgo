package auth

import (
	"context"
	"errors"
	"testing"
)

func TestRegisterIssuesValidAccessAndRefreshTokens(t *testing.T) {
	service := NewService(NewMemoryRepository(), []byte("01234567890123456789012345678901"))

	pair, err := service.Register(context.Background(), Credentials{
		Email:    "person@example.com",
		Password: "correct-horse-battery-staple",
	})
	if err != nil {
		t.Fatal(err)
	}
	if pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Fatal("expected both access and refresh tokens")
	}
	if pair.TokenType != "Bearer" {
		t.Fatalf("expected Bearer token type, got %q", pair.TokenType)
	}

	userID, err := service.ValidateAccess(pair.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if userID == "" {
		t.Fatal("expected access token subject")
	}
}

func TestRegisterNormalizesEmailAndRejectsDuplicate(t *testing.T) {
	service := NewService(NewMemoryRepository(), []byte("01234567890123456789012345678901"))
	ctx := context.Background()

	if _, err := service.Register(ctx, Credentials{
		Email:    " Person@Example.COM ",
		Password: "correct-horse-battery-staple",
	}); err != nil {
		t.Fatal(err)
	}

	_, err := service.Register(ctx, Credentials{
		Email:    "person@example.com",
		Password: "another-strong-password-123",
	})
	if !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("expected email taken, got %v", err)
	}
}

func TestRegisterRejectsWeakPassword(t *testing.T) {
	service := NewService(NewMemoryRepository(), []byte("01234567890123456789012345678901"))
	_, err := service.Register(context.Background(), Credentials{
		Email:    "person@example.com",
		Password: "short",
	})
	if !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("expected weak password error, got %v", err)
	}
}

func TestLoginRejectsWrongPassword(t *testing.T) {
	service := NewService(NewMemoryRepository(), []byte("01234567890123456789012345678901"))
	ctx := context.Background()

	if _, err := service.Register(ctx, Credentials{
		Email:    "person@example.com",
		Password: "correct-horse-battery-staple",
	}); err != nil {
		t.Fatal(err)
	}

	_, err := service.Login(ctx, Credentials{
		Email:    "person@example.com",
		Password: "definitely-the-wrong-password",
	})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected invalid credentials, got %v", err)
	}
}

func TestRefreshRotationDetectsReuseAndRevokesFamily(t *testing.T) {
	service := NewService(NewMemoryRepository(), []byte("01234567890123456789012345678901"))
	ctx := context.Background()

	first, err := service.Register(ctx, Credentials{
		Email:    "person@example.com",
		Password: "correct-horse-battery-staple",
	})
	if err != nil {
		t.Fatal(err)
	}

	second, err := service.Refresh(ctx, first.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if second.RefreshToken == first.RefreshToken {
		t.Fatal("expected refresh token rotation")
	}

	_, err = service.Refresh(ctx, first.RefreshToken)
	if !errors.Is(err, ErrRefreshReuse) {
		t.Fatalf("expected refresh reuse detection, got %v", err)
	}

	_, err = service.Refresh(ctx, second.RefreshToken)
	if !errors.Is(err, ErrInvalidRefresh) {
		t.Fatalf("expected rotated family to be revoked after reuse, got %v", err)
	}
}

func TestValidateAccessRejectsWrongSigningKey(t *testing.T) {
	repo := NewMemoryRepository()
	issuer := NewService(repo, []byte("01234567890123456789012345678901"))
	validator := NewService(repo, []byte("abcdefghijklmnopqrstuvwxyz123456"))

	pair, err := issuer.Register(context.Background(), Credentials{
		Email:    "person@example.com",
		Password: "correct-horse-battery-staple",
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := validator.ValidateAccess(pair.AccessToken); !errors.Is(err, ErrInvalidAccess) {
		t.Fatalf("expected invalid access token, got %v", err)
	}
}
