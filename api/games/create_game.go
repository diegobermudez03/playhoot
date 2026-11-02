package games

import (
	"encoding/json"
	"net/http"

	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/logging"
)

// createGameRequest never names the owner: it is the authorized caller.
type createGameRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
}

type createGameResponse struct {
	GameUUID    string `json:"game_uuid"`
	VersionUUID string `json:"version_uuid"`
	Revision    int64  `json:"revision"`
}

const maxCreateGameBodyBytes = 4 << 10

// handleCreateGame creates an unpublished game for the caller, with its
// version being built at its first revision.
func (h *Handler) handleCreateGame(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userUUID, _, ok := caller(w, r, "creating game")
	if !ok {
		return
	}

	var req createGameRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCreateGameBodyBytes)).Decode(&req); err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	logging.LogFields(ctx, logging.Field("idempotency_key", req.IdempotencyKey))
	if req.IdempotencyKey == "" || len(req.IdempotencyKey) > maxIdempotencyKeyLength {
		httpx.WriteError(w, http.StatusBadRequest, "idempotency_key is required and must be at most 128 characters")
		return
	}

	created, err := h.creator.CreateGame(ctx, userUUID, req.IdempotencyKey)
	if err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusInternalServerError, "creating game failed")
		return
	}
	logging.LogFields(ctx, logging.Field("game_uuid", created.GameUUID))
	httpx.WriteJSON(w, http.StatusCreated, createGameResponse{
		GameUUID:    created.GameUUID,
		VersionUUID: created.VersionUUID,
		Revision:    created.Revision,
	})
}
