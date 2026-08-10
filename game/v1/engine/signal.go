package engine

// SignalKind identifies which of Signal's fields are meaningful.
type SignalKind int

const (
	// SignalKindNamed is the zero value: Name identifies a named
	// platform or lifecycle signal (see engineservice's named lifecycle
	// signal catalog) — the only kind engineservice.NewSnapshot
	// produces, as the "first lifecycle signal" LOGICAL_CONTRACT.md
	// requires.
	SignalKindNamed SignalKind = iota

	// SignalKindIntent identifies a player-submitted user intent:
	// Intent names a program.UserIntentDeclaration, Actor is the
	// submitting user, and Fields holds the intent's own declared
	// parameters.
	SignalKindIntent

	// SignalKindQuestionAnswered identifies a submitted answer to the
	// pending question in the workflow slot named Slot: Respondent is
	// the answering user and Answer the submitted value.
	//
	// Per program.QuestionAnsweredSignalSource, only a validated answer
	// ever reaches a workflow as a signal — engineservice.Step verifies
	// Respondent against the slot's pending recipient, validates
	// Answer against the question's response type and Validation
	// expression, and rejects a stale, duplicate, unauthorized, or
	// invalid submission before any transition is even considered; see
	// ErrInputRejected.
	SignalKindQuestionAnswered

	// SignalKindTimerExpired identifies the expiration of the pending
	// timer in the workflow slot named Slot.
	//
	// Per program.TimerExpiredSignalSource, only a still-current,
	// uncancelled pending timer produces a signal — a stale or
	// cancelled delivery is rejected before any transition is
	// considered; see ErrInputRejected.
	SignalKindTimerExpired

	// SignalKindChildCompleted identifies that the child workflow
	// instance in the child slot named Slot, owned by the instance
	// Path addresses, completed successfully. engineservice.Step reads
	// the child's durable result directly from the slot — Signal
	// carries no payload of its own for this kind — and rejects a
	// stale or duplicate delivery once the slot has already been
	// joined and cleared; see program.ChildCompletedSignalSource and
	// ErrInputRejected.
	SignalKindChildCompleted

	// SignalKindChildFailed identifies that the child workflow instance
	// in the child slot named Slot, owned by the instance Path
	// addresses, terminated through an authored FailControl. See
	// SignalKindChildCompleted and program.ChildFailedSignalSource.
	SignalKindChildFailed

	// SignalKindChildCancelled identifies that the child workflow
	// instance in the child slot named Slot, owned by the instance Path
	// addresses, terminated itself through an authored CancelControl.
	// This is distinct from parent-driven cancellation
	// (CancelChildWorkflowOperation), which never produces a Signal.
	// See SignalKindChildCompleted and program.ChildCancelledSignalSource.
	SignalKindChildCancelled

	// SignalKindAskGroupAnswered identifies a submitted answer to the
	// ask group collecting in the workflow slot named Slot: Respondent
	// is the answering user and Answer the submitted value.
	//
	// Unlike every other SignalKind, this one never itself selects or
	// runs a transition — per program.AskGroupCompletedSignalSource,
	// an ask group "never produces a signal per individual answer".
	// engineservice.Step instead validates Respondent is a current,
	// not-yet-answered recipient of the slot's still-collecting group,
	// validates Answer against the question's response type and
	// Validation expression exactly as for SignalKindQuestionAnswered,
	// and, if accepted, records the answer and re-evaluates the group's
	// completion policy — all as one atomic Commit with no transition
	// selected. A stale, duplicate, unauthorized, or invalid submission
	// is rejected; see ErrInputRejected.
	SignalKindAskGroupAnswered

	// SignalKindAskGroupCompleted identifies that the ask group in the
	// slot named Slot, owned by the instance Path addresses, is
	// completed-awaiting-join — its completion policy was satisfied
	// naturally, or a FinalizeAskGroupOperation forced it. Signal
	// carries no payload of its own; engineservice.Step reads the
	// group's durable "responses", "respondents", and "missing" data
	// directly from the slot. A stale or duplicate delivery once the
	// slot has already been joined and cleared is rejected; see
	// program.AskGroupCompletedSignalSource and ErrInputRejected.
	SignalKindAskGroupCompleted

	// SignalKindTaskGroupCompleted identifies that the task group in
	// the slot named Slot, owned by the instance Path addresses, is
	// completed-awaiting-join — its completion policy was satisfied
	// naturally, or a FinalizeTaskGroupOperation forced it. Signal
	// carries no payload of its own; engineservice.Step reads the
	// group's durable "taskKeys", "terminalKeys", "results", "failures",
	// "cancellations", and "unfinished" data directly from the slot.
	// Unlike a child workflow's individual outcome, no single task in
	// the group ever produces its own signal to the owning workflow —
	// only this aggregate does, once, when the whole group completes.
	// A stale or duplicate delivery once the slot has already been
	// joined and cleared is rejected; see
	// program.TaskGroupCompletedSignalSource and ErrInputRejected.
	SignalKindTaskGroupCompleted
)

// PathStep identifies one step from a parent workflow instance down to
// a child it owns, as part of a Signal.Path — either a child occupying
// a named ChildWorkflowSlot (TaskKey nil), or a task occupying a named
// TaskGroupSlot at the task key TaskKey (TaskKey non-nil).
type PathStep struct {
	Slot    string
	TaskKey Value
}

// Signal is one runtime input to engineservice.Step: something that
// happened, together with whatever payload its schema exposes for
// binding — see engine.SignalPattern and engine.SignalBinding. Which
// fields are meaningful depends on Kind; see each SignalKind constant.
//
// A Signal is always consumed by exactly one Step call; Step never
// applies more than one Signal, and the engine does not recursively
// generate and apply further transitions inside one Step.
type Signal struct {
	Kind SignalKind

	// Path addresses which workflow instance in the current
	// child-workflow tree this Signal targets, as a sequence of
	// PathStep values walked from the root instance down — for example,
	// []PathStep{{Slot: "Opponent"}} targets the child currently
	// occupying the root instance's "Opponent" child slot, and
	// []PathStep{{Slot: "Workers", TaskKey: NumberValue{Value: 3}}}
	// targets the task keyed 3 inside the root instance's "Workers"
	// task-group slot. A nil or empty Path targets the root instance
	// itself.
	//
	// A child-outcome signal (SignalKindChildCompleted,
	// SignalKindChildFailed, SignalKindChildCancelled), an
	// AskGroupCompletedSignalSource, or a TaskGroupCompletedSignalSource
	// targets the parent that owns the terminated child's slot, not the
	// terminated child itself — joining is something the parent's own
	// transition does.
	Path []PathStep

	Name string

	Intent string
	Actor  UserID

	Slot       string
	Respondent UserID
	Answer     Value

	Fields map[string]Value
}
