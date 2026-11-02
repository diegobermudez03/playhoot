package games

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/orchestrator/gameauthoring"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

// maxAssetRequestBodyBytes bounds a removal's body: only its idempotency key.
const maxAssetRequestBodyBytes = 4 << 10

// assetUploadReadTime is how long receiving one upload's body may take,
// beyond the server's ordinary read timeout: up to tens of megabytes.
const assetUploadReadTime = 5 * time.Minute

const (
	codeAssetNotDeclared = "asset_not_declared"
	codeInvalidAsset     = "invalid_asset"
	codeAssetTooLarge    = "asset_too_large"
	codeUploadsBusy      = "uploads_busy"
)

type assetEditRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
}

type assetResponse struct {
	Key          string     `json:"key"`
	Type         string     `json:"type,omitempty"`
	Description  string     `json:"description,omitempty"`
	Optional     bool       `json:"optional"`
	Status       string     `json:"status"`
	SHA256       string     `json:"sha256,omitempty"`
	Size         int64      `json:"size,omitempty"`
	MIMEType     string     `json:"mime_type,omitempty"`
	URL          string     `json:"url,omitempty"`
	URLExpiresAt *time.Time `json:"url_expires_at,omitempty"`
}

type assetEditResponse struct {
	Outcome  string               `json:"outcome"`
	Revision int64                `json:"revision"`
	Asset    *assetResponse       `json:"asset,omitempty"`
	Build    *buildStatusResponse `json:"build,omitempty"`
}

// handleUploadAsset receives the bytes of a declared asset of the game's
// version being built and binds them to it in a new revision. The body is
// the file itself; Play cuts it at the declared type's limit.
func (h *Handler) handleUploadAsset(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userUUID, gameUUID, ok := caller(w, r, "uploading asset")
	if !ok {
		return
	}
	noStore(w)
	key := r.PathValue("key")
	idempotencyKey := r.Header.Get("Idempotency-Key")
	logging.LogFields(ctx, logging.Field("asset_key", key), logging.Field("idempotency_key", idempotencyKey))
	if idempotencyKey == "" || len(idempotencyKey) > maxIdempotencyKeyLength {
		httpx.WriteError(w, http.StatusBadRequest, "the Idempotency-Key header is required and must be at most 128 characters")
		return
	}
	if err := http.NewResponseController(w).SetReadDeadline(time.Now().Add(assetUploadReadTime)); err != nil {
		logging.LogError(ctx, err)
	}
	// One byte past the largest asset lets Play tell a file that is too
	// large from one exactly at the limit.
	body := http.MaxBytesReader(w, r.Body, sessionpkg.MaxDraftAssetBytes+1)

	edit, err := h.editor.UploadDraftAsset(ctx, userUUID, gameUUID, key, body, idempotencyKey, originOf(ctx))
	if errors.Is(err, sessionpkg.ErrUploadsBusy) {
		logging.LogError(ctx, err)
		w.Header().Set("Retry-After", "5")
		httpx.WriteCodedError(w, http.StatusServiceUnavailable, codeUploadsBusy, "too many uploads in progress; try again in a few seconds")
		return
	}
	writeAssetEdit(ctx, w, edit, err, "uploading asset")
}

// handleRemoveAsset unbinds an asset's content from the game's version being
// built in a new revision.
func (h *Handler) handleRemoveAsset(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userUUID, gameUUID, ok := caller(w, r, "removing asset")
	if !ok {
		return
	}
	noStore(w)
	key := r.PathValue("key")
	idempotencyKey, ok := assetEditKey(w, r)
	if !ok {
		return
	}
	logging.LogFields(ctx, logging.Field("asset_key", key))

	edit, err := h.editor.RemoveDraftAsset(ctx, userUUID, gameUUID, key, idempotencyKey, originOf(ctx))
	writeAssetEdit(ctx, w, edit, err, "removing asset")
}

// assetEditKey reads an asset edit's idempotency key, answering the request
// itself and returning false when it is missing or the body is invalid.
func assetEditKey(w http.ResponseWriter, r *http.Request) (string, bool) {
	ctx := r.Context()
	var req assetEditRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAssetRequestBodyBytes)).Decode(&req); err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return "", false
	}
	logging.LogFields(ctx, logging.Field("idempotency_key", req.IdempotencyKey))
	if req.IdempotencyKey == "" || len(req.IdempotencyKey) > maxIdempotencyKeyLength {
		httpx.WriteError(w, http.StatusBadRequest, "idempotency_key is required and must be at most 128 characters")
		return "", false
	}
	return req.IdempotencyKey, true
}

// writeAssetEdit answers attaching or removing an asset. A replay answers
// what the first request did.
func writeAssetEdit(ctx context.Context, w http.ResponseWriter, edit gameauthoring.DraftAssetEdit, err error, what string) {
	if err != nil {
		logging.LogError(ctx, err)
		if errors.Is(err, sessionpkg.ErrIdempotencyConflict) {
			httpx.WriteCodedError(w, http.StatusConflict, codeIdempotencyConflict, "idempotency key reused with a conflicting request")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, what+" failed")
		return
	}
	if !writeAccessRefusal(w, edit.Access) {
		return
	}
	result := edit.Result
	logging.LogFields(ctx, logging.Field("asset_outcome", string(result.Outcome)), logging.Field("replayed", result.Replayed))
	switch result.Outcome {
	case sessionpkg.DraftAssetAttached, sessionpkg.DraftAssetRemoved, sessionpkg.DraftAssetUnchanged:
		response := assetEditResponse{Outcome: string(result.Outcome), Revision: result.Revision, Build: buildStatus(result.Build)}
		if result.Asset != nil {
			asset := assetBody(*result.Asset)
			response.Asset = &asset
		}
		httpx.WriteJSON(w, http.StatusOK, response)
	case sessionpkg.DraftAssetInvalid:
		httpx.WriteJSON(w, http.StatusUnprocessableEntity, invalidChangeResponse{
			ErrorResponse: httpx.ErrorResponse{Code: codeInvalidAsset, Message: "the asset was not attached; see diagnostics"},
			Diagnostics:   diagnostics(result.Diagnostics),
		})
	case sessionpkg.DraftAssetTooLarge:
		httpx.WriteJSON(w, http.StatusRequestEntityTooLarge, invalidChangeResponse{
			ErrorResponse: httpx.ErrorResponse{Code: codeAssetTooLarge, Message: "the file is larger than this asset allows; nothing was stored"},
			Diagnostics:   diagnostics(result.Diagnostics),
		})
	case sessionpkg.DraftAssetNotDeclared:
		httpx.WriteCodedError(w, http.StatusNotFound, codeAssetNotDeclared, "game.project.json declares no such asset of this type; declare it first")
	case sessionpkg.DraftAssetNotEditable:
		httpx.WriteCodedError(w, http.StatusConflict, codeNotEditable, "the version can no longer be changed")
	default:
		logging.LogFields(ctx, logging.Field("unknown_asset_outcome", string(result.Outcome)))
		httpx.WriteError(w, http.StatusInternalServerError, what+" failed")
	}
}

// assetBody is one asset in its response form.
func assetBody(asset sessionpkg.DraftAsset) assetResponse {
	body := assetResponse{
		Key: asset.Key, Type: asset.Type, Description: asset.Description, Optional: asset.Optional,
		Status: string(asset.Status), SHA256: asset.SHA256, Size: asset.Size, MIMEType: asset.MIMEType, URL: asset.URL,
	}
	if !asset.URLExpiresAt.IsZero() {
		expiresAt := asset.URLExpiresAt
		body.URLExpiresAt = &expiresAt
	}
	return body
}
