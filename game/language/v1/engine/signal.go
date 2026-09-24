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
	// slot named Slot is completed-awaiting-join — its completion
	// policy was satisfied naturally, or a FinalizeAskGroupOperation
	// forced it. Signal carries no payload of its own; engineservice.Step
	// reads the group's durable "responses", "respondents", and
	// "missing" data directly from the slot. A stale or duplicate
	// delivery once the slot has already been joined and cleared is
	// rejected; see program.AskGroupCompletedSignalSource and
	// ErrInputRejected.
	SignalKindAskGroupCompleted
)

// Signal is one runtime input to engineservice.Step: something that
// happened, together with whatever payload its schema exposes for
// binding — see engine.SignalPattern and engine.SignalBinding. Which
// fields are meaningful depends on Kind; see each SignalKind constant.
// A Session runs exactly one workflow instance for its entire lifetime,
// so a Signal always targets that one instance - there is no addressing
// concept to resolve.
//
// A Signal is always consumed by exactly one Step call; Step never
// applies more than one Signal, and the engine does not recursively
// generate and apply further transitions inside one Step.
type Signal struct {
	Kind SignalKind

	Name string

	Intent string
	Actor  UserID

	Slot       string
	Respondent UserID
	Answer     Value

	Fields map[string]Value
}
