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

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
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
	// MaxSteps and MaxObservations bound what a scenario asks for.
	MaxSteps        = 2000
	MaxObservations = 50
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

// Service runs simulations.
type Service struct {
	sandbox gameSandbox
	slots   chan struct{}
	now     func() time.Time
}

// New constructs a Service running games in host.
func New(host gameSandbox) *Service {
	return &Service{sandbox: host, slots: make(chan struct{}, maxConcurrentSimulations), now: time.Now}
}

// Simulate plays scenario on script, run under profile, whose game declares
// manifest. A scenario that cannot be played is answered SimulationInvalid;
// a game whose script fails is a result with its ScriptError.
func (s *Service) Simulate(ctx context.Context, script []byte, profile string, manifest sessionpkg.RuntimeManifest, scenario runtime.Scenario) (runtime.SimulationResult, error) {
	defer logging.Step(ctx, "simulategame.simulate").Close()

	began := s.now()
	result, err := s.simulate(ctx, script, profile, manifest, scenario)
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
	logging.LogFields(ctx, logging.Field("simulation_outcome", outcome), logging.Field("simulated_ms", result.SimulatedMs), logging.Field("boundaries", result.Boundaries))
	return result, err
}

func (s *Service) simulate(ctx context.Context, script []byte, profile string, manifest sessionpkg.RuntimeManifest, scenario runtime.Scenario) (runtime.SimulationResult, error) {
	rules, err := gamemanifest.Compile(manifest)
	if err != nil {
		return runtime.SimulationResult{}, fmt.Errorf("reading the build's manifest: %w", err)
	}
	if diagnostics := Validate(rules, scenario); len(diagnostics) > 0 {
		return runtime.SimulationResult{Outcome: runtime.SimulationInvalid, Diagnostics: diagnostics}, nil
	}
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

	run := newRun(s.sandbox, contextID, rules, scenario, s.now)
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
	if len(scenario.ObserveAtMs) > MaxObservations {
		add("observe_at_ms", fmt.Sprintf("at most %d observations", MaxObservations))
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
