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

// FailControl terminates the workflow instance as failed, with Error
// describing the failure. The future compiler defines and validates the
// allowed error-value types.
type FailControl struct {
	Error Expression
}

func (FailControl) isWorkflowControl() {}

// CancelControl terminates the workflow instance as cancelled, with Reason
// describing the cancellation. Cancellation is a distinct outcome from
// both failure and successful completion.
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
