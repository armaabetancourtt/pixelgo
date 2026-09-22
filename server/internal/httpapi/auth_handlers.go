package httpapi

import (
	"errors"
	"net/http"

	"github.com/armaabetancourtt/pixelgo/server/internal/auth"
)

func (s *Server) authRegister(w http.ResponseWriter, r *http.Request) {
	var in auth.Credentials
	if err := decode(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	pair, err := s.auth.Register(r.Context(), in)
	switch {
	case err == nil:
		writeJSON(w, http.StatusCreated, pair)
	case errors.Is(err, auth.ErrEmailTaken):
		writeError(w, http.StatusConflict, "email_taken", "email is already registered")
	case errors.Is(err, auth.ErrWeakPassword):
		writeError(w, http.StatusBadRequest, "weak_password", err.Error())
	case errors.Is(err, auth.ErrInvalidEmail):
		writeError(w, http.StatusBadRequest, "invalid_email", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "could not create account")
	}
}

func (s *Server) authLogin(w http.ResponseWriter, r *http.Request) {
	var in auth.Credentials
	if err := decode(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	pair, err := s.auth.Login(r.Context(), in)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid email or password")
		return
	}
	writeJSON(w, http.StatusOK, pair)
}

func (s *Server) authRefresh(w http.ResponseWriter, r *http.Request) {
	var in auth.RefreshRequest
	if err := decode(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	pair, err := s.auth.Refresh(r.Context(), in.RefreshToken)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, pair)
	case errors.Is(err, auth.ErrRefreshReuse):
		writeError(w, http.StatusUnauthorized, "refresh_reuse_detected", "refresh token family has been revoked")
	default:
		writeError(w, http.StatusUnauthorized, "invalid_refresh", "refresh token is invalid or expired")
	}
}
