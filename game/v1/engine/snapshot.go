package engine

// InitializationInput carries the data needed to create the initial
// Snapshot for one new game instance of a Program.
//
// This is currently an empty placeholder: it establishes the shape of
// engineservice.NewSnapshot's contract without yet defining what
// initialization data a new game instance needs (for example, root
// workflow arguments). Fields are added here as workflow-instantiation
// semantics are implemented.
type InitializationInput struct{}

// Snapshot is a plain-data representation of the complete logical
// position of one game instance: global game state, the root and child
// workflow instances, workflow-local state, pending interactions and
// timers, deterministic random state, and the current engine sequence.
//
// A Snapshot is never mutated in place. engineservice.Step takes a
// Snapshot and produces a new one inside its returned Commit; the
// original Snapshot value remains valid and unchanged.
//
// Snapshot is intentionally incomplete: this version does not yet define
// any state a Snapshot actually holds, or how it records which Program
// it belongs to — that identity, and the check that rejects a Snapshot
// used with the wrong Program, is added once Program and Snapshot carry
// real compiled/instance content.
type Snapshot struct{}
