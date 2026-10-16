package play

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/logging"
	sessionpkg "github.com/diegobermudez03/playhoot/play"
)

// createSessionRequest never names the creator: it is the authorized caller.
type createSessionRequest struct {
	GameUUID       string `json:"game_uuid"`
	IdempotencyKey string `json:"idempotency_key"`
}

type createSessionResponse struct {
	SessionUUID    string    `json:"session_uuid"`
	JoinCode       string    `json:"join_code"`
	LobbyExpiresAt time.Time `json:"lobby_expires_at"`
}

const (
	maxCreateSessionBodyBytes = 4 << 10
	maxIdempotencyKeyLength   = 128
)

// handleCreateSession creates a Session for the authorized caller.
func (h *Handler) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userUUID, ok := auth.UserUUID(ctx)
	if !ok {
		// Only possible if the route was registered without its middleware.
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
