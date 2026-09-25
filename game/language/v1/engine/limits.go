package engine

// Limits bounds one engineservice Turn's execution:
//
//   - MaxOperations caps the number of synchronous operations one
//     internal Step call may execute (every operation in every nested
//     Block, including once per loop iteration);
//   - MaxLoopIterations caps the number of iterations any single
//     ForEachOperation may run;
//   - MaxActiveSlotsPerInstance caps how many occupied interaction
//     slots (pending questions, pending timers, and collecting or
//     awaiting-join ask groups, summed) the one workflow instance may
//     hold at once;
//   - MaxStepsPerTurn caps how many internally-chained Step calls one
//     Turn may require to reach quiescence, draining a prior Step's
//     Commit.InternalSignals — see ExecutionErrorStepChainExceeded.
//
// Every one of these exists for the same reason: an authored — or,
// increasingly, AI-generated — program can describe unbounded work
// (an infinite loop, an unbounded fan-out, an unbounded internal-signal
// chain) that the compiler cannot always reject statically. Limits is
// what makes such a program fail deterministically and immediately,
// atomically, at the exact point that exceeded it, instead of hanging
// or exhausting memory.
//
// Per LOGICAL_CONTRACT.md's determinism guarantee, Limits is one of the
// inputs that engineservice's Turn-level result is a deterministic
// function of: the same inputs always produce the same result, or the
// same error, including the same budget-exceeded error at the same
// point.
type Limits struct {
	MaxOperations     int
	MaxLoopIterations int

	// MaxActiveSlotsPerInstance bounds active-slot count — see the
	// type doc comment. A zero value rejects occupying any slot at
	// all.
	MaxActiveSlotsPerInstance int

	// MaxStepsPerTurn bounds the internal Step-chain count — see the
	// type doc comment. A zero value rejects even the first Step of a
	// Turn.
	MaxStepsPerTurn int
}

// DefaultLimits returns limits generous enough for normal turn-based
// game logic, while still failing a runaway transition — an unbounded
// loop, an oversized collection, or an unbounded internal-signal chain
// — quickly instead of hanging or exhausting memory.
func DefaultLimits() Limits {
	return Limits{
		MaxOperations:             10_000,
		MaxLoopIterations:         10_000,
		MaxActiveSlotsPerInstance: 256,
		MaxStepsPerTurn:           20,
	}
}
