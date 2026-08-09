package program

// WorkflowControl is the single result of a workflow transition, evaluated
// after the transition's synchronous operation block has finished
// executing.
//
// A transition changes a workflow instance's state only through a
// WorkflowControl result; the synchronous operation block that precedes it
// cannot directly choose the next state. Expressions inside a
// WorkflowControl observe the transition's working state, including any
// mutations made by earlier operations in the same transition.
//
// WorkflowControl is a closed interface. Its marker method is unexported
// so that packages outside program cannot introduce unsupported variants;
// the future compiler can safely exhaust all cases with a type switch.
type WorkflowControl interface {
	isWorkflowControl()
}

// GotoControl moves the workflow instance to another declared state.
//
// The target state is a static, explicitly named state; the language does
// not support computing a state name from an expression. GotoControl only
// changes the stored current state — it does not itself execute any
// transition of the target state. A subsequent transition still requires
// another signal.
type GotoControl struct {
	State string
}

func (GotoControl) isWorkflowControl() {}

// StayControl keeps the workflow instance in its current state. It is an
// explicit self-transition result, distinct from omitting a control
// result.
type StayControl struct{}

func (StayControl) isWorkflowControl() {}

// CompleteControl completes the workflow instance successfully, producing
// Result as the workflow's typed result.
//
// A workflow whose declared result type is the unit built-in must still
// provide an explicit result using UnitLiteralExpression{}; nil is not a
// valid language representation of "no result".
type CompleteControl struct {
	Result Expression
}

func (CompleteControl) isWorkflowControl() {}

// FailControl terminates the workflow instance in the authored failed
// outcome, with Error describing the failure. Error must eventually
// compile to the built-in string type; a nil Error may exist in a
// partially constructed, invalid source object, but the future compiler
// must reject it.
//
// FailControl is for explicit, authored process failure — it is not the
// same thing as an engine or transition execution error (such as an
// exceeded execution budget, an invalid runtime index, an occupied-slot
// error, or an invariant violation). An execution error never runs
// FailControl: the offending transition fails atomically, the workflow
// instance remains at its previously committed state, and no authored
// failure outcome is produced. A game that needs a typed business outcome
// (for example Won, NoValidMove, or ParticipantUnavailable) should
// normally represent it as a successful result via a user-declared enum or
// union through CompleteControl instead of overloading FailControl.
//
// When the failing workflow is a child, its parent observes this outcome
// through ChildFailedSignalSource. When it is the root workflow, there is
// no parent to notify; a future session layer may observe the root's
// terminal outcome.
type FailControl struct {
	Error Expression
}

func (FailControl) isWorkflowControl() {}

// CancelControl terminates the workflow instance in the authored cancelled
// outcome, with Reason describing the cancellation. Reason must eventually
// compile to the built-in string type; a nil Reason may exist in a
// partially constructed, invalid source object, but the future compiler
// must reject it. Cancellation is a distinct outcome from both authored
// failure and successful completion.
//
// CancelControl models a workflow cancelling itself; it is unrelated to a
// parent cancelling a running child with CancelChildWorkflowOperation,
// which never produces a child-outcome signal.
//
// When the cancelling workflow is a child, its parent observes this
// outcome through ChildCancelledSignalSource. When it is the root
// workflow, there is no parent to notify; a future session layer may
// observe the root's terminal outcome.
type CancelControl struct {
	Reason Expression
}

func (CancelControl) isWorkflowControl() {}

// ConditionalControl selects one of two workflow-control outcomes based on
// Condition, which must eventually compile to a boolean value.
//
// ConditionalControl may be nested to represent more complex routing. It
// controls the workflow's outcome after the transition's operation block
// has finished executing; it is the workflow-control counterpart to
// IfOperation, which instead controls synchronous operations.
type ConditionalControl struct {
	Condition Expression
	Then      WorkflowControl
	Else      WorkflowControl
}

func (ConditionalControl) isWorkflowControl() {}
