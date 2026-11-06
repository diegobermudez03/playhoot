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

// DiffResponse compares two revisions of a game's version being built: each
// file that differs, with its status and unified diff.
type DiffResponse struct {
	From  int64               `json:"from"`
	To    int64               `json:"to"`
	Files []FileDiffEntryBody `json:"files"`
}

// FileDiffEntryBody is one differing file: status added, modified or deleted
// (the list may grow); diff cut at 8 KiB when truncated, absent when omitted
// because the comparison's 64 KiB were spent (ask for that path alone).
type FileDiffEntryBody struct {
	Path      string `json:"path"`
	Status    string `json:"status"`
	Diff      string `json:"diff,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
	Omitted   bool   `json:"omitted,omitempty"`
}

// handleReadDraftDiff compares revision ?from=N (0: the empty project) with
// ?to=M (the current one by default) of the game's version being built,
// only ?path=P when given.
func (h *Handler) handleReadDraftDiff(w http.ResponseWriter, r *http.Request) {
	userUUID, gameUUID, ok := caller(w, r, "reading draft diff")
	if !ok {
		return
	}
	noStore(w)
	query := r.URL.Query()
	from, err := strconv.ParseInt(query.Get("from"), 10, 64)
	if err != nil || from < 0 {
		httpx.WriteError(w, http.StatusBadRequest, "from must be a revision number, or 0 for the empty project")
		return
	}
	to, refusal := revisionQuery(query.Get("to"))
	if refusal != nil {
		writeRefusal(w, refusal)
		return
	}
	diff, refusal := h.ReadDiff(r.Context(), userUUID, gameUUID, from, to, query.Get("path"))
	if refusal != nil {
		writeRefusal(w, refusal)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, diff)
}

// ReadDiff compares revision from (0: the empty project) with revision to
// (nil: the current one) of the game's version being built, only onlyPath
// when set.
func (h *Handler) ReadDiff(ctx context.Context, userUUID string, rawGameUUID string, from int64, to *int64, onlyPath string) (*DiffResponse, *Refusal) {
	gameUUID, refusal := gameOf(ctx, rawGameUUID)
	if refusal != nil {
		return nil, refusal
	}
	if from < 0 {
		return nil, refuse(http.StatusBadRequest, "from must be a revision number, or 0 for the empty project")
	}
	if refusal := revisionRefusal(to); refusal != nil {
		return nil, refusal
	}
	var upTo int64
	if to != nil {
		upTo = *to
	}
	logging.LogFields(ctx, logging.Field("diff_from", from), logging.Field("diff_to", upTo))
	read, err := h.reader.ReadDraftDiff(ctx, userUUID, gameUUID, from, upTo, onlyPath)
	if err == nil && read.Missing {
		if refusal := h.ensureDraft(ctx, userUUID, gameUUID); refusal != nil {
			return nil, refusal
		}
		read, err = h.reader.ReadDraftDiff(ctx, userUUID, gameUUID, from, upTo, onlyPath)
	}
	if err != nil {
		logging.LogError(ctx, err)
		if errors.Is(err, sessionpkg.ErrDraftRevisionNotFound) {
			return nil, refuseCoded(http.StatusNotFound, codeRevisionNotFound, "the draft has no such revision")
		}
		return nil, refuse(http.StatusInternalServerError, "reading draft diff failed")
	}
	if refusal := accessRefusal(read.Access); refusal != nil {
		return nil, refusal
	}
	if read.Missing {
		logging.LogFields(ctx, logging.Field("wiring_error", "draft still missing after it was ensured"))
		return nil, refuse(http.StatusInternalServerError, "reading draft diff failed")
	}
	files := make([]FileDiffEntryBody, len(read.Diff.Files))
	for i, file := range read.Diff.Files {
		files[i] = FileDiffEntryBody{Path: file.Path, Status: file.Status, Diff: file.Diff, Truncated: file.Truncated, Omitted: file.Omitted}
	}
	return &DiffResponse{From: read.Diff.From, To: read.Diff.To, Files: files}, nil
}
