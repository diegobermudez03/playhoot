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

	// SignalKindTimerExpired identifies the expiration of the pending
	// timer in the workflow slot named Slot.
	//
	// Per program.TimerExpiredSignalSource, only a still-current,
	// uncancelled pending timer produces a signal — a stale or
	// cancelled delivery is rejected before any transition is
	// considered; see ErrInputRejected.
	SignalKindTimerExpired

	// SignalKindKeyedTimerExpired identifies the expiration of the
	// pending timer at the keyed workflow slot named Slot's occurrence
	// Key, generalizing SignalKindTimerExpired to a (slot, key)
	// occurrence.
	SignalKindKeyedTimerExpired

	// SignalKindInteractionAnswered identifies a submitted answer to
	// the Question or Ask Group occurrence addressed by InteractionID:
	// Respondent is the answering user and Answer the submitted value.
	// The engine resolves InteractionID alone to the underlying slot
	// (ordinary or keyed) and occurrence Key, and to whether it behaves
	// as a Question or an Ask Group — a caller never supplies Slot,
	// Key, or its own classification of which kind of interaction it
	// is answering.
	//
	// For a Question occurrence: engineservice.Step verifies Respondent
	// against the occurrence's pending recipient, validates Answer
	// against the question's response type and Validation expression,
	// and rejects a stale, duplicate, unauthorized, invalid, or
	// unresolvable-InteractionID submission before any transition is
	// even considered; see ErrInputRejected.
	//
	// For an Ask Group occurrence: unlike every other SignalKind, this
	// one never itself selects or runs a transition when answering an
	// Ask Group — per program.AskGroupCompletedSignalSource, an ask
	// group "never produces a signal per individual answer".
	// engineservice.Step instead validates Respondent is a current,
	// not-yet-answered recipient of the occurrence's still-collecting
	// group, validates Answer the same way, and, if accepted, records
	// the answer and re-evaluates the group's completion policy — all
	// as one atomic Commit with no transition selected.
	SignalKindInteractionAnswered

	// SignalKindInteractionCompleted identifies that the Ask Group
	// occurrence addressed by InteractionID is completed-awaiting-join
	// — its completion policy was satisfied naturally, or a
	// Finalize(Keyed)AskGroupOperation forced it. Signal carries no
	// other payload; engineservice.Step reads the group's durable
	// "responses", "respondents", and "missing" data directly from the
	// resolved occurrence. A stale, duplicate, or unresolvable-
	// InteractionID delivery (already joined and cleared, or unknown)
	// is rejected; see program.AskGroupCompletedSignalSource and
	// ErrInputRejected. Never produced for a Question occurrence — a
	// Question's own SignalKindInteractionAnswered is what selects its
	// transition directly.
	SignalKindInteractionCompleted
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

	// Slot and Key address a timer occurrence for
	// SignalKindTimerExpired/SignalKindKeyedTimerExpired only. Every
	// other SignalKind is addressed by InteractionID instead — see
	// SignalKindInteractionAnswered/SignalKindInteractionCompleted.
	Slot string
	Key  Value

	// InteractionID addresses the Question or Ask Group occurrence a
	// SignalKindInteractionAnswered/SignalKindInteractionCompleted
	// signal targets. Meaningless for every other SignalKind.
	InteractionID InteractionID

	Respondent UserID
	Answer     Value

	Fields map[string]Value
}
