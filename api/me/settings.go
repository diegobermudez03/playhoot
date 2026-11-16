package me

import (
	"errors"
	"net/http"
	"time"

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/logging"
)

type setModeRequest struct {
	Mode string `json:"mode"`
}

type setLanguageRequest struct {
	Language string `json:"language"`
}

// modeResponse is the mode the account is now in, with the fresh access
// token carrying it. The refresh token the client holds is untouched.
type modeResponse struct {
	Mode            string `json:"mode"`
	AccessToken     string `json:"access_token"`
	AccessExpiresAt string `json:"access_expires_at"`
	AccessExpiresIn int64  `json:"access_expires_in"`
}

type languageResponse struct {
	Language string `json:"language"`
}

// handleSetMode switches the caller between their account's modes and
// answers a fresh access token carrying the new one.
func (h *Handler) handleSetMode(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	caller, ok := auth.Caller(ctx)
	if !ok {
		httpx.WriteError(w, http.StatusInternalServerError, "caller not resolved")
		return
	}
	var req setModeRequest
	if !decode(w, r, &req) {
		return
	}
	result, err := h.modes.SetMode(ctx, caller.UUID, req.Mode)
	if err != nil {
		logging.LogError(ctx, err)
		if errors.Is(err, identity.ErrUserNotFound) {
			httpx.WriteError(w, http.StatusInternalServerError, "caller not found")
			return
		}
		httpx.WriteError(w, http.StatusServiceUnavailable, "switching mode failed")
		return
	}
	switch result.Outcome {
	case identity.ModeSet:
		credentials := result.Credentials
		w.Header().Set("Cache-Control", "no-store")
		httpx.WriteJSON(w, http.StatusOK, modeResponse{
			Mode:            string(identity.NormalizeMode(credentials.User.Mode)),
			AccessToken:     credentials.AccessToken,
			AccessExpiresAt: credentials.AccessExpiresAt.UTC().Format(time.RFC3339),
			AccessExpiresIn: int64(credentials.AccessExpiresAt.Sub(credentials.AccessIssuedAt) / time.Second),
		})
	case identity.ModeInvalid:
		httpx.WriteCodedError(w, http.StatusBadRequest, string(result.Outcome), "mode must be normal or teacher")
	case identity.ModeNotAccount:
		httpx.WriteCodedError(w, http.StatusForbidden, string(result.Outcome), "only an account switches mode")
	default:
		logging.LogFields(ctx, logging.Field("set_mode_outcome", string(result.Outcome)))
		httpx.WriteError(w, http.StatusInternalServerError, "switching mode failed")
	}
}

// handleSetLanguage stores the language the caller reads Playhoot in.
func (h *Handler) handleSetLanguage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	caller, ok := auth.Caller(ctx)
	if !ok {
		httpx.WriteError(w, http.StatusInternalServerError, "caller not resolved")
		return
	}
	var req setLanguageRequest
	if !decode(w, r, &req) {
		return
	}
	outcome, err := h.languages.SetLanguage(ctx, caller.UUID, req.Language)
	if err != nil {
		logging.LogError(ctx, err)
		if errors.Is(err, identity.ErrUserNotFound) {
			httpx.WriteError(w, http.StatusInternalServerError, "caller not found")
			return
		}
		httpx.WriteError(w, http.StatusServiceUnavailable, "choosing the language failed")
		return
	}
	switch outcome {
	case identity.LanguageSet:
		w.Header().Set("Cache-Control", "no-store")
		httpx.WriteJSON(w, http.StatusOK, languageResponse{Language: req.Language})
	case identity.LanguageInvalid:
		httpx.WriteCodedError(w, http.StatusBadRequest, string(outcome), "language must be es, en or pt")
	case identity.LanguageNotAccount:
		httpx.WriteCodedError(w, http.StatusForbidden, string(outcome), "only an account chooses an interface language")
	default:
		logging.LogFields(ctx, logging.Field("set_language_outcome", string(outcome)))
		httpx.WriteError(w, http.StatusInternalServerError, "choosing the language failed")
	}
}
