package games

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/orchestrator/gameauthoring"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

// changeFormat names the change envelope, the same text an AI produces for a
// person to paste.
const changeFormat = "playhoot.project-change.v1"

// maxChangeBodyBytes bounds a change body: the whole project (2 MiB, Play's
// limit) plus its JSON escaping and envelope.
const maxChangeBodyBytes = 4 << 20

type changeFileRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// changeRequest is the change envelope.
type changeRequest struct {
	Format         string              `json:"format"`
	BaseRevision   *int64              `json:"base_revision"`
	Files          []changeFileRequest `json:"files"`
	Delete         []string            `json:"delete"`
	IdempotencyKey string              `json:"idempotency_key"`
}

type restoreRequest struct {
	Revision       int64  `json:"revision"`
	BaseRevision   *int64 `json:"base_revision"`
	IdempotencyKey string `json:"idempotency_key"`
}

type changeResponse struct {
	Outcome  string               `json:"outcome"`
	Revision int64                `json:"revision"`
	Build    *buildStatusResponse `json:"build,omitempty"`
}

type conflictResponse struct {
	httpx.ErrorResponse
	CurrentRevision int64 `json:"current_revision"`
}

type invalidChangeResponse struct {
	httpx.ErrorResponse
	Diagnostics []diagnosticResponse `json:"diagnostics"`
}

// handleApplyDraftChanges applies a change envelope to the game's version
// being built, all of it or nothing.
func (h *Handler) handleApplyDraftChanges(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userUUID, gameUUID, ok := caller(w, r, "applying draft changes")
	if !ok {
		return
	}
	noStore(w)

	var req changeRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxChangeBodyBytes)).Decode(&req); err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	logging.LogFields(ctx, logging.Field("idempotency_key", req.IdempotencyKey), logging.Field("files", len(req.Files)), logging.Field("deletes", len(req.Delete)))
	switch {
	case req.Format != changeFormat:
		httpx.WriteError(w, http.StatusBadRequest, "format must be "+changeFormat)
		return
	case req.BaseRevision == nil:
		httpx.WriteError(w, http.StatusBadRequest, "base_revision is required")
		return
	case req.IdempotencyKey == "" || len(req.IdempotencyKey) > maxIdempotencyKeyLength:
		httpx.WriteError(w, http.StatusBadRequest, "idempotency_key is required and must be at most 128 characters")
		return
	}

	change := sessionpkg.DraftChange{
		BaseRevision:   *req.BaseRevision,
		Delete:         req.Delete,
		IdempotencyKey: req.IdempotencyKey,
		Origin:         originOf(ctx),
	}
	for _, file := range req.Files {
		change.Files = append(change.Files, sessionpkg.DraftFileContent{Path: file.Path, Content: file.Content})
	}
	edit, err := h.editor.ApplyDraftChanges(ctx, userUUID, gameUUID, change)
	writeEdit(ctx, w, edit, err, "applying draft changes")
}

// handleRestoreDraftRevision makes an earlier revision of the game's version
// being built its newest one.
func (h *Handler) handleRestoreDraftRevision(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userUUID, gameUUID, ok := caller(w, r, "restoring draft revision")
	if !ok {
		return
	}
	noStore(w)

	var req restoreRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCreateGameBodyBytes)).Decode(&req); err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	logging.LogFields(ctx, logging.Field("idempotency_key", req.IdempotencyKey), logging.Field("restored_revision", req.Revision))
	switch {
	case req.Revision < 1:
		httpx.WriteError(w, http.StatusBadRequest, "revision must be a positive integer")
		return
	case req.BaseRevision == nil:
		httpx.WriteError(w, http.StatusBadRequest, "base_revision is required")
		return
	case req.IdempotencyKey == "" || len(req.IdempotencyKey) > maxIdempotencyKeyLength:
		httpx.WriteError(w, http.StatusBadRequest, "idempotency_key is required and must be at most 128 characters")
		return
	}

	edit, err := h.editor.RestoreDraftRevision(ctx, userUUID, gameUUID, req.Revision, *req.BaseRevision, req.IdempotencyKey, originOf(ctx))
	writeEdit(ctx, w, edit, err, "restoring draft revision")
}

// writeEdit answers a change or a restore. A replay answers what the first
// request did.
func writeEdit(ctx context.Context, w http.ResponseWriter, edit gameauthoring.DraftEdit, err error, what string) {
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
	logging.LogFields(ctx, logging.Field("draft_outcome", string(result.Outcome)), logging.Field("replayed", result.Replayed))
	switch result.Outcome {
	case sessionpkg.DraftChangeApplied, sessionpkg.DraftChangeUnchanged:
		httpx.WriteJSON(w, http.StatusOK, changeResponse{Outcome: string(result.Outcome), Revision: result.Revision, Build: buildStatus(result.Build)})
	case sessionpkg.DraftChangeConflict:
		httpx.WriteJSON(w, http.StatusConflict, conflictResponse{
			ErrorResponse:   httpx.ErrorResponse{Code: codeRevisionConflict, Message: "the draft changed since base_revision; read it again and redo the change on the current revision"},
			CurrentRevision: result.Revision,
		})
	case sessionpkg.DraftChangeInvalid:
		httpx.WriteJSON(w, http.StatusUnprocessableEntity, invalidChangeResponse{
			ErrorResponse: httpx.ErrorResponse{Code: codeInvalidChange, Message: "the change was not applied; see diagnostics"},
			Diagnostics:   diagnostics(result.Diagnostics),
		})
	case sessionpkg.DraftChangeNotEditable:
		httpx.WriteCodedError(w, http.StatusConflict, codeNotEditable, "the version can no longer be changed")
	default:
		logging.LogFields(ctx, logging.Field("unknown_draft_outcome", string(result.Outcome)))
		httpx.WriteError(w, http.StatusInternalServerError, what+" failed")
	}
}
