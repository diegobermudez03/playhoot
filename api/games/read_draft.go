package games

import (
	"context"
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
	// Build is the build of Revision; LastSuccessfulBuild the version's
	// newest succeeded build, of Revision or an older one.
	Build               *BuildResponse `json:"build"`
	LastSuccessfulBuild *BuildResponse `json:"last_successful_build"`
}

// RevisionResponse is one revision of the version being built.
type RevisionResponse struct {
	Number    int64     `json:"number"`
	Source    string    `json:"source"`
	Files     int       `json:"files"`
	CreatedAt time.Time `json:"created_at"`
}

// handleReadDraft answers a revision of the game's version being built
// (?revision=N, the current one by default), with file contents when
// ?content=true, and its latest change attempts.
func (h *Handler) handleReadDraft(w http.ResponseWriter, r *http.Request) {
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

	snapshot, _, refusal := h.loadDraft(r.Context(), userUUID, gameUUID, revision, withContent)
	if refusal != nil {
		writeRefusal(w, refusal)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, draftBody(snapshot))
}

// loadDraft reads a revision of the game's version being built (0: the
// current one), creating the version first when its creation stopped before
// Play had it.
func (h *Handler) loadDraft(ctx context.Context, userUUID string, gameUUID string, revision int64, withContent bool) (sessionpkg.DraftSnapshot, string, *Refusal) {
	read, err := h.reader.ReadDraft(ctx, userUUID, gameUUID, revision, withContent)
	if err == nil && read.Missing {
		// The game's creation stopped before Play had its version: create it
		// and read once more.
		if refusal := h.ensureDraft(ctx, userUUID, gameUUID); refusal != nil {
			return sessionpkg.DraftSnapshot{}, "", refusal
		}
		read, err = h.reader.ReadDraft(ctx, userUUID, gameUUID, revision, withContent)
	}
	if err != nil {
		logging.LogError(ctx, err)
		if errors.Is(err, sessionpkg.ErrDraftRevisionNotFound) {
			return sessionpkg.DraftSnapshot{}, "", refuseCoded(http.StatusNotFound, codeRevisionNotFound, "the draft has no such revision")
		}
		return sessionpkg.DraftSnapshot{}, "", refuse(http.StatusInternalServerError, "reading draft failed")
	}
	if refusal := accessRefusal(read.Access); refusal != nil {
		return sessionpkg.DraftSnapshot{}, "", refusal
	}
	if read.Missing {
		logging.LogFields(ctx, logging.Field("wiring_error", "draft still missing after it was ensured"))
		return sessionpkg.DraftSnapshot{}, "", refuse(http.StatusInternalServerError, "reading draft failed")
	}
	return read.Snapshot, read.GameName, nil
}

// draftBody is a read of the version being built as the API answers it.
func draftBody(snapshot sessionpkg.DraftSnapshot) draftResponse {
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
	return draftResponse{
		GameUUID:         snapshot.GameUUID,
		VersionUUID:      snapshot.VersionUUID,
		Status:           snapshot.Status,
		CurrentRevision:  snapshot.CurrentRevision,
		Revision:         snapshot.Revision,
		Files:            files,
		Assets:           assets,
		AssetDiagnostics: diagnostics(snapshot.AssetDiagnostics),
		Attempts:         attempts,

		Build:               buildBody(snapshot.Build),
		LastSuccessfulBuild: buildBody(snapshot.LastSuccessfulBuild),
	}
}

// handleListDraftRevisions answers every revision of the game's version
// being built, oldest first.
func (h *Handler) handleListDraftRevisions(w http.ResponseWriter, r *http.Request) {
	userUUID, gameUUID, ok := caller(w, r, "listing draft revisions")
	if !ok {
		return
	}
	noStore(w)
	revisions, refusal := h.ListRevisions(r.Context(), userUUID, gameUUID)
	if refusal != nil {
		writeRefusal(w, refusal)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, revisions)
}

// ListRevisions answers every revision of the game's version being built,
// oldest first.
func (h *Handler) ListRevisions(ctx context.Context, userUUID string, rawGameUUID string) ([]RevisionResponse, *Refusal) {
	gameUUID, refusal := gameOf(ctx, rawGameUUID)
	if refusal != nil {
		return nil, refusal
	}
	listed, err := h.reader.ListDraftRevisions(ctx, userUUID, gameUUID)
	if err == nil && listed.Missing {
		if refusal := h.ensureDraft(ctx, userUUID, gameUUID); refusal != nil {
			return nil, refusal
		}
		listed, err = h.reader.ListDraftRevisions(ctx, userUUID, gameUUID)
	}
	if err != nil {
		logging.LogError(ctx, err)
		return nil, refuse(http.StatusInternalServerError, "listing draft revisions failed")
	}
	if refusal := accessRefusal(listed.Access); refusal != nil {
		return nil, refusal
	}
	if listed.Missing {
		logging.LogFields(ctx, logging.Field("wiring_error", "draft still missing after it was ensured"))
		return nil, refuse(http.StatusInternalServerError, "listing draft revisions failed")
	}
	revisions := make([]RevisionResponse, len(listed.Revisions))
	for i, revision := range listed.Revisions {
		revisions[i] = RevisionResponse{Number: revision.Number, Source: revision.Source, Files: revision.Files, CreatedAt: revision.CreatedAt}
	}
	return revisions, nil
}

// ensureDraft asks the orchestrator to create the game's version being built
// that a read found missing in Play, refusing when it could not or the caller
// may no longer edit the game.
func (h *Handler) ensureDraft(ctx context.Context, userUUID string, gameUUID string) *Refusal {
	access, err := h.editor.EnsureDraft(ctx, userUUID, gameUUID)
	if err != nil {
		logging.LogError(ctx, err)
		return refuse(http.StatusInternalServerError, "preparing draft failed")
	}
	return accessRefusal(access)
}

// writeAccessRefusal answers a caller Game Management did not let edit the
// game, returning false; it returns true when the caller may go on.
func writeAccessRefusal(w http.ResponseWriter, access game.EditableVersionOutcome) bool {
	if refusal := accessRefusal(access); refusal != nil {
		writeRefusal(w, refusal)
		return false
	}
	return true
}

func diagnostics(list []sessionpkg.DraftDiagnostic) []diagnosticResponse {
	result := make([]diagnosticResponse, len(list))
	for i, diagnostic := range list {
		result[i] = diagnosticResponse{Path: diagnostic.Path, Code: diagnostic.Code, Message: diagnostic.Message}
	}
	return result
}
