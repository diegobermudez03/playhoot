package program

// SignalPattern matches one signal by its typed Source and optionally
// binds fields from that signal's payload into immutable lexical names.
//
// A signal pattern is matched by a TransitionDeclaration. Its bindings are
// available to the transition's guard, operation block, and workflow
// control, all as immutable lexical values — they can never be reassigned
// through a SetOperation.
type SignalPattern struct {
	Source   SignalSource
	Bindings []SignalBinding
}

// SignalSource identifies the schema of a signal a transition can match:
// a platform or lifecycle event, a user intent, or a question answer.
//
// SignalSource is a closed interface. Its marker method is unexported so
// that packages outside program cannot introduce unsupported variants; the
// future compiler can safely exhaust all cases with a type switch.
type SignalSource interface {
	isSignalSource()
}

// NamedSignalSource identifies a platform-defined or lifecycle signal by a
// static name, such as WorkflowStarted, SessionCancelled,
// UserDisconnected, or ParentCancelled.
//
// This package does not declare or validate the schema of named signals;
// the future compiler validates whether the signal exists, which fields it
// exposes, and whether requested bindings are valid.
type NamedSignalSource struct {
	Name string
}

func (NamedSignalSource) isSignalSource() {}

// UserIntentSignalSource matches the signal implicitly produced when a
// user submits the named UserIntentDeclaration.
//
// The signal schema exposed to a matching transition is conceptually the
// reserved field "actor" (the User who submitted the intent) plus every
// parameter declared by the referenced UserIntentDeclaration. The future
// compiler validates that Intent refers to an existing user intent and
// that requested bindings match its fields.
type UserIntentSignalSource struct {
	Intent string
}

func (UserIntentSignalSource) isSignalSource() {}

// QuestionAnsweredSignalSource matches the signal produced when a
// validated answer is accepted for the named question slot owned by the
// current workflow (see QuestionSlotDeclaration).
//
// The signal schema exposed to a matching transition is conceptually the
// reserved fields "respondent" (the answering User) and "answer" (the
// submitted value, typed as the slot's question's response type). Because
// a slot already has a statically declared question, this source does not
// itself name a question. Only validated answers reach a workflow this
// way: the future engine verifies the responding user is the slot's
// recipient, validates the response type, evaluates the question's
// authoritative Validation expression, and creates this signal only if the
// answer is accepted — late, duplicate, malformed, or unauthorized
// responses never produce a workflow signal.
type QuestionAnsweredSignalSource struct {
	Slot string
}

func (QuestionAnsweredSignalSource) isSignalSource() {}

// TimerExpiredSignalSource matches the signal produced when the currently
// pending timer in the named timer slot owned by the current workflow
// (see TimerSlotDeclaration) expires.
//
// Timer-expiration signals expose no source-language payload fields in
// this version, so a valid pattern using this source normally has a nil
// Bindings; the future compiler rejects attempts to bind nonexistent
// fields such as a scheduling time or a runtime timer identifier — that
// identity is engine metadata and is never exposed in the authored
// language. Only a timer instance that is still the current, uncancelled
// pending timer for Slot at expiration time produces this signal; stale,
// cancelled, or superseded timer deliveries never do.
type TimerExpiredSignalSource struct {
	Slot string
}

func (TimerExpiredSignalSource) isSignalSource() {}

// SignalBinding binds Field from a matched signal's payload to the
// immutable lexical name Name.
type SignalBinding struct {
	Field string
	Name  string
}
