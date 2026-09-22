package auth

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	passwordAlgorithm  = "pbkdf2-sha256"
	passwordIterations = 600_000
	passwordSaltBytes  = 16
	passwordKeyBytes   = 32
)

var ErrInvalidPasswordHash = errors.New("invalid password hash")

func HashPassword(password string) (string, error) {
	salt := make([]byte, passwordSaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}

	key, err := pbkdf2.Key(
		sha256.New,
		password,
		salt,
		passwordIterations,
		passwordKeyBytes,
	)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf(
		"$%s$%d$%s$%s",
		passwordAlgorithm,
		passwordIterations,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

func VerifyPassword(encoded, password string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 5 || parts[0] != "" || parts[1] != passwordAlgorithm {
		return false, ErrInvalidPasswordHash
	}

	iterations, err := strconv.Atoi(parts[2])
	if err != nil || iterations < 1 {
		return false, ErrInvalidPasswordHash
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(salt) < 8 {
		return false, ErrInvalidPasswordHash
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(expected) < 16 {
		return false, ErrInvalidPasswordHash
	}

	actual, err := pbkdf2.Key(
		sha256.New,
		password,
		salt,
		iterations,
		len(expected),
	)
	if err != nil {
		return false, err
	}

	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}
