package games

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/logging"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

// maxFilesPerRead is the most files one read of several may name.
const maxFilesPerRead = 50

// RevisionChangesResponse is a revision's files, with their hashes and sizes
// but never their contents, and the paths that changed since an earlier
// revision.
type RevisionChangesResponse struct {
	Since    int64               `json:"since"`
	Revision int64               `json:"revision"`
	Files    []draftFileResponse `json:"files"`
	Added    []string            `json:"added"`
	Modified []string            `json:"modified"`
	Deleted  []string            `json:"deleted"`
}

func changesBody(changes *sessionpkg.DraftRevisionChanges) *RevisionChangesResponse {
	if changes == nil {
		return nil
	}
	files := make([]draftFileResponse, len(changes.Files))
	for i, file := range changes.Files {
		files[i] = draftFileResponse{Path: file.Path, SHA256: file.SHA256, Size: file.Size}
	}
	return &RevisionChangesResponse{
		Since: changes.Since, Revision: changes.Revision, Files: files,
		Added: nonNil(changes.Added), Modified: nonNil(changes.Modified), Deleted: nonNil(changes.Deleted),
	}
}

func nonNil(list []string) []string {
	if list == nil {
		return []string{}
	}
	return list
}

// handleReadDraftChanged answers the files of a revision of the game's
// version being built (?revision=N, the current one by default) and what
// changed since an earlier one (?since=N, 0 or absent: since the beginning).
func (h *Handler) handleReadDraftChanged(w http.ResponseWriter, r *http.Request) {
	userUUID, gameUUID, ok := caller(w, r, "reading draft changes")
	if !ok {
		return
	}
	noStore(w)
	query := r.URL.Query()
	var since int64
	if raw := query.Get("since"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 0 {
			httpx.WriteError(w, http.StatusBadRequest, "since must be a revision number, or 0")
			return
		}
		since = parsed
	}
	revision, refusal := revisionQuery(query.Get("revision"))
	if refusal != nil {
		writeRefusal(w, refusal)
		return
	}
	changes, refusal := h.ReadChanges(r.Context(), userUUID, gameUUID, since, revision)
	if refusal != nil {
		writeRefusal(w, refusal)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, changes)
}

// ReadChanges reads the files of a revision (nil: the current one) of the
// game's version being built, without contents, and what changed since an
// earlier revision (0: since the beginning).
func (h *Handler) ReadChanges(ctx context.Context, userUUID string, rawGameUUID string, since int64, revision *int64) (*RevisionChangesResponse, *Refusal) {
	gameUUID, refusal := gameOf(ctx, rawGameUUID)
	if refusal != nil {
		return nil, refusal
	}
	if since < 0 {
		return nil, refuse(http.StatusBadRequest, "since must be a revision number, or 0")
	}
	if refusal := revisionRefusal(revision); refusal != nil {
		return nil, refusal
	}
	var upTo int64
	if revision != nil {
		upTo = *revision
	}
	read, err := h.reader.ReadDraftChanges(ctx, userUUID, gameUUID, since, upTo)
	if err == nil && read.Missing {
		if refusal := h.ensureDraft(ctx, userUUID, gameUUID); refusal != nil {
			return nil, refusal
		}
		read, err = h.reader.ReadDraftChanges(ctx, userUUID, gameUUID, since, upTo)
	}
	if err != nil {
		logging.LogError(ctx, err)
		if errors.Is(err, sessionpkg.ErrDraftRevisionNotFound) {
			return nil, refuseCoded(http.StatusNotFound, codeRevisionNotFound, "the draft has no such revision, or since is past it")
		}
		return nil, refuse(http.StatusInternalServerError, "reading draft changes failed")
	}
	if refusal := accessRefusal(read.Access); refusal != nil {
		return nil, refusal
	}
	if read.Missing {
		logging.LogFields(ctx, logging.Field("wiring_error", "draft still missing after it was ensured"))
		return nil, refuse(http.StatusInternalServerError, "reading draft changes failed")
	}
	return changesBody(&read.Changes), nil
}

// FilesResponse is several files of one revision, with their contents;
// Missing names the paths asked for that the revision does not have.
type FilesResponse struct {
	Revision int64               `json:"revision"`
	Files    []draftFileResponse `json:"files"`
	Missing  []string            `json:"missing"`
}

// handleReadDraftFiles answers the files named by ?path= (repeated) of a
// revision (?revision=N, the current one by default), with their contents.
func (h *Handler) handleReadDraftFiles(w http.ResponseWriter, r *http.Request) {
	userUUID, gameUUID, ok := caller(w, r, "reading draft files")
	if !ok {
		return
	}
	noStore(w)
	query := r.URL.Query()
	revision, refusal := revisionQuery(query.Get("revision"))
	if refusal != nil {
		writeRefusal(w, refusal)
		return
	}
	files, refusal := h.ReadFiles(r.Context(), userUUID, gameUUID, query["path"], revision)
	if refusal != nil {
		writeRefusal(w, refusal)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, files)
}

// ReadFiles reads the files at paths of a revision (nil: the current one) of
// the game's version being built, with their contents.
func (h *Handler) ReadFiles(ctx context.Context, userUUID string, rawGameUUID string, paths []string, revision *int64) (*FilesResponse, *Refusal) {
	gameUUID, refusal := gameOf(ctx, rawGameUUID)
	if refusal != nil {
		return nil, refusal
	}
	switch {
	case len(paths) == 0:
		return nil, refuse(http.StatusBadRequest, "name at least one path")
	case len(paths) > maxFilesPerRead:
		return nil, refuse(http.StatusBadRequest, "name at most "+strconv.Itoa(maxFilesPerRead)+" paths")
	}
	if refusal := revisionRefusal(revision); refusal != nil {
		return nil, refusal
	}
	var number int64
	if revision != nil {
		number = *revision
	}
	snapshot, _, refusal := h.loadDraft(ctx, userUUID, gameUUID, number, true)
	if refusal != nil {
		return nil, refusal
	}
	byPath := make(map[string]sessionpkg.DraftFile, len(snapshot.Files))
	for _, file := range snapshot.Files {
		byPath[file.Path] = file
	}
	response := &FilesResponse{Revision: snapshot.Revision, Files: []draftFileResponse{}, Missing: []string{}}
	seen := make(map[string]bool, len(paths))
	for _, path := range paths {
		if seen[path] {
			continue
		}
		seen[path] = true
		file, found := byPath[path]
		if !found || file.Content == nil {
			response.Missing = append(response.Missing, path)
			continue
		}
		response.Files = append(response.Files, draftFileResponse{Path: file.Path, SHA256: file.SHA256, Size: file.Size, Content: file.Content})
	}
	return response, nil
}
