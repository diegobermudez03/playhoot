package engineservice

import (
	"fmt"

	"github.com/diegobermudez03/playhoot/game/v1/engine"
)

// ExecutionErrorCode identifies the category of an ExecutionError.
//
// The set of codes is expected to grow as real execution semantics are
// implemented. Callers should generally match on the sentinel error
// values below via errors.Is rather than switching on Code directly, so
// that a future ExecutionError carrying additional context still
// compares correctly.
type ExecutionErrorCode int

const (
	// ExecutionErrorUnknown is the zero value. It marks an
	// ExecutionError that has not been assigned a more specific code.
	ExecutionErrorUnknown ExecutionErrorCode = iota

	// ExecutionErrorNotImplemented marks a call into execution behavior
	// this package has not implemented yet. It exists only while the
	// engine's real language semantics are being built out and is
	// expected to disappear once execution is complete.
	ExecutionErrorNotImplemented

	// ExecutionErrorUndefinedReference marks a ReferenceExpression whose
	// name is missing from the engine.Scope Evaluate was given. The
	// compiler already guarantees the name was declared somewhere in
	// scope at compile time; this can still happen because a caller
	// assembles the runtime engine.Scope independently, outside the
	// compiler's control.
	ExecutionErrorUndefinedReference

	// ExecutionErrorDivisionByZero marks a BinaryOperatorDivide or
	// BinaryOperatorModulo evaluation whose right operand is zero.
	ExecutionErrorDivisionByZero

	// ExecutionErrorIndexOutOfRange marks an IndexExpression into a list
	// whose evaluated index is negative, non-integer, or beyond the
	// list's length.
	ExecutionErrorIndexOutOfRange

	// ExecutionErrorKeyNotFound marks an IndexExpression into a map
	// whose evaluated key has no matching entry.
	ExecutionErrorKeyNotFound

	// ExecutionErrorNoMatchingCase marks a MatchExpression whose value
	// matched none of its cases. The compiler does not require match
	// cases to be exhaustive; this is the defined, non-panicking result
	// of reaching a value no case covers.
	ExecutionErrorNoMatchingCase

	// ExecutionErrorInvalidInitialState marks a NewSnapshot call that
	// could not evaluate a global-state field initializer or an
	// invariant's Condition for the new game instance.
	ExecutionErrorInvalidInitialState

	// ExecutionErrorInvariantViolation marks a NewSnapshot call whose
	// evaluated initial global state violates one of p.Invariants. Per
	// program.InvariantDeclaration's documented violation semantics,
	// this rejects initialization entirely rather than producing a
	// Snapshot with invalid state.
	ExecutionErrorInvariantViolation
)

// ExecutionError is the error type returned by NewSnapshot and Step.
//
// An ExecutionError never represents a partially applied change: per the
// engine's atomicity contract, whenever NewSnapshot or Step returns a
// non-nil error, the input engine.Snapshot (if any) is unchanged, no
// engine.Commit is produced, and no engine.Output is considered
// published.
//
// ExecutionError deliberately does not represent compile-time semantic
// problems; those are reported as Diagnostics by Compile, not as errors.
type ExecutionError struct {
	Code    ExecutionErrorCode
	Message string
}

func (e *ExecutionError) Error() string {
	return e.Message
}

// Is lets errors.Is(err, ErrExecutionNotImplemented) and similar checks
// match by Code rather than by pointer identity, so a future
// ExecutionError constructed with additional context still satisfies
// errors.Is against the matching sentinel below.
func (e *ExecutionError) Is(target error) bool {
	t, ok := target.(*ExecutionError)
	if !ok {
		return false
	}
	return e.Code == t.Code
}

// newExecutionError builds an *ExecutionError with the given code and a
// formatted message.
func newExecutionError(code ExecutionErrorCode, format string, args ...any) *ExecutionError {
	return &ExecutionError{Code: code, Message: fmt.Sprintf(format, args...)}
}

// ErrExecutionNotImplemented is returned by any execution operation
// whose real semantics have not been implemented yet. It is a temporary
// marker, not a permanent part of the engine's contract.
var ErrExecutionNotImplemented = &ExecutionError{
	Code:    ExecutionErrorNotImplemented,
	Message: "engineservice: execution semantics are not implemented yet",
}

// NewSnapshot creates the initial engine.Snapshot for one new game
// instance of p, together with the first Signal a caller should pass to
// Step: it binds and validates p.RootWorkflow's Parameters from
// input.RootParameters, evaluates its LocalState, creates its declared
// runtime slots (all empty), evaluates every field of p.GlobalState into
// the instance's initial global state, and evaluates every one of
// p.Invariants against it.
//
// Per LOGICAL_CONTRACT.md, initialization creates the root workflow
// instance and produces its first lifecycle signal — a named
// "WorkflowStarted" Signal — without recursively executing a transition
// for it: the returned Signal is not applied here, only handed back for
// a future Step call to apply.
//
// Initialization is atomic: if a root parameter is missing or does not
// match its declared type, or a local-state, global-state, or invariant
// expression fails to evaluate, or any invariant is false, NewSnapshot
// returns a non-nil error and a zero engine.Snapshot — never a Snapshot
// holding partially initialized or invariant-violating state. Per
// program.InvariantDeclaration's documented violation semantics, an
// invariant violation here is a rejected initialization, not an
// authored outcome.
func NewSnapshot(p engine.Program, input engine.InitializationInput) (engine.Snapshot, engine.Signal, error) {
	root, ok := p.Workflows[p.RootWorkflow]
	if !ok {
		return engine.Snapshot{}, engine.Signal{}, newExecutionError(ExecutionErrorInvalidInitialState,
			"engineservice: root workflow %q is not a compiled workflow", p.RootWorkflow)
	}

	params, err := bindParameters(root.Parameters, input.RootParameters)
	if err != nil {
		return engine.Snapshot{}, engine.Signal{}, err
	}

	localFields, err := evaluateStateFields(p, root.LocalState, engine.Scope{Bindings: fieldValueMap(params)})
	if err != nil {
		return engine.Snapshot{}, engine.Signal{}, err
	}

	rootInstance := engine.WorkflowInstance{
		Workflow:       p.RootWorkflow,
		State:          root.InitialState,
		Parameters:     params,
		LocalState:     engine.RecordValue{TypeName: "local", Fields: localFields},
		QuestionSlots:  newQuestionSlotInstances(root.QuestionSlots),
		AskGroupSlots:  newAskGroupSlotInstances(root.AskGroupSlots),
		TimerSlots:     newTimerSlotInstances(root.TimerSlots),
		ChildSlots:     newChildWorkflowSlotInstances(root.ChildSlots),
		TaskGroupSlots: newTaskGroupSlotInstances(root.TaskGroupSlots),
	}

	globalFields, err := evaluateStateFields(p, p.GlobalState, engine.Scope{})
	if err != nil {
		return engine.Snapshot{}, engine.Signal{}, err
	}
	globalState := engine.RecordValue{TypeName: globalScopeRootName, Fields: globalFields}

	invariantScope := engine.Scope{Bindings: map[string]engine.Value{globalScopeRootName: globalState}}
	for _, inv := range p.Invariants {
		v, err := Evaluate(p, inv.Condition, invariantScope)
		if err != nil {
			return engine.Snapshot{}, engine.Signal{}, newExecutionError(ExecutionErrorInvalidInitialState,
				"engineservice: failed to evaluate invariant %q: %s", inv.Name, err)
		}
		if !v.(engine.BoolValue).Value {
			return engine.Snapshot{}, engine.Signal{}, newExecutionError(ExecutionErrorInvariantViolation,
				"engineservice: invariant %q is violated by the initial state", inv.Name)
		}
	}

	snapshot := engine.Snapshot{
		GlobalState: globalState,
		Root:        rootInstance,
		Random:      engine.RandomState{Seed: input.Seed},
		Sequence:    0,
	}
	return snapshot, engine.Signal{Name: "WorkflowStarted"}, nil
}

// bindParameters validates one argument per declared param, in
// declared order, against args, using Value.Validate to check a
// caller-supplied runtime value against its compiled Type before
// trusting it.
func bindParameters(params []engine.FieldType, args map[string]engine.Value) ([]engine.FieldValue, error) {
	result := make([]engine.FieldValue, 0, len(params))
	for _, p := range params {
		v, ok := args[p.Name]
		if !ok {
			return nil, newExecutionError(ExecutionErrorInvalidInitialState,
				"engineservice: missing root workflow argument %q", p.Name)
		}
		if p.Type != nil && !v.Validate(p.Type) {
			return nil, newExecutionError(ExecutionErrorInvalidInitialState,
				"engineservice: root workflow argument %q does not match its declared type", p.Name)
		}
		result = append(result, engine.FieldValue{Name: p.Name, Value: v})
	}
	return result, nil
}

// evaluateStateFields evaluates fields, in declaration order, into the
// initial state they describe — shared by Program.GlobalState and a
// workflow instance's LocalState.
func evaluateStateFields(p engine.Program, fields []engine.StateField, scope engine.Scope) ([]engine.FieldValue, error) {
	result := make([]engine.FieldValue, 0, len(fields))
	for _, f := range fields {
		v, err := Evaluate(p, f.Initializer, scope)
		if err != nil {
			return nil, newExecutionError(ExecutionErrorInvalidInitialState,
				"engineservice: failed to evaluate initializer for state field %q: %s", f.Name, err)
		}
		result = append(result, engine.FieldValue{Name: f.Name, Value: v})
	}
	return result, nil
}

func fieldValueMap(fields []engine.FieldValue) map[string]engine.Value {
	m := make(map[string]engine.Value, len(fields))
	for _, f := range fields {
		m[f.Name] = f.Value
	}
	return m
}

func newQuestionSlotInstances(slots []engine.QuestionSlot) []engine.QuestionSlotInstance {
	result := make([]engine.QuestionSlotInstance, len(slots))
	for i, s := range slots {
		result[i] = engine.QuestionSlotInstance{Name: s.Name}
	}
	return result
}

func newAskGroupSlotInstances(slots []engine.AskGroupSlot) []engine.AskGroupSlotInstance {
	result := make([]engine.AskGroupSlotInstance, len(slots))
	for i, s := range slots {
		result[i] = engine.AskGroupSlotInstance{Name: s.Name}
	}
	return result
}

func newTimerSlotInstances(slots []string) []engine.TimerSlotInstance {
	result := make([]engine.TimerSlotInstance, len(slots))
	for i, name := range slots {
		result[i] = engine.TimerSlotInstance{Name: name}
	}
	return result
}

func newChildWorkflowSlotInstances(slots []engine.ChildWorkflowSlot) []engine.ChildWorkflowSlotInstance {
	result := make([]engine.ChildWorkflowSlotInstance, len(slots))
	for i, s := range slots {
		result[i] = engine.ChildWorkflowSlotInstance{Name: s.Name}
	}
	return result
}

func newTaskGroupSlotInstances(slots []engine.TaskGroupSlot) []engine.TaskGroupSlotInstance {
	result := make([]engine.TaskGroupSlotInstance, len(slots))
	for i, s := range slots {
		result[i] = engine.TaskGroupSlotInstance{Name: s.Name}
	}
	return result
}

// Step applies exactly one Signal to snapshot and returns the atomic
// result as an engine.Commit.
//
// Step never mutates snapshot in place; on success it returns a new
// engine.Snapshot inside the engine.Commit, leaving snapshot itself
// valid and unchanged. If Step returns a non-nil error, snapshot is
// unchanged, no engine.Commit is produced, and no engine.Output is
// considered published — the step is atomic from the caller's
// perspective, per LOGICAL_CONTRACT.md.
//
// This version does not yet verify that snapshot belongs to p — that
// check returns once engine.Program and engine.Snapshot carry real
// compiled/instance identity — and does not implement the remaining
// execution contract described in engine/README.md's "Execution
// contract" section (transition selection, guard evaluation, operation
// execution, invariant validation, projection recalculation, or output
// creation). It always returns ErrExecutionNotImplemented.
func Step(p engine.Program, snapshot engine.Snapshot, signal engine.Signal) (engine.Commit, error) {
	return engine.Commit{}, ErrExecutionNotImplemented
}
