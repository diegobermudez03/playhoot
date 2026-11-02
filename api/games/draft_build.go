package games

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/logging"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/workflows/versiondrafting"
)

// codeBuildNotFound answers a revision that has no build.
const codeBuildNotFound = "build_not_found"

// buildWaitSlack is how much longer than its wait a waiting build read may
// take to write its answer.
const buildWaitSlack = 10 * time.Second

type buildDiagnosticResponse struct {
	Stage   string `json:"stage"`
	Code    string `json:"code,omitempty"`
	File    string `json:"file,omitempty"`
	Line    int    `json:"line,omitempty"`
	Column  int    `json:"column,omitempty"`
	Message string `json:"message"`
}

type buildArtifactResponse struct {
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type buildResponse struct {
	Revision    int64                     `json:"revision"`
	Status      string                    `json:"status"`
	Diagnostics []buildDiagnosticResponse `json:"diagnostics"`
	Server      *buildArtifactResponse    `json:"server,omitempty"`
	Client      *buildArtifactResponse    `json:"client,omitempty"`
	AspectRatio string                    `json:"aspect_ratio,omitempty"`
	Reused      bool                      `json:"reused"`
	QueuedAt    time.Time                 `json:"queued_at"`
	StartedAt   *time.Time                `json:"started_at,omitempty"`
	FinishedAt  *time.Time                `json:"finished_at,omitempty"`
}

// buildStatusResponse is the build an edit made, as its answer carries it.
type buildStatusResponse struct {
	Status string `json:"status"`
}

func buildBody(build *sessionpkg.DraftBuild) *buildResponse {
	if build == nil {
		return nil
	}
	diagnostics := make([]buildDiagnosticResponse, len(build.Diagnostics))
	for i, d := range build.Diagnostics {
		diagnostics[i] = buildDiagnosticResponse{Stage: d.Stage, Code: d.Code, File: d.File, Line: d.Line, Column: d.Column, Message: d.Message}
	}
	response := &buildResponse{
		Revision:    build.Revision,
		Status:      string(build.Status),
		Diagnostics: diagnostics,
		AspectRatio: build.AspectRatio,
		Reused:      build.Reused,
		QueuedAt:    build.QueuedAt,
		StartedAt:   build.StartedAt,
		FinishedAt:  build.FinishedAt,
	}
	if build.Server != nil {
		response.Server = &buildArtifactResponse{SHA256: build.Server.SHA256, Size: build.Server.Size}
	}
	if build.Client != nil {
		response.Client = &buildArtifactResponse{SHA256: build.Client.SHA256, Size: build.Client.Size}
	}
	return response
}

func buildStatus(build *sessionpkg.DraftBuild) *buildStatusResponse {
	if build == nil {
		return nil
	}
	return &buildStatusResponse{Status: string(build.Status)}
}

// handleReadDraftBuild answers the build of a revision of the game's version
// being built (?revision=N, the current one by default). With ?wait=S (at
// most 20) it answers once the build ended or S seconds passed.
func (h *Handler) handleReadDraftBuild(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userUUID, gameUUID, ok := caller(w, r, "reading draft build")
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
	var wait time.Duration
	if raw := query.Get("wait"); raw != "" {
		seconds, err := strconv.Atoi(raw)
		if err != nil || seconds < 0 || time.Duration(seconds)*time.Second > versiondrafting.MaxBuildWait {
			httpx.WriteError(w, http.StatusBadRequest, "wait must be a whole number of seconds from 0 to 20")
			return
		}
		wait = time.Duration(seconds) * time.Second
		if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(wait + buildWaitSlack)); err != nil {
			logging.LogError(ctx, err)
		}
	}

	read, err := h.reader.ReadDraftBuild(ctx, userUUID, gameUUID, revision, wait)
	if err == nil && read.Missing {
		// The game's creation stopped before Play had its version: create it,
		// which queues its first build, and read once more.
		if !h.ensureDraft(w, r, userUUID, gameUUID) {
			return
		}
		read, err = h.reader.ReadDraftBuild(ctx, userUUID, gameUUID, revision, wait)
	}
	if err != nil {
		logging.LogError(ctx, err)
		if errors.Is(err, sessionpkg.ErrDraftBuildNotFound) {
			httpx.WriteCodedError(w, http.StatusNotFound, codeBuildNotFound, "that revision has no build")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "reading draft build failed")
		return
	}
	if !writeAccessRefusal(w, read.Access) {
		return
	}
	if read.Missing {
		logging.LogFields(ctx, logging.Field("wiring_error", "draft still missing after it was ensured"))
		httpx.WriteError(w, http.StatusInternalServerError, "reading draft build failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, buildBody(&read.Build))
}
