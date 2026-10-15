package session

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/logging"
	sessionpkg "github.com/diegobermudez03/playhoot/play"
)

const (
	maxCreateSessionBodyBytes = 4 << 10
	maxIdempotencyKeyLength   = 128
)

// handleCreateSession assumes r's context already carries a started request
// log (Server's own restObservability wraps every REST route with one) and
// that auth.Require already authorized the caller - it only ever calls
// logging.LogFields/LogError, never Start or FinishRequestLog itself.
//
// It records no metrics of its own: how long the request took and how it
// ended over HTTP is counted for every route by restObservability, and how
// many Sessions were actually created is for the Session domain to count,
// since this endpoint is only one way in.
func (h *Handler) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userUUID, ok := auth.UserUUID(ctx)
	if !ok {
		// Reaching the handler without the middleware's caller is a wiring
		// bug, never an anonymous caller to serve.
		logging.LogFields(ctx, logging.Field("wiring_error", "create session reached without an authorized caller"))
		httpx.WriteError(w, http.StatusInternalServerError, "creating session failed")
		return
	}

	var req createSessionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCreateSessionBodyBytes)).Decode(&req); err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	logging.LogFields(ctx,
		logging.Field("game_uuid", req.GameUUID),
		logging.Field("idempotency_key", req.IdempotencyKey),
	)

	gameUUID, err := uuid.Parse(req.GameUUID)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "game_uuid must be a valid UUID")
		return
	}
	if req.IdempotencyKey == "" || len(req.IdempotencyKey) > maxIdempotencyKeyLength {
		httpx.WriteError(w, http.StatusBadRequest, "idempotency_key is required and must be at most 128 characters")
		return
	}

	created, err := h.creator.CreateSession(ctx,
		userUUID,
		gameUUID.String(),
		req.IdempotencyKey,
	)
	if err != nil {
		logging.LogError(ctx, err)
		if errors.Is(err, sessionpkg.ErrIdempotencyConflict) {
			httpx.WriteError(w, http.StatusConflict, "idempotency key reused with a conflicting request")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "creating session failed")
		return
	}

	switch created.Outcome {
	case sessionpkg.CreateSessionCreated:
		logging.LogFields(ctx, logging.Field("session_uuid", created.SessionUUID))
		httpx.WriteJSON(w, http.StatusCreated, createSessionResponse{
			SessionUUID:    created.SessionUUID,
			JoinCode:       created.JoinCode,
			LobbyExpiresAt: created.ExpiresAt,
		})
	case sessionpkg.CreateSessionGameUnavailable:
		httpx.WriteError(w, http.StatusNotFound, "game not found or not currently playable")
	default:
		logging.LogFields(ctx, logging.Field("create_session_outcome", string(created.Outcome)))
		httpx.WriteError(w, http.StatusInternalServerError, "creating session failed")
	}
}
