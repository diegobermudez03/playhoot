// Package scenariodoc reads a playhoot.scenario.v1 document: what a
// simulation plays and what it expects. Decode checks its shape only; what
// depends on the game (its players, commands and inputs) is checked when it
// is simulated.
package scenariodoc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
)

// Format is the format a scenario document declares.
const Format = "playhoot.scenario.v1"

// DefaultSeed is the seed of a scenario that names none.
const DefaultSeed = 1

// Of values: the authoritative global state, or a participant's view (P1..Pn).
const OfGlobal = "global"

// Ops an expectation may use.
const (
	OpEq     = "eq"
	OpNe     = "ne"
	OpExists = "exists"
	OpAbsent = "absent"
	OpLt     = "lt"
	OpLte    = "lte"
	OpGt     = "gt"
	OpGte    = "gte"
	OpIn     = "in"
	OpLen    = "len"
)

// Scenario is what a simulation plays: how many synthetic participants (P1
// to Pn), the seed of the game's randomness, how long it runs in game time,
// what each participant does and when, the instants to observe and what is
// expected.
type Scenario struct {
	Players     int
	Seed        uint64
	DurationMs  int64
	Steps       []Step
	Holds       []Hold
	ObserveAtMs []int64
	Expect      []Expectation
}

// Step is one thing a participant does at AtMs of game time: send a command,
// or change its inputs to a full snapshot. Exactly one is set.
type Step struct {
	AtMs    int64
	Player  string
	Command *Command
	Inputs  json.RawMessage
}

// Command is a command a step sends.
type Command struct {
	Name    string
	Payload json.RawMessage
}

// Hold keeps Player's inputs at Inputs from FromMs and releases them (the
// neutral snapshot) at ToMs.
type Hold struct {
	FromMs int64
	ToMs   int64
	Player string
	Inputs json.RawMessage
}

// Expectation is what Path of Of must satisfy at AtMs (nil: when the
// simulation ends), compared by Op with Value.
type Expectation struct {
	AtMs  *int64
	Of    string
	Path  string
	Op    string
	Value json.RawMessage
}

// Diagnostic is one reason a scenario cannot be played.
type Diagnostic struct {
	Field   string
	Message string
}

type document struct {
	Format      *string          `json:"format"`
	Players     *int             `json:"players"`
	Seed        *uint64          `json:"seed"`
	DurationMs  *int64           `json:"duration_ms"`
	Steps       []stepDocument   `json:"steps"`
	Holds       []holdDocument   `json:"holds"`
	ObserveAtMs []int64          `json:"observe_at_ms"`
	Expect      []expectDocument `json:"expect"`
}

type stepDocument struct {
	AtMs    int64            `json:"at_ms"`
	Player  string           `json:"player"`
	Command *commandDocument `json:"command"`
	Inputs  json.RawMessage  `json:"inputs"`
}

type commandDocument struct {
	Name    string          `json:"name"`
	Payload json.RawMessage `json:"payload"`
}

type holdDocument struct {
	FromMs int64           `json:"from_ms"`
	ToMs   int64           `json:"to_ms"`
	Player string          `json:"player"`
	Inputs json.RawMessage `json:"inputs"`
}

type expectDocument struct {
	AtMs  *int64          `json:"at_ms"`
	Of    string          `json:"of"`
	Path  string          `json:"path"`
	Op    string          `json:"op"`
	Value json.RawMessage `json:"value"`
}

var participantPattern = regexp.MustCompile(`^P[1-9][0-9]*$`)

// Decode reads raw as a scenario document, answering why it cannot be one
// when it is not. Fields it does not know are ignored.
func Decode(raw []byte) (Scenario, []Diagnostic) {
	var doc document
	if err := json.Unmarshal(raw, &doc); err != nil {
		return Scenario{}, []Diagnostic{{Field: "", Message: "the scenario is not a JSON object of the expected shape: " + err.Error()}}
	}
	var diagnostics []Diagnostic
	add := func(field, message string) {
		diagnostics = append(diagnostics, Diagnostic{Field: field, Message: message})
	}
	if doc.Format == nil || *doc.Format != Format {
		add("format", "must be "+Format)
	}
	if doc.Players == nil {
		add("players", "is required")
	}
	if doc.DurationMs == nil {
		add("duration_ms", "is required")
	}
	scenario := Scenario{Seed: DefaultSeed, ObserveAtMs: doc.ObserveAtMs}
	if doc.Players != nil {
		scenario.Players = *doc.Players
	}
	if doc.DurationMs != nil {
		scenario.DurationMs = *doc.DurationMs
	}
	if doc.Seed != nil {
		scenario.Seed = *doc.Seed
	}
	for i, step := range doc.Steps {
		field := fmt.Sprintf("steps[%d]", i)
		hasInputs := len(step.Inputs) > 0 && !bytes.Equal(bytes.TrimSpace(step.Inputs), []byte("null"))
		if (step.Command == nil) == !hasInputs {
			add(field, "a step sends either a command or inputs")
		}
		converted := Step{AtMs: step.AtMs, Player: step.Player}
		if step.Command != nil {
			if step.Command.Name == "" {
				add(field+".command.name", "is required")
			}
			converted.Command = &Command{Name: step.Command.Name, Payload: step.Command.Payload}
		}
		if hasInputs {
			converted.Inputs = step.Inputs
		}
		scenario.Steps = append(scenario.Steps, converted)
	}
	for i, hold := range doc.Holds {
		field := fmt.Sprintf("holds[%d]", i)
		if hold.FromMs < 0 || hold.ToMs <= hold.FromMs {
			add(field, "from_ms must be at least 0 and before to_ms")
		}
		if len(hold.Inputs) == 0 || bytes.Equal(bytes.TrimSpace(hold.Inputs), []byte("null")) {
			add(field+".inputs", "is required: the inputs held")
		}
		scenario.Holds = append(scenario.Holds, Hold{FromMs: hold.FromMs, ToMs: hold.ToMs, Player: hold.Player, Inputs: hold.Inputs})
	}
	for i, expectation := range doc.Expect {
		field := fmt.Sprintf("expect[%d]", i)
		of := expectation.Of
		if of == "" {
			of = OfGlobal
		}
		if of != OfGlobal && !participantPattern.MatchString(of) {
			add(field+".of", `must be "global" or a participant (P1, P2, ...)`)
		}
		if expectation.AtMs != nil && *expectation.AtMs < 0 {
			add(field+".at_ms", "must not be negative; leave it out to check the end")
		}
		if _, err := ParsePath(expectation.Path); err != nil {
			add(field+".path", err.Error())
		}
		if message := checkOp(expectation.Op, expectation.Value); message != "" {
			add(field, message)
		}
		scenario.Expect = append(scenario.Expect, Expectation{AtMs: expectation.AtMs, Of: of, Path: expectation.Path, Op: expectation.Op, Value: expectation.Value})
	}
	if len(diagnostics) > 0 {
		return Scenario{}, diagnostics
	}
	return scenario, nil
}

// checkOp is why op cannot compare with value, or "".
func checkOp(op string, value json.RawMessage) string {
	var decoded any
	present := len(bytes.TrimSpace(value)) > 0
	if present && json.Unmarshal(value, &decoded) != nil {
		return "value is not valid JSON"
	}
	switch op {
	case OpExists, OpAbsent:
		if present {
			return op + " takes no value"
		}
	case OpEq, OpNe:
		if !present {
			return op + " needs a value"
		}
	case OpLt, OpLte, OpGt, OpGte:
		if _, ok := decoded.(float64); !ok {
			return op + " needs a number as value"
		}
	case OpIn:
		if _, ok := decoded.([]any); !ok {
			return "in needs an array of the accepted values as value"
		}
	case OpLen:
		number, ok := decoded.(float64)
		if !ok || number < 0 || number != float64(int64(number)) {
			return "len needs a whole number as value"
		}
	default:
		return fmt.Sprintf("op %q is unknown: eq, ne, exists, absent, lt, lte, gt, gte, in, len", op)
	}
	return ""
}
