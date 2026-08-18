package engine

// Output is one declarative, external action produced by a committed
// Step: the engine describes what should happen — open a question,
// schedule a timer, emit a client-facing effect, and so on — without
// performing it. The engine never sends messages or operates a real
// clock itself; an application layer built on top of engineservice
// decides how to persist, schedule, or deliver each Output.
//
// Output variants form a closed set controlled by this package, the
// same way program's Expression, Operation, and similar concepts are
// closed; callers outside the package are expected to eventually
// type-switch over them exhaustively.
type Output interface {
	isOutput()
}

// OpenQuestionOutput reports that the question named Question was
// opened for Recipient in the workflow slot Slot, with the given
// Arguments as its captured parameters.
type OpenQuestionOutput struct {
	Slot      string
	Recipient UserID
	Question  string
	Arguments []FieldValue
}

func (OpenQuestionOutput) isOutput() {}

// CloseQuestionOutput reports that the pending question in the workflow
// slot Slot, previously opened for Recipient, was closed.
type CloseQuestionOutput struct {
	Slot      string
	Recipient UserID
}

func (CloseQuestionOutput) isOutput() {}

// ScheduleTimerOutput reports that a timer was scheduled in the
// workflow slot Slot, to fire after DelayMilliseconds. The engine
// itself does not schedule anything — see LOGICAL_CONTRACT.md — an
// application layer is what turns this into a real, delayed delivery
// of the matching TimerExpiredSignalSource signal.
type ScheduleTimerOutput struct {
	Slot              string
	DelayMilliseconds float64
}

func (ScheduleTimerOutput) isOutput() {}

// CancelTimerOutput reports that the pending timer in the workflow slot
// Slot was cancelled.
type CancelTimerOutput struct {
	Slot string
}

func (CancelTimerOutput) isOutput() {}

// EmitEffectOutput reports that one instance of the named effect Effect
// was produced for Recipients, with the given Arguments. Per
// program.EffectDeclaration, an effect is presentation-only: a client
// that misses one leaves authoritative state unaffected.
type EmitEffectOutput struct {
	Effect     string
	Recipients []UserID
	Arguments  []FieldValue
}

func (EmitEffectOutput) isOutput() {}

// ActivatePresentationOutput reports that the presentation named Name
// was newly mounted for Recipient in the presentation slot Slot,
// showing the view named View with the visible model Model. Per
// program.PresentationSlotDeclaration, at most one presentation may
// occupy a given (Slot, Recipient) pair at a time — see
// engineservice's presentation.go.
type ActivatePresentationOutput struct {
	Slot      string
	Recipient UserID
	Name      string
	View      string
	Model     Value
}

func (ActivatePresentationOutput) isOutput() {}

// UpdatePresentationOutput reports that the presentation named Name,
// already active for Recipient in the presentation slot Slot, now has
// the visible model Model — recomputed from the same, already-committed
// snapshot an ActivatePresentationOutput or a prior
// UpdatePresentationOutput for the same (Slot, Recipient) last reported.
type UpdatePresentationOutput struct {
	Slot      string
	Recipient UserID
	Name      string
	Model     Value
}

func (UpdatePresentationOutput) isOutput() {}

// RemovePresentationOutput reports that the presentation named Name,
// previously active for Recipient in the presentation slot Slot, was
// unmounted — its owning workflow-level scope, state, or pending
// question ended, or it was superseded.
type RemovePresentationOutput struct {
	Slot      string
	Recipient UserID
	Name      string
}

func (RemovePresentationOutput) isOutput() {}

// WorkflowCompletedOutput reports that the workflow instance addressed
// by Path — running the compiled Workflow named Workflow — reached
// Outcome as the terminal result of the transition that just committed.
//
// Path is empty when the terminated instance is the root: per
// WorkflowOutcome's documented "when it is the root, there is no parent
// to notify", a WorkflowCompletedOutput is how a session layer observes
// that directly, ending the game instance. A non-empty Path reports a
// child, ask-group task, or task-group task instead — informational
// only, since its owning parent already observes the same outcome
// through its own ChildCompletedSignalSource, ChildFailedSignalSource,
// ChildCancelledSignalSource, AskGroupCompletedSignalSource, or
// TaskGroupCompletedSignalSource and reacts to it, if at all, through an
// ordinary transition rather than through this Output.
//
// Exactly one WorkflowCompletedOutput is ever produced per Step call,
// for the one instance Step's selected transition control terminated —
// never for a descendant discarded along with it (see instance.go's
// documented "disappears when the parent workflow terminates"), since
// those were never separately observed to reach their own outcome.
type WorkflowCompletedOutput struct {
	Path     []PathStep
	Workflow string
	Outcome  WorkflowOutcome
}

func (WorkflowCompletedOutput) isOutput() {}
