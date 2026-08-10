package engine

// View is the compiled representation of one program.ViewDeclaration:
// a pure transformation from a typed visible model and client-local
// state into a declarative UI element tree, plus client-side event
// handling.
//
// The engine only compiles and validates a View; it never mounts,
// renders, or executes it — see program.ViewDeclaration's doc comment.
// Root, and every LocalState field initializer, is compiled against a
// scope containing only the implicit "model" binding (typed ModelType)
// and, within Root, the reserved "local" root (typed by LocalState) —
// never "global", "resources", or any other workflow- or
// execution-specific binding.
type View struct {
	Name       string
	ModelType  Type
	LocalState []StateField
	Root       UIElement
}
