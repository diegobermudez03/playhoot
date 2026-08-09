package engine

// Workflow is the compiled representation of one
// program.WorkflowDeclaration.
//
// Workflow is intentionally incomplete: Transition does not yet carry
// compiled operations (see Transition's doc comment), so a Workflow
// cannot yet be executed — only resolved and semantically validated, per
// this compiler step's expected result. Execution semantics are added
// once operations are compiled.
type Workflow struct {
	Name       string
	Parameters []FieldType
	ResultType Type
	LocalState []StateField

	QuestionSlots  []QuestionSlot
	AskGroupSlots  []AskGroupSlot
	TimerSlots     []string
	ChildSlots     []ChildWorkflowSlot
	TaskGroupSlots []TaskGroupSlot

	// Presentations are this workflow's workflow-level presentations,
	// active for the lifetime of a workflow instance.
	Presentations []Presentation

	// InitialState names the state a new workflow instance begins in.
	// The compiler guarantees it names a state in States.
	InitialState string

	// GlobalTransitions are workflow-level fallback transitions that
	// may apply from any current state. A future execution step, not
	// this compiler, resolves the priority between a state-local
	// transition and a global transition for the same signal.
	GlobalTransitions []Transition

	States []WorkflowState
}

// QuestionSlot is the compiled representation of one
// program.QuestionSlotDeclaration. The compiler guarantees Question
// names a declared program.QuestionDeclaration.
type QuestionSlot struct {
	Name         string
	Question     string
	Presentation *QuestionPresentation
}

// AskGroupSlot is the compiled representation of one
// program.AskGroupSlotDeclaration. The compiler guarantees Question
// names a declared program.QuestionDeclaration.
type AskGroupSlot struct {
	Name         string
	Question     string
	Presentation *QuestionPresentation
}

// QuestionPresentation is the compiled representation of one
// program.QuestionPresentationDeclaration. Unlike the source form, it
// carries no Slot field: it is always reached through the
// QuestionSlot/AskGroupSlot that owns it, which already identifies the
// slot.
type QuestionPresentation struct {
	Projection          string
	ProjectionArguments []CallArgument
	View                string
}

// ChildWorkflowSlot is the compiled representation of one
// program.ChildWorkflowSlotDeclaration. The compiler guarantees Workflow
// names a declared Workflow.
type ChildWorkflowSlot struct {
	Name     string
	Workflow string
}

// TaskGroupSlot is the compiled representation of one
// program.TaskGroupSlotDeclaration. The compiler guarantees Workflow
// names a declared Workflow and KeyType is resolved.
type TaskGroupSlot struct {
	Name     string
	Workflow string
	KeyType  Type
}

// Presentation is the compiled representation of one
// program.PresentationDeclaration, used for both a Workflow's
// workflow-level Presentations and a WorkflowState's state-level ones.
type Presentation struct {
	Name                string
	Slot                string
	Targets             Expression
	Projection          string
	ProjectionArguments []CallArgument
	View                string
}

// WorkflowState is the compiled representation of one
// program.WorkflowStateDeclaration.
type WorkflowState struct {
	Name          string
	Presentations []Presentation
	Transitions   []Transition
}

// Transition is the compiled representation of one
// program.TransitionDeclaration.
//
// Transition does not yet carry compiled operations: program's model
// executes a transition as match signal, bind, evaluate Guard, run
// Operations, then evaluate Control, but this compiler step only
// resolves and validates Signal, Guard, and Control — Operations
// (mutation, interactions, structured concurrency) is a separate,
// larger compilation concern left for a future step.
type Transition struct {
	Name    string
	Signal  SignalPattern
	Guard   Expression
	Control WorkflowControl
}
