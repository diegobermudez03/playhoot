package games

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/logging"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

type draftFileResponse struct {
	Path    string  `json:"path"`
	SHA256  string  `json:"sha256"`
	Size    int64   `json:"size"`
	Content *string `json:"content,omitempty"`
}

type diagnosticResponse struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type attemptResponse struct {
	Kind         string               `json:"kind"`
	Outcome      string               `json:"outcome"`
	BaseRevision int64                `json:"base_revision"`
	Revision     *int64               `json:"revision,omitempty"`
	Origin       string               `json:"origin"`
	Diagnostics  []diagnosticResponse `json:"diagnostics"`
	CreatedAt    time.Time            `json:"created_at"`
}

type draftResponse struct {
	GameUUID        string              `json:"game_uuid"`
	VersionUUID     string              `json:"version_uuid"`
	Status          string              `json:"status"`
	CurrentRevision int64               `json:"current_revision"`
	Revision        int64               `json:"revision"`
	Files           []draftFileResponse `json:"files"`
	Assets          []assetResponse     `json:"assets"`
	// AssetDiagnostics say why no asset is declared when game.project.json
	// cannot be read.
	AssetDiagnostics []diagnosticResponse `json:"asset_diagnostics"`
	Attempts         []attemptResponse    `json:"attempts"`
}

type revisionResponse struct {
	Number    int64     `json:"number"`
	Source    string    `json:"source"`
	Files     int       `json:"files"`
	CreatedAt time.Time `json:"created_at"`
}

// handleReadDraft answers a revision of the game's version being built
// (?revision=N, the current one by default), with file contents when
// ?content=true, and its latest change attempts.
func (h *Handler) handleReadDraft(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userUUID, gameUUID, ok := caller(w, r, "reading draft")
	if !ok {
		return
	}
	noStore(w)
	query := r.URL.Query()
	var revision int64
	if raw := query.Get("revision"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 1 {
			httpx.WriteError(w, http.StatusBadRequest, "revision must be a positive integer")
			return
		}
		revision = parsed
	}
	withContent := false
	switch query.Get("content") {
	case "", "false":
	case "true":
		withContent = true
	default:
		httpx.WriteError(w, http.StatusBadRequest, "content must be true or false")
		return
	}

	read, err := h.reader.ReadDraft(ctx, userUUID, gameUUID, revision, withContent)
	if err == nil && read.Missing {
		// The game's creation stopped before Play had its version: create it
		// and read once more.
		if !h.ensureDraft(w, r, userUUID, gameUUID) {
			return
		}
		read, err = h.reader.ReadDraft(ctx, userUUID, gameUUID, revision, withContent)
	}
	if err != nil {
		logging.LogError(ctx, err)
		if errors.Is(err, sessionpkg.ErrDraftRevisionNotFound) {
			httpx.WriteCodedError(w, http.StatusNotFound, codeRevisionNotFound, "the draft has no such revision")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "reading draft failed")
		return
	}
	if !writeAccessRefusal(w, read.Access) {
		return
	}
	if read.Missing {
		logging.LogFields(ctx, logging.Field("wiring_error", "draft still missing after it was ensured"))
		httpx.WriteError(w, http.StatusInternalServerError, "reading draft failed")
		return
	}

	snapshot := read.Snapshot
	files := make([]draftFileResponse, len(snapshot.Files))
	for i, file := range snapshot.Files {
		files[i] = draftFileResponse{Path: file.Path, SHA256: file.SHA256, Size: file.Size, Content: file.Content}
	}
	attempts := make([]attemptResponse, len(snapshot.Attempts))
	for i, attempt := range snapshot.Attempts {
		attempts[i] = attemptResponse{
			Kind:         string(attempt.Kind),
			Outcome:      attempt.Outcome,
			BaseRevision: attempt.BaseRevision,
			Revision:     attempt.Revision,
			Origin:       attempt.Origin,
			Diagnostics:  diagnostics(attempt.Diagnostics),
			CreatedAt:    attempt.CreatedAt,
		}
	}
	assets := make([]assetResponse, len(snapshot.Assets))
	for i, asset := range snapshot.Assets {
		assets[i] = assetBody(asset)
	}
	httpx.WriteJSON(w, http.StatusOK, draftResponse{
		GameUUID:         snapshot.GameUUID,
		VersionUUID:      snapshot.VersionUUID,
		Status:           snapshot.Status,
		CurrentRevision:  snapshot.CurrentRevision,
		Revision:         snapshot.Revision,
		Files:            files,
		Assets:           assets,
		AssetDiagnostics: diagnostics(snapshot.AssetDiagnostics),
		Attempts:         attempts,
	})
}

// handleListDraftRevisions answers every revision of the game's version
// being built, oldest first.
func (h *Handler) handleListDraftRevisions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userUUID, gameUUID, ok := caller(w, r, "listing draft revisions")
	if !ok {
		return
	}
	noStore(w)

	listed, err := h.reader.ListDraftRevisions(ctx, userUUID, gameUUID)
	if err == nil && listed.Missing {
		if !h.ensureDraft(w, r, userUUID, gameUUID) {
			return
		}
		listed, err = h.reader.ListDraftRevisions(ctx, userUUID, gameUUID)
	}
	if err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusInternalServerError, "listing draft revisions failed")
		return
	}
	if !writeAccessRefusal(w, listed.Access) {
		return
	}
	if listed.Missing {
		logging.LogFields(ctx, logging.Field("wiring_error", "draft still missing after it was ensured"))
		httpx.WriteError(w, http.StatusInternalServerError, "listing draft revisions failed")
		return
	}
	revisions := make([]revisionResponse, len(listed.Revisions))
	for i, revision := range listed.Revisions {
		revisions[i] = revisionResponse{Number: revision.Number, Source: revision.Source, Files: revision.Files, CreatedAt: revision.CreatedAt}
	}
	httpx.WriteJSON(w, http.StatusOK, revisions)
}

// ensureDraft asks the orchestrator to create the game's version being built
// that a read found missing in Play, returning false after answering when it
// could not or the caller may no longer edit the game.
func (h *Handler) ensureDraft(w http.ResponseWriter, r *http.Request, userUUID string, gameUUID string) bool {
	ctx := r.Context()
	access, err := h.editor.EnsureDraft(ctx, userUUID, gameUUID)
	if err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusInternalServerError, "preparing draft failed")
		return false
	}
	return writeAccessRefusal(w, access)
}

// writeAccessRefusal answers a caller Game Management did not let edit the
// game, returning false; it returns true when the caller may go on.
func writeAccessRefusal(w http.ResponseWriter, access game.EditableVersionOutcome) bool {
	switch access {
	case game.EditableVersionFound:
		return true
	case game.EditableVersionNotEditable:
		httpx.WriteCodedError(w, http.StatusConflict, codeNotEditable, "the game has no version being built")
	default:
		// not_found: unknown, deleted, or another person's game, never told
		// apart.
		httpx.WriteError(w, http.StatusNotFound, "game not found")
	}
	return false
}

func diagnostics(list []sessionpkg.DraftDiagnostic) []diagnosticResponse {
	result := make([]diagnosticResponse, len(list))
	for i, diagnostic := range list {
		result[i] = diagnosticResponse{Path: diagnostic.Path, Code: diagnostic.Code, Message: diagnostic.Message}
	}
	return result
}
