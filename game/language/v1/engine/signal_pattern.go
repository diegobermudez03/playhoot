package engine

// SignalSource is the compiled representation of one
// program.SignalSource: the schema of a signal a transition can react
// to. The compiler guarantees a NamedSignalSource names a signal in
// engineservice's lifecycle-signal catalog, and every *SignalSource
// naming a Slot names a slot declared on the enclosing Workflow of the
// matching kind.
//
// SignalSource is a closed interface, mirroring program's own
// closed-interface pattern.
type SignalSource interface {
	isSignalSource()
}

// NamedSignalSource identifies a platform-defined or lifecycle signal
// by its static, catalog-resolved Name — see engineservice's named
// lifecycle signal catalog, which includes at least "WorkflowStarted".
type NamedSignalSource struct {
	Name string
}

func (NamedSignalSource) isSignalSource() {}

// UserIntentSignalSource matches the signal produced when a user submits
// the named, resolved UserIntentDeclaration Intent.
type UserIntentSignalSource struct {
	Intent string
}

func (UserIntentSignalSource) isSignalSource() {}

// QuestionAnsweredSignalSource matches the signal produced when a
// validated answer is accepted for the named QuestionSlot Slot.
type QuestionAnsweredSignalSource struct {
	Slot string
}

func (QuestionAnsweredSignalSource) isSignalSource() {}

// TimerExpiredSignalSource matches the signal produced when the pending
// timer in the named timer slot Slot expires.
type TimerExpiredSignalSource struct {
	Slot string
}

func (TimerExpiredSignalSource) isSignalSource() {}

// ChildCompletedSignalSource matches the signal produced when the child
// workflow in the named ChildWorkflowSlot Slot completes successfully.
type ChildCompletedSignalSource struct {
	Slot string
}

func (ChildCompletedSignalSource) isSignalSource() {}

// ChildFailedSignalSource matches the signal produced when the child
// workflow in the named ChildWorkflowSlot Slot terminates through an
// authored FailControl.
type ChildFailedSignalSource struct {
	Slot string
}

func (ChildFailedSignalSource) isSignalSource() {}

// ChildCancelledSignalSource matches the signal produced when the child
// workflow in the named ChildWorkflowSlot Slot terminates through an
// authored CancelControl.
type ChildCancelledSignalSource struct {
	Slot string
}

func (ChildCancelledSignalSource) isSignalSource() {}

// AskGroupCompletedSignalSource matches the signal produced when the ask
// group in the named AskGroupSlot Slot completes.
type AskGroupCompletedSignalSource struct {
	Slot string
}

func (AskGroupCompletedSignalSource) isSignalSource() {}

// TaskGroupCompletedSignalSource matches the signal produced when the
// task group in the named TaskGroupSlot Slot completes.
type TaskGroupCompletedSignalSource struct {
	Slot string
}

func (TaskGroupCompletedSignalSource) isSignalSource() {}

// SignalBinding binds Field from a matched signal's schema (see
// SignalSource) to the immutable lexical name Name, in scope for the
// enclosing Transition's Guard and Control.
type SignalBinding struct {
	Field string
	Name  string
}

// SignalPattern is the compiled representation of one
// program.SignalPattern: which SignalSource a Transition reacts to, and
// which of that signal's schema fields it binds into scope.
type SignalPattern struct {
	Source   SignalSource
	Bindings []SignalBinding
}
