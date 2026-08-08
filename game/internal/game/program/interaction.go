package program

// UserIntentDeclaration declares a typed, unsolicited action that a
// connected user may initiate, such as playing a card or selecting a
// move.
//
// A user intent defines only the authoritative input contract: which
// parameters accompany the action. It does not define which workflow
// handles it, when it is legal, whether the emitting user is authorized,
// or how it affects game state — those decisions belong to workflow
// transitions and guards.
//
// The user who emits the intent is implicit system metadata, not a
// declared parameter. Conceptually, a user intent defines a signal schema
// that exposes a reserved "actor" field alongside its declared parameters,
// and an existing SignalPattern/SignalBinding can bind both: a transition
// might bind "actor" to a lexical name for the emitting user and bind each
// intent parameter by its declared name. This package does not add a new
// signal type for user intents or validate that relationship.
type UserIntentDeclaration struct {
	Name       string
	Parameters []FieldDeclaration
}

// QuestionDeclaration declares a reusable, typed request that an
// authoritative workflow may later open for one or more users.
//
// A question declaration is a contract, not a concrete instance: it does
// not determine target users, timeouts, ask-group membership, how many
// responses are required, or what happens when a user does not answer.
// Those are invocation-level policies added in a later step.
type QuestionDeclaration struct {
	Name string

	// Parameters are immutable values captured when a concrete question
	// instance is opened, describing the context needed to present and
	// validate the question. Different instances of the same question
	// may receive different parameter values.
	Parameters []FieldDeclaration

	// ResponseType is the mandatory type every submitted answer is
	// validated against before any workflow receives it. A question with
	// no meaningful domain result must declare
	// BuiltinTypeReference{Type: BuiltinTypeUnit} rather than leaving
	// ResponseType nil.
	ResponseType TypeReference

	// Validation is an optional, pure expression used for additional
	// authoritative semantic validation of a submitted answer, beyond
	// ResponseType compatibility. A nil Validation means only
	// response-type validation applies.
	//
	// When present, the future compiler evaluates Validation in a scope
	// containing resources, global state, the reserved bindings
	// "respondent" (the answering User) and "answer" (the submitted
	// value, typed as ResponseType), and each declared parameter by name.
	// Validation must be pure, must not mutate state, and must eventually
	// compile to bool; it may reject an answer even when its runtime type
	// matches ResponseType.
	Validation Expression
}

// EffectDeclaration declares a reusable, typed, client-facing presentation
// event produced by authoritative game logic, such as an animation, sound,
// or notification.
//
// An effect declaration defines only its typed payload — not its target
// users, view hierarchy, animation implementation, delivery transport,
// duration, or rendering behavior. Those concerns are added through later
// UI declarations and effect-emission operations.
//
// Effects are presentation-only and never represent authoritative game
// state: if a client misses an effect, the authoritative state remains
// correct and reconstructible from projections or snapshots. This package
// does not model external business side effects such as database writes,
// HTTP requests, payments, emails, or webhooks.
type EffectDeclaration struct {
	Name       string
	Parameters []FieldDeclaration
}
