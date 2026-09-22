package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/mail"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrEmailTaken         = errors.New("email already registered")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrInvalidRefresh     = errors.New("invalid refresh token")
	ErrRefreshReuse       = errors.New("refresh token reuse detected")
	ErrInvalidAccess      = errors.New("invalid access token")
	ErrWeakPassword       = errors.New("password must be 12-72 bytes")
	ErrInvalidEmail       = errors.New("invalid email")
)

const (
	accessTTL  = 15 * time.Minute
	refreshTTL = 30 * 24 * time.Hour
)

type Repository interface {
	CreateUser(ctx context.Context, email, passwordHash string) (User, error)
	GetUserByEmail(ctx context.Context, email string) (User, error)
	CreateRefresh(ctx context.Context, userID, familyID, tokenHash string, expiresAt time.Time) error
	RotateRefresh(ctx context.Context, tokenHash, replacementHash string, replacementExpiresAt, now time.Time) (string, error)
}

type Service struct {
	repo      Repository
	jwtSecret []byte
	issuer    string
	audience  string
	now       func() time.Time
}

func NewService(repo Repository, jwtSecret []byte) *Service {
	return &Service{
		repo:      repo,
		jwtSecret: append([]byte(nil), jwtSecret...),
		issuer:    "pixelgo",
		audience:  "pixelgo-mobile",
		now:       time.Now,
	}
}

func (s *Service) Register(ctx context.Context, in Credentials) (TokenPair, error) {
	email, err := normalizeEmail(in.Email)
	if err != nil {
		return TokenPair{}, err
	}
	if err := validatePassword(in.Password); err != nil {
		return TokenPair{}, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return TokenPair{}, err
	}

	user, err := s.repo.CreateUser(ctx, email, string(hash))
	if err != nil {
		return TokenPair{}, err
	}
	return s.issueSession(ctx, user.ID)
}

func (s *Service) Login(ctx context.Context, in Credentials) (TokenPair, error) {
	email, err := normalizeEmail(in.Email)
	if err != nil {
		return TokenPair{}, ErrInvalidCredentials
	}

	user, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		return TokenPair{}, ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(in.Password)); err != nil {
		return TokenPair{}, ErrInvalidCredentials
	}
	return s.issueSession(ctx, user.ID)
}

func (s *Service) Refresh(ctx context.Context, refreshToken string) (TokenPair, error) {
	if refreshToken == "" {
		return TokenPair{}, ErrInvalidRefresh
	}

	newRefresh, newHash, err := newOpaqueToken()
	if err != nil {
		return TokenPair{}, err
	}

	now := s.now().UTC()
	userID, err := s.repo.RotateRefresh(
		ctx,
		hashOpaqueToken(refreshToken),
		newHash,
		now.Add(refreshTTL),
		now,
	)
	if err != nil {
		return TokenPair{}, err
	}

	access, err := s.issueAccess(userID, now)
	if err != nil {
		return TokenPair{}, err
	}

	return TokenPair{
		AccessToken:      access,
		RefreshToken:     newRefresh,
		TokenType:        "Bearer",
		ExpiresInSeconds: int64(accessTTL / time.Second),
	}, nil
}

func (s *Service) ValidateAccess(raw string) (string, error) {
	claims := &jwt.RegisteredClaims{}
	token, err := jwt.ParseWithClaims(
		raw,
		claims,
		func(token *jwt.Token) (any, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, ErrInvalidAccess
			}
			return s.jwtSecret, nil
		},
		jwt.WithValidMethods([]string{"HS256"}),
		jwt.WithIssuer(s.issuer),
		jwt.WithAudience(s.audience),
		jwt.WithExpirationRequired(),
	)
	if err != nil || !token.Valid || claims.Subject == "" {
		return "", ErrInvalidAccess
	}
	return claims.Subject, nil
}

func (s *Service) issueSession(ctx context.Context, userID string) (TokenPair, error) {
	now := s.now().UTC()
	access, err := s.issueAccess(userID, now)
	if err != nil {
		return TokenPair{}, err
	}

	refresh, refreshHash, err := newOpaqueToken()
	if err != nil {
		return TokenPair{}, err
	}
	familyID, err := randomHex(16)
	if err != nil {
		return TokenPair{}, err
	}
	if err := s.repo.CreateRefresh(
		ctx,
		userID,
		familyID,
		refreshHash,
		now.Add(refreshTTL),
	); err != nil {
		return TokenPair{}, err
	}

	return TokenPair{
		AccessToken:      access,
		RefreshToken:     refresh,
		TokenType:        "Bearer",
		ExpiresInSeconds: int64(accessTTL / time.Second),
	}, nil
}

func (s *Service) issueAccess(userID string, now time.Time) (string, error) {
	jti, err := randomHex(16)
	if err != nil {
		return "", err
	}

	claims := jwt.RegisteredClaims{
		Issuer:    s.issuer,
		Subject:   userID,
		Audience:  jwt.ClaimStrings{s.audience},
		ExpiresAt: jwt.NewNumericDate(now.Add(accessTTL)),
		NotBefore: jwt.NewNumericDate(now.Add(-5 * time.Second)),
		IssuedAt:  jwt.NewNumericDate(now),
		ID:        jti,
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.jwtSecret)
}

func normalizeEmail(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email {
		return "", ErrInvalidEmail
	}
	return email, nil
}

func validatePassword(password string) error {
	size := len([]byte(password))
	if size < 12 || size > 72 {
		return ErrWeakPassword
	}
	return nil
}

func newOpaqueToken() (string, string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw[:])
	return token, hashOpaqueToken(token), nil
}

func hashOpaqueToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func randomHex(size int) (string, error) {
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}
