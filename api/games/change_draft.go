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

// ChangeFile is one file of a change, with its whole new content.
type ChangeFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// ChangeRequest is the change envelope.
type ChangeRequest struct {
	Format         string       `json:"format"`
	BaseRevision   *int64       `json:"base_revision"`
	Files          []ChangeFile `json:"files"`
	Delete         []string     `json:"delete"`
	IdempotencyKey string       `json:"idempotency_key"`
}

// RestoreRequest makes an earlier revision the newest one.
type RestoreRequest struct {
	Revision       int64  `json:"revision"`
	BaseRevision   *int64 `json:"base_revision"`
	IdempotencyKey string `json:"idempotency_key"`
}

// ChangeResponse is a change or a restore that was decided.
type ChangeResponse struct {
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
	var req ChangeRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxChangeBodyBytes)).Decode(&req); err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	response, refusal := h.ApplyChanges(ctx, Caller{UserUUID: userUUID, Origin: originOf(ctx)}, gameUUID, req)
	writeChange(w, response, refusal)
}

// ApplyChanges applies a change envelope to the game's version being built,
// all of it or nothing.
func (h *Handler) ApplyChanges(ctx context.Context, by Caller, rawGameUUID string, req ChangeRequest) (ChangeResponse, *Refusal) {
	gameUUID, refusal := gameOf(ctx, rawGameUUID)
	if refusal != nil {
		return ChangeResponse{}, refusal
	}
	logging.LogFields(ctx, logging.Field("idempotency_key", req.IdempotencyKey), logging.Field("files", len(req.Files)), logging.Field("deletes", len(req.Delete)))
	switch {
	case req.Format != changeFormat:
		return ChangeResponse{}, refuse(http.StatusBadRequest, "format must be "+changeFormat)
	case req.BaseRevision == nil:
		return ChangeResponse{}, refuse(http.StatusBadRequest, "base_revision is required")
	}
	if refusal := idempotencyKeyRefusal(req.IdempotencyKey, "idempotency_key"); refusal != nil {
		return ChangeResponse{}, refusal
	}
	change := sessionpkg.DraftChange{
		BaseRevision:   *req.BaseRevision,
		Delete:         req.Delete,
		IdempotencyKey: req.IdempotencyKey,
		Origin:         by.Origin,
	}
	for _, file := range req.Files {
		change.Files = append(change.Files, sessionpkg.DraftFileContent{Path: file.Path, Content: file.Content})
	}
	edit, err := h.editor.ApplyDraftChanges(ctx, by.UserUUID, gameUUID, change)
	return editAnswer(ctx, edit, err, "applying draft changes")
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
	var req RestoreRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCreateGameBodyBytes)).Decode(&req); err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	response, refusal := h.RestoreRevision(ctx, Caller{UserUUID: userUUID, Origin: originOf(ctx)}, gameUUID, req)
	writeChange(w, response, refusal)
}

// RestoreRevision makes an earlier revision of the game's version being built
// its newest one.
func (h *Handler) RestoreRevision(ctx context.Context, by Caller, rawGameUUID string, req RestoreRequest) (ChangeResponse, *Refusal) {
	gameUUID, refusal := gameOf(ctx, rawGameUUID)
	if refusal != nil {
		return ChangeResponse{}, refusal
	}
	logging.LogFields(ctx, logging.Field("idempotency_key", req.IdempotencyKey), logging.Field("restored_revision", req.Revision))
	switch {
	case req.Revision < 1:
		return ChangeResponse{}, refuse(http.StatusBadRequest, "revision must be a positive integer")
	case req.BaseRevision == nil:
		return ChangeResponse{}, refuse(http.StatusBadRequest, "base_revision is required")
	}
	if refusal := idempotencyKeyRefusal(req.IdempotencyKey, "idempotency_key"); refusal != nil {
		return ChangeResponse{}, refusal
	}
	edit, err := h.editor.RestoreDraftRevision(ctx, by.UserUUID, gameUUID, req.Revision, *req.BaseRevision, req.IdempotencyKey, by.Origin)
	return editAnswer(ctx, edit, err, "restoring draft revision")
}

// writeChange answers a decided change or restore over HTTP.
func writeChange(w http.ResponseWriter, response ChangeResponse, refusal *Refusal) {
	if refusal != nil {
		writeRefusal(w, refusal)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, response)
}

// editAnswer is a change or a restore as answered. A replay answers what the
// first request did.
func editAnswer(ctx context.Context, edit gameauthoring.DraftEdit, err error, what string) (ChangeResponse, *Refusal) {
	if err != nil {
		logging.LogError(ctx, err)
		if errors.Is(err, sessionpkg.ErrIdempotencyConflict) {
			return ChangeResponse{}, refuseCoded(http.StatusConflict, codeIdempotencyConflict, "idempotency key reused with a conflicting request")
		}
		return ChangeResponse{}, refuse(http.StatusInternalServerError, what+" failed")
	}
	if refusal := accessRefusal(edit.Access); refusal != nil {
		return ChangeResponse{}, refusal
	}
	result := edit.Result
	logging.LogFields(ctx, logging.Field("draft_outcome", string(result.Outcome)), logging.Field("replayed", result.Replayed))
	switch result.Outcome {
	case sessionpkg.DraftChangeApplied, sessionpkg.DraftChangeUnchanged:
		return ChangeResponse{Outcome: string(result.Outcome), Revision: result.Revision, Build: buildStatus(result.Build)}, nil
	case sessionpkg.DraftChangeConflict:
		return ChangeResponse{}, &Refusal{Status: http.StatusConflict, Body: conflictResponse{
			ErrorResponse:   httpx.ErrorResponse{Code: codeRevisionConflict, Message: "the draft changed since base_revision; read it again and redo the change on the current revision"},
			CurrentRevision: result.Revision,
		}}
	case sessionpkg.DraftChangeInvalid:
		return ChangeResponse{}, &Refusal{Status: http.StatusUnprocessableEntity, Body: invalidChangeResponse{
			ErrorResponse: httpx.ErrorResponse{Code: codeInvalidChange, Message: "the change was not applied; see diagnostics"},
			Diagnostics:   diagnostics(result.Diagnostics),
		}}
	case sessionpkg.DraftChangeNotEditable:
		return ChangeResponse{}, refuseCoded(http.StatusConflict, codeNotEditable, "the version can no longer be changed")
	default:
		logging.LogFields(ctx, logging.Field("unknown_draft_outcome", string(result.Outcome)))
		return ChangeResponse{}, refuse(http.StatusInternalServerError, what+" failed")
	}
}
