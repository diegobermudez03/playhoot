package games

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	composergameauthoring "github.com/diegobermudez03/playhoot/composer/gameauthoring"
	"github.com/diegobermudez03/playhoot/game"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

func TestHandleReadDraftChanged(t *testing.T) {
	changes := sessionpkg.DraftRevisionChanges{
		Since: 2, Revision: 4,
		Files:    []sessionpkg.DraftFile{{Path: "server/game.ts", SHA256: "s", Size: 3}, {Path: "shared/new.ts", SHA256: "n", Size: 1}},
		Added:    []string{"shared/new.ts"},
		Modified: []string{"server/game.ts"},
		Deleted:  []string{"shared/old.ts"},
	}
	get := func(target string) test {
		return test{method: http.MethodGet, target: target, gameUUID: gameUUID, noStore: true}
	}
	tests := map[string]func(t *testing.T, deps dependencies) test{
		"since_a_revision_up_to_the_current_one": func(t *testing.T, deps dependencies) test {
			deps.reader.EXPECT().ReadDraftChanges(gomock.Any(), callerUUID, gameUUID, int64(2), int64(0)).
				Return(composergameauthoring.DraftChanges{Access: game.EditableVersionFound, Changes: changes}, nil)
			tc := get("/?since=2")
			tc.expectedCode = http.StatusOK
			tc.expectedBody = `{"since":2,"revision":4,"files":[{"path":"server/game.ts","sha256":"s","size":3},{"path":"shared/new.ts","sha256":"n","size":1}],
				"added":["shared/new.ts"],"modified":["server/game.ts"],"deleted":["shared/old.ts"]}`
			return tc
		},
		"up_to_a_pinned_revision": func(t *testing.T, deps dependencies) test {
			deps.reader.EXPECT().ReadDraftChanges(gomock.Any(), callerUUID, gameUUID, int64(0), int64(3)).
				Return(composergameauthoring.DraftChanges{Access: game.EditableVersionFound, Changes: sessionpkg.DraftRevisionChanges{Revision: 3}}, nil)
			tc := get("/?revision=3")
			tc.expectedCode = http.StatusOK
			tc.expectedBody = `{"since":0,"revision":3,"files":[],"added":[],"modified":[],"deleted":[]}`
			return tc
		},
		"a_revision_it_lacks": func(t *testing.T, deps dependencies) test {
			deps.reader.EXPECT().ReadDraftChanges(gomock.Any(), callerUUID, gameUUID, int64(9), int64(0)).
				Return(composergameauthoring.DraftChanges{}, sessionpkg.ErrDraftRevisionNotFound)
			tc := get("/?since=9")
			tc.expectedCode = http.StatusNotFound
			return tc
		},
		"a_negative_since": func(t *testing.T, deps dependencies) test {
			tc := get("/?since=-1")
			tc.expectedCode = http.StatusBadRequest
			return tc
		},
		"not_owner": func(t *testing.T, deps dependencies) test {
			deps.reader.EXPECT().ReadDraftChanges(gomock.Any(), callerUUID, gameUUID, int64(0), int64(0)).
				Return(composergameauthoring.DraftChanges{Access: game.EditableVersionNotFound}, nil)
			tc := get("/")
			tc.expectedCode = http.StatusNotFound
			return tc
		},
	}
	run(t, tests, func(h *Handler) http.HandlerFunc { return h.handleReadDraftChanged })
}

func TestHandleReadDraftFiles(t *testing.T) {
	snapshot := aiSnapshot("export default 1;\n")
	get := func(target string) test {
		return test{method: http.MethodGet, target: target, gameUUID: gameUUID, noStore: true}
	}
	tests := map[string]func(t *testing.T, deps dependencies) test{
		"several_files_and_the_missing_ones": func(t *testing.T, deps dependencies) test {
			deps.reader.EXPECT().ReadDraft(gomock.Any(), callerUUID, gameUUID, int64(0), true).
				Return(composergameauthoring.DraftRead{Access: game.EditableVersionFound, Snapshot: snapshot}, nil)
			tc := get("/?path=server/game.ts&path=notes.md&path=shared/none.ts&path=server/game.ts")
			tc.expectedCode = http.StatusOK
			tc.expectedBody = `{"revision":3,"files":[
				{"path":"server/game.ts","sha256":"c","size":18,"content":"export default 1;\n"},
				{"path":"notes.md","sha256":"b","size":25,"content":"# Notes\nFirst to 20 wins."}],
				"missing":["shared/none.ts"]}`
			return tc
		},
		"of_a_pinned_revision": func(t *testing.T, deps dependencies) test {
			deps.reader.EXPECT().ReadDraft(gomock.Any(), callerUUID, gameUUID, int64(2), true).
				Return(composergameauthoring.DraftRead{Access: game.EditableVersionFound, Snapshot: snapshot}, nil)
			tc := get("/?path=notes.md&revision=2")
			tc.expectedCode = http.StatusOK
			return tc
		},
		"no_path": func(t *testing.T, deps dependencies) test {
			tc := get("/")
			tc.expectedCode = http.StatusBadRequest
			return tc
		},
		"too_many_paths": func(t *testing.T, deps dependencies) test {
			tc := get("/?path=" + strings.Repeat("a&path=", maxFilesPerRead) + "a")
			tc.expectedCode = http.StatusBadRequest
			return tc
		},
	}
	run(t, tests, func(h *Handler) http.HandlerFunc { return h.handleReadDraftFiles })
}

func TestAIContextKeepsTheFirstDiagnostics(t *testing.T) {
	h, deps := newTestHandler(gomock.NewController(t))
	snapshot := aiSnapshot("x")
	for range maxContextDiagnostics + 5 {
		snapshot.Build.Diagnostics = append(snapshot.Build.Diagnostics, sessionpkg.DraftBuildDiagnostic{Stage: "types", File: "server/game.ts", Line: 1, Message: "again"})
	}
	deps.reader.EXPECT().ReadDraft(gomock.Any(), callerUUID, gameUUID, int64(0), true).
		Return(composergameauthoring.DraftRead{Access: game.EditableVersionFound, Snapshot: snapshot}, nil)

	read, refusal := h.ReadContext(callerCtx(), callerUUID, gameUUID, "", false)

	require.Nil(t, refusal)
	require.Len(t, read.Response.Draft.Build.Diagnostics, maxContextDiagnostics)
	require.Equal(t, maxContextDiagnostics+6, read.Response.Draft.Build.DiagnosticCount)
	for _, file := range read.Response.Draft.Files {
		require.Nil(t, file.Content)
	}
	require.NotNil(t, read.Response.Notes)
}
