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

	// ExecutionErrorSlotOccupied marks an OpenQuestionOperation or
	// ScheduleTimerOperation targeting a slot that already holds a
	// pending question or timer.
	ExecutionErrorSlotOccupied

	// ExecutionErrorInvalidTimerDelay marks a ScheduleTimerOperation
	// whose evaluated DelayMilliseconds is not a finite, non-negative
	// integer.
	ExecutionErrorInvalidTimerDelay

	// ExecutionErrorInputRejected marks a Step call for a
	// SignalKindQuestionAnswered, SignalKindTimerExpired, or
	// child-outcome Signal that did not pass authoritative validation:
	// the targeted slot was already empty (stale or duplicate), the
	// answer's respondent did not match the slot's pending recipient
	// (unauthorized), the submitted answer failed response-type or
	// Validation checks (invalid), or a child-outcome signal's slot did
	// not hold a matching terminal outcome awaiting join. See
	// ErrInputRejected.
	ExecutionErrorInputRejected

	// ExecutionErrorChildOutcomeNotJoined marks a
	// CancelChildWorkflowOperation targeting a child slot that holds a
	// terminal outcome still awaiting join — that outcome must be
	// consumed through its corresponding child-outcome signal first,
	// never silently discarded by cancellation.
	ExecutionErrorChildOutcomeNotJoined

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

	// ExecutionErrorDuplicateTaskKey marks a SpawnTaskGroupChildOperation
	// whose evaluated Key is already used by another task in the same,
	// currently building task group.
	ExecutionErrorDuplicateTaskKey

	// ExecutionErrorTaskGroupNotJoined marks a CancelTaskGroupOperation
	// targeting a task-group slot that holds a terminal outcome still
	// awaiting join — that outcome must be consumed through
	// TaskGroupCompletedSignalSource first, never silently discarded by
	// cancellation.
	ExecutionErrorTaskGroupNotJoined

	// ExecutionErrorTaskGroupLeftBuilding marks a Step call whose
	// transition began a task group (BeginTaskGroupOperation) but ended
	// without sealing or cancelling it — per
	// program.BeginTaskGroupOperation's documented rule, a task group
	// must never remain in the building phase once its opening
	// transition commits.
	ExecutionErrorTaskGroupLeftBuilding
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

func newChildWorkflowSlotInstances(slots []engine.ChildWorkflowSlot) []engine.ChildWorkflowSlotInstance {
	result := make([]engine.ChildWorkflowSlotInstance, len(slots))
	for i, s := range slots {
		result[i] = engine.ChildWorkflowSlotInstance{Name: s.Name}
	}
	return result
}

// newChildInstance creates the initial WorkflowInstance for one child
// spawned by a SpawnChildWorkflowOperation: it binds workflow's declared
// Parameters from args by name, evaluates its LocalState, creates its
// declared runtime slots (all empty), and places it in its declared
// InitialState — mirroring NewSnapshot's construction of the root
// instance. The compiler already guarantees args matches workflow's
// Parameters exactly (see compileSpawnChildWorkflow), so, unlike
// bindParameters, this does not re-validate each value's type.
func newChildInstance(p engine.Program, workflow engine.Workflow, args []engine.FieldValue) (engine.WorkflowInstance, error) {
	argMap := fieldValueMap(args)
	params := make([]engine.FieldValue, 0, len(workflow.Parameters))
	for _, decl := range workflow.Parameters {
		v, ok := argMap[decl.Name]
		if !ok {
			return engine.WorkflowInstance{}, newExecutionError(ExecutionErrorUnknown,
				"engineservice: missing child workflow argument %q", decl.Name)
		}
		params = append(params, engine.FieldValue{Name: decl.Name, Value: v})
	}

	localFields, err := evaluateStateFields(p, workflow.LocalState, engine.Scope{Bindings: argMap})
	if err != nil {
		return engine.WorkflowInstance{}, err
	}

	return engine.WorkflowInstance{
		Workflow:       workflow.Name,
		State:          workflow.InitialState,
		Parameters:     params,
		LocalState:     engine.RecordValue{TypeName: "local", Fields: localFields},
		QuestionSlots:  newQuestionSlotInstances(workflow.QuestionSlots),
		AskGroupSlots:  newAskGroupSlotInstances(workflow.AskGroupSlots),
		TimerSlots:     newTimerSlotInstances(workflow.TimerSlots),
		ChildSlots:     newChildWorkflowSlotInstances(workflow.ChildSlots),
		TaskGroupSlots: newTaskGroupSlotInstances(workflow.TaskGroupSlots),
	}, nil
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
//
// signal.Path addresses which workflow instance in the child-workflow
// tree this call targets — see engine.Signal's doc comment. This version
// does not yet produce any engine.Trace content.
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
	// from the transition-selection flow below.
	if signal.Kind == engine.SignalKindAskGroupAnswered {
		return stepAskGroupAnswer(p, snapshot, signal)
	}

	target, ok := resolveInstance(snapshot.Root, signal.Path)
	if !ok || target.Outcome != nil {
		return engine.Commit{}, ErrSignalRejected
	}

	workflow, ok := p.Workflows[target.Workflow]
	if !ok {
		return engine.Commit{}, newExecutionError(ExecutionErrorUnknown, "engineservice: workflow %q is not compiled", target.Workflow)
	}

	// Per program.QuestionAnsweredSignalSource, program.TimerExpiredSignalSource,
	// program.ChildCompletedSignalSource, program.ChildFailedSignalSource, and
	// program.ChildCancelledSignalSource, a signal of these kinds only
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
	case engine.SignalKindChildCompleted:
		if err := validateChildOutcome(target, signal, engine.WorkflowOutcomeCompleted); err != nil {
			return engine.Commit{}, err
		}
	case engine.SignalKindChildFailed:
		if err := validateChildOutcome(target, signal, engine.WorkflowOutcomeFailed); err != nil {
			return engine.Commit{}, err
		}
	case engine.SignalKindChildCancelled:
		if err := validateChildOutcome(target, signal, engine.WorkflowOutcomeCancelled); err != nil {
			return engine.Commit{}, err
		}
	case engine.SignalKindAskGroupCompleted:
		if err := validateAskGroupCompletion(target, signal); err != nil {
			return engine.Commit{}, err
		}
	case engine.SignalKindTaskGroupCompleted:
		if err := validateTaskGroupCompletion(target, signal); err != nil {
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
		program:        p,
		workflow:       workflow,
		path:           signal.Path,
		global:         snapshot.GlobalState,
		local:          target.LocalState,
		random:         snapshot.Random,
		limits:         limits,
		questionSlots:  append([]engine.QuestionSlotInstance{}, target.QuestionSlots...),
		timerSlots:     append([]engine.TimerSlotInstance{}, target.TimerSlots...),
		childSlots:     append([]engine.ChildWorkflowSlotInstance{}, target.ChildSlots...),
		askGroupSlots:  append([]engine.AskGroupSlotInstance{}, target.AskGroupSlots...),
		taskGroupSlots: append([]engine.TaskGroupSlotInstance{}, target.TaskGroupSlots...),
	}

	// Accepting a validated answer, expiration, child outcome, or
	// ask-group completion clears its slot, atomic with everything else
	// this step does — if the step fails for any other reason below,
	// this candidate clearing is discarded along with it, and the slot
	// remains occupied.
	switch signal.Kind {
	case engine.SignalKindQuestionAnswered:
		if idx, ok := ctx.findQuestionSlot(signal.Slot); ok {
			ctx.questionSlots[idx] = engine.QuestionSlotInstance{Name: signal.Slot}
		}
	case engine.SignalKindTimerExpired:
		if idx, ok := ctx.findTimerSlot(signal.Slot); ok {
			ctx.timerSlots[idx] = engine.TimerSlotInstance{Name: signal.Slot}
		}
	case engine.SignalKindChildCompleted, engine.SignalKindChildFailed, engine.SignalKindChildCancelled:
		if idx, ok := ctx.findChildSlot(signal.Slot); ok {
			ctx.childSlots[idx] = engine.ChildWorkflowSlotInstance{Name: signal.Slot}
		}
	case engine.SignalKindAskGroupCompleted:
		if idx, ok := ctx.findAskGroupSlot(signal.Slot); ok {
			ctx.askGroupSlots[idx] = engine.AskGroupSlotInstance{Name: signal.Slot}
		}
	case engine.SignalKindTaskGroupCompleted:
		if idx, ok := ctx.findTaskGroupSlot(signal.Slot); ok {
			ctx.taskGroupSlots[idx] = engine.TaskGroupSlotInstance{Name: signal.Slot}
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

	// Per program.BeginTaskGroupOperation's documented rule, a task
	// group must never remain in the building phase once its opening
	// transition commits — every BeginTaskGroupOperation must be
	// followed, within the same transition, by a SealTaskGroupOperation
	// or a CancelTaskGroupOperation.
	for _, s := range ctx.taskGroupSlots {
		if s.Group != nil && s.Group.Phase == engine.TaskGroupPhaseBuilding {
			return engine.Commit{}, newExecutionError(ExecutionErrorTaskGroupLeftBuilding,
				"engineservice: task-group slot %q must be sealed or cancelled before its opening transition ends", s.Name)
		}
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
	newTarget.ChildSlots = ctx.childSlots
	newTarget.AskGroupSlots = ctx.askGroupSlots
	newTarget.TaskGroupSlots = ctx.taskGroupSlots
	if outcome.changed {
		newTarget.State = outcome.state
	}
	if outcome.outcome != nil {
		newTarget.Outcome = outcome.outcome
		// Per ChildWorkflowSlotDeclaration's documented "disappears when
		// the parent workflow terminates": once this instance itself
		// reaches a terminal outcome, every child slot, ask-group slot,
		// and task-group slot it owns — running or terminal-awaiting-join
		// — is discarded along with its entire subtree. Nothing can ever
		// join a slot belonging to an instance that no longer runs any
		// transitions.
		newTarget.ChildSlots = clearedChildSlots(newTarget.ChildSlots)
		newTarget.AskGroupSlots = clearedAskGroupSlots(newTarget.AskGroupSlots)
		newTarget.TaskGroupSlots = clearedTaskGroupSlots(newTarget.TaskGroupSlots)
	}

	newRoot, err := applyInstancePath(snapshot.Root, signal.Path, newTarget)
	if err != nil {
		return engine.Commit{}, err
	}

	commit := engine.Commit{
		Snapshot: engine.Snapshot{
			GlobalState: ctx.global,
			Root:        newRoot,
			Random:      ctx.random,
			Sequence:    snapshot.Sequence + 1,
		},
		InternalSignals: ctx.internalSignals,
		Outputs:         ctx.outputs,
		ConsumedSignal:  signal,
	}
	return commit, nil
}

// ErrSignalRejected is returned by Step when signal has no applicable
// transition: none of the current state's transitions, or the
// workflow's GlobalTransitions, matched it, or the one that matched had
// a Guard that evaluated to false. See ExecutionErrorSignalRejected.
var ErrSignalRejected = &ExecutionError{
	Code:    ExecutionErrorSignalRejected,
	Message: "engineservice: signal was rejected: no transition matched, or its guard was false",
}

// ErrInputRejected is returned by Step for a SignalKindQuestionAnswered
// or SignalKindTimerExpired Signal that failed authoritative
// validation — see ExecutionErrorInputRejected. Because an accepted
// answer or expiration clears its slot atomically with the rest of the
// step that handles it, a duplicate delivery of the same input always
// finds the slot already empty and is rejected here too — "stale" and
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

// validateChildOutcome implements program.ChildCompletedSignalSource's,
// program.ChildFailedSignalSource's, and program.ChildCancelledSignalSource's
// shared acceptance rule: the named child slot on instance must
// currently hold a terminal outcome of exactly the kind want, awaiting
// join. Per validateQuestionAnswer's doc comment, this has the same
// narrow "reopened slot" gap; it also doubles as the "duplicate
// delivery after joining" check, since an accepted child-outcome signal
// clears its slot atomically with the rest of the step that handles it.
func validateChildOutcome(instance engine.WorkflowInstance, signal engine.Signal, want engine.WorkflowOutcomeKind) error {
	slot, ok := findInstanceChildSlot(instance, signal.Slot)
	if !ok || slot.Child == nil || slot.Child.Outcome == nil || slot.Child.Outcome.Kind != want {
		return ErrInputRejected
	}
	return nil
}

func findInstanceChildSlot(instance engine.WorkflowInstance, name string) (engine.ChildWorkflowSlotInstance, bool) {
	for _, s := range instance.ChildSlots {
		if s.Name == name {
			return s, true
		}
	}
	return engine.ChildWorkflowSlotInstance{}, false
}

// resolveInstance walks path from root, following each PathStep's
// ChildSlot or TaskGroupSlot by name, and returns a copy of the
// workflow instance it addresses — see engine.Signal.Path. An empty
// path returns root itself. resolveInstance fails if any step names an
// undeclared or empty slot, an unknown task key, or a task inside a
// task group that has already completed — a task-group task is never
// individually addressable once its owning group is
// completed-awaiting-join, whether or not that specific task reached
// its own authored terminal outcome (see TaskGroupPhaseCompleted).
func resolveInstance(root engine.WorkflowInstance, path []engine.PathStep) (engine.WorkflowInstance, bool) {
	current := root
	for _, step := range path {
		if step.TaskKey == nil {
			slot, ok := findInstanceChildSlot(current, step.Slot)
			if !ok || slot.Child == nil {
				return engine.WorkflowInstance{}, false
			}
			current = *slot.Child
			continue
		}
		slot, ok := findInstanceTaskGroupSlot(current, step.Slot)
		if !ok || slot.Group == nil || slot.Group.Phase == engine.TaskGroupPhaseCompleted {
			return engine.WorkflowInstance{}, false
		}
		task, ok := findGroupTask(*slot.Group, step.TaskKey)
		if !ok {
			return engine.WorkflowInstance{}, false
		}
		current = task.Child
	}
	return current, true
}

// applyInstancePath reconstructs root with the instance at path replaced
// by updated, copying only the WorkflowInstance, ChildWorkflowSlotInstance,
// and TaskGroupState values along that path — mirroring applyPath's
// copy-on-write discipline for global/local state, but for the
// child-workflow tree. An empty path replaces root itself.
//
// When the final PathStep addresses a task-group task (TaskKey
// non-nil) and updated has just reached a terminal outcome, this also
// performs the owning TaskGroupState's aggregation bookkeeping in the
// same pass: appending the task's Key to TerminalOrder and, if that
// newly satisfies the group's completion policy, marking it
// TaskGroupPhaseCompleted — see taskGroupPolicySatisfied. This is the
// only place a task's terminal outcome and its group's aggregation ever
// update together, atomically, since no per-task signal ever exists for
// a caller to drive that update separately (see program.SpawnTaskGroupChildOperation).
func applyInstancePath(root engine.WorkflowInstance, path []engine.PathStep, updated engine.WorkflowInstance) (engine.WorkflowInstance, error) {
	if len(path) == 0 {
		return updated, nil
	}
	step := path[0]

	if step.TaskKey == nil {
		idx, ok := findChildSlotIndex(root.ChildSlots, step.Slot)
		if !ok || root.ChildSlots[idx].Child == nil {
			return engine.WorkflowInstance{}, newExecutionError(ExecutionErrorUnknown, "engineservice: no child instance in slot %q", step.Slot)
		}
		updatedChild, err := applyInstancePath(*root.ChildSlots[idx].Child, path[1:], updated)
		if err != nil {
			return engine.WorkflowInstance{}, err
		}
		newSlots := make([]engine.ChildWorkflowSlotInstance, len(root.ChildSlots))
		copy(newSlots, root.ChildSlots)
		newSlots[idx] = engine.ChildWorkflowSlotInstance{Name: step.Slot, Child: &updatedChild}
		root.ChildSlots = newSlots
		return root, nil
	}

	slotIdx, ok := findTaskGroupSlotIndex(root.TaskGroupSlots, step.Slot)
	if !ok || root.TaskGroupSlots[slotIdx].Group == nil {
		return engine.WorkflowInstance{}, newExecutionError(ExecutionErrorUnknown, "engineservice: no task group in slot %q", step.Slot)
	}
	group := root.TaskGroupSlots[slotIdx].Group
	taskIdx, ok := findGroupTaskIndex(*group, step.TaskKey)
	if !ok {
		return engine.WorkflowInstance{}, newExecutionError(ExecutionErrorUnknown, "engineservice: no task for the given key in slot %q", step.Slot)
	}

	updatedChild, err := applyInstancePath(group.Tasks[taskIdx].Child, path[1:], updated)
	if err != nil {
		return engine.WorkflowInstance{}, err
	}

	newGroup := *group
	newTasks := make([]engine.TaskGroupTask, len(group.Tasks))
	copy(newTasks, group.Tasks)
	newTasks[taskIdx] = engine.TaskGroupTask{Key: step.TaskKey, Child: updatedChild}
	newGroup.Tasks = newTasks

	if len(path) == 1 && updatedChild.Outcome != nil && !containsValue(newGroup.TerminalOrder, step.TaskKey) {
		newGroup.TerminalOrder = append(append([]engine.Value{}, newGroup.TerminalOrder...), step.TaskKey)
		if newGroup.Phase != engine.TaskGroupPhaseCompleted && taskGroupPolicySatisfied(newGroup) {
			newGroup.Phase = engine.TaskGroupPhaseCompleted
		}
	}

	newSlots := make([]engine.TaskGroupSlotInstance, len(root.TaskGroupSlots))
	copy(newSlots, root.TaskGroupSlots)
	newSlots[slotIdx] = engine.TaskGroupSlotInstance{Name: step.Slot, Group: &newGroup}
	root.TaskGroupSlots = newSlots
	return root, nil
}

func findChildSlotIndex(slots []engine.ChildWorkflowSlotInstance, name string) (int, bool) {
	for i, s := range slots {
		if s.Name == name {
			return i, true
		}
	}
	return 0, false
}

// clearedChildSlots returns a copy of slots with every entry emptied —
// see ChildWorkflowSlotDeclaration's documented "disappears when the
// parent workflow terminates".
func clearedChildSlots(slots []engine.ChildWorkflowSlotInstance) []engine.ChildWorkflowSlotInstance {
	cleared := make([]engine.ChildWorkflowSlotInstance, len(slots))
	for i, s := range slots {
		cleared[i] = engine.ChildWorkflowSlotInstance{Name: s.Name}
	}
	return cleared
}

func clearedAskGroupSlots(slots []engine.AskGroupSlotInstance) []engine.AskGroupSlotInstance {
	cleared := make([]engine.AskGroupSlotInstance, len(slots))
	for i, s := range slots {
		cleared[i] = engine.AskGroupSlotInstance{Name: s.Name}
	}
	return cleared
}

func clearedTaskGroupSlots(slots []engine.TaskGroupSlotInstance) []engine.TaskGroupSlotInstance {
	cleared := make([]engine.TaskGroupSlotInstance, len(slots))
	for i, s := range slots {
		cleared[i] = engine.TaskGroupSlotInstance{Name: s.Name}
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

// signalSchemaFields builds the field-name-to-value map a Signal's
// schema exposes for binding — see engineservice's compileSignalSource
// for the matching compile-time schema each SignalKind resolves to. A
// child-outcome or ask-group-completion signal carries no payload of its
// own; its fields come from instance's own slot, read before Step clears
// it.
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
	case engine.SignalKindChildCompleted:
		slot, _ := findInstanceChildSlot(instance, signal.Slot)
		return map[string]engine.Value{"result": slot.Child.Outcome.Result}
	case engine.SignalKindChildFailed:
		slot, _ := findInstanceChildSlot(instance, signal.Slot)
		return map[string]engine.Value{"error": engine.StringValue{Value: slot.Child.Outcome.Error}}
	case engine.SignalKindChildCancelled:
		slot, _ := findInstanceChildSlot(instance, signal.Slot)
		return map[string]engine.Value{"reason": engine.StringValue{Value: slot.Child.Outcome.Reason}}
	case engine.SignalKindAskGroupCompleted:
		slot, _ := findInstanceAskGroupSlot(instance, signal.Slot)
		return askGroupCompletionFields(p, workflow, slot.Pending, signal.Slot)
	case engine.SignalKindTaskGroupCompleted:
		slot, _ := findInstanceTaskGroupSlot(instance, signal.Slot)
		return taskGroupCompletionFields(p, workflow, slot.Group, signal.Slot)
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
	case engine.ChildCompletedSignalSource:
		return signal.Kind == engine.SignalKindChildCompleted && s.Slot == signal.Slot
	case engine.ChildFailedSignalSource:
		return signal.Kind == engine.SignalKindChildFailed && s.Slot == signal.Slot
	case engine.ChildCancelledSignalSource:
		return signal.Kind == engine.SignalKindChildCancelled && s.Slot == signal.Slot
	case engine.AskGroupCompletedSignalSource:
		return signal.Kind == engine.SignalKindAskGroupCompleted && s.Slot == signal.Slot
	case engine.TaskGroupCompletedSignalSource:
		return signal.Kind == engine.SignalKindTaskGroupCompleted && s.Slot == signal.Slot
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
