package program

// ViewDeclaration defines one reusable declarative client interface: a
// pure transformation from a typed visible model and client-local state
// into a declarative UI element tree, plus the client-side event handling
// needed to answer questions, emit user intents, and mutate its own local
// state.
//
// A future presentation declaration will mount a view instance, supplying
// its model (typically produced by a ProjectionDeclaration) and creating
// its independent local-state instance. This package only represents the
// authored declaration; it performs no mounting, rendering, or execution.
//
// # Implicit model binding
//
// Every view has one implicit immutable binding, "model", typed as
// ModelType. It is not a regular parameter and is supplied by whatever
// future presentation mounts the view. A view must not directly reference
// authoritative or execution-specific roots such as global, resources,
// workflow-local state, signal, actor, answer, respondent, or viewer — the
// privacy boundary belongs to ProjectionDeclaration, which is responsible
// for producing exactly the client-visible data the view needs, exposed
// through model. This package preserves such out-of-scope references so
// the future compiler can report deterministic diagnostics; it performs
// no such validation itself.
//
// # Client-local state
//
// LocalState reuses the existing StateDeclaration model, but for a view it
// describes client-only state: it is instantiated independently for every
// mounted view instance, mutable only by that client (through
// SetLocalStateAction), initialized when the instance is mounted, retained
// for as long as that instance stays mounted, and discarded on unmount — a
// model update alone must not reset it. Local state is never authoritative,
// is never part of the game snapshot, and is never visible to server
// workflows unless a client explicitly sends it through an
// AnswerQuestionAction or EmitUserIntentAction. Local-state field
// initializers may reference model and pure client-safe built-in
// functions, but must not mutate anything; this package reuses
// StateDeclaration rather than introducing a separate client-state
// declaration type.
//
// # Reactive rendering
//
// A mounted view's element tree is conceptually rederived from Root
// whenever its model or local state changes, or when the instance is
// (re)mounted; a view never manually mutates a rendered element, and this
// package does not define diffing, rendering technology, or delivery
// details.
type ViewDeclaration struct {
	Name       string
	ModelType  TypeReference
	LocalState StateDeclaration
	Root       UIElement
}
