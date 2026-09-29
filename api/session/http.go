package session

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/logging"
	sessionpkg "github.com/diegobermudez03/playhoot/session"
)

// handleCreateSession assumes r's context already carries a started
// request log (Server's own restObservability wraps every REST route
// with one) - it only ever calls logging.LogFields/LogError, never Start
// or FinishRequestLog itself.
func (h *Handler) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req createSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	logging.LogFields(ctx,
		logging.Field("game_uuid", req.GameUUID),
		logging.Field("host_user_uuid", req.HostUserUUID),
	)

	created, err := h.creator.CreateSession(ctx,
		sessionpkg.GameUUID(req.GameUUID),
		sessionpkg.UserUUID(req.HostUserUUID),
		sessionpkg.IdempotencyKey(req.IdempotencyKey),
	)
	if err != nil {
		writeCreateSessionError(w, ctx, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, createSessionResponse{
		SessionUUID:    string(created.SessionUUID),
		JoinCode:       uint(created.JoinCode),
		LobbyExpiresAt: created.LobbyExpiresAt,
	})
}

// writeCreateSessionError maps CreateSession's sentinel errors to the HTTP
// status a client should branch on; anything else is an unexpected
// failure.
func writeCreateSessionError(w http.ResponseWriter, ctx context.Context, err error) {
	logging.LogError(ctx, err)
	switch {
	case errors.Is(err, sessionpkg.ErrGameNotFound):
		httpx.WriteError(w, http.StatusNotFound, "game not found or not currently playable")
	case errors.Is(err, sessionpkg.ErrIdempotencyKeyRequired):
		httpx.WriteError(w, http.StatusBadRequest, "idempotency_key is required")
	case errors.Is(err, sessionpkg.ErrIdempotencyConflict):
		httpx.WriteError(w, http.StatusConflict, "idempotency key reused with a conflicting request")
	case errors.Is(err, sessionpkg.ErrIdempotencyInFlight):
		httpx.WriteError(w, http.StatusConflict, "idempotency key claim is still in flight")
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "creating session failed")
	}
}
