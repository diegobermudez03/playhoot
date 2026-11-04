package games

import (
	"context"
	"net/http"
	"time"

	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/logging"
)

// GameSummaryResponse is one of the caller's games in a list.
type GameSummaryResponse struct {
	GameUUID   string    `json:"game_uuid"`
	Name       string    `json:"name"`
	Published  bool      `json:"published"`
	Visibility string    `json:"visibility"`
	Editing    bool      `json:"editing"`
	Publishing bool      `json:"publishing"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// handleListGames answers the caller's games, the most recently changed
// first.
func (h *Handler) handleListGames(w http.ResponseWriter, r *http.Request) {
	userUUID, _, ok := caller(w, r, "listing games")
	if !ok {
		return
	}
	noStore(w)
	games, refusal := h.ListGames(r.Context(), userUUID)
	if refusal != nil {
		writeRefusal(w, refusal)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, games)
}

// ListGames answers userUUID's games, the most recently changed first.
func (h *Handler) ListGames(ctx context.Context, userUUID string) ([]GameSummaryResponse, *Refusal) {
	listed, err := h.cards.Card.ListGames(ctx, userUUID)
	if err != nil {
		logging.LogError(ctx, err)
		return nil, refuse(http.StatusInternalServerError, "listing games failed")
	}
	games := make([]GameSummaryResponse, len(listed))
	for i, summary := range listed {
		games[i] = GameSummaryResponse{
			GameUUID: summary.GameUUID, Name: summary.Name, Published: summary.Published, Visibility: string(summary.Visibility),
			Editing: summary.Editing, Publishing: summary.Publishing, UpdatedAt: summary.UpdatedAt,
		}
	}
	return games, nil
}
