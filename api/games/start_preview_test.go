package games

import (
	"errors"
	"net/http"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/orchestrator/gameauthoring"
	"github.com/diegobermudez03/playhoot/play/runtime"
)

func TestHandleStartPreview(t *testing.T) {
	post := func(body string) test {
		return test{method: http.MethodPost, target: "/", gameUUID: gameUUID, body: body, noStore: true}
	}
	answer := func(deps dependencies, revision int64, players int, result runtime.StartPreviewResult) {
		deps.editor.EXPECT().StartPreview(gomock.Any(), callerUUID, gameUUID, revision, players).
			Return(gameauthoring.PreviewStart{Access: game.EditableVersionFound, Result: result}, nil)
	}
	tests := map[string]func(t *testing.T, deps dependencies) test{
		"started": func(t *testing.T, deps dependencies) test {
			answer(deps, 4, 2, runtime.StartPreviewResult{Outcome: runtime.PreviewStarted, PreviewUUID: "p-1", Revision: 4, Participants: []string{"P1", "P2"}})
			tc := post(`{"revision":4,"players":2}`)
			tc.expectedCode = http.StatusCreated
			tc.expectedBody = `{"preview_uuid":"p-1","revision":4,"participants":["P1","P2"]}`
			return tc
		},
		"build_not_ready": func(t *testing.T, deps dependencies) test {
			answer(deps, 0, 1, runtime.StartPreviewResult{Outcome: runtime.PreviewBuildNotReady, BuildStatus: "FAILED"})
			tc := post(`{"players":1}`)
			tc.expectedCode = http.StatusConflict
			tc.expectedBody = `{"code":"build_not_ready","message":"only a revision whose build SUCCEEDED can be previewed","build_status":"FAILED"}`
			return tc
		},
		"invalid_players": func(t *testing.T, deps dependencies) test {
			answer(deps, 0, 9, runtime.StartPreviewResult{Outcome: runtime.PreviewInvalidPlayers, MinPlayers: 1, MaxPlayers: 8})
			tc := post(`{"players":9}`)
			tc.expectedCode = http.StatusUnprocessableEntity
			tc.expectedBody = `{"code":"invalid_players","message":"the game is played by min..max players","min":1,"max":8}`
			return tc
		},
		"unavailable": func(t *testing.T, deps dependencies) test {
			answer(deps, 0, 1, runtime.StartPreviewResult{Outcome: runtime.PreviewUnavailable})
			tc := post(`{"players":1}`)
			tc.expectedCode = http.StatusServiceUnavailable
			return tc
		},
		"script_failure": func(t *testing.T, deps dependencies) test {
			answer(deps, 0, 1, runtime.StartPreviewResult{Outcome: runtime.PreviewScriptFailure})
			tc := post(`{"players":1}`)
			tc.expectedCode = http.StatusUnprocessableEntity
			return tc
		},
		"someone_who_may_not_edit": func(t *testing.T, deps dependencies) test {
			deps.editor.EXPECT().StartPreview(gomock.Any(), callerUUID, gameUUID, int64(0), 1).Return(gameauthoring.PreviewStart{Access: game.EditableVersionNotFound}, nil)
			tc := post(`{"players":1}`)
			tc.expectedCode = http.StatusNotFound
			return tc
		},
		"no_players": func(t *testing.T, deps dependencies) test {
			tc := post(`{}`)
			tc.expectedCode = http.StatusBadRequest
			return tc
		},
		"failure": func(t *testing.T, deps dependencies) test {
			deps.editor.EXPECT().StartPreview(gomock.Any(), callerUUID, gameUUID, int64(0), 1).Return(gameauthoring.PreviewStart{}, errors.New("down"))
			tc := post(`{"players":1}`)
			tc.expectedCode = http.StatusInternalServerError
			return tc
		},
	}
	run(t, tests, func(h *Handler) http.HandlerFunc { return h.handleStartPreview })
}
