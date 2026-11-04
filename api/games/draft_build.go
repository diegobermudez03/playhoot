package games

import (
	"context"
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

// BuildResponse is a build of a revision of the version being built.
type BuildResponse struct {
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

func buildBody(build *sessionpkg.DraftBuild) *BuildResponse {
	if build == nil {
		return nil
	}
	diagnostics := make([]buildDiagnosticResponse, len(build.Diagnostics))
	for i, d := range build.Diagnostics {
		diagnostics[i] = buildDiagnosticResponse{Stage: d.Stage, Code: d.Code, File: d.File, Line: d.Line, Column: d.Column, Message: d.Message}
	}
	response := &BuildResponse{
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
	revision, refusal := revisionQuery(query.Get("revision"))
	if refusal != nil {
		writeRefusal(w, refusal)
		return
	}
	var wait time.Duration
	if raw := query.Get("wait"); raw != "" {
		seconds, err := strconv.Atoi(raw)
		if err != nil {
			writeRefusal(w, waitRefusal())
			return
		}
		if wait, refusal = BuildWait(seconds); refusal != nil {
			writeRefusal(w, refusal)
			return
		}
		if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(wait + buildWaitSlack)); err != nil {
			logging.LogError(ctx, err)
		}
	}
	build, refusal := h.ReadBuild(ctx, userUUID, gameUUID, revision, wait)
	if refusal != nil {
		writeRefusal(w, refusal)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, build)
}

// BuildWait is how long a build read waits for seconds, from 0 to 20.
func BuildWait(seconds int) (time.Duration, *Refusal) {
	if seconds < 0 || time.Duration(seconds)*time.Second > versiondrafting.MaxBuildWait {
		return 0, waitRefusal()
	}
	return time.Duration(seconds) * time.Second, nil
}

func waitRefusal() *Refusal {
	return refuse(http.StatusBadRequest, "wait must be a whole number of seconds from 0 to 20")
}

// revisionQuery reads an optional ?revision=N.
func revisionQuery(raw string) (*int64, *Refusal) {
	if raw == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil, refuse(http.StatusBadRequest, "revision must be a positive integer")
	}
	return &parsed, revisionRefusal(&parsed)
}

// ReadBuild answers the build of a revision of the game's version being
// built (nil: the current one), waiting up to wait for it to end.
func (h *Handler) ReadBuild(ctx context.Context, userUUID string, rawGameUUID string, revision *int64, wait time.Duration) (*BuildResponse, *Refusal) {
	gameUUID, refusal := gameOf(ctx, rawGameUUID)
	if refusal != nil {
		return nil, refusal
	}
	if refusal := revisionRefusal(revision); refusal != nil {
		return nil, refusal
	}
	var number int64
	if revision != nil {
		number = *revision
	}
	read, err := h.reader.ReadDraftBuild(ctx, userUUID, gameUUID, number, wait)
	if err == nil && read.Missing {
		// The game's creation stopped before Play had its version: create it,
		// which queues its first build, and read once more.
		if refusal := h.ensureDraft(ctx, userUUID, gameUUID); refusal != nil {
			return nil, refusal
		}
		read, err = h.reader.ReadDraftBuild(ctx, userUUID, gameUUID, number, wait)
	}
	if err != nil {
		logging.LogError(ctx, err)
		if errors.Is(err, sessionpkg.ErrDraftBuildNotFound) {
			return nil, refuseCoded(http.StatusNotFound, codeBuildNotFound, "that revision has no build")
		}
		return nil, refuse(http.StatusInternalServerError, "reading draft build failed")
	}
	if refusal := accessRefusal(read.Access); refusal != nil {
		return nil, refusal
	}
	if read.Missing {
		logging.LogFields(ctx, logging.Field("wiring_error", "draft still missing after it was ensured"))
		return nil, refuse(http.StatusInternalServerError, "reading draft build failed")
	}
	return buildBody(&read.Build), nil
}
