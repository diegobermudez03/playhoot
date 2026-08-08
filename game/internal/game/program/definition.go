package program

// Definition is the root source-level representation of an authored game.
//
// Definition is intentionally incomplete: questions, projections, views,
// and invariants will be added in later steps.
//
// Declarations are stored in slices rather than maps to preserve author
// order. Order matters for deterministic diagnostics, and duplicate names
// must remain representable until semantic compilation resolves them.
type Definition struct {
	Metadata Metadata
	Types    []TypeDeclaration

	// Resources declares the game's immutable, program-level data. See
	// ResourceDeclaration for its semantics.
	Resources []ResourceDeclaration

	// GlobalState declares the mutable state instantiated for every new
	// game session. See StateDeclaration for its semantics.
	GlobalState StateDeclaration

	// RootWorkflow names the workflow used to start a game instance. The
	// future engine compiler validates that it refers to a declared
	// workflow in Workflows.
	RootWorkflow string

	// Workflows declares the game's finite-state processes. See
	// WorkflowDeclaration for its semantics.
	Workflows []WorkflowDeclaration
}
