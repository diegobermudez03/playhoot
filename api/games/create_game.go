package games

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/orchestrator/gameauthoring"
)

// CreateGameRequest never names the owner: it is the caller.
type CreateGameRequest struct {
	Name           string `json:"name"`
	IdempotencyKey string `json:"idempotency_key"`
	// TemplateGameUUID, when set, starts the game from that template.
	TemplateGameUUID string `json:"template_game_uuid,omitempty"`
}

// CreateGameResponse is a game created with its version being built.
type CreateGameResponse struct {
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
	var req CreateGameRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCreateGameBodyBytes)).Decode(&req); err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	created, refusal := h.CreateGame(ctx, userUUID, req)
	if refusal != nil {
		writeRefusal(w, refusal)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, created)
}

// CreateGame creates an unpublished game for userUUID, with its version being
// built at its first revision.
func (h *Handler) CreateGame(ctx context.Context, userUUID string, req CreateGameRequest) (CreateGameResponse, *Refusal) {
	logging.LogFields(ctx, logging.Field("idempotency_key", req.IdempotencyKey))
	if refusal := idempotencyKeyRefusal(req.IdempotencyKey, "idempotency_key"); refusal != nil {
		return CreateGameResponse{}, refusal
	}
	if req.TemplateGameUUID != "" {
		template, err := uuid.Parse(req.TemplateGameUUID)
		if err != nil {
			return CreateGameResponse{}, templateNotFound()
		}
		req.TemplateGameUUID = template.String()
		logging.LogFields(ctx, logging.Field("template_game_uuid", req.TemplateGameUUID))
	}
	created, err := h.creator.CreateGame(ctx, userUUID, req.Name, req.IdempotencyKey, req.TemplateGameUUID)
	if err != nil {
		logging.LogError(ctx, err)
		switch {
		case errors.Is(err, game.ErrInvalidCard):
			return CreateGameResponse{}, refuseCoded(http.StatusUnprocessableEntity, codeInvalidCard, "name is required and holds at most 80 characters")
		case errors.Is(err, gameauthoring.ErrTemplateNotFound):
			return CreateGameResponse{}, templateNotFound()
		case errors.Is(err, game.ErrIdempotencyConflict):
			return CreateGameResponse{}, refuseCoded(http.StatusConflict, codeIdempotencyConflict, "idempotency key reused with a conflicting request")
		default:
			return CreateGameResponse{}, refuse(http.StatusInternalServerError, "creating game failed")
		}
	}
	logging.LogFields(ctx, logging.Field("game_uuid", created.GameUUID))
	return CreateGameResponse{GameUUID: created.GameUUID, VersionUUID: created.VersionUUID, Revision: created.Revision}, nil
}
