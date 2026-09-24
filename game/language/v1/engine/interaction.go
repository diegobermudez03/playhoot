package engine

// InteractionID identifies one addressable Question or Ask Group
// occurrence (ordinary or keyed), assigned by the engine itself as part
// of Snapshot's own deterministic state — never computed by a caller.
// Zero is reserved to mean "no interaction": it is never assigned to a
// real occurrence, so an unset InteractionID field is never mistaken
// for a live one.
//
// InteractionID values are monotonically increasing and never reused
// for the lifetime of a Snapshot, even after their occurrence closes —
// this is what guarantees a stale or replayed answer/completion
// submission can never accidentally address a different, later
// occurrence, the same guarantee Slot(+Key)-addressing previously
// relied on the occupied-tuple check for. See Snapshot.NextInteractionID.
//
// Timer occurrences are not addressed by an InteractionID — a caller
// does not answer a timer the way it answers a Question, so
// TimerExpiredSignalSource/KeyedTimerExpiredSignalSource keep
// Signal.Slot(+Key) addressing unchanged.
type InteractionID uint64

// InteractionKind identifies which underlying execution semantics an
// addressed interaction has: an ordinary/keyed Question's
// transition-selecting behavior, or an ordinary/keyed Ask Group's
// non-transition-selecting collect-and-reevaluate behavior. Carried on
// the Output produced when the interaction opens, so a caller never
// needs the compiled Program to classify what it just received by
// looking up which slot collection a name belongs to.
type InteractionKind int

const (
	// InteractionKindQuestion is the zero value: the addressed
	// occurrence is an ordinary or keyed Question.
	InteractionKindQuestion InteractionKind = iota

	// InteractionKindAskGroup: the addressed occurrence is an ordinary
	// or keyed Ask Group.
	InteractionKindAskGroup
)
