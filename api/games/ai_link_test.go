package games

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	composergameauthoring "github.com/diegobermudez03/playhoot/composer/gameauthoring"
	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/orchestrator/gameauthoring"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

const testLinkToken = "phl_0123456789abcdefghijklmnopqrstuvwxyzABCDEFG"

func TestAILinkOwnerEndpoints(t *testing.T) {
	expires := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	created := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)

	run(t, map[string]func(t *testing.T, deps dependencies) test{
		"the_owner_gets_the_link_once": func(t *testing.T, deps dependencies) test {
			deps.owner.EXPECT().CreateAILink(gomock.Any(), callerUUID, gameUUID).Return(gameauthoring.AILinkCreation{
				Access: game.EditableVersionNotEditable, Link: identity.CreatedResourceLink{Token: testLinkToken, CreatedAt: created, ExpiresAt: expires},
			}, nil)
			return test{method: http.MethodPost, target: "/", gameUUID: gameUUID, expectedCode: http.StatusCreated, noStore: true,
				expectedBody: `{"url":"` + testPublicBaseURL + `/ai/v1/` + testLinkToken + `","expires_at":"2026-11-09T12:00:00Z"}`}
		},
		"another_person_gets_not_found": func(t *testing.T, deps dependencies) test {
			deps.owner.EXPECT().CreateAILink(gomock.Any(), callerUUID, gameUUID).Return(gameauthoring.AILinkCreation{Access: game.EditableVersionNotFound}, nil)
			return test{method: http.MethodPost, target: "/", gameUUID: gameUUID, expectedCode: http.StatusNotFound}
		},
		"a_failure_is_an_error": func(t *testing.T, deps dependencies) test {
			deps.owner.EXPECT().CreateAILink(gomock.Any(), callerUUID, gameUUID).Return(gameauthoring.AILinkCreation{}, errors.New("down"))
			return test{method: http.MethodPost, target: "/", gameUUID: gameUUID, expectedCode: http.StatusInternalServerError}
		},
	}, func(h *Handler) http.HandlerFunc { return h.handleCreateAILink })

	run(t, map[string]func(t *testing.T, deps dependencies) test{
		"an_active_link_is_told_without_its_token": func(t *testing.T, deps dependencies) test {
			deps.status.EXPECT().AILinkStatus(gomock.Any(), callerUUID, gameUUID).Return(composergameauthoring.AILinkStatusRead{
				Access: game.EditableVersionFound, Status: identity.ResourceLinkStatus{Active: true, CreatedAt: created, ExpiresAt: expires},
			}, nil)
			return test{method: http.MethodGet, target: "/", gameUUID: gameUUID, expectedCode: http.StatusOK, noStore: true,
				expectedBody: `{"active":true,"created_at":"2026-11-02T12:00:00Z","expires_at":"2026-11-09T12:00:00Z"}`}
		},
		"no_link": func(t *testing.T, deps dependencies) test {
			deps.status.EXPECT().AILinkStatus(gomock.Any(), callerUUID, gameUUID).Return(composergameauthoring.AILinkStatusRead{Access: game.EditableVersionFound}, nil)
			return test{method: http.MethodGet, target: "/", gameUUID: gameUUID, expectedCode: http.StatusOK, expectedBody: `{"active":false}`}
		},
		"another_person_gets_not_found": func(t *testing.T, deps dependencies) test {
			deps.status.EXPECT().AILinkStatus(gomock.Any(), callerUUID, gameUUID).Return(composergameauthoring.AILinkStatusRead{Access: game.EditableVersionNotFound}, nil)
			return test{method: http.MethodGet, target: "/", gameUUID: gameUUID, expectedCode: http.StatusNotFound}
		},
	}, func(h *Handler) http.HandlerFunc { return h.handleAILinkStatus })

	run(t, map[string]func(t *testing.T, deps dependencies) test{
		"the_owner_revokes": func(t *testing.T, deps dependencies) test {
			deps.owner.EXPECT().RevokeAILink(gomock.Any(), callerUUID, gameUUID).Return(game.EditableVersionFound, nil)
			return test{method: http.MethodPost, target: "/", gameUUID: gameUUID, expectedCode: http.StatusNoContent}
		},
		"another_person_gets_not_found": func(t *testing.T, deps dependencies) test {
			deps.owner.EXPECT().RevokeAILink(gomock.Any(), callerUUID, gameUUID).Return(game.EditableVersionNotFound, nil)
			return test{method: http.MethodPost, target: "/", gameUUID: gameUUID, expectedCode: http.StatusNotFound}
		},
	}, func(h *Handler) http.HandlerFunc { return h.handleRevokeAILink })
}

// aiMux serves the handler's AI link routes as the server does, without its
// rate limits.
func aiMux(h *Handler) *http.ServeMux {
	mux := http.NewServeMux()
	for _, route := range h.RESTRoutes() {
		if strings.Contains(route.Pattern, aiPathPrefix) {
			mux.HandleFunc(route.Pattern, route.Authorization(route.Handler))
		}
	}
	return mux
}

func validLink() identity.ResolvedResourceLink {
	return identity.ResolvedResourceLink{Outcome: identity.ResourceLinkValid, UserUUID: callerUUID, Kind: game.ResourceKind, ResourceID: gameUUID, ExpiresAt: time.Now().Add(time.Hour)}
}

func aiSnapshot(content string) sessionpkg.DraftSnapshot {
	project := `{"format":"playhoot.project.v1","name":"Tap Race"}`
	notes := "# Notes\nFirst to 20 wins."
	return sessionpkg.DraftSnapshot{
		GameUUID: gameUUID, VersionUUID: versionUUID, Status: "DRAFT", CurrentRevision: 3, Revision: 3,
		Files: []sessionpkg.DraftFile{
			{Path: "game.project.json", SHA256: "a", Size: int64(len(project)), Content: &project},
			{Path: "notes.md", SHA256: "b", Size: int64(len(notes)), Content: &notes},
			{Path: "server/game.ts", SHA256: "c", Size: int64(len(content)), Content: &content},
		},
		Assets: []sessionpkg.DraftAsset{{Key: "cardBack", Type: "image", Description: "Card back, 512x768", Status: "missing"}},
		Build: &sessionpkg.DraftBuild{Revision: 3, Status: "FAILED", Diagnostics: []sessionpkg.DraftBuildDiagnostic{
			{Stage: "types", File: "server/game.ts", Line: 20, Column: 13, Message: "Property 'tapz' does not exist"},
		}},
		LastSuccessfulBuild: &sessionpkg.DraftBuild{Revision: 1, Status: "SUCCEEDED"},
	}
}

func serve(h *Handler, method string, target string, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	aiMux(h).ServeHTTP(recorder, httptest.NewRequest(method, target, strings.NewReader(body)))
	return recorder
}

func requireAIHeaders(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	require.Equal(t, "private, no-store", recorder.Header().Get("Cache-Control"))
	require.Equal(t, "no-referrer", recorder.Header().Get("Referrer-Policy"))
	require.Equal(t, "noindex", recorder.Header().Get("X-Robots-Tag"))
}

func TestAILinkRoutes(t *testing.T) {
	page := aiPathPrefix + testLinkToken

	t.Run("a link that does not work is refused without saying why", func(t *testing.T) {
		for _, resolved := range []identity.ResolvedResourceLink{
			{Outcome: identity.ResourceLinkUnknown},
			{Outcome: identity.ResourceLinkExpired},
			{Outcome: identity.ResourceLinkRevoked},
			{Outcome: identity.ResourceLinkValid, UserUUID: callerUUID, Kind: "organization", ResourceID: gameUUID},
		} {
			h, deps := newTestHandler(gomock.NewController(t))
			deps.resolver.EXPECT().ResolveResourceLink(gomock.Any(), testLinkToken).Return(resolved, nil)

			recorder := serve(h, http.MethodGet, page, "")
			require.Equal(t, http.StatusUnauthorized, recorder.Code)
			require.JSONEq(t, `{"code":"ai_link_invalid","message":"this link no longer works: ask the game's owner for a new one in Playhoot"}`, recorder.Body.String())
			requireAIHeaders(t, recorder)
		}
	})

	t.Run("the context page has what an AI needs, sources inline", func(t *testing.T) {
		h, deps := newTestHandler(gomock.NewController(t))
		deps.resolver.EXPECT().ResolveResourceLink(gomock.Any(), testLinkToken).Return(validLink(), nil)
		deps.reader.EXPECT().ReadDraft(gomock.Any(), callerUUID, gameUUID, int64(0), true).Return(composergameauthoring.DraftRead{Access: game.EditableVersionFound, Snapshot: aiSnapshot("export const x = 1; // ```"), GameName: "Tap Race"}, nil)

		recorder := serve(h, http.MethodGet, page, "")
		require.Equal(t, http.StatusOK, recorder.Code)
		require.Equal(t, "text/markdown; charset=utf-8", recorder.Header().Get("Content-Type"))
		requireAIHeaders(t, recorder)
		body := recorder.Body.String()
		for _, expected := range []string{
			"# Playhoot game: Tap Race",
			"Observed revision **3** at 2026-11-02T12:00:00Z UTC",
			"## How to change this game",
			`"base_revision": 3`,
			"**Cambios de mi IA**",
			testPublicBaseURL + page + "/changes",
			"## Build of revision 3",
			"Status: **FAILED**",
			"- `server/game.ts:20:13` [types] Property 'tapz' does not exist",
			"Last successful build: revision 1.",
			"- `cardBack` (image, required): **missing** — Card back, 512x768",
			"## Notes (notes.md)\n\n# Notes\nFirst to 20 wins.",
			"### `server/game.ts`\n\n````ts\nexport const x = 1; // ```\n````",
		} {
			require.Contains(t, body, expected)
		}
		require.NotContains(t, body, "### `notes.md`", "the notes are shown once")
	})

	t.Run("large sources are linked, not inlined", func(t *testing.T) {
		h, deps := newTestHandler(gomock.NewController(t))
		deps.resolver.EXPECT().ResolveResourceLink(gomock.Any(), testLinkToken).Return(validLink(), nil)
		deps.reader.EXPECT().ReadDraft(gomock.Any(), callerUUID, gameUUID, int64(0), true).Return(composergameauthoring.DraftRead{Access: game.EditableVersionFound, Snapshot: aiSnapshot(strings.Repeat("x", maxInlineSourceBytes))}, nil)

		body := serve(h, http.MethodGet, page, "").Body.String()
		require.Contains(t, body, "The sources are too large for this page")
		require.Contains(t, body, testPublicBaseURL+page+"/files/server/game.ts?revision=3")
		require.NotContains(t, body, "xxxxxxxxxx")
	})

	t.Run("the context as JSON carries no source contents", func(t *testing.T) {
		h, deps := newTestHandler(gomock.NewController(t))
		deps.resolver.EXPECT().ResolveResourceLink(gomock.Any(), testLinkToken).Return(validLink(), nil)
		deps.reader.EXPECT().ReadDraft(gomock.Any(), callerUUID, gameUUID, int64(0), true).Return(composergameauthoring.DraftRead{Access: game.EditableVersionFound, Snapshot: aiSnapshot("x"), GameName: "Tap Race"}, nil)

		recorder := serve(h, http.MethodGet, page+"?format=json", "")
		require.Equal(t, http.StatusOK, recorder.Code)
		requireAIHeaders(t, recorder)
		body := recorder.Body.String()
		require.Contains(t, body, `"format":"playhoot.ai-context.v1"`)
		require.Contains(t, body, `"game_name":"Tap Race"`)
		require.Contains(t, body, `"sources_inline":false`)
		require.NotContains(t, body, `"content"`, "the JSON context lists files, it never carries them")
		require.Contains(t, body, `"notes":"`, "notes.md comes with the context")
		require.Contains(t, body, `{"path":"server/game.ts","sha256":"c","size":1}`)
		require.Contains(t, body, `"changes":"`+testPublicBaseURL+page+`/changes"`)
		require.Contains(t, body, `"changed":"`+testPublicBaseURL+page+`/changed"`)
		require.Contains(t, body, `"diff":"`+testPublicBaseURL+page+`/diff"`)
		require.Contains(t, body, `"sdk_docs_url":"`+testPublicBaseURL+page+`/sdk/index.md"`, "through a link, the guide of the toolchain")
	})

	t.Run("the context page links the SDK guide on the web", func(t *testing.T) {
		h, deps := newTestHandler(gomock.NewController(t))
		deps.resolver.EXPECT().ResolveResourceLink(gomock.Any(), testLinkToken).Return(validLink(), nil)
		deps.reader.EXPECT().ReadDraft(gomock.Any(), callerUUID, gameUUID, int64(0), true).Return(composergameauthoring.DraftRead{Access: game.EditableVersionFound, Snapshot: aiSnapshot("x")}, nil)

		body := serve(h, http.MethodGet, page, "").Body.String()
		require.Contains(t, body, testPublicBaseURL+page+"/sdk/index.md")
		require.Contains(t, body, testPublicBaseURL+page+"/sdk/capabilities.md")
	})

	t.Run("a file of a pinned revision", func(t *testing.T) {
		h, deps := newTestHandler(gomock.NewController(t))
		deps.resolver.EXPECT().ResolveResourceLink(gomock.Any(), testLinkToken).Return(validLink(), nil).Times(2)
		deps.reader.EXPECT().ReadDraft(gomock.Any(), callerUUID, gameUUID, int64(2), true).Return(composergameauthoring.DraftRead{Access: game.EditableVersionFound, Snapshot: aiSnapshot("export {}")}, nil).Times(2)

		recorder := serve(h, http.MethodGet, page+"/files/server/game.ts?revision=2", "")
		require.Equal(t, http.StatusOK, recorder.Code)
		require.Equal(t, "export {}", recorder.Body.String())
		require.Equal(t, "text/plain; charset=utf-8", recorder.Header().Get("Content-Type"))

		missing := serve(h, http.MethodGet, page+"/files/client/Nope.tsx?revision=2", "")
		require.Equal(t, http.StatusNotFound, missing.Code)
		require.Contains(t, missing.Body.String(), codeFileNotFound)
	})

	t.Run("several files, what changed and the revisions through the link", func(t *testing.T) {
		h, deps := newTestHandler(gomock.NewController(t))
		deps.resolver.EXPECT().ResolveResourceLink(gomock.Any(), testLinkToken).Return(validLink(), nil).Times(3)
		deps.reader.EXPECT().ReadDraft(gomock.Any(), callerUUID, gameUUID, int64(0), true).Return(composergameauthoring.DraftRead{Access: game.EditableVersionFound, Snapshot: aiSnapshot("export {}")}, nil)
		deps.reader.EXPECT().ReadDraftChanges(gomock.Any(), callerUUID, gameUUID, int64(2), int64(0)).Return(composergameauthoring.DraftChanges{Access: game.EditableVersionFound, Changes: sessionpkg.DraftRevisionChanges{Since: 2, Revision: 3}}, nil)
		deps.reader.EXPECT().ListDraftRevisions(gomock.Any(), callerUUID, gameUUID, int64(0), 0).Return(composergameauthoring.DraftRevisions{Access: game.EditableVersionFound}, nil)

		files := serve(h, http.MethodGet, page+"/files?path=server/game.ts", "")
		require.Equal(t, http.StatusOK, files.Code)
		require.Contains(t, files.Body.String(), `"content":"export {}"`)
		requireAIHeaders(t, files)
		changed := serve(h, http.MethodGet, page+"/changed?since=2", "")
		require.Equal(t, http.StatusOK, changed.Code)
		require.Contains(t, changed.Body.String(), `"since":2`)
		require.Equal(t, http.StatusOK, serve(h, http.MethodGet, page+"/revisions", "").Code)
	})

	t.Run("a change through the link comes from the API, as the owner", func(t *testing.T) {
		h, deps := newTestHandler(gomock.NewController(t))
		deps.resolver.EXPECT().ResolveResourceLink(gomock.Any(), testLinkToken).Return(validLink(), nil)
		deps.editor.EXPECT().ApplyDraftChanges(gomock.Any(), callerUUID, gameUUID, gomock.Cond(func(change sessionpkg.DraftChange) bool {
			return change.Origin == sessionpkg.DraftOriginAPI && change.BaseRevision == 3
		})).Return(gameauthoring.DraftEdit{Access: game.EditableVersionFound, Result: sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeApplied, Revision: 4}}, nil)

		recorder := serve(h, http.MethodPost, page+"/changes", `{"format":"playhoot.project-change.v1","base_revision":3,"files":[{"path":"notes.md","content":"x"}],"delete":[],"idempotency_key":"k1"}`)
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		requireAIHeaders(t, recorder)
	})

	t.Run("the token is never logged", func(t *testing.T) {
		h, deps := newTestHandler(gomock.NewController(t))
		deps.resolver.EXPECT().ResolveResourceLink(gomock.Any(), testLinkToken).Return(identity.ResolvedResourceLink{Outcome: identity.ResourceLinkRevoked}, nil)

		var output bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&output, nil))
		ctx := logging.Start(context.Background())
		recorder := httptest.NewRecorder()
		aiMux(h).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, page, nil).WithContext(ctx))
		logging.FinishRequestLog(ctx, logger, "GET /ai/v1/{token}")

		require.Equal(t, http.StatusUnauthorized, recorder.Code)
		require.NotEmpty(t, output.String())
		require.NotContains(t, output.String(), testLinkToken)
	})
}
