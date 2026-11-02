package games

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/orchestrator/gameauthoring"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

func TestHandleUploadAsset(t *testing.T) {
	put := func(key string) test {
		tc := test{method: http.MethodPut, target: "/v1/games/" + gameUUID + "/draft/assets/cardBack", gameUUID: gameUUID, pathValues: map[string]string{"key": "cardBack"}, body: "png bytes", noStore: true}
		if key != "" {
			tc.headers = map[string]string{"Idempotency-Key": key}
		}
		return tc
	}
	answers := func(deps dependencies, result sessionpkg.DraftAssetResult, err error) {
		deps.editor.EXPECT().UploadDraftAsset(gomock.Any(), callerUUID, gameUUID, "cardBack", gomock.Any(), "key-3", sessionpkg.DraftOriginWeb).
			Return(gameauthoring.DraftAssetEdit{Access: game.EditableVersionFound, Result: result}, err)
	}
	tests := map[string]func(t *testing.T, deps dependencies) test{
		"attached": func(t *testing.T, deps dependencies) test {
			asset := sessionpkg.DraftAsset{Key: "cardBack", Type: "image", Description: "Card back", Status: sessionpkg.DraftAssetReady, SHA256: "p", Size: 3, MIMEType: "image/png"}
			answers(deps, sessionpkg.DraftAssetResult{Outcome: sessionpkg.DraftAssetAttached, Revision: 3, Asset: &asset}, nil)
			tc := put("key-3")
			tc.expectedCode = http.StatusOK
			tc.expectedBody = `{"outcome":"attached","revision":3,"asset":{"key":"cardBack","type":"image","description":"Card back","optional":false,"status":"ready","sha256":"p","size":3,"mime_type":"image/png"}}`
			return tc
		},
		"the_body_reaches_play_as_sent": func(t *testing.T, deps dependencies) test {
			deps.editor.EXPECT().UploadDraftAsset(gomock.Any(), callerUUID, gameUUID, "cardBack", gomock.Any(), "key-3", sessionpkg.DraftOriginWeb).
				DoAndReturn(func(_ context.Context, _ string, _ string, _ string, content io.Reader, _ string, _ string) (gameauthoring.DraftAssetEdit, error) {
					read, err := io.ReadAll(content)
					require.NoError(t, err)
					require.Equal(t, "png bytes", string(read))
					return gameauthoring.DraftAssetEdit{Access: game.EditableVersionFound, Result: sessionpkg.DraftAssetResult{Outcome: sessionpkg.DraftAssetUnchanged, Revision: 3}}, nil
				})
			tc := put("key-3")
			tc.expectedCode = http.StatusOK
			tc.expectedBody = `{"outcome":"unchanged","revision":3}`
			return tc
		},
		"invalid": func(t *testing.T, deps dependencies) test {
			answers(deps, sessionpkg.DraftAssetResult{Outcome: sessionpkg.DraftAssetInvalid, Revision: 2, Diagnostics: []sessionpkg.DraftDiagnostic{{Path: "assets/cardBack", Code: "wrong_content_type", Message: "the file is audio/wav"}}}, nil)
			tc := put("key-3")
			tc.expectedCode = http.StatusUnprocessableEntity
			tc.expectedBody = `{"code":"invalid_asset","message":"the asset was not attached; see diagnostics","diagnostics":[{"path":"assets/cardBack","code":"wrong_content_type","message":"the file is audio/wav"}]}`
			return tc
		},
		"too_large": func(t *testing.T, deps dependencies) test {
			answers(deps, sessionpkg.DraftAssetResult{Outcome: sessionpkg.DraftAssetTooLarge, Revision: 2, Diagnostics: []sessionpkg.DraftDiagnostic{{Path: "assets/cardBack", Code: "asset_too_large", Message: "too large"}}}, nil)
			tc := put("key-3")
			tc.expectedCode = http.StatusRequestEntityTooLarge
			tc.expectedBody = `{"code":"asset_too_large","message":"the file is larger than this asset allows; nothing was stored","diagnostics":[{"path":"assets/cardBack","code":"asset_too_large","message":"too large"}]}`
			return tc
		},
		"not_declared": func(t *testing.T, deps dependencies) test {
			answers(deps, sessionpkg.DraftAssetResult{Outcome: sessionpkg.DraftAssetNotDeclared, Revision: 2}, nil)
			tc := put("key-3")
			tc.expectedCode = http.StatusNotFound
			tc.expectedBody = `{"code":"asset_not_declared","message":"game.project.json declares no such asset of this type; declare it first"}`
			return tc
		},
		"not_editable": func(t *testing.T, deps dependencies) test {
			answers(deps, sessionpkg.DraftAssetResult{Outcome: sessionpkg.DraftAssetNotEditable, Revision: 2}, nil)
			tc := put("key-3")
			tc.expectedCode = http.StatusConflict
			return tc
		},
		"missing_idempotency_key": func(t *testing.T, deps dependencies) test {
			tc := put("")
			tc.expectedCode = http.StatusBadRequest
			return tc
		},
		"another_persons_game": func(t *testing.T, deps dependencies) test {
			deps.editor.EXPECT().UploadDraftAsset(gomock.Any(), callerUUID, gameUUID, "cardBack", gomock.Any(), "key-3", sessionpkg.DraftOriginWeb).
				Return(gameauthoring.DraftAssetEdit{Access: game.EditableVersionNotFound}, nil)
			tc := put("key-3")
			tc.expectedCode = http.StatusNotFound
			return tc
		},
		"uploads_busy": func(t *testing.T, deps dependencies) test {
			answers(deps, sessionpkg.DraftAssetResult{}, sessionpkg.ErrUploadsBusy)
			tc := put("key-3")
			tc.expectedCode = http.StatusServiceUnavailable
			tc.expectedBody = `{"code":"uploads_busy","message":"too many uploads in progress; try again in a few seconds"}`
			return tc
		},
		"reused_key": func(t *testing.T, deps dependencies) test {
			answers(deps, sessionpkg.DraftAssetResult{}, sessionpkg.ErrIdempotencyConflict)
			tc := put("key-3")
			tc.expectedCode = http.StatusConflict
			return tc
		},
		"failure": func(t *testing.T, deps dependencies) test {
			answers(deps, sessionpkg.DraftAssetResult{}, errors.New("down"))
			tc := put("key-3")
			tc.expectedCode = http.StatusInternalServerError
			return tc
		},
	}
	run(t, tests, func(h *Handler) http.HandlerFunc { return h.handleUploadAsset })
}

func TestHandleRemoveAsset(t *testing.T) {
	post := func(body string) test {
		return test{method: http.MethodPost, target: "/v1/games/" + gameUUID + "/draft/assets/cardBack/remove", gameUUID: gameUUID, pathValues: map[string]string{"key": "cardBack"}, body: body, noStore: true}
	}
	tests := map[string]func(t *testing.T, deps dependencies) test{
		"removed": func(t *testing.T, deps dependencies) test {
			deps.editor.EXPECT().RemoveDraftAsset(gomock.Any(), callerUUID, gameUUID, "cardBack", "key-4", sessionpkg.DraftOriginWeb).
				Return(gameauthoring.DraftAssetEdit{Access: game.EditableVersionFound, Result: sessionpkg.DraftAssetResult{Outcome: sessionpkg.DraftAssetRemoved, Revision: 4}}, nil)
			tc := post(`{"idempotency_key":"key-4"}`)
			tc.expectedCode = http.StatusOK
			tc.expectedBody = `{"outcome":"removed","revision":4}`
			return tc
		},
		"missing_key": func(t *testing.T, deps dependencies) test {
			tc := post(`{}`)
			tc.expectedCode = http.StatusBadRequest
			return tc
		},
	}
	run(t, tests, func(h *Handler) http.HandlerFunc { return h.handleRemoveAsset })
}
