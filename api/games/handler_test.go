package games

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	composergameauthoring "github.com/diegobermudez03/playhoot/composer/gameauthoring"
	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/orchestrator/gameauthoring"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

const (
	callerUUID  = "0f8fad5b-d9cb-469f-a165-70867728950e"
	gameUUID    = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	versionUUID = "16fd2706-8baf-433b-82eb-8c7fada847da"
)

type dependencies struct {
	creator *MockgameCreator
	editor  *MockdraftEditor
	reader  *MockdraftReader
}

type test struct {
	method       string
	target       string
	gameUUID     string
	pathValues   map[string]string
	body         string
	headers      map[string]string
	expectedCode int
	expectedBody string
	noStore      bool
}

func callerCtx() context.Context {
	return auth.WithCaller(context.Background(), identity.User{UUID: callerUUID})
}

func run(t *testing.T, tests map[string]func(t *testing.T, deps dependencies) test, handle func(h *Handler) http.HandlerFunc) {
	t.Helper()
	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			deps := dependencies{creator: NewMockgameCreator(ctrl), editor: NewMockdraftEditor(ctrl), reader: NewMockdraftReader(ctrl)}
			h := NewHandler(nil, deps.creator, deps.editor, deps.reader)
			tc := setup(t, deps)

			request := httptest.NewRequest(tc.method, tc.target, strings.NewReader(tc.body)).WithContext(callerCtx())
			if tc.gameUUID != "" {
				request.SetPathValue("gameUUID", tc.gameUUID)
			}
			for name, value := range tc.pathValues {
				request.SetPathValue(name, value)
			}
			for name, value := range tc.headers {
				request.Header.Set(name, value)
			}
			recorder := httptest.NewRecorder()
			handle(h)(recorder, request)

			require.Equal(t, tc.expectedCode, recorder.Code, recorder.Body.String())
			if tc.expectedBody != "" {
				require.JSONEq(t, tc.expectedBody, recorder.Body.String())
			}
			if tc.noStore {
				require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
			}
		})
	}
}

func TestHandleCreateGame(t *testing.T) {
	post := func(body string) test {
		return test{method: http.MethodPost, target: "/v1/games", body: body}
	}
	tests := map[string]func(t *testing.T, deps dependencies) test{
		"game_created": func(t *testing.T, deps dependencies) test {
			deps.creator.EXPECT().CreateGame(gomock.Any(), callerUUID, "key-1").
				Return(gameauthoring.CreatedGame{GameUUID: gameUUID, VersionUUID: versionUUID, Revision: 1}, nil)
			tc := post(`{"idempotency_key":"key-1"}`)
			tc.expectedCode = http.StatusCreated
			tc.expectedBody = `{"game_uuid":"` + gameUUID + `","version_uuid":"` + versionUUID + `","revision":1}`
			return tc
		},
		"missing_key": func(t *testing.T, deps dependencies) test {
			tc := post(`{}`)
			tc.expectedCode = http.StatusBadRequest
			return tc
		},
		"invalid_body": func(t *testing.T, deps dependencies) test {
			tc := post(`{`)
			tc.expectedCode = http.StatusBadRequest
			return tc
		},
		"creation_fails": func(t *testing.T, deps dependencies) test {
			deps.creator.EXPECT().CreateGame(gomock.Any(), callerUUID, "key-1").Return(gameauthoring.CreatedGame{}, errors.New("down"))
			tc := post(`{"idempotency_key":"key-1"}`)
			tc.expectedCode = http.StatusInternalServerError
			return tc
		},
	}
	run(t, tests, func(h *Handler) http.HandlerFunc { return h.handleCreateGame })
}

func TestHandleReadDraft(t *testing.T) {
	content := "export {};\n"
	revision := int64(2)
	created := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	snapshot := sessionpkg.DraftSnapshot{
		VersionUUID: versionUUID, GameUUID: gameUUID, Status: "DRAFT", CurrentRevision: 2, Revision: 2,
		Files: []sessionpkg.DraftFile{{Path: "server/game.ts", SHA256: "abc", Size: 11, Content: &content}},
		Attempts: []sessionpkg.DraftChangeAttempt{
			{Kind: sessionpkg.DraftAttemptAssetUpload, Outcome: string(sessionpkg.DraftAssetAttached), BaseRevision: 1, Revision: &revision, Origin: "web", CreatedAt: created},
			{Kind: sessionpkg.DraftAttemptChange, Outcome: string(sessionpkg.DraftChangeInvalid), BaseRevision: 1, Origin: "web", Diagnostics: []sessionpkg.DraftDiagnostic{{Path: "/x", Code: "invalid_path", Message: "bad"}}, CreatedAt: created},
		},
		Assets: []sessionpkg.DraftAsset{
			{Key: "cardBack", Type: "image", Description: "Card back", Status: sessionpkg.DraftAssetReady, SHA256: "p", Size: 3, MIMEType: "image/png", URL: "https://asset", URLExpiresAt: created},
			{Key: "flip", Type: "audio", Description: "Flip", Optional: true, Status: sessionpkg.DraftAssetMissing},
		},
		Build: &sessionpkg.DraftBuild{Revision: 2, Status: sessionpkg.DraftBuildFailed, QueuedAt: created, FinishedAt: &created,
			Diagnostics: []sessionpkg.DraftBuildDiagnostic{{Stage: "types", File: "server/game.ts", Line: 3, Column: 5, Message: "no"}}},
		LastSuccessfulBuild: &sessionpkg.DraftBuild{Revision: 1, Status: sessionpkg.DraftBuildSucceeded, QueuedAt: created, AspectRatio: "16:9",
			Server: &sessionpkg.DraftBuildArtifact{SHA256: "s", Size: 7}, Client: &sessionpkg.DraftBuildArtifact{SHA256: "c", Size: 9}},
	}
	get := func(target string) test {
		return test{method: http.MethodGet, target: target, gameUUID: gameUUID, noStore: true}
	}
	tests := map[string]func(t *testing.T, deps dependencies) test{
		"current_revision_with_content": func(t *testing.T, deps dependencies) test {
			deps.reader.EXPECT().ReadDraft(gomock.Any(), callerUUID, gameUUID, int64(0), true).
				Return(composergameauthoring.DraftRead{Access: game.EditableVersionFound, Snapshot: snapshot}, nil)
			tc := get("/v1/games/" + gameUUID + "/draft?content=true")
			tc.expectedCode = http.StatusOK
			tc.expectedBody = `{"game_uuid":"` + gameUUID + `","version_uuid":"` + versionUUID + `","status":"DRAFT","current_revision":2,"revision":2,
				"files":[{"path":"server/game.ts","sha256":"abc","size":11,"content":"export {};\n"}],
				"assets":[{"key":"cardBack","type":"image","description":"Card back","optional":false,"status":"ready","sha256":"p","size":3,"mime_type":"image/png","url":"https://asset","url_expires_at":"2026-11-01T12:00:00Z"},
					{"key":"flip","type":"audio","description":"Flip","optional":true,"status":"missing"}],
				"asset_diagnostics":[],
				"attempts":[{"kind":"asset_upload","outcome":"attached","base_revision":1,"revision":2,"origin":"web","diagnostics":[],"created_at":"2026-11-01T12:00:00Z"},
					{"kind":"change","outcome":"invalid","base_revision":1,"origin":"web","diagnostics":[{"path":"/x","code":"invalid_path","message":"bad"}],"created_at":"2026-11-01T12:00:00Z"}],
				"build":{"revision":2,"status":"FAILED","diagnostics":[{"stage":"types","file":"server/game.ts","line":3,"column":5,"message":"no"}],"reused":false,
					"queued_at":"2026-11-01T12:00:00Z","finished_at":"2026-11-01T12:00:00Z"},
				"last_successful_build":{"revision":1,"status":"SUCCEEDED","diagnostics":[],"server":{"sha256":"s","size":7},"client":{"sha256":"c","size":9},
					"aspect_ratio":"16:9","reused":false,"queued_at":"2026-11-01T12:00:00Z"}}`
			return tc
		},
		"a_given_revision_without_content": func(t *testing.T, deps dependencies) test {
			deps.reader.EXPECT().ReadDraft(gomock.Any(), callerUUID, gameUUID, int64(1), false).
				Return(composergameauthoring.DraftRead{Access: game.EditableVersionFound, Snapshot: snapshot}, nil)
			tc := get("/v1/games/" + gameUUID + "/draft?revision=1")
			tc.expectedCode = http.StatusOK
			return tc
		},
		"a_version_missing_in_play_is_created_and_read_again": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.reader.EXPECT().ReadDraft(gomock.Any(), callerUUID, gameUUID, int64(0), false).
					Return(composergameauthoring.DraftRead{Access: game.EditableVersionFound, Missing: true}, nil),
				deps.editor.EXPECT().EnsureDraft(gomock.Any(), callerUUID, gameUUID).Return(game.EditableVersionFound, nil),
				deps.reader.EXPECT().ReadDraft(gomock.Any(), callerUUID, gameUUID, int64(0), false).
					Return(composergameauthoring.DraftRead{Access: game.EditableVersionFound, Snapshot: snapshot}, nil),
			)
			tc := get("/v1/games/" + gameUUID + "/draft")
			tc.expectedCode = http.StatusOK
			return tc
		},
		"a_failed_repair_is_an_error": func(t *testing.T, deps dependencies) test {
			deps.reader.EXPECT().ReadDraft(gomock.Any(), callerUUID, gameUUID, int64(0), false).
				Return(composergameauthoring.DraftRead{Access: game.EditableVersionFound, Missing: true}, nil)
			deps.editor.EXPECT().EnsureDraft(gomock.Any(), callerUUID, gameUUID).Return(game.EditableVersionOutcome(""), errors.New("database down"))
			tc := get("/v1/games/" + gameUUID + "/draft")
			tc.expectedCode = http.StatusInternalServerError
			return tc
		},
		"still_missing_after_the_repair_is_an_error": func(t *testing.T, deps dependencies) test {
			deps.reader.EXPECT().ReadDraft(gomock.Any(), callerUUID, gameUUID, int64(0), false).
				Return(composergameauthoring.DraftRead{Access: game.EditableVersionFound, Missing: true}, nil).Times(2)
			deps.editor.EXPECT().EnsureDraft(gomock.Any(), callerUUID, gameUUID).Return(game.EditableVersionFound, nil)
			tc := get("/v1/games/" + gameUUID + "/draft")
			tc.expectedCode = http.StatusInternalServerError
			return tc
		},
		"not_owner_or_unknown_is_not_found": func(t *testing.T, deps dependencies) test {
			deps.reader.EXPECT().ReadDraft(gomock.Any(), callerUUID, gameUUID, int64(0), false).
				Return(composergameauthoring.DraftRead{Access: game.EditableVersionNotFound}, nil)
			tc := get("/v1/games/" + gameUUID + "/draft")
			tc.expectedCode = http.StatusNotFound
			return tc
		},
		"no_version_being_built": func(t *testing.T, deps dependencies) test {
			deps.reader.EXPECT().ReadDraft(gomock.Any(), callerUUID, gameUUID, int64(0), false).
				Return(composergameauthoring.DraftRead{Access: game.EditableVersionNotEditable}, nil)
			tc := get("/v1/games/" + gameUUID + "/draft")
			tc.expectedCode = http.StatusConflict
			tc.expectedBody = `{"code":"not_editable","message":"the game has no version being built"}`
			return tc
		},
		"revision_past_the_current_one": func(t *testing.T, deps dependencies) test {
			deps.reader.EXPECT().ReadDraft(gomock.Any(), callerUUID, gameUUID, int64(9), false).
				Return(composergameauthoring.DraftRead{}, sessionpkg.ErrDraftRevisionNotFound)
			tc := get("/v1/games/" + gameUUID + "/draft?revision=9")
			tc.expectedCode = http.StatusNotFound
			tc.expectedBody = `{"code":"revision_not_found","message":"the draft has no such revision"}`
			return tc
		},
		"invalid_revision": func(t *testing.T, deps dependencies) test {
			tc := get("/v1/games/" + gameUUID + "/draft?revision=zero")
			tc.expectedCode = http.StatusBadRequest
			return tc
		},
		"game_uuid_not_a_uuid": func(t *testing.T, deps dependencies) test {
			tc := get("/v1/games/nope/draft")
			tc.gameUUID = "nope"
			tc.noStore = false
			tc.expectedCode = http.StatusNotFound
			return tc
		},
	}
	run(t, tests, func(h *Handler) http.HandlerFunc { return h.handleReadDraft })
}

func TestHandleApplyDraftChanges(t *testing.T) {
	change := sessionpkg.DraftChange{
		BaseRevision: 1, Files: []sessionpkg.DraftFileContent{{Path: "notes.md", Content: "hi"}},
		Delete: []string{"shared/old.ts"}, IdempotencyKey: "key-1", Origin: sessionpkg.DraftOriginWeb,
	}
	body := `{"format":"playhoot.project-change.v1","base_revision":1,"files":[{"path":"notes.md","content":"hi"}],"delete":["shared/old.ts"],"idempotency_key":"key-1"}`
	post := func(body string) test {
		return test{method: http.MethodPost, target: "/v1/games/" + gameUUID + "/draft/changes", gameUUID: gameUUID, body: body, noStore: true}
	}
	answers := func(deps dependencies, edit gameauthoring.DraftEdit, err error) {
		deps.editor.EXPECT().ApplyDraftChanges(gomock.Any(), callerUUID, gameUUID, change).Return(edit, err)
	}
	found := game.EditableVersionFound
	tests := map[string]func(t *testing.T, deps dependencies) test{
		"applied": func(t *testing.T, deps dependencies) test {
			answers(deps, gameauthoring.DraftEdit{Access: found, Result: sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeApplied, Revision: 2}}, nil)
			tc := post(body)
			tc.expectedCode = http.StatusOK
			tc.expectedBody = `{"outcome":"applied","revision":2}`
			return tc
		},
		"a_change_that_changes_nothing": func(t *testing.T, deps dependencies) test {
			answers(deps, gameauthoring.DraftEdit{Access: found, Result: sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeUnchanged, Revision: 1}}, nil)
			tc := post(body)
			tc.expectedCode = http.StatusOK
			tc.expectedBody = `{"outcome":"unchanged","revision":1}`
			return tc
		},
		"a_replay_answers_the_same": func(t *testing.T, deps dependencies) test {
			answers(deps, gameauthoring.DraftEdit{Access: found, Result: sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeApplied, Revision: 2, Replayed: true}}, nil)
			tc := post(body)
			tc.expectedCode = http.StatusOK
			tc.expectedBody = `{"outcome":"applied","revision":2}`
			return tc
		},
		"stale_base_is_a_conflict": func(t *testing.T, deps dependencies) test {
			answers(deps, gameauthoring.DraftEdit{Access: found, Result: sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeConflict, Revision: 3}}, nil)
			tc := post(body)
			tc.expectedCode = http.StatusConflict
			tc.expectedBody = `{"code":"revision_conflict","message":"the draft changed since base_revision; read it again and redo the change on the current revision","current_revision":3}`
			return tc
		},
		"invalid_change_has_diagnostics": func(t *testing.T, deps dependencies) test {
			answers(deps, gameauthoring.DraftEdit{Access: found, Result: sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeInvalid, Revision: 1,
				Diagnostics: []sessionpkg.DraftDiagnostic{{Path: "../x", Code: "invalid_path", Message: "outside the project"}}}}, nil)
			tc := post(body)
			tc.expectedCode = http.StatusUnprocessableEntity
			tc.expectedBody = `{"code":"invalid_change","message":"the change was not applied; see diagnostics","diagnostics":[{"path":"../x","code":"invalid_path","message":"outside the project"}]}`
			return tc
		},
		"version_no_longer_editable": func(t *testing.T, deps dependencies) test {
			answers(deps, gameauthoring.DraftEdit{Access: found, Result: sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeNotEditable}}, nil)
			tc := post(body)
			tc.expectedCode = http.StatusConflict
			return tc
		},
		"not_owner": func(t *testing.T, deps dependencies) test {
			answers(deps, gameauthoring.DraftEdit{Access: game.EditableVersionNotFound}, nil)
			tc := post(body)
			tc.expectedCode = http.StatusNotFound
			return tc
		},
		"reused_key": func(t *testing.T, deps dependencies) test {
			answers(deps, gameauthoring.DraftEdit{}, sessionpkg.ErrIdempotencyConflict)
			tc := post(body)
			tc.expectedCode = http.StatusConflict
			tc.expectedBody = `{"code":"idempotency_conflict","message":"idempotency key reused with a conflicting request"}`
			return tc
		},
		"unknown_format": func(t *testing.T, deps dependencies) test {
			tc := post(strings.Replace(body, "project-change.v1", "project-change.v9", 1))
			tc.expectedCode = http.StatusBadRequest
			return tc
		},
		"missing_base_revision": func(t *testing.T, deps dependencies) test {
			tc := post(`{"format":"playhoot.project-change.v1","files":[],"idempotency_key":"key-1"}`)
			tc.expectedCode = http.StatusBadRequest
			return tc
		},
		"missing_key": func(t *testing.T, deps dependencies) test {
			tc := post(`{"format":"playhoot.project-change.v1","base_revision":1}`)
			tc.expectedCode = http.StatusBadRequest
			return tc
		},
		"failure": func(t *testing.T, deps dependencies) test {
			answers(deps, gameauthoring.DraftEdit{}, errors.New("down"))
			tc := post(body)
			tc.expectedCode = http.StatusInternalServerError
			return tc
		},
	}
	run(t, tests, func(h *Handler) http.HandlerFunc { return h.handleApplyDraftChanges })
}

func TestHandleRestoreDraftRevision(t *testing.T) {
	post := func(body string) test {
		return test{method: http.MethodPost, target: "/v1/games/" + gameUUID + "/draft/restore", gameUUID: gameUUID, body: body, noStore: true}
	}
	tests := map[string]func(t *testing.T, deps dependencies) test{
		"restored": func(t *testing.T, deps dependencies) test {
			deps.editor.EXPECT().RestoreDraftRevision(gomock.Any(), callerUUID, gameUUID, int64(1), int64(2), "key-2", sessionpkg.DraftOriginWeb).
				Return(gameauthoring.DraftEdit{Access: game.EditableVersionFound, Result: sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeApplied, Revision: 3}}, nil)
			tc := post(`{"revision":1,"base_revision":2,"idempotency_key":"key-2"}`)
			tc.expectedCode = http.StatusOK
			tc.expectedBody = `{"outcome":"applied","revision":3}`
			return tc
		},
		"invalid_revision": func(t *testing.T, deps dependencies) test {
			tc := post(`{"revision":0,"base_revision":2,"idempotency_key":"key-2"}`)
			tc.expectedCode = http.StatusBadRequest
			return tc
		},
	}
	run(t, tests, func(h *Handler) http.HandlerFunc { return h.handleRestoreDraftRevision })
}

func TestHandleListDraftRevisions(t *testing.T) {
	created := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	tests := map[string]func(t *testing.T, deps dependencies) test{
		"listed": func(t *testing.T, deps dependencies) test {
			deps.reader.EXPECT().ListDraftRevisions(gomock.Any(), callerUUID, gameUUID).
				Return(composergameauthoring.DraftRevisions{Access: game.EditableVersionFound, Revisions: []sessionpkg.DraftRevision{{Number: 1, Source: "template", Files: 5, CreatedAt: created}}}, nil)
			return test{method: http.MethodGet, target: "/", gameUUID: gameUUID, noStore: true, expectedCode: http.StatusOK,
				expectedBody: `[{"number":1,"source":"template","files":5,"created_at":"2026-11-01T12:00:00Z"}]`}
		},
		"a_version_missing_in_play_is_created_and_listed_again": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.reader.EXPECT().ListDraftRevisions(gomock.Any(), callerUUID, gameUUID).
					Return(composergameauthoring.DraftRevisions{Access: game.EditableVersionFound, Missing: true}, nil),
				deps.editor.EXPECT().EnsureDraft(gomock.Any(), callerUUID, gameUUID).Return(game.EditableVersionFound, nil),
				deps.reader.EXPECT().ListDraftRevisions(gomock.Any(), callerUUID, gameUUID).
					Return(composergameauthoring.DraftRevisions{Access: game.EditableVersionFound, Revisions: []sessionpkg.DraftRevision{{Number: 1, Source: "template", Files: 5, CreatedAt: created}}}, nil),
			)
			return test{method: http.MethodGet, target: "/", gameUUID: gameUUID, noStore: true, expectedCode: http.StatusOK}
		},
		"not_owner": func(t *testing.T, deps dependencies) test {
			deps.reader.EXPECT().ListDraftRevisions(gomock.Any(), callerUUID, gameUUID).
				Return(composergameauthoring.DraftRevisions{Access: game.EditableVersionNotFound}, nil)
			return test{method: http.MethodGet, target: "/", gameUUID: gameUUID, expectedCode: http.StatusNotFound}
		},
	}
	run(t, tests, func(h *Handler) http.HandlerFunc { return h.handleListDraftRevisions })
}
