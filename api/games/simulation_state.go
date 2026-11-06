package games

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/play/runtime"
)

const (
	codeSimulationNotFound = "simulation_not_found"
	codeNotObserved        = "not_observed"
	codePathNotFound       = "path_not_found"
	codeInvalidStateRead   = "invalid_state_read"
)

// SimulationStateResponse is one value of a kept simulation result. A value
// too large to answer comes as too_large with its size and its keys (an
// object) or length (an array): ask for a narrower path.
type SimulationStateResponse struct {
	ResultID      string          `json:"result_id"`
	Of            string          `json:"of"`
	Path          string          `json:"path"`
	RequestedAtMs *int64          `json:"requested_at_ms"`
	AtMs          int64           `json:"at_ms"`
	Final         bool            `json:"final,omitempty"`
	Value         json.RawMessage `json:"value,omitempty"`
	TooLarge      bool            `json:"too_large,omitempty"`
	Size          int             `json:"size,omitempty"`
	Keys          []string        `json:"keys,omitempty"`
	Length        *int            `json:"length,omitempty"`
}

type notObservedResponse struct {
	httpx.ErrorResponse
	ObservedAtMs []int64 `json:"observed_at_ms"`
}

// handleReadSimulationState answers one value of a kept simulation result:
// ?at_ms=N (absent: the end), ?of=global|P1.. (default global), ?path=a.b[0].
func (h *Handler) handleReadSimulationState(w http.ResponseWriter, r *http.Request) {
	userUUID, gameUUID, ok := caller(w, r, "reading simulation state")
	if !ok {
		return
	}
	noStore(w)
	query := r.URL.Query()
	var atMs *int64
	if raw := query.Get("at_ms"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 0 {
			httpx.WriteError(w, http.StatusBadRequest, "at_ms must be an instant observed, in milliseconds; leave it out for the end")
			return
		}
		atMs = &parsed
	}
	state, refusal := h.ReadSimulationState(r.Context(), userUUID, gameUUID, r.PathValue("resultID"), atMs, query.Get("of"), query.Get("path"))
	if refusal != nil {
		writeRefusal(w, refusal)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, state)
}

// ReadSimulationState reads one value of a kept simulation result of the
// game's version being built: at path of the global state or a player's view
// (of, default global), in the observation for atMs (nil: the end).
func (h *Handler) ReadSimulationState(ctx context.Context, userUUID string, rawGameUUID string, resultID string, atMs *int64, of string, path string) (*SimulationStateResponse, *Refusal) {
	gameUUID, refusal := gameOf(ctx, rawGameUUID)
	if refusal != nil {
		return nil, refusal
	}
	if of == "" {
		of = "global"
	}
	read, err := h.reader.ReadSimulationState(ctx, userUUID, gameUUID, resultID, atMs, of, path)
	if err != nil {
		logging.LogError(ctx, err)
		if errors.Is(err, runtime.ErrSimulationResultNotFound) {
			return nil, refuseCoded(http.StatusNotFound, codeSimulationNotFound, "no kept simulation result has that id: it expired or is unknown; simulate again")
		}
		return nil, refuse(http.StatusInternalServerError, "reading simulation state failed")
	}
	if refusal := accessRefusal(read.Access); refusal != nil {
		return nil, refusal
	}
	state := read.Read
	switch state.Outcome {
	case runtime.StateReadFound:
	case runtime.StateReadNotObserved:
		observed := state.Observed
		if observed == nil {
			observed = []int64{}
		}
		return nil, &Refusal{Status: http.StatusNotFound, Body: notObservedResponse{
			ErrorResponse: httpx.ErrorResponse{Code: codeNotObserved, Message: "the simulation did not observe that instant; read one of observed_at_ms, or leave at_ms out for the end"},
			ObservedAtMs:  observed,
		}}
	case runtime.StateReadPathNotFound:
		return nil, refuseCoded(http.StatusNotFound, codePathNotFound, "nothing at that path; read a shorter one to see what is there")
	case runtime.StateReadInvalid:
		return nil, refuseCoded(http.StatusBadRequest, codeInvalidStateRead, state.Problem)
	default:
		logging.LogFields(ctx, logging.Field("unknown_state_read", state.Outcome))
		return nil, refuse(http.StatusInternalServerError, "reading simulation state failed")
	}
	body := &SimulationStateResponse{
		ResultID: resultID, Of: state.Of, Path: state.Path, AtMs: state.AtMs, Final: state.Final, Value: state.Value,
		TooLarge: state.TooLarge, Size: state.Size, Keys: state.Keys, Length: state.Length,
	}
	if !state.Final {
		requested := state.RequestedAtMs
		body.RequestedAtMs = &requested
	}
	return body, nil
}
