// Package simulategame plays a scenario on a game's backend script in a
// fresh sandbox context, on a virtual clock: no live Session, connection or
// persistence. Inputs and commands are admitted, limited and scheduled into
// boundaries by the same rules a live Session uses (gamemanifest, livelimits,
// boundaryplan), so a simulated game runs exactly as a played one.
package simulategame

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/play/internal/business/scenariodoc"
	"github.com/diegobermudez03/playhoot/play/runtime"
	"github.com/diegobermudez03/playhoot/play/runtime/internal/business/gamemanifest"
	"github.com/diegobermudez03/playhoot/play/runtime/sandbox"
	"github.com/diegobermudez03/playhoot/play/runtime/sandbox/scriptcheck"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

// gameSandbox is the sandbox a simulation runs its game in.
type gameSandbox interface {
	CreateContext(ctx context.Context, spec sandbox.ContextSpec) (string, error)
	Start(ctx context.Context, contextID string, input sandbox.StartInput) (sandbox.StartResult, error)
	Execute(ctx context.Context, contextID string, batch sandbox.Batch) (sandbox.ExecutionResult, error)
	Project(ctx context.Context, contextID string, participantID string) (sandbox.ProjectResult, error)
	DiscardContext(ctx context.Context, contextID string) error
}

// Limits of one simulation.
const (
	// MaxDurationMs is the most game time a scenario may run.
	MaxDurationMs = 10 * 60 * 1000
	// MaxSteps, MaxHolds, MaxObservations and MaxExpectations bound what a
	// scenario asks for.
	MaxSteps        = 2000
	MaxHolds        = 500
	MaxObservations = 50
	MaxExpectations = 100
	// WallBudget is the most real time a simulation runs; past it, it stops
	// with what it simulated.
	WallBudget = 10 * time.Second
	// MaxOutputBytes bounds the observations, events, errors and responses
	// a result carries; past it, the simulation stops with what it has.
	MaxOutputBytes = 8 << 20
	// maxConcurrentSimulations is how many simulations run at once on this
	// instance.
	maxConcurrentSimulations = 2
)

var (
	simulationOutcomeLabel = metrics.NewLabel("outcome", "invalid", "duration", "finished", "budget", "script_error", "busy", "failed")

	simulationsTotal = metrics.NewCounter(
		"play_runtime_simulations_total",
		"Simulations of a scenario, by how they ended: invalid scenario, reached its duration, the game finished, out of budget, the script failed, no slot (busy) or a platform failure.",
		simulationOutcomeLabel,
	)
	simulationDuration = metrics.NewHistogram(
		"play_runtime_simulation_duration_seconds",
		"Real time one simulation took.",
		[]float64{0.1, 0.25, 0.5, 1, 2, 5, 10},
	)
	simulatedSeconds = metrics.NewHistogram(
		"play_runtime_simulated_seconds",
		"Game time one simulation simulated.",
		[]float64{1, 5, 15, 30, 60, 120, 300, 600},
	)
	simulationBoundaries = metrics.NewHistogram(
		"play_runtime_simulation_boundaries",
		"Boundaries one simulation executed.",
		[]float64{10, 50, 100, 300, 600, 1500, 3000, 6000},
	)
)

// resultStore keeps simulation results for a while: Play's Session domain
// (keepsimulationresults).
type resultStore interface {
	KeepSimulationResult(ctx context.Context, versionUUID string, resultUUID string, result json.RawMessage, expiresAt time.Time) error
	FindSimulationResult(ctx context.Context, resultUUID string, versionUUID string) (json.RawMessage, bool, error)
}

// Service runs simulations and keeps their results for a while.
type Service struct {
	sandbox gameSandbox
	results resultStore
	slots   chan struct{}
	now     func() time.Time
	newID   func() string
}

// New constructs a Service running games in host and keeping their results
// in results.
func New(host gameSandbox, results resultStore) *Service {
	return &Service{sandbox: host, results: results, slots: make(chan struct{}, maxConcurrentSimulations), now: time.Now, newID: uuid.NewString}
}

// Simulate plays scenario on script, run under profile, whose game declares
// manifest, and keeps the result for versionUUID (ID, until ExpiresAt). A
// scenario that cannot be played is answered SimulationInvalid; a game whose
// script fails is a result with its ScriptError.
func (s *Service) Simulate(ctx context.Context, versionUUID string, script []byte, profile string, manifest sessionpkg.RuntimeManifest, scenario runtime.Scenario) (runtime.SimulationResult, error) {
	defer logging.Step(ctx, "simulategame.simulate").Close()

	began := s.now()
	result, err := s.simulate(ctx, script, profile, manifest, scenario)
	if err == nil && result.Outcome == runtime.SimulationRan {
		err = s.keep(ctx, versionUUID, &result)
	}
	outcome := "failed"
	switch {
	case errors.Is(err, runtime.ErrSimulationsBusy):
		outcome = "busy"
	case err != nil:
	case result.Outcome == runtime.SimulationInvalid:
		outcome = "invalid"
	default:
		outcome = string(result.Stopped)
		simulatedSeconds.Observe(ctx, float64(result.SimulatedMs)/1000)
		simulationBoundaries.Observe(ctx, float64(result.Boundaries))
	}
	simulationsTotal.Inc(ctx, simulationOutcomeLabel.Value(outcome))
	simulationDuration.ObserveDuration(ctx, s.now().Sub(began))
	logging.LogFields(ctx, logging.Field("simulation_outcome", outcome), logging.Field("simulated_ms", result.SimulatedMs), logging.Field("boundaries", result.Boundaries), logging.Field("simulation_id", result.ID))
	return result, err
}

// keep names result and stores it for versionUUID until it expires.
func (s *Service) keep(ctx context.Context, versionUUID string, result *runtime.SimulationResult) error {
	result.ID, result.ExpiresAt = s.newID(), s.now().UTC().Add(ResultTTL).Truncate(time.Microsecond)
	encoded, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("encoding the simulation result: %w", err)
	}
	if err := s.results.KeepSimulationResult(ctx, versionUUID, result.ID, encoded, result.ExpiresAt); err != nil {
		return fmt.Errorf("keeping the simulation result: %w", err)
	}
	return nil
}

// Result is the kept result id names, when versionUUID's and not expired;
// else runtime.ErrSimulationResultNotFound.
func (s *Service) Result(ctx context.Context, id string, versionUUID string) (runtime.SimulationResult, error) {
	encoded, found, err := s.results.FindSimulationResult(ctx, id, versionUUID)
	if err != nil {
		return runtime.SimulationResult{}, fmt.Errorf("finding the simulation result: %w", err)
	}
	if !found {
		return runtime.SimulationResult{}, runtime.ErrSimulationResultNotFound
	}
	var result runtime.SimulationResult
	if err := json.Unmarshal(encoded, &result); err != nil {
		return runtime.SimulationResult{}, fmt.Errorf("decoding the simulation result: %w", err)
	}
	return result, nil
}

// ReadState reads one value of the kept result id names (versionUUID's): at
// path of the global state or a participant's view (of), in the observation
// taken for atMs (nil: the final one).
func (s *Service) ReadState(ctx context.Context, id string, versionUUID string, atMs *int64, of string, path string) (runtime.SimulationStateRead, error) {
	result, err := s.Result(ctx, id, versionUUID)
	if err != nil {
		return runtime.SimulationStateRead{}, err
	}
	read := readState(result, atMs, of, path)
	logging.LogFields(ctx, logging.Field("state_read", read.Outcome))
	return read, nil
}

func (s *Service) simulate(ctx context.Context, script []byte, profile string, manifest sessionpkg.RuntimeManifest, scenario runtime.Scenario) (runtime.SimulationResult, error) {
	rules, err := gamemanifest.Compile(manifest)
	if err != nil {
		return runtime.SimulationResult{}, fmt.Errorf("reading the build's manifest: %w", err)
	}
	if diagnostics := Validate(rules, scenario); len(diagnostics) > 0 {
		return runtime.SimulationResult{Outcome: runtime.SimulationInvalid, Diagnostics: diagnostics}, nil
	}
	played := withHolds(rules, scenario)
	played.ObserveAtMs = observationPoints(scenario)
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		return runtime.SimulationResult{}, runtime.ErrSimulationsBusy
	}

	budgeted, cancel := context.WithTimeout(ctx, WallBudget)
	defer cancel()
	contextID, err := s.sandbox.CreateContext(budgeted, sandbox.ContextSpec{GameVersionUUID: "simulation", Script: script, Profile: scriptcheck.Profile(profile)})
	if errors.Is(err, sandbox.ErrNoCapacity) {
		return runtime.SimulationResult{}, runtime.ErrSimulationsBusy
	}
	if err != nil {
		return runtime.SimulationResult{}, fmt.Errorf("creating the simulation's context: %w", err)
	}
	defer func() { _ = s.sandbox.DiscardContext(context.WithoutCancel(ctx), contextID) }()

	run := newRun(s.sandbox, contextID, rules, played, s.now)
	if err := run.play(budgeted); err != nil {
		if budgeted.Err() != nil && ctx.Err() == nil {
			run.result.Stopped = runtime.SimulationStoppedBudget
		} else {
			return runtime.SimulationResult{}, err
		}
	}
	if err := run.observeFinal(context.WithoutCancel(ctx)); err != nil {
		return runtime.SimulationResult{}, err
	}
	checkExpectations(scenario, &run.result)
	return run.result, nil
}

// Validate returns why scenario cannot be played on a game with rules, or
// nothing.
func Validate(rules gamemanifest.Rules, scenario runtime.Scenario) []runtime.SimulationDiagnostic {
	var diagnostics []runtime.SimulationDiagnostic
	add := func(field, message string) {
		diagnostics = append(diagnostics, runtime.SimulationDiagnostic{Field: field, Message: message})
	}
	if scenario.Players < rules.MinPlayers || scenario.Players > rules.MaxPlayers {
		add("players", fmt.Sprintf("this game plays with %d to %d players", rules.MinPlayers, rules.MaxPlayers))
	}
	if scenario.DurationMs <= 0 || scenario.DurationMs > MaxDurationMs {
		add("duration_ms", fmt.Sprintf("must be from 1 to %d", MaxDurationMs))
	}
	if len(scenario.Steps) > MaxSteps {
		add("steps", fmt.Sprintf("at most %d steps", MaxSteps))
	}
	if len(scenario.Holds) > MaxHolds {
		add("holds", fmt.Sprintf("at most %d holds", MaxHolds))
	}
	if len(scenario.Expect) > MaxExpectations {
		add("expect", fmt.Sprintf("at most %d expectations", MaxExpectations))
	}
	if points := observationPoints(scenario); len(points) > MaxObservations {
		add("observe_at_ms", fmt.Sprintf("at most %d instants observed, the expectations' included", MaxObservations))
	}
	players := participants(scenario.Players)
	for i, step := range scenario.Steps {
		field := fmt.Sprintf("steps[%d]", i)
		if !slices.Contains(players, step.Player) {
			add(field+".player", fmt.Sprintf("%q does not play: players are P1 to P%d", step.Player, max(scenario.Players, 1)))
		}
		if step.AtMs < 0 || step.AtMs > scenario.DurationMs {
			add(field+".at_ms", "must be from 0 to duration_ms")
		}
		if (step.Command == nil) == (step.Inputs == nil) {
			add(field, "a step sends either a command or inputs")
		}
		if step.Command != nil && step.Command.Name == "" {
			add(field+".command.name", "is required")
		}
	}
	for i, at := range scenario.ObserveAtMs {
		if at < 0 || at > scenario.DurationMs {
			add(fmt.Sprintf("observe_at_ms[%d]", i), "must be from 0 to duration_ms")
		}
	}
	held := map[string][]runtime.ScenarioHold{}
	for i, hold := range scenario.Holds {
		field := fmt.Sprintf("holds[%d]", i)
		if !slices.Contains(players, hold.Player) {
			add(field+".player", fmt.Sprintf("%q does not play: players are P1 to P%d", hold.Player, max(scenario.Players, 1)))
		}
		if hold.FromMs < 0 || hold.ToMs <= hold.FromMs || hold.ToMs > scenario.DurationMs {
			add(field, "from_ms and to_ms must be within 0..duration_ms, from_ms before to_ms")
		}
		for _, other := range held[hold.Player] {
			if hold.FromMs < other.ToMs && other.FromMs < hold.ToMs {
				add(field, "overlaps another hold of the same player")
				break
			}
		}
		held[hold.Player] = append(held[hold.Player], hold)
	}
	for i, expectation := range scenario.Expect {
		field := fmt.Sprintf("expect[%d]", i)
		if expectation.Of != scenariodoc.OfGlobal && !slices.Contains(players, expectation.Of) {
			add(field+".of", fmt.Sprintf("%q does not play: players are P1 to P%d", expectation.Of, max(scenario.Players, 1)))
		}
		if expectation.AtMs != nil && (*expectation.AtMs < 0 || *expectation.AtMs > scenario.DurationMs) {
			add(field+".at_ms", "must be from 0 to duration_ms; leave it out to check the end")
		}
	}
	return diagnostics
}

// participants are the synthetic participants of n players.
func participants(n int) []string {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("P%d", i+1)
	}
	return ids
}

// virtualTime is the instant atMs of game time, as the rate limiters take it.
func virtualTime(atMs int64) time.Time {
	return time.UnixMilli(atMs)
}

// errOutputFull stops a simulation whose result reached MaxOutputBytes.
var errOutputFull = errors.New("simulation output is full")

// encodedSize is how much a value adds to a result.
func encodedSize(value any) int {
	encoded, err := json.Marshal(value)
	if err != nil {
		return 0
	}
	return len(encoded)
}
