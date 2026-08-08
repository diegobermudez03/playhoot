package program

// SignalPattern identifies one signal schema by name and optionally binds
// fields from that signal's payload into immutable lexical names.
//
// A signal pattern is matched by a TransitionDeclaration. Its bindings are
// available to the transition's guard, operation block, and workflow
// control, all as immutable lexical values — they can never be reassigned
// through a SetOperation.
//
// Generic signal declarations (the schemas signals are matched against)
// are not part of this step; they will come from engine-provided lifecycle
// events, user intents, question outcomes, timers, child-workflow
// completion, and connection/session events added in a later step.
type SignalPattern struct {
	Name     string
	Bindings []SignalBinding
}

// SignalBinding binds Field from a matched signal's payload to the
// immutable lexical name Name.
type SignalBinding struct {
	Field string
	Name  string
}
