package games

import (
	"context"
	"encoding/json"
	"errors"
	"io"
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

// AssetEditResponse is an asset upload or removal that was decided. Changes
// are the revision's source files with their hashes.
type AssetEditResponse struct {
	Outcome  string                   `json:"outcome"`
	Revision int64                    `json:"revision"`
	Asset    *assetResponse           `json:"asset,omitempty"`
	Build    *buildStatusResponse     `json:"build,omitempty"`
	Changes  *RevisionChangesResponse `json:"changes,omitempty"`
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
	if err := http.NewResponseController(w).SetReadDeadline(time.Now().Add(assetUploadReadTime)); err != nil {
		logging.LogError(ctx, err)
	}
	// One byte past the largest asset lets Play tell a file that is too
	// large from one exactly at the limit.
	body := http.MaxBytesReader(w, r.Body, sessionpkg.MaxDraftAssetBytes+1)
	response, refusal := h.UploadAsset(ctx, Caller{UserUUID: userUUID, Origin: originOf(ctx)}, gameUUID, r.PathValue("key"), r.Header.Get("Idempotency-Key"), body)
	writeAssetEdit(w, response, refusal)
}

// UploadAsset binds content, the bytes of a declared asset key of the
// game's version being built, to it in a new revision. Play cuts content at
// the declared type's limit.
func (h *Handler) UploadAsset(ctx context.Context, by Caller, rawGameUUID string, key string, idempotencyKey string, content io.Reader) (AssetEditResponse, *Refusal) {
	gameUUID, refusal := gameOf(ctx, rawGameUUID)
	if refusal != nil {
		return AssetEditResponse{}, refusal
	}
	logging.LogFields(ctx, logging.Field("asset_key", key), logging.Field("idempotency_key", idempotencyKey))
	if refusal := idempotencyKeyRefusal(idempotencyKey, "the Idempotency-Key header"); refusal != nil {
		return AssetEditResponse{}, refusal
	}
	edit, err := h.editor.UploadDraftAsset(ctx, by.UserUUID, gameUUID, key, content, idempotencyKey, by.Origin)
	if errors.Is(err, sessionpkg.ErrUploadsBusy) {
		logging.LogError(ctx, err)
		refusal := refuseCoded(http.StatusServiceUnavailable, codeUploadsBusy, "too many uploads in progress; try again in a few seconds")
		refusal.RetryAfter = "5"
		return AssetEditResponse{}, refusal
	}
	return assetEditAnswer(ctx, edit, err, "uploading asset")
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
	var req assetEditRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAssetRequestBodyBytes)).Decode(&req); err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	response, refusal := h.RemoveAsset(ctx, Caller{UserUUID: userUUID, Origin: originOf(ctx)}, gameUUID, r.PathValue("key"), req.IdempotencyKey)
	writeAssetEdit(w, response, refusal)
}

// RemoveAsset unbinds an asset's content from the game's version being built
// in a new revision.
func (h *Handler) RemoveAsset(ctx context.Context, by Caller, rawGameUUID string, key string, idempotencyKey string) (AssetEditResponse, *Refusal) {
	gameUUID, refusal := gameOf(ctx, rawGameUUID)
	if refusal != nil {
		return AssetEditResponse{}, refusal
	}
	logging.LogFields(ctx, logging.Field("idempotency_key", idempotencyKey))
	if refusal := idempotencyKeyRefusal(idempotencyKey, "idempotency_key"); refusal != nil {
		return AssetEditResponse{}, refusal
	}
	logging.LogFields(ctx, logging.Field("asset_key", key))
	edit, err := h.editor.RemoveDraftAsset(ctx, by.UserUUID, gameUUID, key, idempotencyKey, by.Origin)
	return assetEditAnswer(ctx, edit, err, "removing asset")
}

// writeAssetEdit answers a decided asset edit over HTTP.
func writeAssetEdit(w http.ResponseWriter, response AssetEditResponse, refusal *Refusal) {
	if refusal != nil {
		writeRefusal(w, refusal)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, response)
}

// assetEditAnswer is attaching or removing an asset as answered. A replay
// answers what the first request did.
func assetEditAnswer(ctx context.Context, edit gameauthoring.DraftAssetEdit, err error, what string) (AssetEditResponse, *Refusal) {
	if err != nil {
		logging.LogError(ctx, err)
		if errors.Is(err, sessionpkg.ErrIdempotencyConflict) {
			return AssetEditResponse{}, refuseCoded(http.StatusConflict, codeIdempotencyConflict, "idempotency key reused with a conflicting request")
		}
		return AssetEditResponse{}, refuse(http.StatusInternalServerError, what+" failed")
	}
	if refusal := accessRefusal(edit.Access); refusal != nil {
		return AssetEditResponse{}, refusal
	}
	result := edit.Result
	logging.LogFields(ctx, logging.Field("asset_outcome", string(result.Outcome)), logging.Field("replayed", result.Replayed))
	switch result.Outcome {
	case sessionpkg.DraftAssetAttached, sessionpkg.DraftAssetRemoved, sessionpkg.DraftAssetUnchanged:
		response := AssetEditResponse{Outcome: string(result.Outcome), Revision: result.Revision, Build: buildStatus(result.Build), Changes: changesBody(result.Changes)}
		if result.Asset != nil {
			asset := assetBody(*result.Asset)
			response.Asset = &asset
		}
		return response, nil
	case sessionpkg.DraftAssetInvalid:
		return AssetEditResponse{}, &Refusal{Status: http.StatusUnprocessableEntity, Body: invalidChangeResponse{
			ErrorResponse: httpx.ErrorResponse{Code: codeInvalidAsset, Message: "the asset was not attached; see diagnostics"},
			Diagnostics:   diagnostics(result.Diagnostics),
		}}
	case sessionpkg.DraftAssetTooLarge:
		return AssetEditResponse{}, &Refusal{Status: http.StatusRequestEntityTooLarge, Body: invalidChangeResponse{
			ErrorResponse: httpx.ErrorResponse{Code: codeAssetTooLarge, Message: "the file is larger than this asset allows; nothing was stored"},
			Diagnostics:   diagnostics(result.Diagnostics),
		}}
	case sessionpkg.DraftAssetNotDeclared:
		return AssetEditResponse{}, refuseCoded(http.StatusNotFound, codeAssetNotDeclared, "game.project.json declares no such asset of this type; declare it first")
	case sessionpkg.DraftAssetNotEditable:
		return AssetEditResponse{}, refuseCoded(http.StatusConflict, codeNotEditable, "the version can no longer be changed")
	default:
		logging.LogFields(ctx, logging.Field("unknown_asset_outcome", string(result.Outcome)))
		return AssetEditResponse{}, refuse(http.StatusInternalServerError, what+" failed")
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
