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
//
// InteractionID addresses this occurrence for answering — see
// SignalKindInteractionAnswered — and never changes for its lifetime.
// Kind reports whether this occurrence behaves as an ordinary Question
// or as one recipient's own opened question within an Ask Group (see
// below); a caller never needs the compiled Program to tell the two
// apart. When an Ask Group opens for several recipients, every
// recipient's own OpenQuestionOutput for that occurrence carries the
// same InteractionID — the group, not any one recipient's question, is
// the addressable occurrence.
type OpenQuestionOutput struct {
	Slot          string
	Recipient     UserID
	Question      string
	Arguments     []FieldValue
	InteractionID InteractionID
	Kind          InteractionKind
}

func (OpenQuestionOutput) isOutput() {}

// CloseQuestionOutput reports that the pending question in the workflow
// slot Slot, previously opened for Recipient, was closed.
//
// InteractionID is the same value OpenQuestionOutput carried when this
// occurrence opened — a caller that tracks occurrences by InteractionID
// never needs Slot/Recipient to correlate this closure with the
// occurrence it already knows about.
type CloseQuestionOutput struct {
	Slot          string
	Recipient     UserID
	InteractionID InteractionID
}

func (CloseQuestionOutput) isOutput() {}

// OpenKeyedQuestionOutput reports that the question named Question was
// opened for Recipient at the keyed workflow slot Slot's occurrence Key,
// with the given Arguments as its captured parameters, generalizing
// OpenQuestionOutput to a (slot, key) occurrence. Reused for a keyed
// ask-group's per-recipient opened questions, exactly like
// OpenQuestionOutput is already reused for an ordinary ask group's. See
// OpenQuestionOutput's doc comment for InteractionID/Kind's meaning,
// unchanged here beyond the added Key.
type OpenKeyedQuestionOutput struct {
	Slot          string
	Key           Value
	Recipient     UserID
	Question      string
	Arguments     []FieldValue
	InteractionID InteractionID
	Kind          InteractionKind
}

func (OpenKeyedQuestionOutput) isOutput() {}

// CloseKeyedQuestionOutput reports that the pending question at the
// keyed workflow slot Slot's occurrence Key, previously opened for
// Recipient, was closed, generalizing CloseQuestionOutput to a (slot,
// key) occurrence. See CloseQuestionOutput's doc comment for
// InteractionID's meaning.
type CloseKeyedQuestionOutput struct {
	Slot          string
	Key           Value
	Recipient     UserID
	InteractionID InteractionID
}

func (CloseKeyedQuestionOutput) isOutput() {}

// ScheduleKeyedTimerOutput reports that a timer was scheduled at the
// keyed workflow slot Slot's occurrence Key, to fire after
// DelayMilliseconds, generalizing ScheduleTimerOutput to a (slot, key)
// occurrence.
type ScheduleKeyedTimerOutput struct {
	Slot              string
	Key               Value
	DelayMilliseconds float64
}

func (ScheduleKeyedTimerOutput) isOutput() {}

// CancelKeyedTimerOutput reports that the pending timer at the keyed
// workflow slot Slot's occurrence Key was cancelled, generalizing
// CancelTimerOutput to a (slot, key) occurrence.
type CancelKeyedTimerOutput struct {
	Slot string
	Key  Value
}

func (CancelKeyedTimerOutput) isOutput() {}

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

// RunCompletedOutput reports that the one instance a Session runs
// reached Outcome as the terminal result of the transition that just
// committed. This is how a session layer observes that directly, ending
// the game instance. Which compiled workflow backed the instance is an
// authoring/internal detail this Output does not carry - a caller never
// needs it to react to the instance ending.
//
// Exactly one RunCompletedOutput is ever produced per Step call, for the
// transition control that terminated the instance.
type RunCompletedOutput struct {
	Outcome RunOutcome
}

func (RunCompletedOutput) isOutput() {}
