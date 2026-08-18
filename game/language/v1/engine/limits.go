package engine

// Limits bounds one engineservice.Step call's execution:
//
//   - MaxOperations caps the number of synchronous operations it may
//     execute (every operation in every nested Block, including once
//     per loop iteration);
//   - MaxLoopIterations caps the number of iterations any single
//     ForEachOperation may run;
//   - MaxWorkflowDepth caps how deep the child-workflow tree a
//     SpawnChildWorkflowOperation or SpawnTaskGroupChildOperation may
//     grow — the root instance is at depth 0, and spawning into a
//     slot at depth MaxWorkflowDepth-1 would create a child at depth
//     MaxWorkflowDepth, which fails the transition atomically instead;
//   - MaxActiveSlotsPerInstance caps how many occupied interaction
//     slots (pending questions, pending timers, running or
//     awaiting-join children, collecting or awaiting-join ask groups,
//     and individual task-group tasks, summed) a single workflow
//     instance may hold at once.
//
// Every one of these exists for the same reason: an authored — or,
// increasingly, AI-generated — program can describe unbounded work
// (an infinite loop, an unbounded recursive spawn, an unbounded
// fan-out) that the compiler cannot always reject statically. Limits
// is what makes such a program fail deterministically and immediately,
// atomically, at the exact operation that exceeded it, instead of
// hanging or exhausting memory.
//
// Per LOGICAL_CONTRACT.md's determinism guarantee, Limits is one of the
// inputs — together with the compiled Program, Snapshot, and Signal —
// that a Commit is a deterministic function of: the same inputs always
// produce the same Commit, or the same error, including the same
// budget-exceeded error at the same point.
type Limits struct {
	MaxOperations     int
	MaxLoopIterations int

	// MaxWorkflowDepth bounds workflow depth — see the type doc
	// comment. A zero value means the root instance itself already
	// exceeds it, rejecting any spawn.
	MaxWorkflowDepth int

	// MaxActiveSlotsPerInstance bounds active-slot count — see the
	// type doc comment. A zero value rejects occupying any slot at
	// all.
	MaxActiveSlotsPerInstance int
}

// DefaultLimits returns limits generous enough for normal turn-based
// game logic, while still failing a runaway transition — an unbounded
// loop, an oversized collection, an unbounded recursive spawn, an
// unbounded fan-out — quickly instead of hanging or exhausting memory.
func DefaultLimits() Limits {
	return Limits{
		MaxOperations:             10_000,
		MaxLoopIterations:         10_000,
		MaxWorkflowDepth:          8,
		MaxActiveSlotsPerInstance: 256,
	}
}
