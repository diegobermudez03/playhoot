package runtime

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/diegobermudez03/playhoot/play/internal/business/scenariodoc"
)

// ErrSimulationsBusy is returned when this instance runs as many simulations
// as it may at once; the caller tries again in a few seconds.
var ErrSimulationsBusy = errors.New("too many simulations in progress")

// A scenario and its parts, as a playhoot.scenario.v1 document holds them.
type (
	Scenario            = scenariodoc.Scenario
	ScenarioStep        = scenariodoc.Step
	ScenarioCommand     = scenariodoc.Command
	ScenarioHold        = scenariodoc.Hold
	ScenarioExpectation = scenariodoc.Expectation
)

// ScenarioFormat is the format a scenario document declares.
const ScenarioFormat = scenariodoc.Format

// DecodeScenario reads a playhoot.scenario.v1 document, or answers why its
// shape is not one; what depends on the game is checked when simulated.
func DecodeScenario(raw []byte) (Scenario, []SimulationDiagnostic) {
	return scenariodoc.Decode(raw)
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

	// Expectations are the scenario's expectations checked, in its order;
	// Passed is nil when it has none.
	Expectations []ExpectationResult
	Passed       *bool

	// ID names the kept result until ExpiresAt, for reading it again.
	ID        string
	ExpiresAt time.Time
}

// ExpectationResult is how one expectation fared: checked on the
// observation at ObservedAtMs (the final one when Final), with the value
// found there (nil when absent). NotObserved means the simulation stopped
// (script error, budget) before that instant.
type ExpectationResult struct {
	Index        int
	Expectation  ScenarioExpectation
	Passed       bool
	ObservedAtMs int64
	Final        bool
	NotObserved  bool
	Actual       json.RawMessage
	Problem      string
}

// SimulationDiagnostic is one reason a scenario cannot be played.
type SimulationDiagnostic = scenariodoc.Diagnostic

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

// ErrSimulationResultNotFound is returned when a kept result does not exist,
// expired, or belongs to another version.
var ErrSimulationResultNotFound = errors.New("simulation result not found")

// How a read of a kept result's state ended.
const (
	StateReadFound        = "found"
	StateReadNotObserved  = "not_observed"
	StateReadPathNotFound = "path_not_found"
	StateReadInvalid      = "invalid"
)

// SimulationStateRead is one value of a kept result: at Path of the global
// state or a participant's view (Of), in the observation taken for
// RequestedAtMs (the final one when Final). A value larger than a read
// answers comes as TooLarge with its Size and its Keys (an object) or
// Length (an array). NotObserved lists the instants observed.
type SimulationStateRead struct {
	Outcome       string
	Of            string
	Path          string
	RequestedAtMs int64
	AtMs          int64
	Final         bool
	Value         json.RawMessage
	TooLarge      bool
	Size          int
	Keys          []string
	Length        *int
	Observed      []int64
	Problem       string
}
