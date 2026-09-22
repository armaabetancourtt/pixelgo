package httpapi

import (
	"errors"
	"net/http"

	"github.com/armaabetancourtt/pixelgo/server/internal/auth"
)

func (s *Server) registerUser(w http.ResponseWriter, r *http.Request) {
	var in auth.Credentials
	if err := decode(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	session, err := s.auth.Register(r.Context(), in)
	switch {
	case err == nil:
		writeJSON(w, http.StatusCreated, session)
	case errors.Is(err, auth.ErrEmailTaken):
		writeError(w, http.StatusConflict, "email_taken", "email is already registered")
	case errors.Is(err, auth.ErrWeakPassword), errors.Is(err, auth.ErrInvalidEmail):
		writeError(w, http.StatusBadRequest, "invalid_credentials", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "could not create account")
	}
}

func (s *Server) loginUser(w http.ResponseWriter, r *http.Request) {
	var in auth.Credentials
	if err := decode(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	session, err := s.auth.Login(r.Context(), in)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, session)
	case errors.Is(err, auth.ErrInvalidCredentials):
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid email or password")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "could not log in")
	}
}

func (s *Server) refreshSession(w http.ResponseWriter, r *http.Request) {
	var in auth.RefreshRequest
	if err := decode(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	session, err := s.auth.Refresh(r.Context(), in.RefreshToken)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, session)
	case errors.Is(err, auth.ErrRefreshReuse):
		writeError(w, http.StatusUnauthorized, "refresh_reuse_detected", "refresh token family was revoked")
	case errors.Is(err, auth.ErrInvalidRefresh):
		writeError(w, http.StatusUnauthorized, "invalid_refresh_token", "refresh token is invalid or expired")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "could not refresh session")
	}
}
