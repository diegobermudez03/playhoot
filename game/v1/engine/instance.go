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
	// LocalState and, until a future step adds operations that mutate
	// it, unchanged for the instance's lifetime. Its TypeName is always
	// the reserved scope root name "local" — never a name declared in
	// Program.Types.
	LocalState RecordValue

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
	Responses  []AskGroupResponse
	Completed  bool
}

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
