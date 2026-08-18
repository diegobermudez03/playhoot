package engine

// InitializationInput carries the data needed to create the initial
// Snapshot for one new game instance of a Program.
type InitializationInput struct {
	// RootParameters supplies one argument value per the root
	// workflow's declared Parameters, by name. engineservice.NewSnapshot
	// validates each against its declared Type with Value.Validate
	// before trusting it, since these values come from outside the
	// compiler's control.
	RootParameters map[string]Value

	// Seed seeds the new instance's RandomState. The engine itself
	// never reads operating-system randomness (see
	// LOGICAL_CONTRACT.md); a caller that needs real unpredictability
	// must draw Seed from its own legitimate source (for example, when
	// a session is created) and supply it here, once, so every random
	// value the engine ever produces afterward is a deterministic
	// function of it.
	Seed uint64
}

// Snapshot is a plain-data representation of the complete logical
// position of one game instance: global game state, the root and child
// workflow instances (with their own current state, parameters,
// workflow-local state, and declared runtime slots), deterministic
// random state, and the current engine sequence.
//
// A Snapshot is never mutated in place. engineservice.Step takes a
// Snapshot and produces a new one inside its returned Commit; the
// original Snapshot value remains valid and unchanged. A Snapshot is
// self-contained enough to stop and resume execution between any two
// steps — see LOGICAL_CONTRACT.md — without needing anything beyond
// itself and the Program it belongs to.
//
// Snapshot does not record which Program it belongs to as an explicit
// identity — instead, engineservice.CheckSnapshotCompatibility and
// Step's own internal checks verify compatibility structurally, by
// confirming every workflow name anywhere in the Snapshot's instance
// tree is actually compiled by the Program in hand.
type Snapshot struct {
	// GlobalState is this game instance's mutable global state,
	// evaluated once by engineservice.NewSnapshot from
	// Program.GlobalState and, until a future step adds operations that
	// mutate it, unchanged for the instance's lifetime. Its TypeName is
	// always the reserved scope root name "global" — see
	// program.InvariantDeclaration's documented "global" root — not a
	// name declared in Program.Types.
	GlobalState RecordValue

	// Root is the root workflow instance — see program.Definition's
	// RootWorkflow and Program.RootWorkflow — and, through its
	// ChildSlots and TaskGroupSlots, the root of this game instance's
	// entire child-workflow tree.
	Root WorkflowInstance

	// Random is this game instance's deterministic random state.
	Random RandomState

	// Sequence is the number of steps committed against this game
	// instance so far. engineservice.NewSnapshot always starts it at 0;
	// a future engineservice.Step increments it by exactly one per
	// committed Commit.
	Sequence uint64
}
