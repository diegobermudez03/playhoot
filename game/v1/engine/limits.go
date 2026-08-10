package engine

// Limits bounds one engineservice.Step call's execution: the maximum
// number of synchronous operations it may execute (every operation in
// every nested Block, including once per loop iteration), and the
// maximum number of iterations any single ForEachOperation may run
// before failing the transition atomically.
//
// Per LOGICAL_CONTRACT.md's determinism guarantee, Limits is one of the
// inputs — together with the compiled Program, Snapshot, and Signal —
// that a Commit is a deterministic function of: the same inputs always
// produce the same Commit, or the same error, including the same
// budget-exceeded error at the same point.
type Limits struct {
	MaxOperations     int
	MaxLoopIterations int
}

// DefaultLimits returns limits generous enough for normal turn-based
// game logic, while still failing a runaway transition — an unbounded
// loop, an oversized collection — quickly instead of hanging.
func DefaultLimits() Limits {
	return Limits{MaxOperations: 10_000, MaxLoopIterations: 10_000}
}
