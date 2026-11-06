package games

import (
	"net/http"
	"testing"

	"go.uber.org/mock/gomock"

	composergameauthoring "github.com/diegobermudez03/playhoot/composer/gameauthoring"
	"github.com/diegobermudez03/playhoot/game"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

func TestHandleReadDraftDiff(t *testing.T) {
	diff := sessionpkg.DraftRevisionDiff{From: 2, To: 4, Files: []sessionpkg.DraftRevisionFileDiff{
		{Path: "notes.md", Status: "modified", Diff: "-a\n+b\n"},
		{Path: "server/game.ts", Status: "modified", Diff: "-x\n", Truncated: true},
		{Path: "shared/big.ts", Status: "added", Omitted: true},
	}}
	get := func(target string) test {
		return test{method: http.MethodGet, target: target, gameUUID: gameUUID, noStore: true}
	}
	tests := map[string]func(t *testing.T, deps dependencies) test{
		"from_a_revision_to_the_current_one": func(t *testing.T, deps dependencies) test {
			deps.reader.EXPECT().ReadDraftDiff(gomock.Any(), callerUUID, gameUUID, int64(2), int64(0), "").
				Return(composergameauthoring.DraftDiff{Access: game.EditableVersionFound, Diff: diff}, nil)
			tc := get("/?from=2")
			tc.expectedCode = http.StatusOK
			tc.expectedBody = `{"from":2,"to":4,"files":[{"path":"notes.md","status":"modified","diff":"-a\n+b\n"},
				{"path":"server/game.ts","status":"modified","diff":"-x\n","truncated":true},{"path":"shared/big.ts","status":"added","omitted":true}]}`
			return tc
		},
		"one_path_between_pinned_revisions": func(t *testing.T, deps dependencies) test {
			deps.reader.EXPECT().ReadDraftDiff(gomock.Any(), callerUUID, gameUUID, int64(0), int64(3), "notes.md").
				Return(composergameauthoring.DraftDiff{Access: game.EditableVersionFound, Diff: sessionpkg.DraftRevisionDiff{To: 3, Files: []sessionpkg.DraftRevisionFileDiff{}}}, nil)
			tc := get("/?from=0&to=3&path=notes.md")
			tc.expectedCode = http.StatusOK
			tc.expectedBody = `{"from":0,"to":3,"files":[]}`
			return tc
		},
		"a_revision_it_lacks": func(t *testing.T, deps dependencies) test {
			deps.reader.EXPECT().ReadDraftDiff(gomock.Any(), callerUUID, gameUUID, int64(9), int64(0), "").
				Return(composergameauthoring.DraftDiff{}, sessionpkg.ErrDraftRevisionNotFound)
			tc := get("/?from=9")
			tc.expectedCode = http.StatusNotFound
			tc.expectedBody = `{"code":"revision_not_found","message":"the draft has no such revision"}`
			return tc
		},
		"from_is_required": func(t *testing.T, deps dependencies) test {
			tc := get("/")
			tc.expectedCode = http.StatusBadRequest
			return tc
		},
		"not_owner": func(t *testing.T, deps dependencies) test {
			deps.reader.EXPECT().ReadDraftDiff(gomock.Any(), callerUUID, gameUUID, int64(1), int64(0), "").
				Return(composergameauthoring.DraftDiff{Access: game.EditableVersionNotFound}, nil)
			tc := get("/?from=1")
			tc.expectedCode = http.StatusNotFound
			return tc
		},
	}
	run(t, tests, func(h *Handler) http.HandlerFunc { return h.handleReadDraftDiff })
}
