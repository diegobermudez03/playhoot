package engine

// WorkflowControl is the compiled representation of one
// program.WorkflowControl: the single result of a workflow transition,
// evaluated after (a future step's) operation block finishes.
//
// WorkflowControl is a closed interface, mirroring program's own
// closed-interface pattern.
type WorkflowControl interface {
	isWorkflowControl()
}

// GotoControl moves the workflow instance to the declared state named
// State. The compiler guarantees State names a state that exists in the
// same workflow.
type GotoControl struct {
	State string
}

func (GotoControl) isWorkflowControl() {}

// StayControl keeps the workflow instance in its current state.
type StayControl struct{}

func (StayControl) isWorkflowControl() {}

// CompleteControl completes the workflow instance successfully,
// producing Result. The compiler guarantees Result statically matches
// the workflow's declared ResultType.
type CompleteControl struct {
	Result Expression
}

func (CompleteControl) isWorkflowControl() {}

// FailControl terminates the workflow instance in the authored failed
// outcome, with Error describing the failure. The compiler guarantees
// Error is statically string.
type FailControl struct {
	Error Expression
}

func (FailControl) isWorkflowControl() {}

// CancelControl terminates the workflow instance in the authored
// cancelled outcome, with Reason describing the cancellation. The
// compiler guarantees Reason is statically string.
type CancelControl struct {
	Reason Expression
}

func (CancelControl) isWorkflowControl() {}

// ConditionalControl selects Then or Else based on Condition, which the
// compiler guarantees is statically bool.
type ConditionalControl struct {
	Condition Expression
	Then      WorkflowControl
	Else      WorkflowControl
}

func (ConditionalControl) isWorkflowControl() {}

// MatchControl selects exactly one Cases entry's Control by inspecting
// Value against its Pattern — see MatchPattern.
type MatchControl struct {
	Value Expression
	Cases []MatchControlCase
}

func (MatchControl) isWorkflowControl() {}

// MatchControlCase pairs one MatchPattern with the WorkflowControl
// selected when that pattern matches. Pattern's lexical bindings, if
// any, are in scope only within Control.
type MatchControlCase struct {
	Pattern MatchPattern
	Control WorkflowControl
}
