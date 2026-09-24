package runtime

import (
	"fmt"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
)

// globalScopeRootName and resourcesScopeRootName alias engine's own
// reserved scope-root constants so the runtime and the compiler (a
// separate internal package) agree on exactly these names.
const (
	globalScopeRootName    = engine.GlobalScopeRootName
	resourcesScopeRootName = engine.ResourcesScopeRootName
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

	// ExecutionErrorInvariantViolation marks a NewSnapshot or Step call
	// whose evaluated candidate global state violates one of
	// p.Invariants. Per program.InvariantDeclaration's documented
	// violation semantics, this rejects the entire initialization or
	// step, not an authored outcome.
	ExecutionErrorInvariantViolation

	// ExecutionErrorSnapshotProgramMismatch marks a Step call whose
	// snapshot does not belong to p: its root instance's Workflow does
	// not name p's own RootWorkflow.
	ExecutionErrorSnapshotProgramMismatch

	// ExecutionErrorSignalRejected marks a Step call that found no
	// applicable transition for the given Signal — no transition
	// matched it at all, or the one that matched had a Guard that
	// evaluated to false. This is a defined, non-error outcome
	// described in LOGICAL_CONTRACT.md as a "rejected signal": the
	// original Snapshot is unchanged, exactly as for any other
	// ExecutionError.
	ExecutionErrorSignalRejected

	// ExecutionErrorBudgetExceeded marks a Step call whose transition
	// executed more synchronous operations than engine.Limits.MaxOperations
	// allows.
	ExecutionErrorBudgetExceeded

	// ExecutionErrorLoopLimitExceeded marks a Step call whose transition
	// ran a ForEachOperation over more elements than
	// engine.Limits.MaxLoopIterations allows.
	ExecutionErrorLoopLimitExceeded

	// ExecutionErrorInvalidRandomRange marks a DrawRandomOperation whose
	// RandomIntegerGenerator bounds are not both finite integers with
	// Minimum not exceeding Maximum.
	ExecutionErrorInvalidRandomRange

	// ExecutionErrorEmptyRandomCollection marks a DrawRandomOperation
	// using RandomElementGenerator whose Collection evaluated to an
	// empty list.
	ExecutionErrorEmptyRandomCollection

	// ExecutionErrorSlotOccupied marks an OpenQuestionOperation,
	// ScheduleTimerOperation, OpenAskGroupOperation, or any of their
	// keyed counterparts targeting a slot (or (slot, key) occurrence)
	// that already holds a pending question, timer, or ask group.
	ExecutionErrorSlotOccupied

	// ExecutionErrorInvalidTimerDelay marks a ScheduleTimerOperation
	// whose evaluated DelayMilliseconds is not a finite, non-negative
	// integer.
	ExecutionErrorInvalidTimerDelay

	// ExecutionErrorInputRejected marks a Step call for a
	// SignalKindQuestionAnswered, SignalKindTimerExpired,
	// SignalKindAskGroupCompleted, or any of their keyed counterparts
	// that did not pass authoritative validation: the targeted slot (or
	// (slot, key) occurrence) was already empty (stale or duplicate),
	// the answer's respondent did not match the slot's pending recipient
	// (unauthorized), or the submitted answer failed response-type or
	// Validation checks (invalid). See ErrInputRejected.
	ExecutionErrorInputRejected

	// ExecutionErrorDuplicateRecipient marks an OpenAskGroupOperation
	// whose evaluated Recipients contains the same user identity more
	// than once.
	ExecutionErrorDuplicateRecipient

	// ExecutionErrorInvalidQuorum marks an OpenAskGroupOperation whose
	// Completion is an AskGroupQuorumPolicy evaluating to something
	// other than a positive integer no greater than the number of
	// recipients, or an AskGroupFirstResponsePolicy opened with no
	// recipients.
	ExecutionErrorInvalidQuorum

	// ExecutionErrorAskGroupNotJoined marks a CancelAskGroupOperation
	// targeting an ask-group slot that holds a terminal outcome still
	// awaiting join — that outcome must be consumed through
	// AskGroupCompletedSignalSource first, never silently discarded by
	// cancellation.
	ExecutionErrorAskGroupNotJoined

	// ExecutionErrorPresentationSlotOccupied marks a committed
	// transition whose resulting active-presentation set would occupy
	// the same presentation slot more than once for the same user —
	// either because one Presentation's evaluated Targets contains the
	// same user more than once, or because two different presentations
	// (workflow-level, state-level, or pending-question) target the
	// same user on the same PresentationSlot at once. Per
	// program.PresentationSlotDeclaration, a user may have at most one
	// active presentation per slot at a time.
	ExecutionErrorPresentationSlotOccupied

	// ExecutionErrorActiveSlotLimitExceeded marks an operation that
	// would occupy a new interaction slot (a question, a timer, or an
	// ask-group) on an instance that already holds
	// engine.Limits.MaxActiveSlotsPerInstance occupied slots. This
	// guards against unbounded fan-out.
	ExecutionErrorActiveSlotLimitExceeded
)

// String names code for logs, diagnostics, and debugging — see
// "stable error categories" in engine/README.md: this mapping from
// ExecutionErrorCode to a human-readable name is itself part of the
// engine's stability contract, alongside the sentinel Err* values and
// the Code values themselves. A future code is always added at the end
// of the const block and given an entry here; an existing name is never
// reused for a different meaning.
func (c ExecutionErrorCode) String() string {
	switch c {
	case ExecutionErrorUnknown:
		return "unknown"
	case ExecutionErrorUndefinedReference:
		return "undefined_reference"
	case ExecutionErrorDivisionByZero:
		return "division_by_zero"
	case ExecutionErrorIndexOutOfRange:
		return "index_out_of_range"
	case ExecutionErrorKeyNotFound:
		return "key_not_found"
	case ExecutionErrorNoMatchingCase:
		return "no_matching_case"
	case ExecutionErrorInvalidInitialState:
		return "invalid_initial_state"
	case ExecutionErrorInvariantViolation:
		return "invariant_violation"
	case ExecutionErrorSnapshotProgramMismatch:
		return "snapshot_program_mismatch"
	case ExecutionErrorSignalRejected:
		return "signal_rejected"
	case ExecutionErrorBudgetExceeded:
		return "budget_exceeded"
	case ExecutionErrorLoopLimitExceeded:
		return "loop_limit_exceeded"
	case ExecutionErrorInvalidRandomRange:
		return "invalid_random_range"
	case ExecutionErrorEmptyRandomCollection:
		return "empty_random_collection"
	case ExecutionErrorSlotOccupied:
		return "slot_occupied"
	case ExecutionErrorInvalidTimerDelay:
		return "invalid_timer_delay"
	case ExecutionErrorInputRejected:
		return "input_rejected"
	case ExecutionErrorDuplicateRecipient:
		return "duplicate_recipient"
	case ExecutionErrorInvalidQuorum:
		return "invalid_quorum"
	case ExecutionErrorAskGroupNotJoined:
		return "ask_group_not_joined"
	case ExecutionErrorPresentationSlotOccupied:
		return "presentation_slot_occupied"
	case ExecutionErrorActiveSlotLimitExceeded:
		return "active_slot_limit_exceeded"
	default:
		return "unknown"
	}
}

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
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Is lets errors.Is(err, ErrSignalRejected) and similar checks match by
// Code rather than by pointer identity, so a future
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
		Workflow:           p.RootWorkflow,
		State:              root.InitialState,
		Parameters:         params,
		LocalState:         engine.RecordValue{TypeName: "local", Fields: localFields},
		QuestionSlots:      newQuestionSlotInstances(root.QuestionSlots),
		AskGroupSlots:      newAskGroupSlotInstances(root.AskGroupSlots),
		TimerSlots:         newTimerSlotInstances(root.TimerSlots),
		KeyedQuestionSlots: newKeyedQuestionSlotInstances(root.KeyedQuestionSlots),
		KeyedAskGroupSlots: newKeyedAskGroupSlotInstances(root.KeyedAskGroupSlots),
		KeyedTimerSlots:    newKeyedTimerSlotInstances(root.KeyedTimerSlots),
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
		Random:      engine.RandomState{State: input.Seed},
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

func newKeyedQuestionSlotInstances(slots []engine.KeyedQuestionSlot) []engine.KeyedQuestionSlotInstance {
	result := make([]engine.KeyedQuestionSlotInstance, len(slots))
	for i, s := range slots {
		result[i] = engine.KeyedQuestionSlotInstance{Name: s.Name}
	}
	return result
}

func newKeyedAskGroupSlotInstances(slots []engine.KeyedAskGroupSlot) []engine.KeyedAskGroupSlotInstance {
	result := make([]engine.KeyedAskGroupSlotInstance, len(slots))
	for i, s := range slots {
		result[i] = engine.KeyedAskGroupSlotInstance{Name: s.Name}
	}
	return result
}

func newKeyedTimerSlotInstances(slots []engine.KeyedTimerSlot) []engine.KeyedTimerSlotInstance {
	result := make([]engine.KeyedTimerSlotInstance, len(slots))
	for i, s := range slots {
		result[i] = engine.KeyedTimerSlotInstance{Name: s.Name}
	}
	return result
}

// Step applies exactly one Signal to snapshot and returns the atomic
// result as an engine.Commit.
//
// Step follows program's documented per-transition order: locate the
// root instance's current WorkflowState, select one transition (a
// state-local transition for signal takes priority over a
// GlobalTransition for the same signal; the global one only applies
// when the current state has none — see engine.Workflow's
// GlobalTransitions doc comment), bind signal, evaluate Guard, execute
// Operations against a candidate copy of global and local state, apply
// exactly one WorkflowControl, then validate every invariant against
// the candidate global state. If no transition matches signal, or the
// one that does has a false Guard, Step returns
// ErrSignalRejected — a defined outcome, not a bug.
//
// Step never mutates snapshot in place: every operation goes through
// execContext's copy-on-write update (see execute.go), so the returned
// engine.Commit's Snapshot is a new value and snapshot itself remains
// valid and unchanged. If Step returns a non-nil error for any other
// reason — an evaluation failure, or an invariant violated by the
// candidate state — snapshot is equally unchanged, no engine.Commit is
// produced, and no engine.Output is considered published, per
// LOGICAL_CONTRACT.md.
//
// limits bounds the transition's execution — see engine.Limits — and is
// itself part of what a Commit is a deterministic function of: the same
// program, snapshot, signal, and limits always produce the same result.
// A Session runs exactly one workflow instance, so signal always targets
// it directly.
func Step(p engine.Program, snapshot engine.Snapshot, signal engine.Signal, limits engine.Limits) (engine.Commit, error) {
	if snapshot.Root.Workflow != p.RootWorkflow {
		return engine.Commit{}, newExecutionError(ExecutionErrorSnapshotProgramMismatch,
			"engineservice: snapshot's root instance runs workflow %q, but this program's root workflow is %q", snapshot.Root.Workflow, p.RootWorkflow)
	}
	if snapshot.Root.Outcome != nil {
		return engine.Commit{}, ErrSignalRejected
	}

	// Per program.AskGroupCompletedSignalSource's documented "never
	// produces a signal per individual answer", a submitted answer to a
	// still-collecting ask group never itself selects or runs a
	// transition — it only records the answer and re-evaluates the
	// group's completion policy. This is handled entirely separately
	// from the transition-selection flow below. The keyed variant is the
	// identical behavior scoped to one (slot, key) occurrence.
	if signal.Kind == engine.SignalKindAskGroupAnswered {
		return stepAskGroupAnswer(p, snapshot, signal)
	}
	if signal.Kind == engine.SignalKindKeyedAskGroupAnswered {
		return stepKeyedAskGroupAnswer(p, snapshot, signal)
	}

	target := snapshot.Root
	if target.Outcome != nil {
		return engine.Commit{}, ErrSignalRejected
	}

	workflow, ok := p.Workflows[target.Workflow]
	if !ok {
		return engine.Commit{}, newExecutionError(ExecutionErrorUnknown, "engineservice: workflow %q is not compiled", target.Workflow)
	}

	// Per program.QuestionAnsweredSignalSource and
	// program.TimerExpiredSignalSource, a signal of these kinds only
	// exists once authoritative validation accepts it — a stale,
	// duplicate, unauthorized, or invalid submission is rejected here,
	// before any transition is even considered.
	switch signal.Kind {
	case engine.SignalKindQuestionAnswered:
		if err := validateQuestionAnswer(p, target, signal); err != nil {
			return engine.Commit{}, err
		}
	case engine.SignalKindTimerExpired:
		if err := validateTimerExpiration(target, signal); err != nil {
			return engine.Commit{}, err
		}
	case engine.SignalKindAskGroupCompleted:
		if err := validateAskGroupCompletion(target, signal); err != nil {
			return engine.Commit{}, err
		}
	case engine.SignalKindKeyedQuestionAnswered:
		if err := validateKeyedQuestionAnswer(p, target, signal); err != nil {
			return engine.Commit{}, err
		}
	case engine.SignalKindKeyedTimerExpired:
		if err := validateKeyedTimerExpiration(target, signal); err != nil {
			return engine.Commit{}, err
		}
	case engine.SignalKindKeyedAskGroupCompleted:
		if err := validateKeyedAskGroupCompletion(target, signal); err != nil {
			return engine.Commit{}, err
		}
	}

	state, ok := workflowStateByName(workflow, target.State)
	if !ok {
		return engine.Commit{}, newExecutionError(ExecutionErrorUnknown, "engineservice: workflow %q has no state named %q", workflow.Name, target.State)
	}

	transition, ok := selectTransition(state, workflow, signal)
	if !ok {
		return engine.Commit{}, ErrSignalRejected
	}

	scope := instanceBaseScope(target, snapshot.GlobalState)
	fields := signalSchemaFields(p, workflow, target, signal)
	for _, b := range transition.Signal.Bindings {
		scope = extendScope(scope, b.Name, fields[b.Field])
	}

	if transition.Guard != nil {
		v, err := Evaluate(p, transition.Guard, scope)
		if err != nil {
			return engine.Commit{}, err
		}
		if !v.(engine.BoolValue).Value {
			return engine.Commit{}, ErrSignalRejected
		}
	}

	ctx := &execContext{
		program:            p,
		workflow:           workflow,
		global:             snapshot.GlobalState,
		local:              target.LocalState,
		random:             snapshot.Random,
		limits:             limits,
		questionSlots:      append([]engine.QuestionSlotInstance{}, target.QuestionSlots...),
		timerSlots:         append([]engine.TimerSlotInstance{}, target.TimerSlots...),
		askGroupSlots:      append([]engine.AskGroupSlotInstance{}, target.AskGroupSlots...),
		keyedQuestionSlots: append([]engine.KeyedQuestionSlotInstance{}, target.KeyedQuestionSlots...),
		keyedTimerSlots:    append([]engine.KeyedTimerSlotInstance{}, target.KeyedTimerSlots...),
		keyedAskGroupSlots: append([]engine.KeyedAskGroupSlotInstance{}, target.KeyedAskGroupSlots...),
	}

	// Accepting a validated answer, expiration, or ask-group completion
	// clears its slot, atomic with everything else this step does — if
	// the step fails for any other reason below, this candidate
	// clearing is discarded along with it, and the slot remains
	// occupied. The keyed variants clear only the one (slot, key) entry
	// that was accepted, leaving every other key's occurrence under the
	// same slot untouched.
	switch signal.Kind {
	case engine.SignalKindQuestionAnswered:
		if idx, ok := ctx.findQuestionSlot(signal.Slot); ok {
			ctx.questionSlots[idx] = engine.QuestionSlotInstance{Name: signal.Slot}
		}
	case engine.SignalKindTimerExpired:
		if idx, ok := ctx.findTimerSlot(signal.Slot); ok {
			ctx.timerSlots[idx] = engine.TimerSlotInstance{Name: signal.Slot}
		}
	case engine.SignalKindAskGroupCompleted:
		if idx, ok := ctx.findAskGroupSlot(signal.Slot); ok {
			ctx.askGroupSlots[idx] = engine.AskGroupSlotInstance{Name: signal.Slot}
		}
	case engine.SignalKindKeyedQuestionAnswered:
		if idx, ok := ctx.findKeyedQuestionSlot(signal.Slot); ok {
			if pIdx, ok := findKeyedQuestionPending(ctx.keyedQuestionSlots[idx].Pending, signal.Key); ok {
				ctx.keyedQuestionSlots[idx] = engine.KeyedQuestionSlotInstance{
					Name:    signal.Slot,
					Pending: removeKeyedQuestionPending(ctx.keyedQuestionSlots[idx].Pending, pIdx),
				}
			}
		}
	case engine.SignalKindKeyedTimerExpired:
		if idx, ok := ctx.findKeyedTimerSlot(signal.Slot); ok {
			if pIdx, ok := findKeyedTimerPending(ctx.keyedTimerSlots[idx].Pending, signal.Key); ok {
				pending := ctx.keyedTimerSlots[idx].Pending
				result := make([]engine.KeyedPendingTimer, 0, len(pending)-1)
				result = append(result, pending[:pIdx]...)
				result = append(result, pending[pIdx+1:]...)
				ctx.keyedTimerSlots[idx] = engine.KeyedTimerSlotInstance{Name: signal.Slot, Pending: result}
			}
		}
	case engine.SignalKindKeyedAskGroupCompleted:
		if idx, ok := ctx.findKeyedAskGroupSlot(signal.Slot); ok {
			if pIdx, ok := findKeyedAskGroupPending(ctx.keyedAskGroupSlots[idx].Pending, signal.Key); ok {
				pending := ctx.keyedAskGroupSlots[idx].Pending
				result := make([]engine.KeyedPendingAskGroup, 0, len(pending)-1)
				result = append(result, pending[:pIdx]...)
				result = append(result, pending[pIdx+1:]...)
				ctx.keyedAskGroupSlots[idx] = engine.KeyedAskGroupSlotInstance{Name: signal.Slot, Pending: result}
			}
		}
	}

	scope, err := execBlock(ctx, transition.Operations, scope)
	if err != nil {
		return engine.Commit{}, err
	}

	outcome, err := applyControl(p, transition.Control, scope)
	if err != nil {
		return engine.Commit{}, err
	}

	invariantScope := engine.Scope{Bindings: map[string]engine.Value{globalScopeRootName: ctx.global}}
	for _, inv := range p.Invariants {
		v, err := Evaluate(p, inv.Condition, invariantScope)
		if err != nil {
			return engine.Commit{}, err
		}
		if !v.(engine.BoolValue).Value {
			return engine.Commit{}, newExecutionError(ExecutionErrorInvariantViolation,
				"engineservice: invariant %q is violated after transition %q", inv.Name, transition.Name)
		}
	}

	newTarget := target
	newTarget.LocalState = ctx.local
	newTarget.QuestionSlots = ctx.questionSlots
	newTarget.TimerSlots = ctx.timerSlots
	newTarget.AskGroupSlots = ctx.askGroupSlots
	newTarget.KeyedQuestionSlots = ctx.keyedQuestionSlots
	newTarget.KeyedTimerSlots = ctx.keyedTimerSlots
	newTarget.KeyedAskGroupSlots = ctx.keyedAskGroupSlots
	if outcome.changed {
		newTarget.State = outcome.state
	}
	if outcome.outcome != nil {
		newTarget.Outcome = outcome.outcome
		// Once this instance reaches a terminal outcome, every ask-group
		// slot it owns — collecting or awaiting-join, ordinary or keyed
		// — is discarded: nothing can ever join a slot belonging to an
		// instance that no longer runs any transitions.
		newTarget.AskGroupSlots = clearedAskGroupSlots(newTarget.AskGroupSlots)
		newTarget.KeyedAskGroupSlots = clearedKeyedAskGroupSlots(newTarget.KeyedAskGroupSlots)

		ctx.outputs = append(ctx.outputs, engine.WorkflowCompletedOutput{
			Workflow: workflow.Name,
			Outcome:  *outcome.outcome,
		})
	}

	newRoot := newTarget

	// Per program.ProjectionDeclaration's documented "only a
	// successfully committed snapshot may ever be projected", active
	// presentations are recalculated only now, against the already
	// fully validated candidate state — never speculatively, and never
	// before invariants passed. A genesis WorkflowStarted delivery has
	// no "before": nothing about this instance was ever presented to a
	// client prior to its own first transition.
	var beforeActive []presentationEntry
	if !(signal.Kind == engine.SignalKindNamed && signal.Name == "WorkflowStarted") {
		beforeActive, err = deriveActivePresentations(p, workflow, target, snapshot.GlobalState)
		if err != nil {
			return engine.Commit{}, err
		}
	}
	afterActive, err := deriveActivePresentations(p, workflow, newTarget, ctx.global)
	if err != nil {
		return engine.Commit{}, err
	}
	ctx.outputs = append(ctx.outputs, diffPresentations(beforeActive, afterActive)...)

	commit := engine.Commit{
		Snapshot: engine.Snapshot{
			GlobalState: ctx.global,
			Root:        newRoot,
			Random:      ctx.random,
			Sequence:    snapshot.Sequence + 1,
		},
		InternalSignals: ctx.internalSignals,
		Outputs:         ctx.outputs,
		Trace: engine.Trace{
			Workflow:       workflow.Name,
			TransitionName: transition.Name,
			GuardEvaluated: transition.Guard != nil,
			GuardResult:    transition.Guard != nil,
			StateBefore:    target.State,
			StateAfter:     newTarget.State,
			Outcome:        newTarget.Outcome,
			OperationCount: ctx.opCount,
			Outputs:        ctx.outputs,
		},
		ConsumedSignal: signal,
	}
	return commit, nil
}

// ErrSignalRejected and ErrInputRejected are the two "stale signal"
// outcomes Step ever returns, and they mean structurally different
// things — a caller that wants to explain, log, or react differently
// to each should switch on Code (or use errors.Is against these
// sentinels) rather than treating "rejected" as one category:
//
//   - ErrSignalRejected means nothing in the instance's compiled
//     Workflow was ever willing to react to this signal at all — no
//     transition's SignalSource matched it, or the one that matched had
//     a false Guard. This is also the outcome for a signal delivered
//     after the one workflow instance has already terminated.
//   - ErrInputRejected means addressing AND matching both succeeded —
//     something was clearly willing to react to a signal of this
//     shape — but authoritative, kind-specific validation rejected the
//     concrete payload: a stale or duplicate answer/expiration/outcome
//     delivered to a slot already cleared, an unauthorized respondent,
//     or an answer failing its response type or Validation expression.
//
// Both are non-error, defined outcomes, not bugs: snapshot is
// unchanged, no Commit is produced, and retrying with the same
// (unchanged) inputs always reproduces the identical rejection — see
// LOGICAL_CONTRACT.md's determinism guarantee.

// ErrSignalRejected is returned by Step when signal has no applicable
// transition: none of the current state's transitions, or the
// workflow's GlobalTransitions, matched it, or the one that matched had
// a Guard that evaluated to false. See ExecutionErrorSignalRejected.
var ErrSignalRejected = &ExecutionError{
	Code:    ExecutionErrorSignalRejected,
	Message: "engineservice: signal was rejected: no transition matched, or its guard was false",
}

// ErrInputRejected is returned by Step for a SignalKindQuestionAnswered,
// SignalKindTimerExpired, SignalKindAskGroupCompleted, or any of their
// keyed counterparts that failed authoritative validation — see
// ExecutionErrorInputRejected. Because an accepted answer or expiration
// clears its slot (or (slot, key) occurrence) atomically with the rest
// of the step that handles it, a duplicate delivery of the same input
// always finds it already empty and is rejected here too — "stale" and
// "duplicate" are the same check.
var ErrInputRejected = &ExecutionError{
	Code:    ExecutionErrorInputRejected,
	Message: "engineservice: input was rejected: stale, duplicate, unauthorized, or invalid",
}

// validateQuestionAnswer implements program.QuestionAnsweredSignalSource's
// documented acceptance rule: the slot must currently hold a pending
// question, signal.Respondent must be that question's recipient, and
// signal.Answer must satisfy the question's ResponseType and, if
// present, its Validation expression.
//
// This does not detect a slot that was closed and reopened for an
// unrelated question between when a client sent its answer and when
// this runs — the pending question's identity is not tracked beyond
// "is this slot currently occupied, and by whom" — a known, narrow gap
// left for whenever a stronger identity is needed.
func validateQuestionAnswer(p engine.Program, instance engine.WorkflowInstance, signal engine.Signal) error {
	slot, ok := findInstanceQuestionSlot(instance, signal.Slot)
	if !ok || slot.Pending == nil {
		return ErrInputRejected
	}
	if slot.Pending.Recipient != signal.Respondent {
		return ErrInputRejected
	}

	workflow, ok := p.Workflows[instance.Workflow]
	if !ok {
		return newExecutionError(ExecutionErrorUnknown, "engineservice: workflow %q is not compiled", instance.Workflow)
	}
	slotDecl, ok := workflowQuestionSlot(workflow, signal.Slot)
	if !ok {
		return newExecutionError(ExecutionErrorUnknown, "engineservice: workflow %q has no question slot named %q", instance.Workflow, signal.Slot)
	}
	question, ok := p.Questions[slotDecl.Question]
	if !ok {
		return newExecutionError(ExecutionErrorUnknown, "engineservice: question %q is not compiled", slotDecl.Question)
	}

	if signal.Answer == nil || !signal.Answer.Validate(question.ResponseType) {
		return ErrInputRejected
	}
	if question.Validation != nil {
		bindings := map[string]engine.Value{"respondent": engine.UserValue{ID: signal.Respondent}, "answer": signal.Answer}
		for _, arg := range slot.Pending.Arguments {
			bindings[arg.Name] = arg.Value
		}
		v, err := Evaluate(p, question.Validation, engine.Scope{Bindings: bindings})
		if err != nil {
			return err
		}
		if !v.(engine.BoolValue).Value {
			return ErrInputRejected
		}
	}
	return nil
}

// validateTimerExpiration implements program.TimerExpiredSignalSource's
// documented acceptance rule: the slot must currently hold a pending
// timer. Per validateQuestionAnswer's doc comment, this has the same
// narrow "reopened slot" gap.
func validateTimerExpiration(instance engine.WorkflowInstance, signal engine.Signal) error {
	slot, ok := findInstanceTimerSlot(instance, signal.Slot)
	if !ok || !slot.Pending {
		return ErrInputRejected
	}
	return nil
}

func findInstanceQuestionSlot(instance engine.WorkflowInstance, name string) (engine.QuestionSlotInstance, bool) {
	for _, s := range instance.QuestionSlots {
		if s.Name == name {
			return s, true
		}
	}
	return engine.QuestionSlotInstance{}, false
}

func findInstanceTimerSlot(instance engine.WorkflowInstance, name string) (engine.TimerSlotInstance, bool) {
	for _, s := range instance.TimerSlots {
		if s.Name == name {
			return s, true
		}
	}
	return engine.TimerSlotInstance{}, false
}

// clearedAskGroupSlots returns a copy of slots with every entry emptied.
func clearedAskGroupSlots(slots []engine.AskGroupSlotInstance) []engine.AskGroupSlotInstance {
	cleared := make([]engine.AskGroupSlotInstance, len(slots))
	for i, s := range slots {
		cleared[i] = engine.AskGroupSlotInstance{Name: s.Name}
	}
	return cleared
}

// clearedKeyedAskGroupSlots returns a copy of slots with every (slot,
// key) occurrence discarded — the keyed generalization of
// clearedAskGroupSlots, applied when the owning instance reaches a
// terminal outcome.
func clearedKeyedAskGroupSlots(slots []engine.KeyedAskGroupSlotInstance) []engine.KeyedAskGroupSlotInstance {
	cleared := make([]engine.KeyedAskGroupSlotInstance, len(slots))
	for i, s := range slots {
		cleared[i] = engine.KeyedAskGroupSlotInstance{Name: s.Name}
	}
	return cleared
}

func workflowQuestionSlot(workflow engine.Workflow, name string) (engine.QuestionSlot, bool) {
	for _, s := range workflow.QuestionSlots {
		if s.Name == name {
			return s, true
		}
	}
	return engine.QuestionSlot{}, false
}

func workflowKeyedQuestionSlot(workflow engine.Workflow, name string) (engine.KeyedQuestionSlot, bool) {
	for _, s := range workflow.KeyedQuestionSlots {
		if s.Name == name {
			return s, true
		}
	}
	return engine.KeyedQuestionSlot{}, false
}

func workflowKeyedAskGroupSlot(workflow engine.Workflow, name string) (engine.KeyedAskGroupSlot, bool) {
	for _, s := range workflow.KeyedAskGroupSlots {
		if s.Name == name {
			return s, true
		}
	}
	return engine.KeyedAskGroupSlot{}, false
}

// findInstanceKeyedQuestionSlot returns instance's keyed question slot
// named name, if any.
func findInstanceKeyedQuestionSlot(instance engine.WorkflowInstance, name string) (engine.KeyedQuestionSlotInstance, bool) {
	for _, s := range instance.KeyedQuestionSlots {
		if s.Name == name {
			return s, true
		}
	}
	return engine.KeyedQuestionSlotInstance{}, false
}

// findInstanceKeyedTimerSlot returns instance's keyed timer slot named
// name, if any.
func findInstanceKeyedTimerSlot(instance engine.WorkflowInstance, name string) (engine.KeyedTimerSlotInstance, bool) {
	for _, s := range instance.KeyedTimerSlots {
		if s.Name == name {
			return s, true
		}
	}
	return engine.KeyedTimerSlotInstance{}, false
}

// findInstanceKeyedAskGroupSlot returns instance's keyed ask-group slot
// named name, if any.
func findInstanceKeyedAskGroupSlot(instance engine.WorkflowInstance, name string) (engine.KeyedAskGroupSlotInstance, bool) {
	for _, s := range instance.KeyedAskGroupSlots {
		if s.Name == name {
			return s, true
		}
	}
	return engine.KeyedAskGroupSlotInstance{}, false
}

func findInstanceKeyedAskGroupSlotIndex(instance engine.WorkflowInstance, name string) (int, bool) {
	for i, s := range instance.KeyedAskGroupSlots {
		if s.Name == name {
			return i, true
		}
	}
	return 0, false
}

// validateKeyedQuestionAnswer implements
// program.KeyedQuestionAnsweredSignalSource's documented acceptance
// rule, scoped to the specific (slot, key) occurrence signal.Key
// addresses — the keyed generalization of validateQuestionAnswer, with
// the identical narrow "reopened (slot, key)" gap.
func validateKeyedQuestionAnswer(p engine.Program, instance engine.WorkflowInstance, signal engine.Signal) error {
	slot, ok := findInstanceKeyedQuestionSlot(instance, signal.Slot)
	if !ok {
		return ErrInputRejected
	}
	pIdx, ok := findKeyedQuestionPending(slot.Pending, signal.Key)
	if !ok {
		return ErrInputRejected
	}
	pending := slot.Pending[pIdx]
	if pending.Recipient != signal.Respondent {
		return ErrInputRejected
	}

	workflow, ok := p.Workflows[instance.Workflow]
	if !ok {
		return newExecutionError(ExecutionErrorUnknown, "engineservice: workflow %q is not compiled", instance.Workflow)
	}
	slotDecl, ok := workflowKeyedQuestionSlot(workflow, signal.Slot)
	if !ok {
		return newExecutionError(ExecutionErrorUnknown, "engineservice: workflow %q has no keyed question slot named %q", instance.Workflow, signal.Slot)
	}
	question, ok := p.Questions[slotDecl.Question]
	if !ok {
		return newExecutionError(ExecutionErrorUnknown, "engineservice: question %q is not compiled", slotDecl.Question)
	}

	if signal.Answer == nil || !signal.Answer.Validate(question.ResponseType) {
		return ErrInputRejected
	}
	if question.Validation != nil {
		bindings := map[string]engine.Value{"respondent": engine.UserValue{ID: signal.Respondent}, "answer": signal.Answer}
		for _, arg := range pending.Arguments {
			bindings[arg.Name] = arg.Value
		}
		v, err := Evaluate(p, question.Validation, engine.Scope{Bindings: bindings})
		if err != nil {
			return err
		}
		if !v.(engine.BoolValue).Value {
			return ErrInputRejected
		}
	}
	return nil
}

// validateKeyedTimerExpiration implements
// program.KeyedTimerExpiredSignalSource's documented acceptance rule,
// scoped to the specific (slot, key) occurrence signal.Key addresses —
// the keyed generalization of validateTimerExpiration.
func validateKeyedTimerExpiration(instance engine.WorkflowInstance, signal engine.Signal) error {
	slot, ok := findInstanceKeyedTimerSlot(instance, signal.Slot)
	if !ok {
		return ErrInputRejected
	}
	if _, ok := findKeyedTimerPending(slot.Pending, signal.Key); !ok {
		return ErrInputRejected
	}
	return nil
}

// validateKeyedAskGroupCompletion implements
// program.KeyedAskGroupCompletedSignalSource's acceptance rule, scoped
// to the specific (slot, key) occurrence signal.Key addresses — the
// keyed generalization of validateAskGroupCompletion.
func validateKeyedAskGroupCompletion(instance engine.WorkflowInstance, signal engine.Signal) error {
	slot, ok := findInstanceKeyedAskGroupSlot(instance, signal.Slot)
	if !ok {
		return ErrInputRejected
	}
	pIdx, ok := findKeyedAskGroupPending(slot.Pending, signal.Key)
	if !ok || !slot.Pending[pIdx].Completed {
		return ErrInputRejected
	}
	return nil
}

// signalSchemaFields builds the field-name-to-value map a Signal's
// schema exposes for binding — see engineservice's compileSignalSource
// for the matching compile-time schema each SignalKind resolves to. An
// ask-group-completion signal carries no payload of its own; its fields
// come from instance's own slot, read before Step clears it.
func signalSchemaFields(p engine.Program, workflow engine.Workflow, instance engine.WorkflowInstance, signal engine.Signal) map[string]engine.Value {
	switch signal.Kind {
	case engine.SignalKindIntent:
		fields := make(map[string]engine.Value, len(signal.Fields)+1)
		for k, v := range signal.Fields {
			fields[k] = v
		}
		fields["actor"] = engine.UserValue{ID: signal.Actor}
		return fields
	case engine.SignalKindQuestionAnswered:
		return map[string]engine.Value{"respondent": engine.UserValue{ID: signal.Respondent}, "answer": signal.Answer}
	case engine.SignalKindAskGroupCompleted:
		slot, _ := findInstanceAskGroupSlot(instance, signal.Slot)
		return askGroupCompletionFields(p, workflow, slot.Pending, signal.Slot)
	case engine.SignalKindKeyedQuestionAnswered:
		return map[string]engine.Value{"key": signal.Key, "respondent": engine.UserValue{ID: signal.Respondent}, "answer": signal.Answer}
	case engine.SignalKindKeyedTimerExpired:
		return map[string]engine.Value{"key": signal.Key}
	case engine.SignalKindKeyedAskGroupCompleted:
		slot, _ := findInstanceKeyedAskGroupSlot(instance, signal.Slot)
		var pending *engine.PendingAskGroup
		if pIdx, ok := findKeyedAskGroupPending(slot.Pending, signal.Key); ok {
			pending = &slot.Pending[pIdx].PendingAskGroup
		}
		fields := askGroupCompletionFields(p, workflow, pending, signal.Slot)
		fields["key"] = signal.Key
		return fields
	default:
		return signal.Fields
	}
}

// workflowStateByName returns workflow's state named name, if any.
func workflowStateByName(workflow engine.Workflow, name string) (engine.WorkflowState, bool) {
	for _, s := range workflow.States {
		if s.Name == name {
			return s, true
		}
	}
	return engine.WorkflowState{}, false
}

// selectTransition implements state-local transition precedence with
// global-transition fallback: it returns the current state's own
// transition for signal if one exists, and only otherwise falls back to
// checking workflow.GlobalTransitions.
func selectTransition(state engine.WorkflowState, workflow engine.Workflow, signal engine.Signal) (engine.Transition, bool) {
	for _, t := range state.Transitions {
		if signalMatchesSource(t.Signal.Source, signal) {
			return t, true
		}
	}
	for _, t := range workflow.GlobalTransitions {
		if signalMatchesSource(t.Signal.Source, signal) {
			return t, true
		}
	}
	return engine.Transition{}, false
}

// signalMatchesSource reports whether signal is what source matches.
func signalMatchesSource(source engine.SignalSource, signal engine.Signal) bool {
	switch s := source.(type) {
	case engine.NamedSignalSource:
		return signal.Kind == engine.SignalKindNamed && s.Name == signal.Name
	case engine.UserIntentSignalSource:
		return signal.Kind == engine.SignalKindIntent && s.Intent == signal.Intent
	case engine.QuestionAnsweredSignalSource:
		return signal.Kind == engine.SignalKindQuestionAnswered && s.Slot == signal.Slot
	case engine.TimerExpiredSignalSource:
		return signal.Kind == engine.SignalKindTimerExpired && s.Slot == signal.Slot
	case engine.AskGroupCompletedSignalSource:
		return signal.Kind == engine.SignalKindAskGroupCompleted && s.Slot == signal.Slot
	case engine.KeyedQuestionAnsweredSignalSource:
		return signal.Kind == engine.SignalKindKeyedQuestionAnswered && s.Slot == signal.Slot
	case engine.KeyedTimerExpiredSignalSource:
		return signal.Kind == engine.SignalKindKeyedTimerExpired && s.Slot == signal.Slot
	case engine.KeyedAskGroupCompletedSignalSource:
		return signal.Kind == engine.SignalKindKeyedAskGroupCompleted && s.Slot == signal.Slot
	default:
		return false
	}
}

// instanceBaseScope builds the scope a transition's Guard, Operations,
// and Control evaluate in: the instance's own Parameters bound directly
// by name, plus the reserved "local" and "global" roots. "resources" is
// added automatically by Evaluate — see evaluate.go's withResources.
func instanceBaseScope(instance engine.WorkflowInstance, global engine.RecordValue) engine.Scope {
	bindings := make(map[string]engine.Value, len(instance.Parameters)+2)
	for _, p := range instance.Parameters {
		bindings[p.Name] = p.Value
	}
	bindings["local"] = instance.LocalState
	bindings[globalScopeRootName] = global
	return engine.Scope{Bindings: bindings}
}
