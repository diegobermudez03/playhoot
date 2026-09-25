package engine

// WorkflowInstance is the persistable runtime state of the one workflow a
// Session runs for its entire lifetime.
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
	// LocalState and mutated in place by Step's compiled
	// operations. Its TypeName is always the reserved scope root name
	// "local" — never a name declared in Program.Types.
	LocalState RecordValue

	// Outcome is nil while this instance is still running. Once a
	// transition applies a CompleteControl, FailControl, or
	// CancelControl to it, Outcome is set and no further transition may
	// apply to this instance — see program.WorkflowControl's variants
	// for the outcomes they produce.
	Outcome *WorkflowOutcome

	QuestionSlots []QuestionSlotInstance
	AskGroupSlots []AskGroupSlotInstance
	TimerSlots    []TimerSlotInstance

	KeyedQuestionSlots []KeyedQuestionSlotInstance
	KeyedAskGroupSlots []KeyedAskGroupSlotInstance
	KeyedTimerSlots    []KeyedTimerSlotInstance
}

// QuestionSlotInstance is the runtime occupancy of one declared
// QuestionSlot: at most one pending question at a time. Pending is nil
// exactly when the slot is empty.
type QuestionSlotInstance struct {
	Name    string
	Pending *PendingQuestion
}

// PendingQuestion is one concrete, in-flight question instance: the
// user it was opened for and the arguments captured when it opened —
// see program.OpenQuestionOperation. Step is what creates
// one. InteractionID is assigned once at open time from
// Snapshot.NextInteractionID and never changes afterward.
type PendingQuestion struct {
	Recipient     UserID
	Arguments     []FieldValue
	InteractionID InteractionID
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

	// InteractionID addresses this whole group occurrence — assigned
	// once at open time and shared by every recipient's own
	// OpenQuestionOutput for it, since the group, not any one
	// recipient's question, is what a caller answers/completes against.
	InteractionID InteractionID

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

// KeyedQuestionSlotInstance is the runtime occupancy of one declared
// KeyedQuestionSlot: zero or more simultaneously pending questions, one
// per occupied key. At most one entry in Pending ever shares the same
// Key (compared via Value.Equal) — this is the (slot, key) occupancy
// invariant KeyedQuestionSlotDeclaration documents, generalizing
// QuestionSlotInstance's single-Pending shape to a per-key collection.
// Pending is searched linearly by key equality, mirroring how
// MapValue.Entries is already searched elsewhere in this package — an
// arbitrary authored key type has no cheaper canonical hash to bucket
// by.
type KeyedQuestionSlotInstance struct {
	Name    string
	Pending []KeyedPendingQuestion
}

// KeyedPendingQuestion is one concrete, in-flight question occurrence at
// a specific Key — see program.OpenKeyedQuestionOperation. InteractionID
// is assigned once at open time and never changes afterward, exactly
// like PendingQuestion's own.
type KeyedPendingQuestion struct {
	Key           Value
	Recipient     UserID
	Arguments     []FieldValue
	InteractionID InteractionID
}

// KeyedAskGroupSlotInstance is the runtime occupancy of one declared
// KeyedAskGroupSlot: zero or more simultaneously collecting or
// completed-awaiting-join ask-group occurrences, one per occupied key.
// See KeyedQuestionSlotInstance's doc comment for the shared (slot,
// key)-occupancy/linear-search rationale.
type KeyedAskGroupSlotInstance struct {
	Name    string
	Pending []KeyedPendingAskGroup
}

// KeyedPendingAskGroup is one concrete, in-flight ask-group occurrence
// at a specific Key — see program.OpenKeyedAskGroupOperation. It embeds
// PendingAskGroup unchanged: a keyed occurrence's own collection
// semantics (Recipients, Arguments, Responses, Completed,
// CompletionKind, QuorumCount) are identical to an ordinary ask group's,
// only the addressing gains a Key.
type KeyedPendingAskGroup struct {
	Key Value
	PendingAskGroup
}

// KeyedTimerSlotInstance is the runtime occupancy of one declared
// KeyedTimerSlot: zero or more simultaneously pending timers, one per
// occupied key. See KeyedQuestionSlotInstance's doc comment for the
// shared (slot, key)-occupancy/linear-search rationale, and
// TimerSlotInstance's doc comment for why only pending-or-not is
// recorded, never a deadline.
type KeyedTimerSlotInstance struct {
	Name    string
	Pending []KeyedPendingTimer
}

// KeyedPendingTimer is one concrete, in-flight timer occurrence at a
// specific Key — see program.ScheduleKeyedTimerOperation.
type KeyedPendingTimer struct {
	Key Value
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

// WorkflowOutcome is the terminal outcome of the one WorkflowInstance a
// Session runs. Only the field matching Kind is meaningful: Result for
// WorkflowOutcomeCompleted, Error for WorkflowOutcomeFailed, Reason for
// WorkflowOutcomeCancelled. A session layer observes it directly through
// WorkflowCompletedOutput.
type WorkflowOutcome struct {
	Kind   WorkflowOutcomeKind
	Result Value
	Error  string
	Reason string
}
