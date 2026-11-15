package games

import (
	"context"
	"errors"
	"io"
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
	"github.com/diegobermudez03/playhoot/orchestrator/gameauthoring"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

func TestHandleRequestAssetUpload(t *testing.T) {
	expires := time.Date(2026, 11, 7, 10, 15, 0, 0, time.UTC)
	draft := composergameauthoring.DraftRead{Access: game.EditableVersionFound, Snapshot: sessionpkg.DraftSnapshot{Assets: []sessionpkg.DraftAsset{
		{Key: "music", Type: "audio", Status: sessionpkg.DraftAssetStatus("missing")},
		{Key: "orphan", Status: sessionpkg.DraftAssetStatus("undeclared")},
	}}}
	request := func(key string) test {
		return test{method: http.MethodPost, target: "/", gameUUID: gameUUID, pathValues: map[string]string{"key": key}, noStore: true}
	}
	run(t, map[string]func(t *testing.T, deps dependencies) test{
		"a_url_for_a_declared_asset": func(t *testing.T, deps dependencies) test {
			deps.reader.EXPECT().ReadDraft(gomock.Any(), callerUUID, gameUUID, int64(0), false).Return(draft, nil)
			deps.grants.EXPECT().CreateResourceGrant(gomock.Any(), callerUUID, game.ResourceKind, gameUUID, "asset_upload web music").
				Return(identity.CreatedResourceGrant{GrantUUID: "g1", Token: "phg_t", ExpiresAt: expires}, nil)
			tc := request("music")
			tc.expectedCode = http.StatusOK
			tc.check = func(t *testing.T, body string) {
				require.Contains(t, body, `"upload_url":"`+testPublicBaseURL+`/v1/asset-uploads/phg_t"`)
				require.Contains(t, body, `"method":"PUT","key":"music","type":"audio","max_bytes":10485760,"expires_at":"2026-11-07T10:15:00Z"`)
			}
			return tc
		},
		"an_undeclared_key": func(t *testing.T, deps dependencies) test {
			deps.reader.EXPECT().ReadDraft(gomock.Any(), callerUUID, gameUUID, int64(0), false).Return(draft, nil)
			tc := request("orphan")
			tc.expectedCode = http.StatusNotFound
			tc.expectedBody = `{"code":"asset_not_declared","message":"game.project.json declares no such asset; declare it first"}`
			return tc
		},
		"too_many_live_urls": func(t *testing.T, deps dependencies) test {
			deps.reader.EXPECT().ReadDraft(gomock.Any(), callerUUID, gameUUID, int64(0), false).Return(draft, nil)
			deps.grants.EXPECT().CreateResourceGrant(gomock.Any(), callerUUID, game.ResourceKind, gameUUID, gomock.Any()).Return(identity.CreatedResourceGrant{}, identity.ErrTooManyResourceGrants)
			tc := request("music")
			tc.expectedCode = http.StatusTooManyRequests
			return tc
		},
		"not_the_callers_game": func(t *testing.T, deps dependencies) test {
			deps.reader.EXPECT().ReadDraft(gomock.Any(), callerUUID, gameUUID, int64(0), false).Return(composergameauthoring.DraftRead{Access: game.EditableVersionNotFound}, nil)
			tc := request("music")
			tc.expectedCode = http.StatusNotFound
			return tc
		},
	}, func(h *Handler) http.HandlerFunc { return h.handleRequestAssetUpload })
}

func TestGrantUpload(t *testing.T) {
	claimed := identity.ClaimedResourceGrant{Outcome: identity.ResourceGrantClaimed, GrantUUID: "g1", UserUUID: callerUUID, Kind: game.ResourceKind, ResourceID: gameUUID, Scope: "asset_upload mcp music"}
	put := func(h *Handler) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPut, "/v1/asset-uploads/phg_t", strings.NewReader("OggS bytes")).WithContext(context.Background())
		request.SetPathValue("token", "phg_t")
		recorder := httptest.NewRecorder()
		h.grantUploadAccess(h.handleGrantUpload)(recorder, request)
		return recorder
	}

	t.Run("an_attached_upload_uses_the_url_up_as_the_grants_user_and_origin", func(t *testing.T) {
		h, deps := newTestHandler(gomock.NewController(t))
		deps.grants.EXPECT().ClaimResourceGrant(gomock.Any(), "phg_t").Return(claimed, nil)
		deps.editor.EXPECT().UploadDraftAsset(gomock.Any(), callerUUID, gameUUID, "music", gomock.Any(), "upload-g1", sessionpkg.DraftOriginMCP).
			DoAndReturn(func(_ context.Context, _, _, _ string, content io.Reader, _, _ string) (gameauthoring.DraftAssetEdit, error) {
				read, err := io.ReadAll(content)
				require.NoError(t, err)
				require.Equal(t, "OggS bytes", string(read))
				return gameauthoring.DraftAssetEdit{Access: game.EditableVersionFound, Result: sessionpkg.DraftAssetResult{Outcome: sessionpkg.DraftAssetAttached, Revision: 4}}, nil
			})

		recorder := put(h)

		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		require.Equal(t, "private, no-store", recorder.Header().Get("Cache-Control"))
	})

	t.Run("a_failed_upload_gives_the_url_back", func(t *testing.T) {
		h, deps := newTestHandler(gomock.NewController(t))
		deps.grants.EXPECT().ClaimResourceGrant(gomock.Any(), "phg_t").Return(claimed, nil)
		deps.editor.EXPECT().UploadDraftAsset(gomock.Any(), callerUUID, gameUUID, "music", gomock.Any(), "upload-g1", sessionpkg.DraftOriginMCP).
			Return(gameauthoring.DraftAssetEdit{}, errors.New("connection reset"))
		deps.grants.EXPECT().ReleaseResourceGrant(gomock.Any(), "g1").Return(nil)

		require.Equal(t, http.StatusInternalServerError, put(h).Code)
	})

	t.Run("a_used_url_is_gone", func(t *testing.T) {
		h, deps := newTestHandler(gomock.NewController(t))
		deps.grants.EXPECT().ClaimResourceGrant(gomock.Any(), "phg_t").Return(identity.ClaimedResourceGrant{Outcome: identity.ResourceGrantUsed}, nil)

		recorder := put(h)

		require.Equal(t, http.StatusGone, recorder.Code)
		require.JSONEq(t, `{"code":"upload_url_used","message":"this upload URL was already used: ask for a new one"}`, recorder.Body.String())
	})

	t.Run("an_expired_url_or_another_grant_is_refused", func(t *testing.T) {
		h, deps := newTestHandler(gomock.NewController(t))
		deps.grants.EXPECT().ClaimResourceGrant(gomock.Any(), "phg_t").Return(identity.ClaimedResourceGrant{Outcome: identity.ResourceGrantExpired}, nil)
		require.Equal(t, http.StatusUnauthorized, put(h).Code)

		other := claimed
		other.Scope = "something_else"
		deps.grants.EXPECT().ClaimResourceGrant(gomock.Any(), "phg_t").Return(other, nil)
		deps.grants.EXPECT().ReleaseResourceGrant(gomock.Any(), "g1").Return(nil)
		require.Equal(t, http.StatusUnauthorized, put(h).Code, "a grant for something else is left as it was")
	})
}

func TestMeasures(t *testing.T) {
	require.Equal(t, "", measures(sessionpkg.DraftAsset{Key: "a"}), "nothing uploaded, nothing measured")
	require.Equal(t, " (2048 bytes, 640x480)", measures(sessionpkg.DraftAsset{Size: 2048, Width: 640, Height: 480}))
	require.Equal(t, " (9000 bytes, 2.5 s)", measures(sessionpkg.DraftAsset{Size: 9000, DurationMs: 2500}))
	require.Equal(t, " (10 bytes)", measures(sessionpkg.DraftAsset{Size: 10}))
}
