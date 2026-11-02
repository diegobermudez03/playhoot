package runtime

import (
	"encoding/json"
	"errors"
)

// ErrSimulationsBusy is returned when this instance runs as many simulations
// as it may at once; the caller tries again in a few seconds.
var ErrSimulationsBusy = errors.New("too many simulations in progress")

// Scenario is what a simulation plays: how many synthetic participants (P1
// to Pn), the seed of the game's randomness, how long it runs in game time,
// what each participant does and when, and the instants to observe.
type Scenario struct {
	Players     int
	Seed        uint64
	DurationMs  int64
	Steps       []ScenarioStep
	ObserveAtMs []int64
}

// ScenarioStep is one thing a participant does at AtMs of game time: send a
// command, or change its inputs to a full snapshot. Exactly one is set.
type ScenarioStep struct {
	AtMs    int64
	Player  string
	Command *ScenarioCommand
	Inputs  json.RawMessage
}

// ScenarioCommand is a command a step sends.
type ScenarioCommand struct {
	Name    string
	Payload json.RawMessage
}

// SimulationOutcome is whether a scenario was simulated.
type SimulationOutcome string

const (
	// SimulationRan means the scenario was played: its result holds what
	// happened, a script failure included.
	SimulationRan SimulationOutcome = "ran"

	// SimulationInvalid means the scenario cannot be played; Diagnostics say
	// why.
	SimulationInvalid SimulationOutcome = "invalid"
)

// SimulationStop is why a simulation stopped.
type SimulationStop string

const (
	// SimulationStoppedDuration means it reached the scenario's duration.
	SimulationStoppedDuration SimulationStop = "duration"
	// SimulationStoppedFinished means the game finished.
	SimulationStoppedFinished SimulationStop = "finished"
	// SimulationStoppedBudget means it used up its time or output budget;
	// what was simulated until then is kept.
	SimulationStoppedBudget SimulationStop = "budget"
	// SimulationStoppedScriptError means the game's script failed.
	SimulationStoppedScriptError SimulationStop = "script_error"
)

// SimulationResult is what a simulation produced. Everything but Outcome
// and Diagnostics is set only when it ran.
type SimulationResult struct {
	Outcome     SimulationOutcome
	Diagnostics []SimulationDiagnostic

	Players      []string
	Seed         uint64
	Finished     bool
	FinishedAtMs *int64
	Stopped      SimulationStop

	// Observations are the game at each instant asked for, taken at the
	// first tick at or after it (0: right after start).
	Observations []SimulationObservation
	// Final is the game when the simulation stopped.
	Final SimulationObservation

	Commands []SimulatedCommand
	Rejected []SimulationRejection
	Events   []SimulatedEvent
	Errors   []SimulatedError

	ScriptError *SimulationScriptError

	// Boundaries is how many boundaries executed.
	Boundaries int
	// SimulatedMs is how much game time was simulated.
	SimulatedMs int64
}

// SimulationDiagnostic is one reason a scenario cannot be played.
type SimulationDiagnostic struct {
	Field   string
	Message string
}

// SimulationObservation is the game's whole state and each participant's
// view at AtMs, the instant RequestedAtMs asked for (equal for the final one).
type SimulationObservation struct {
	RequestedAtMs int64
	AtMs          int64
	State         json.RawMessage
	Views         map[string]json.RawMessage
}

// SimulatedCommand is a command the game executed and what it answered.
type SimulatedCommand struct {
	AtMs     int64
	Player   string
	Name     string
	Response json.RawMessage
}

// SimulationRejection is a step the platform refused before the game saw it,
// as it would refuse it in a live Session.
type SimulationRejection struct {
	AtMs   int64
	Player string
	// Kind is "command" or "inputs".
	Kind string
	// Reason is invalid (does not match the manifest) or the name of the
	// live limit that refused it.
	Reason string
	Detail string
}

// SimulatedEvent is a logical event the game emitted, for To (none:
// everyone).
type SimulatedEvent struct {
	AtMs  int64
	To    []string
	Event json.RawMessage
}

// SimulatedError is a logical error the game returned to one participant.
type SimulatedError struct {
	AtMs   int64
	Player string
	Error  json.RawMessage
}

// SimulationScriptError is where and how the game's script failed.
type SimulationScriptError struct {
	AtMs    int64
	Message string
}
