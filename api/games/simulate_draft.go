package games

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/play/runtime"
	"github.com/diegobermudez03/playhoot/play/runtime/usecases/simulategame"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

const (
	scenarioFormat         = "playhoot.scenario.v1"
	simulationResultFormat = "playhoot.simulation-result.v1"

	// maxScenarioBodyBytes bounds a scenario request.
	maxScenarioBodyBytes = 1 << 20
	// simulationWriteSlack is how much longer than its budget a simulation
	// may take to write its answer.
	simulationWriteSlack = 10 * time.Second

	codeInvalidScenario  = "invalid_scenario"
	codeBuildNotReady    = "build_not_ready"
	codeSimulationsBusy  = "simulations_busy"
	defaultScenarioSeed  = 1
	simulationRetryAfter = "5"
)

type scenarioRequest struct {
	Format      string                `json:"format"`
	Revision    int64                 `json:"revision"`
	Players     int                   `json:"players"`
	Seed        *uint64               `json:"seed"`
	DurationMs  int64                 `json:"duration_ms"`
	Steps       []scenarioStepRequest `json:"steps"`
	ObserveAtMs []int64               `json:"observe_at_ms"`
}

type scenarioStepRequest struct {
	AtMs    int64                   `json:"at_ms"`
	Player  string                  `json:"player"`
	Command *scenarioCommandRequest `json:"command"`
	Inputs  json.RawMessage         `json:"inputs"`
}

type scenarioCommandRequest struct {
	Name    string          `json:"name"`
	Payload json.RawMessage `json:"payload"`
}

type simulationDiagnosticResponse struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type invalidScenarioResponse struct {
	httpx.ErrorResponse
	Diagnostics []simulationDiagnosticResponse `json:"diagnostics"`
}

type buildNotReadyResponse struct {
	httpx.ErrorResponse
	Revision    int64  `json:"revision,omitempty"`
	BuildStatus string `json:"build_status,omitempty"`
}

type observationResponse struct {
	RequestedAtMs int64                      `json:"requested_at_ms"`
	AtMs          int64                      `json:"at_ms"`
	State         json.RawMessage            `json:"state"`
	Views         map[string]json.RawMessage `json:"views"`
}

type simulatedCommandResponse struct {
	AtMs     int64           `json:"at_ms"`
	Player   string          `json:"player"`
	Name     string          `json:"name"`
	Response json.RawMessage `json:"response"`
}

type simulationRejectionResponse struct {
	AtMs   int64  `json:"at_ms"`
	Player string `json:"player"`
	Kind   string `json:"kind"`
	Reason string `json:"reason"`
	Detail string `json:"detail,omitempty"`
}

type simulatedEventResponse struct {
	AtMs  int64           `json:"at_ms"`
	To    []string        `json:"to,omitempty"`
	Event json.RawMessage `json:"event"`
}

type simulatedErrorResponse struct {
	AtMs   int64           `json:"at_ms"`
	Player string          `json:"player"`
	Error  json.RawMessage `json:"error"`
}

type scriptErrorResponse struct {
	AtMs    int64  `json:"at_ms"`
	Message string `json:"message"`
}

type simulationResponse struct {
	Format       string                        `json:"format"`
	Revision     int64                         `json:"revision"`
	Players      []string                      `json:"players"`
	Seed         uint64                        `json:"seed"`
	Finished     bool                          `json:"finished"`
	FinishedAtMs *int64                        `json:"finished_at_ms,omitempty"`
	Stopped      string                        `json:"stopped"`
	SimulatedMs  int64                         `json:"simulated_ms"`
	Boundaries   int                           `json:"boundaries"`
	Observations []observationResponse         `json:"observations"`
	Final        observationResponse           `json:"final"`
	Commands     []simulatedCommandResponse    `json:"commands"`
	Rejected     []simulationRejectionResponse `json:"rejected"`
	Events       []simulatedEventResponse      `json:"events"`
	Errors       []simulatedErrorResponse      `json:"errors"`
	ScriptError  *scriptErrorResponse          `json:"script_error,omitempty"`
}

// handleSimulateDraft plays a scenario on a revision's build of the game's
// version being built and answers what happened, at once: nothing is kept.
func (h *Handler) handleSimulateDraft(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userUUID, gameUUID, ok := caller(w, r, "simulating draft")
	if !ok {
		return
	}
	noStore(w)
	var req scenarioRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxScenarioBodyBytes)).Decode(&req); err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body: the scenario must be one JSON object of at most 1 MiB")
		return
	}
	if req.Format != scenarioFormat {
		httpx.WriteJSON(w, http.StatusUnprocessableEntity, invalidScenarioResponse{
			ErrorResponse: httpx.ErrorResponse{Code: codeInvalidScenario, Message: "the scenario was not simulated; see diagnostics"},
			Diagnostics:   []simulationDiagnosticResponse{{Field: "format", Message: "must be " + scenarioFormat}},
		})
		return
	}
	if req.Revision < 0 {
		httpx.WriteError(w, http.StatusBadRequest, "revision must not be negative")
		return
	}
	if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(simulategame.WallBudget + simulationWriteSlack)); err != nil {
		logging.LogError(ctx, err)
	}

	scenario := scenarioOf(req)
	read, err := h.reader.SimulateDraft(ctx, userUUID, gameUUID, req.Revision, scenario)
	if err == nil && read.Missing {
		// The game's creation stopped before Play had its version: create it,
		// which queues its first build, and try once more.
		if !h.ensureDraft(w, r, userUUID, gameUUID) {
			return
		}
		read, err = h.reader.SimulateDraft(ctx, userUUID, gameUUID, req.Revision, scenario)
	}
	if err != nil {
		logging.LogError(ctx, err)
		switch {
		case errors.Is(err, sessionpkg.ErrDraftBuildNotFound):
			httpx.WriteJSON(w, http.StatusConflict, buildNotReadyResponse{
				ErrorResponse: httpx.ErrorResponse{Code: codeBuildNotReady, Message: "that revision has no build to simulate"},
			})
		case errors.Is(err, runtime.ErrSimulationsBusy):
			w.Header().Set("Retry-After", simulationRetryAfter)
			httpx.WriteCodedError(w, http.StatusServiceUnavailable, codeSimulationsBusy, "too many simulations in progress; try again in a few seconds")
		default:
			httpx.WriteError(w, http.StatusInternalServerError, "simulating draft failed")
		}
		return
	}
	if !writeAccessRefusal(w, read.Access) {
		return
	}
	if read.Missing {
		logging.LogFields(ctx, logging.Field("wiring_error", "draft still missing after it was ensured"))
		httpx.WriteError(w, http.StatusInternalServerError, "simulating draft failed")
		return
	}
	if !read.Ready {
		httpx.WriteJSON(w, http.StatusConflict, buildNotReadyResponse{
			ErrorResponse: httpx.ErrorResponse{Code: codeBuildNotReady, Message: "only a revision whose build SUCCEEDED can be simulated"},
			Revision:      read.Revision,
			BuildStatus:   string(read.BuildStatus),
		})
		return
	}
	result := read.Result
	switch result.Outcome {
	case runtime.SimulationRan:
		httpx.WriteJSON(w, http.StatusOK, simulationBody(read.Revision, result))
	case runtime.SimulationInvalid:
		diagnostics := make([]simulationDiagnosticResponse, len(result.Diagnostics))
		for i, d := range result.Diagnostics {
			diagnostics[i] = simulationDiagnosticResponse{Field: d.Field, Message: d.Message}
		}
		httpx.WriteJSON(w, http.StatusUnprocessableEntity, invalidScenarioResponse{
			ErrorResponse: httpx.ErrorResponse{Code: codeInvalidScenario, Message: "the scenario was not simulated; see diagnostics"},
			Diagnostics:   diagnostics,
		})
	default:
		logging.LogFields(ctx, logging.Field("unknown_simulation_outcome", string(result.Outcome)))
		httpx.WriteError(w, http.StatusInternalServerError, "simulating draft failed")
	}
}

func scenarioOf(req scenarioRequest) runtime.Scenario {
	scenario := runtime.Scenario{Players: req.Players, Seed: defaultScenarioSeed, DurationMs: req.DurationMs, ObserveAtMs: req.ObserveAtMs}
	if req.Seed != nil {
		scenario.Seed = *req.Seed
	}
	for _, step := range req.Steps {
		converted := runtime.ScenarioStep{AtMs: step.AtMs, Player: step.Player}
		if step.Command != nil {
			converted.Command = &runtime.ScenarioCommand{Name: step.Command.Name, Payload: step.Command.Payload}
		}
		if len(step.Inputs) > 0 {
			converted.Inputs = step.Inputs
		}
		scenario.Steps = append(scenario.Steps, converted)
	}
	return scenario
}

func observationBody(observation runtime.SimulationObservation) observationResponse {
	views := observation.Views
	if views == nil {
		views = map[string]json.RawMessage{}
	}
	return observationResponse{RequestedAtMs: observation.RequestedAtMs, AtMs: observation.AtMs, State: observation.State, Views: views}
}

func simulationBody(revision int64, result runtime.SimulationResult) simulationResponse {
	body := simulationResponse{
		Format:       simulationResultFormat,
		Revision:     revision,
		Players:      result.Players,
		Seed:         result.Seed,
		Finished:     result.Finished,
		FinishedAtMs: result.FinishedAtMs,
		Stopped:      string(result.Stopped),
		SimulatedMs:  result.SimulatedMs,
		Boundaries:   result.Boundaries,
		Observations: make([]observationResponse, len(result.Observations)),
		Final:        observationBody(result.Final),
		Commands:     make([]simulatedCommandResponse, len(result.Commands)),
		Rejected:     make([]simulationRejectionResponse, len(result.Rejected)),
		Events:       make([]simulatedEventResponse, len(result.Events)),
		Errors:       make([]simulatedErrorResponse, len(result.Errors)),
	}
	for i, observation := range result.Observations {
		body.Observations[i] = observationBody(observation)
	}
	for i, command := range result.Commands {
		body.Commands[i] = simulatedCommandResponse{AtMs: command.AtMs, Player: command.Player, Name: command.Name, Response: command.Response}
	}
	for i, rejected := range result.Rejected {
		body.Rejected[i] = simulationRejectionResponse{AtMs: rejected.AtMs, Player: rejected.Player, Kind: rejected.Kind, Reason: rejected.Reason, Detail: rejected.Detail}
	}
	for i, event := range result.Events {
		body.Events[i] = simulatedEventResponse{AtMs: event.AtMs, To: event.To, Event: event.Event}
	}
	for i, returned := range result.Errors {
		body.Errors[i] = simulatedErrorResponse{AtMs: returned.AtMs, Player: returned.Player, Error: returned.Error}
	}
	if result.ScriptError != nil {
		body.ScriptError = &scriptErrorResponse{AtMs: result.ScriptError.AtMs, Message: result.ScriptError.Message}
	}
	return body
}
