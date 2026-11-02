package games

import (
	"net/http"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	composergameauthoring "github.com/diegobermudez03/playhoot/composer/gameauthoring"
	"github.com/diegobermudez03/playhoot/game"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

func TestHandleReadDraftBuild(t *testing.T) {
	created := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	succeeded := sessionpkg.DraftBuild{Revision: 3, Status: sessionpkg.DraftBuildSucceeded, QueuedAt: created, StartedAt: &created, FinishedAt: &created,
		AspectRatio: "16:9", Reused: true, Server: &sessionpkg.DraftBuildArtifact{SHA256: "s", Size: 7}, Client: &sessionpkg.DraftBuildArtifact{SHA256: "c", Size: 9}}
	get := func(target string) test {
		return test{method: http.MethodGet, target: target, gameUUID: gameUUID, noStore: true}
	}
	tests := map[string]func(t *testing.T, deps dependencies) test{
		"waits_for_the_revision's_build": func(t *testing.T, deps dependencies) test {
			deps.reader.EXPECT().ReadDraftBuild(gomock.Any(), callerUUID, gameUUID, int64(3), 20*time.Second).
				Return(composergameauthoring.DraftBuildRead{Access: game.EditableVersionFound, Build: succeeded}, nil)
			tc := get("/?revision=3&wait=20")
			tc.expectedCode = http.StatusOK
			tc.expectedBody = `{"revision":3,"status":"SUCCEEDED","diagnostics":[],"server":{"sha256":"s","size":7},"client":{"sha256":"c","size":9},
				"aspect_ratio":"16:9","reused":true,"queued_at":"2026-11-02T12:00:00Z","started_at":"2026-11-02T12:00:00Z","finished_at":"2026-11-02T12:00:00Z"}`
			return tc
		},
		"the_current_revision_without_waiting": func(t *testing.T, deps dependencies) test {
			deps.reader.EXPECT().ReadDraftBuild(gomock.Any(), callerUUID, gameUUID, int64(0), time.Duration(0)).
				Return(composergameauthoring.DraftBuildRead{Access: game.EditableVersionFound, Build: sessionpkg.DraftBuild{Revision: 4, Status: sessionpkg.DraftBuildQueued, QueuedAt: created}}, nil)
			tc := get("/")
			tc.expectedCode = http.StatusOK
			tc.expectedBody = `{"revision":4,"status":"QUEUED","diagnostics":[],"reused":false,"queued_at":"2026-11-02T12:00:00Z"}`
			return tc
		},
		"wait_over_the_limit": func(t *testing.T, deps dependencies) test {
			tc := get("/?wait=21")
			tc.expectedCode = http.StatusBadRequest
			return tc
		},
		"a_revision_without_build": func(t *testing.T, deps dependencies) test {
			deps.reader.EXPECT().ReadDraftBuild(gomock.Any(), callerUUID, gameUUID, int64(2), time.Duration(0)).
				Return(composergameauthoring.DraftBuildRead{}, sessionpkg.ErrDraftBuildNotFound)
			tc := get("/?revision=2")
			tc.expectedCode = http.StatusNotFound
			tc.expectedBody = `{"code":"build_not_found","message":"that revision has no build"}`
			return tc
		},
		"a_version_missing_in_play_is_created_and_read_again": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.reader.EXPECT().ReadDraftBuild(gomock.Any(), callerUUID, gameUUID, int64(0), time.Duration(0)).
					Return(composergameauthoring.DraftBuildRead{Access: game.EditableVersionFound, Missing: true}, nil),
				deps.editor.EXPECT().EnsureDraft(gomock.Any(), callerUUID, gameUUID).Return(game.EditableVersionFound, nil),
				deps.reader.EXPECT().ReadDraftBuild(gomock.Any(), callerUUID, gameUUID, int64(0), time.Duration(0)).
					Return(composergameauthoring.DraftBuildRead{Access: game.EditableVersionFound, Build: succeeded}, nil),
			)
			tc := get("/")
			tc.expectedCode = http.StatusOK
			return tc
		},
		"not_owner": func(t *testing.T, deps dependencies) test {
			deps.reader.EXPECT().ReadDraftBuild(gomock.Any(), callerUUID, gameUUID, int64(0), time.Duration(0)).
				Return(composergameauthoring.DraftBuildRead{Access: game.EditableVersionNotFound}, nil)
			tc := get("/")
			tc.expectedCode = http.StatusNotFound
			return tc
		},
	}
	run(t, tests, func(h *Handler) http.HandlerFunc { return h.handleReadDraftBuild })
}
