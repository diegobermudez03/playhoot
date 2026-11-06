package games

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	composergameauthoring "github.com/diegobermudez03/playhoot/composer/gameauthoring"
	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/play/runtime"
	"github.com/diegobermudez03/playhoot/play/runtime/usecases/simulategame"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

const (
	simulationResultFormat  = "playhoot.simulation-result.v1"
	simulationSummaryFormat = "playhoot.simulation-summary.v1"
	simulationBatchFormat   = "playhoot.simulation-batch.v1"

	// DetailFull asks a simulation for every observation, command, event,
	// error and refusal instead of the summary.
	DetailFull = "full"

	// maxScenarioBodyBytes bounds a scenario request.
	maxScenarioBodyBytes = 1 << 20
	// simulationWriteSlack is how much longer than its budget a simulation
	// may take to write its answer.
	simulationWriteSlack = 10 * time.Second
	// summaryListed is how many refusals and errors a summary lists; the
	// rest are counted.
	summaryListed = 10

	codeInvalidScenario  = "invalid_scenario"
	codeBuildNotReady    = "build_not_ready"
	codeSimulationsBusy  = "simulations_busy"
	codeScenarioNotFound = "scenario_not_found"
	simulationRetryAfter = "5"
)

// SimulateRequest is what to simulate on a revision (0: the current one):
// the scenario given, the one saved at ScenarioPath, or every saved one
// (All). Detail "full" answers everything a single scenario did; the
// default is a summary.
type SimulateRequest struct {
	Revision     int64
	Detail       string
	ScenarioPath string
	All          bool
	Scenario     json.RawMessage
}

// simulateEnvelope is the request fields a simulation body carries next to
// (or instead of) a scenario's.
type simulateEnvelope struct {
	Revision     int64  `json:"revision"`
	Detail       string `json:"detail"`
	ScenarioPath string `json:"scenario_path"`
	All          bool   `json:"all"`
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

// ExpectationResponse is one expectation as checked: at_ms null is the end.
type ExpectationResponse struct {
	Index        int             `json:"index"`
	AtMs         *int64          `json:"at_ms"`
	Of           string          `json:"of"`
	Path         string          `json:"path"`
	Op           string          `json:"op"`
	Expected     json.RawMessage `json:"expected,omitempty"`
	Passed       bool            `json:"passed"`
	ObservedAtMs *int64          `json:"observed_at_ms,omitempty"`
	Final        bool            `json:"final,omitempty"`
	NotObserved  bool            `json:"not_observed,omitempty"`
	Actual       json.RawMessage `json:"actual,omitempty"`
	Problem      string          `json:"problem,omitempty"`
}

// ExpectationsSummary counts a scenario's expectations and lists the
// failed ones.
type ExpectationsSummary struct {
	Total  int                   `json:"total"`
	Passed int                   `json:"passed"`
	Failed []ExpectationResponse `json:"failed"`
}

// SimulationCounts is how many of each thing a simulation produced.
type SimulationCounts struct {
	Commands int `json:"commands"`
	Rejected int `json:"rejected"`
	Events   int `json:"events"`
	Errors   int `json:"errors"`
}

// SimulationSummary is what a simulated scenario did, without states: read
// them with the result's state route while it is kept (expires_at).
type SimulationSummary struct {
	Format           string                        `json:"format"`
	ResultID         string                        `json:"result_id"`
	ExpiresAt        time.Time                     `json:"expires_at"`
	Revision         int64                         `json:"revision"`
	ToolchainVersion string                        `json:"toolchain_version,omitempty"`
	Players          []string                      `json:"players"`
	Seed             uint64                        `json:"seed"`
	Passed           *bool                         `json:"passed,omitempty"`
	Expectations     *ExpectationsSummary          `json:"expectations,omitempty"`
	Finished         bool                          `json:"finished"`
	FinishedAtMs     *int64                        `json:"finished_at_ms,omitempty"`
	Stopped          string                        `json:"stopped"`
	SimulatedMs      int64                         `json:"simulated_ms"`
	Boundaries       int                           `json:"boundaries"`
	ObservedAtMs     []int64                       `json:"observed_at_ms"`
	Counts           SimulationCounts              `json:"counts"`
	Rejected         []simulationRejectionResponse `json:"rejected"`
	Errors           []simulatedErrorResponse      `json:"errors"`
	ScriptError      *scriptErrorResponse          `json:"script_error,omitempty"`
}

// SimulationResponse is everything a simulated scenario did.
type SimulationResponse struct {
	Format           string                        `json:"format"`
	ResultID         string                        `json:"result_id"`
	ExpiresAt        time.Time                     `json:"expires_at"`
	Revision         int64                         `json:"revision"`
	ToolchainVersion string                        `json:"toolchain_version,omitempty"`
	Players          []string                      `json:"players"`
	Seed             uint64                        `json:"seed"`
	Passed           *bool                         `json:"passed,omitempty"`
	Expectations     []ExpectationResponse         `json:"expectations,omitempty"`
	Finished         bool                          `json:"finished"`
	FinishedAtMs     *int64                        `json:"finished_at_ms,omitempty"`
	Stopped          string                        `json:"stopped"`
	SimulatedMs      int64                         `json:"simulated_ms"`
	Boundaries       int                           `json:"boundaries"`
	Observations     []observationResponse         `json:"observations"`
	Final            observationResponse           `json:"final"`
	Commands         []simulatedCommandResponse    `json:"commands"`
	Rejected         []simulationRejectionResponse `json:"rejected"`
	Events           []simulatedEventResponse      `json:"events"`
	Errors           []simulatedErrorResponse      `json:"errors"`
	ScriptError      *scriptErrorResponse          `json:"script_error,omitempty"`
}

// SavedScenarioResult is one saved scenario of a batch: its summary, or why
// it could not run.
type SavedScenarioResult struct {
	Path        string                         `json:"path"`
	Summary     *SimulationSummary             `json:"summary,omitempty"`
	Diagnostics []simulationDiagnosticResponse `json:"diagnostics,omitempty"`
}

// SimulationBatch is every saved scenario run: passed is absent when none
// has expectations and all ran.
type SimulationBatch struct {
	Format           string                `json:"format"`
	Revision         int64                 `json:"revision"`
	ToolchainVersion string                `json:"toolchain_version,omitempty"`
	Passed           *bool                 `json:"passed,omitempty"`
	Scenarios        []SavedScenarioResult `json:"scenarios"`
	Skipped          []string              `json:"skipped"`
}

// invalidScenarioBody refuses a body that is not one JSON object within its
// limit.
const invalidScenarioBody = "invalid request body: the scenario must be one JSON object of at most 1 MiB"

// handleSimulateDraft plays a scenario (given, or saved in the project) on a
// revision's build of the game's version being built.
func (h *Handler) handleSimulateDraft(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userUUID, gameUUID, ok := caller(w, r, "simulating draft")
	if !ok {
		return
	}
	noStore(w)
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxScenarioBodyBytes))
	if err != nil {
		logging.LogError(ctx, err)
		httpx.WriteError(w, http.StatusBadRequest, invalidScenarioBody)
		return
	}
	req, refusal := DecodeSimulateRequest(raw)
	if refusal != nil {
		writeRefusal(w, refusal)
		return
	}
	budget := simulategame.WallBudget
	if req.All {
		budget *= composergameauthoring.MaxSavedScenarios
	}
	if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(budget + simulationWriteSlack)); err != nil {
		logging.LogError(ctx, err)
	}
	result, refusal := h.Simulate(ctx, userUUID, gameUUID, req)
	if refusal != nil {
		writeRefusal(w, refusal)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

// DecodeSimulateRequest reads a simulation body: a scenario document with
// the request's fields (revision, detail) next to it, or only the request's
// fields with scenario_path or all.
func DecodeSimulateRequest(raw []byte) (SimulateRequest, *Refusal) {
	var envelope simulateEnvelope
	if len(raw) > maxScenarioBodyBytes || json.Unmarshal(raw, &envelope) != nil {
		return SimulateRequest{}, refuse(http.StatusBadRequest, invalidScenarioBody)
	}
	req := SimulateRequest{Revision: envelope.Revision, Detail: envelope.Detail, ScenarioPath: envelope.ScenarioPath, All: envelope.All}
	if req.ScenarioPath == "" && !req.All {
		req.Scenario = raw
	}
	return req, nil
}

// Simulate plays the scenario req names on a revision's build of the game's
// version being built and answers a summary (or everything, Detail full),
// or a batch for every saved scenario. Results are kept a while.
func (h *Handler) Simulate(ctx context.Context, userUUID string, rawGameUUID string, req SimulateRequest) (any, *Refusal) {
	gameUUID, refusal := gameOf(ctx, rawGameUUID)
	if refusal != nil {
		return nil, refusal
	}
	switch {
	case req.Revision < 0:
		return nil, refuse(http.StatusBadRequest, "revision must not be negative")
	case req.Detail != "" && req.Detail != DetailFull && req.Detail != "summary":
		return nil, refuse(http.StatusBadRequest, `detail is "summary" (the default) or "full"`)
	case req.All && req.ScenarioPath != "":
		return nil, refuse(http.StatusBadRequest, "give scenario_path or all, not both")
	}
	logging.LogFields(ctx, logging.Field("scenario_path", req.ScenarioPath), logging.Field("all_scenarios", req.All), logging.Field("detail", req.Detail))
	if req.ScenarioPath != "" || req.All {
		return h.simulateSaved(ctx, userUUID, gameUUID, req)
	}
	scenario, diagnostics := runtime.DecodeScenario(req.Scenario)
	if len(diagnostics) > 0 {
		return nil, invalidScenario(diagnostics)
	}
	read, err := h.reader.SimulateDraft(ctx, userUUID, gameUUID, req.Revision, scenario)
	if err == nil && read.Missing {
		// The game's creation stopped before Play had its version: create it,
		// which queues its first build, and try once more.
		if refusal := h.ensureDraft(ctx, userUUID, gameUUID); refusal != nil {
			return nil, refusal
		}
		read, err = h.reader.SimulateDraft(ctx, userUUID, gameUUID, req.Revision, scenario)
	}
	if refusal := simulationRefusal(ctx, err, read.Access, read.Missing, read.Ready, read.Revision, read.BuildStatus); refusal != nil {
		return nil, refusal
	}
	result := read.Result
	switch result.Outcome {
	case runtime.SimulationRan:
		if req.Detail == DetailFull {
			return simulationBody(read.Revision, read.ToolchainVersion, result), nil
		}
		return summaryBody(read.Revision, read.ToolchainVersion, result), nil
	case runtime.SimulationInvalid:
		return nil, invalidScenario(result.Diagnostics)
	default:
		logging.LogFields(ctx, logging.Field("unknown_simulation_outcome", string(result.Outcome)))
		return nil, refuse(http.StatusInternalServerError, "simulating draft failed")
	}
}

// simulateSaved plays the saved scenario req names, or all of them.
func (h *Handler) simulateSaved(ctx context.Context, userUUID string, gameUUID string, req SimulateRequest) (any, *Refusal) {
	read, err := h.reader.SimulateSavedScenarios(ctx, userUUID, gameUUID, req.Revision, req.ScenarioPath)
	if err == nil && read.Missing {
		if refusal := h.ensureDraft(ctx, userUUID, gameUUID); refusal != nil {
			return nil, refusal
		}
		read, err = h.reader.SimulateSavedScenarios(ctx, userUUID, gameUUID, req.Revision, req.ScenarioPath)
	}
	if refusal := simulationRefusal(ctx, err, read.Access, read.Missing, read.Ready, read.Revision, read.BuildStatus); refusal != nil {
		return nil, refusal
	}
	if read.NotFound {
		return nil, refuseCoded(http.StatusNotFound, codeScenarioNotFound, "the revision has no saved scenario at scenario_path (scenarios/<name>.json)")
	}
	if req.ScenarioPath != "" {
		run := read.Runs[0]
		if len(run.Diagnostics) > 0 {
			return nil, invalidScenario(run.Diagnostics)
		}
		switch run.Result.Outcome {
		case runtime.SimulationRan:
			if req.Detail == DetailFull {
				return simulationBody(read.Revision, read.ToolchainVersion, run.Result), nil
			}
			return summaryBody(read.Revision, read.ToolchainVersion, run.Result), nil
		default:
			return nil, invalidScenario(run.Result.Diagnostics)
		}
	}
	batch := SimulationBatch{Format: simulationBatchFormat, Revision: read.Revision, ToolchainVersion: read.ToolchainVersion, Scenarios: []SavedScenarioResult{}, Skipped: read.Skipped}
	if batch.Skipped == nil {
		batch.Skipped = []string{}
	}
	judged, passed := false, true
	for _, run := range read.Runs {
		entry := SavedScenarioResult{Path: run.Path}
		diagnostics := run.Diagnostics
		if len(diagnostics) == 0 && run.Result.Outcome != runtime.SimulationRan {
			diagnostics = run.Result.Diagnostics
		}
		if len(diagnostics) > 0 || run.Result.Outcome != runtime.SimulationRan {
			entry.Diagnostics = diagnosticsBody(diagnostics)
			judged, passed = true, false
		} else {
			summary := summaryBody(read.Revision, read.ToolchainVersion, run.Result)
			entry.Summary = &summary
			if run.Result.Passed != nil {
				judged, passed = true, passed && *run.Result.Passed
			}
		}
		batch.Scenarios = append(batch.Scenarios, entry)
	}
	if judged {
		batch.Passed = &passed
	}
	return batch, nil
}

// simulationRefusal is the refusal of a simulation that could not run:
// errors, access, a version still missing, or a build not ready.
func simulationRefusal(ctx context.Context, err error, access game.EditableVersionOutcome, missing bool, ready bool, revision int64, status sessionpkg.DraftBuildStatus) *Refusal {
	if err != nil {
		logging.LogError(ctx, err)
		switch {
		case errors.Is(err, sessionpkg.ErrDraftBuildNotFound):
			return &Refusal{Status: http.StatusConflict, Body: buildNotReadyResponse{
				ErrorResponse: httpx.ErrorResponse{Code: codeBuildNotReady, Message: "that revision has no build to simulate"},
			}}
		case errors.Is(err, runtime.ErrSimulationsBusy):
			refusal := refuseCoded(http.StatusServiceUnavailable, codeSimulationsBusy, "too many simulations in progress; try again in a few seconds")
			refusal.RetryAfter = simulationRetryAfter
			return refusal
		default:
			return refuse(http.StatusInternalServerError, "simulating draft failed")
		}
	}
	if refusal := accessRefusal(access); refusal != nil {
		return refusal
	}
	if missing {
		logging.LogFields(ctx, logging.Field("wiring_error", "draft still missing after it was ensured"))
		return refuse(http.StatusInternalServerError, "simulating draft failed")
	}
	if !ready {
		return &Refusal{Status: http.StatusConflict, Body: buildNotReadyResponse{
			ErrorResponse: httpx.ErrorResponse{Code: codeBuildNotReady, Message: "only a revision whose build SUCCEEDED can be simulated"},
			Revision:      revision,
			BuildStatus:   string(status),
		}}
	}
	return nil
}

func invalidScenario(diagnostics []runtime.SimulationDiagnostic) *Refusal {
	return &Refusal{Status: http.StatusUnprocessableEntity, Body: invalidScenarioResponse{
		ErrorResponse: httpx.ErrorResponse{Code: codeInvalidScenario, Message: "the scenario was not simulated; see diagnostics"},
		Diagnostics:   diagnosticsBody(diagnostics),
	}}
}

func diagnosticsBody(diagnostics []runtime.SimulationDiagnostic) []simulationDiagnosticResponse {
	body := make([]simulationDiagnosticResponse, len(diagnostics))
	for i, diagnostic := range diagnostics {
		body[i] = simulationDiagnosticResponse{Field: diagnostic.Field, Message: diagnostic.Message}
	}
	return body
}

func observationBody(observation runtime.SimulationObservation) observationResponse {
	views := observation.Views
	if views == nil {
		views = map[string]json.RawMessage{}
	}
	return observationResponse{RequestedAtMs: observation.RequestedAtMs, AtMs: observation.AtMs, State: observation.State, Views: views}
}

func expectationBody(checked runtime.ExpectationResult) ExpectationResponse {
	body := ExpectationResponse{
		Index: checked.Index, AtMs: checked.Expectation.AtMs, Of: checked.Expectation.Of, Path: checked.Expectation.Path,
		Op: checked.Expectation.Op, Expected: checked.Expectation.Value, Passed: checked.Passed, Final: checked.Final,
		NotObserved: checked.NotObserved, Actual: checked.Actual, Problem: checked.Problem,
	}
	if !checked.NotObserved {
		observed := checked.ObservedAtMs
		body.ObservedAtMs = &observed
	}
	return body
}

func rejectionBody(rejected runtime.SimulationRejection) simulationRejectionResponse {
	return simulationRejectionResponse{AtMs: rejected.AtMs, Player: rejected.Player, Kind: rejected.Kind, Reason: rejected.Reason, Detail: rejected.Detail}
}

func errorBody(returned runtime.SimulatedError) simulatedErrorResponse {
	return simulatedErrorResponse{AtMs: returned.AtMs, Player: returned.Player, Error: returned.Error}
}

func scriptErrorBody(result runtime.SimulationResult) *scriptErrorResponse {
	if result.ScriptError == nil {
		return nil
	}
	return &scriptErrorResponse{AtMs: result.ScriptError.AtMs, Message: result.ScriptError.Message}
}

// summaryBody is result without its states, events or command answers.
func summaryBody(revision int64, toolchain string, result runtime.SimulationResult) SimulationSummary {
	body := SimulationSummary{
		Format: simulationSummaryFormat, ResultID: result.ID, ExpiresAt: result.ExpiresAt, Revision: revision, ToolchainVersion: toolchain,
		Players: result.Players, Seed: result.Seed, Passed: result.Passed, Finished: result.Finished, FinishedAtMs: result.FinishedAtMs,
		Stopped: string(result.Stopped), SimulatedMs: result.SimulatedMs, Boundaries: result.Boundaries,
		ObservedAtMs: make([]int64, len(result.Observations)),
		Counts:       SimulationCounts{Commands: len(result.Commands), Rejected: len(result.Rejected), Events: len(result.Events), Errors: len(result.Errors)},
		Rejected:     []simulationRejectionResponse{},
		Errors:       []simulatedErrorResponse{},
		ScriptError:  scriptErrorBody(result),
	}
	for i, observation := range result.Observations {
		body.ObservedAtMs[i] = observation.RequestedAtMs
	}
	if len(result.Expectations) > 0 {
		summary := &ExpectationsSummary{Total: len(result.Expectations), Failed: []ExpectationResponse{}}
		for _, checked := range result.Expectations {
			if checked.Passed {
				summary.Passed++
				continue
			}
			summary.Failed = append(summary.Failed, expectationBody(checked))
		}
		body.Expectations = summary
	}
	for i, rejected := range result.Rejected {
		if i == summaryListed {
			break
		}
		body.Rejected = append(body.Rejected, rejectionBody(rejected))
	}
	for i, returned := range result.Errors {
		if i == summaryListed {
			break
		}
		body.Errors = append(body.Errors, errorBody(returned))
	}
	return body
}

func simulationBody(revision int64, toolchain string, result runtime.SimulationResult) SimulationResponse {
	body := SimulationResponse{
		Format:           simulationResultFormat,
		ResultID:         result.ID,
		ExpiresAt:        result.ExpiresAt,
		Revision:         revision,
		ToolchainVersion: toolchain,
		Players:          result.Players,
		Seed:             result.Seed,
		Passed:           result.Passed,
		Finished:         result.Finished,
		FinishedAtMs:     result.FinishedAtMs,
		Stopped:          string(result.Stopped),
		SimulatedMs:      result.SimulatedMs,
		Boundaries:       result.Boundaries,
		Observations:     make([]observationResponse, len(result.Observations)),
		Final:            observationBody(result.Final),
		Commands:         make([]simulatedCommandResponse, len(result.Commands)),
		Rejected:         make([]simulationRejectionResponse, len(result.Rejected)),
		Events:           make([]simulatedEventResponse, len(result.Events)),
		Errors:           make([]simulatedErrorResponse, len(result.Errors)),
		ScriptError:      scriptErrorBody(result),
	}
	for _, checked := range result.Expectations {
		body.Expectations = append(body.Expectations, expectationBody(checked))
	}
	for i, observation := range result.Observations {
		body.Observations[i] = observationBody(observation)
	}
	for i, command := range result.Commands {
		body.Commands[i] = simulatedCommandResponse{AtMs: command.AtMs, Player: command.Player, Name: command.Name, Response: command.Response}
	}
	for i, rejected := range result.Rejected {
		body.Rejected[i] = rejectionBody(rejected)
	}
	for i, event := range result.Events {
		body.Events[i] = simulatedEventResponse{AtMs: event.AtMs, To: event.To, Event: event.Event}
	}
	for i, returned := range result.Errors {
		body.Errors[i] = errorBody(returned)
	}
	return body
}
