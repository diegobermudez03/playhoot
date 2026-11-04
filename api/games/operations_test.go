package games

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/orchestrator/gameauthoring"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

func TestApplyChangesActsForTheCallerFromItsOrigin(t *testing.T) {
	h, deps := newTestHandler(gomock.NewController(t))
	base := int64(1)
	change := sessionpkg.DraftChange{
		BaseRevision: 1, Files: []sessionpkg.DraftFileContent{{Path: "notes.md", Content: "hi"}},
		Delete: []string{}, IdempotencyKey: "key-1", Origin: sessionpkg.DraftOriginMCP,
	}
	deps.editor.EXPECT().ApplyDraftChanges(gomock.Any(), callerUUID, gameUUID, change).
		Return(gameauthoring.DraftEdit{Access: game.EditableVersionFound, Result: sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeApplied, Revision: 2}}, nil)

	answered, refusal := h.ApplyChanges(context.Background(), Caller{UserUUID: callerUUID, Origin: sessionpkg.DraftOriginMCP}, gameUUID, ChangeRequest{
		Format: changeFormat, BaseRevision: &base, Files: []ChangeFile{{Path: "notes.md", Content: "hi"}}, Delete: []string{}, IdempotencyKey: "key-1",
	})
	require.Nil(t, refusal)
	require.Equal(t, ChangeResponse{Outcome: "applied", Revision: 2}, answered)

	_, refusal = h.ApplyChanges(context.Background(), Caller{UserUUID: callerUUID}, "not-a-uuid", ChangeRequest{})
	require.Equal(t, http.StatusNotFound, refusal.Status, "a game nobody can have is refused before any domain is called")
}

func TestHandleListGames(t *testing.T) {
	updated := time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)
	tests := map[string]func(t *testing.T, deps dependencies) test{
		"the_callers_games": func(t *testing.T, deps dependencies) test {
			deps.card.EXPECT().ListGames(gomock.Any(), callerUUID).Return([]game.GameSummary{
				{GameUUID: gameUUID, Name: "Quiz", Published: true, Visibility: game.Public, Editing: false, Publishing: false, UpdatedAt: updated},
			}, nil)
			return test{
				method: http.MethodGet, target: "/v1/games", expectedCode: http.StatusOK, noStore: true,
				expectedBody: `[{"game_uuid":"` + gameUUID + `","name":"Quiz","published":true,"visibility":"public","editing":false,"publishing":false,"updated_at":"2026-11-03T09:00:00Z"}]`,
			}
		},
	}
	run(t, tests, func(h *Handler) http.HandlerFunc { return h.handleListGames })
}
