package games

import (
	"encoding/json"
	"net/http"

	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/play/runtime"
)

const (
	maxStartPreviewBodyBytes = 4 << 10

	codeInvalidPlayers       = "invalid_players"
	codePreviewUnavailable   = "preview_unavailable"
	codePreviewScriptFailure = "preview_script_failure"
	previewRetryAfter        = "5"
)

type startPreviewRequest struct {
	Revision int64 `json:"revision"`
	Players  int   `json:"players"`
}

type startPreviewResponse struct {
	PreviewUUID  string   `json:"preview_uuid"`
	Revision     int64    `json:"revision"`
	Participants []string `json:"participants"`
}

type invalidPlayersResponse struct {
	httpx.ErrorResponse
	Min int `json:"min"`
	Max int `json:"max"`
}

// handleStartPreview starts a preview of a revision's build of the game's
// version being built, ending the game's earlier preview. The owner then
// connects to it at /v1/play/previews/{previewUUID}/connect.
func (h *Handler) handleStartPreview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userUUID, gameUUID, ok := caller(w, r, "starting preview")
	if !ok {
		return
	}
	noStore(w)
	var req startPreviewRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxStartPreviewBodyBytes)).Decode(&req); err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Revision < 0 {
		httpx.WriteError(w, http.StatusBadRequest, "revision must not be negative")
		return
	}
	if req.Players <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "players must be a positive number")
		return
	}

	started, err := h.editor.StartPreview(ctx, userUUID, gameUUID, req.Revision, req.Players)
	if err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusInternalServerError, "starting preview failed")
		return
	}
	if !writeAccessRefusal(w, started.Access) {
		return
	}
	result := started.Result
	switch result.Outcome {
	case runtime.PreviewStarted:
		httpx.WriteJSON(w, http.StatusCreated, startPreviewResponse{PreviewUUID: result.PreviewUUID, Revision: result.Revision, Participants: result.Participants})
	case runtime.PreviewBuildNotReady:
		httpx.WriteJSON(w, http.StatusConflict, buildNotReadyResponse{
			ErrorResponse: httpx.ErrorResponse{Code: codeBuildNotReady, Message: "only a revision whose build SUCCEEDED can be previewed"},
			BuildStatus:   result.BuildStatus,
		})
	case runtime.PreviewInvalidPlayers:
		httpx.WriteJSON(w, http.StatusUnprocessableEntity, invalidPlayersResponse{
			ErrorResponse: httpx.ErrorResponse{Code: codeInvalidPlayers, Message: "the game is played by min..max players"},
			Min:           result.MinPlayers, Max: result.MaxPlayers,
		})
	case runtime.PreviewScriptFailure:
		httpx.WriteCodedError(w, http.StatusUnprocessableEntity, codePreviewScriptFailure, "the game's start failed in its script; simulate it to see why")
	case runtime.PreviewUnavailable:
		w.Header().Set("Retry-After", previewRetryAfter)
		httpx.WriteCodedError(w, http.StatusServiceUnavailable, codePreviewUnavailable, "no room to run a preview now; try again in a few seconds")
	default:
		logging.LogFields(ctx, logging.Field("unknown_preview_outcome", string(result.Outcome)))
		httpx.WriteError(w, http.StatusInternalServerError, "starting preview failed")
	}
}
