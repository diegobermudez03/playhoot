package engine

// WorkflowInstance is the persistable runtime state of one running
// workflow — the root, or any child anywhere in its child-workflow tree.
//
// WorkflowInstance is plain data: nothing here executes anything, and
// nothing enforces workflow structure or invariants. It exists so a
// Snapshot can represent a complete "logical position" of a game
// instance, well enough to stop and resume execution between any two
// steps — see LOGICAL_CONTRACT.md.
type WorkflowInstance struct {
	// Workflow names the compiled Program.Workflows entry this instance
	// is running.
	Workflow string

	// State names the WorkflowState this instance currently occupies.
	State string

	// Parameters are the immutable argument values bound when this
	// instance was created, matching its Workflow's declared
	// Parameters.
	Parameters []FieldValue

	// LocalState is this instance's mutable, workflow-local state,
	// evaluated once at creation from its Workflow's declared
	// LocalState and mutated in place by engineservice.Step's compiled
	// operations. Its TypeName is always the reserved scope root name
	// "local" — never a name declared in Program.Types.
	LocalState RecordValue

	// Outcome is nil while this instance is still running. Once a
	// transition applies a CompleteControl, FailControl, or
	// CancelControl to it, Outcome is set and no further transition may
	// apply to this instance — see program.WorkflowControl's variants
	// for the outcomes they produce.
	Outcome *WorkflowOutcome

	QuestionSlots  []QuestionSlotInstance
	AskGroupSlots  []AskGroupSlotInstance
	TimerSlots     []TimerSlotInstance
	ChildSlots     []ChildWorkflowSlotInstance
	TaskGroupSlots []TaskGroupSlotInstance
}

// QuestionSlotInstance is the runtime occupancy of one declared
// QuestionSlot: at most one pending question at a time. Pending is nil
// exactly when the slot is empty.
type QuestionSlotInstance struct {
	Name    string
	Pending *PendingQuestion
}

// PendingQuestion is one concrete, in-flight question instance: the
// user it was opened for and the arguments captured when it opened — see
// program.OpenQuestionOperation. A future step, once operations are
// compiled, is what creates one.
type PendingQuestion struct {
	Recipient UserID
	Arguments []FieldValue
}

// AskGroupSlotInstance is the runtime occupancy of one declared
// AskGroupSlot. Pending is nil exactly when the slot is empty.
type AskGroupSlotInstance struct {
	Name    string
	Pending *PendingAskGroup
}

// PendingAskGroup is one concrete, in-flight ask-group instance:
// conceptually collecting while Completed is false, and
// completed-awaiting-join once it is true — see
// program.AskGroupSlotDeclaration.
type PendingAskGroup struct {
	Recipients []UserID

	// Arguments are the question's shared arguments, evaluated once for
	// the whole group when it was opened — see
	// program.OpenAskGroupOperation's documented "shared arguments".
	Arguments []FieldValue

	// Responses holds every accepted answer, in the order each was
	// accepted — this is both the durable record used to compute
	// AskGroupCompletedSignalSource's "respondents" field and the input
	// to CompletionKind's evaluation.
	Responses []AskGroupResponse

	Completed bool

	// CompletionKind and QuorumCount capture the AskGroupCompletionPolicy
	// this group was opened with, evaluated once at open time — see
	// program.AskGroupCompletionPolicy's documented "evaluated once ...
	// does not change afterward". QuorumCount is only meaningful when
	// CompletionKind is AskGroupCompletionQuorum.
	CompletionKind AskGroupCompletionKind
	QuorumCount    int
}

// AskGroupCompletionKind identifies which AskGroupCompletionPolicy
// variant a PendingAskGroup was opened with.
type AskGroupCompletionKind int

const (
	// AskGroupCompletionAllResponses is the zero value: the group
	// completes once every unique recipient has an accepted answer.
	AskGroupCompletionAllResponses AskGroupCompletionKind = iota

	// AskGroupCompletionFirstResponse: the group completes on the first
	// accepted answer.
	AskGroupCompletionFirstResponse

	// AskGroupCompletionQuorum: the group completes once QuorumCount
	// unique recipients have an accepted answer.
	AskGroupCompletionQuorum
)

// AskGroupResponse is one accepted answer collected by a
// PendingAskGroup.
type AskGroupResponse struct {
	Respondent UserID
	Answer     Value
}

// TimerSlotInstance is the runtime occupancy of one declared TimerSlot.
//
// Per LOGICAL_CONTRACT.md's "no real timer scheduling", this records
// only whether a timer is currently pending, never a deadline or
// duration — time enters the engine through explicit signal data, not
// through a wall-clock value tracked here.
type TimerSlotInstance struct {
	Name    string
	Pending bool
}

// ChildWorkflowSlotInstance is the runtime occupancy of one declared
// ChildWorkflowSlot: at most one child instance at a time. Child is nil
// exactly when the slot is empty; this is the recursive edge that makes
// a Snapshot's workflow instances form a tree.
type ChildWorkflowSlotInstance struct {
	Name  string
	Child *WorkflowInstance
}

// TaskGroupSlotInstance is the runtime occupancy of one declared
// TaskGroupSlot: a dynamically sized collection of same-workflow child
// instances, each addressed by its own task Key. An empty Tasks means
// the slot is empty.
type TaskGroupSlotInstance struct {
	Name  string
	Tasks []TaskGroupTask
}

// TaskGroupTask is one child instance owned by a TaskGroupSlotInstance,
// addressed by Key.
type TaskGroupTask struct {
	Key   Value
	Child WorkflowInstance
}

// WorkflowOutcomeKind identifies which terminal outcome a
// WorkflowOutcome represents.
type WorkflowOutcomeKind int

const (
	// WorkflowOutcomeCompleted marks a workflow instance that completed
	// successfully through CompleteControl.
	WorkflowOutcomeCompleted WorkflowOutcomeKind = iota

	// WorkflowOutcomeFailed marks a workflow instance that terminated
	// through an authored FailControl.
	WorkflowOutcomeFailed

	// WorkflowOutcomeCancelled marks a workflow instance that
	// terminated through an authored CancelControl.
	WorkflowOutcomeCancelled
)

func (k WorkflowOutcomeKind) String() string {
	switch k {
	case WorkflowOutcomeCompleted:
		return "completed"
	case WorkflowOutcomeFailed:
		return "failed"
	case WorkflowOutcomeCancelled:
		return "cancelled"
	default:
		return "unknown"
	}
}

// WorkflowOutcome is the terminal outcome of one WorkflowInstance. Only
// the field matching Kind is meaningful: Result for
// WorkflowOutcomeCompleted, Error for WorkflowOutcomeFailed, Reason for
// WorkflowOutcomeCancelled.
//
// When the terminated instance is a child, its parent observes this
// outcome through ChildCompletedSignalSource, ChildFailedSignalSource,
// or ChildCancelledSignalSource. When it is the root, per
// program.FailControl's and program.CancelControl's documentation,
// there is no parent to notify — a future session layer may observe it
// here directly.
type WorkflowOutcome struct {
	Kind   WorkflowOutcomeKind
	Result Value
	Error  string
	Reason string
}
