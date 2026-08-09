package program

// RandomGenerator identifies how a DrawRandomOperation produces its value.
//
// A random draw is never represented as a pure Expression or a pure
// function, because drawing advances the authoritative, snapshot-owned
// random state — allowing it inside a pure context would introduce hidden
// state changes into what is supposed to be deterministic, side-effect-free
// evaluation. Randomness may only be produced by DrawRandomOperation
// inside a workflow transition's synchronous operation block.
//
// RandomGenerator is a closed interface. Its marker method is unexported
// so that packages outside program cannot introduce unsupported variants;
// the future compiler can safely exhaust all cases with a type switch.
type RandomGenerator interface {
	isRandomGenerator()
}

// RandomIntegerGenerator produces one uniformly selected integer, typed as
// the built-in number, from the inclusive range [Minimum, Maximum].
//
// Minimum and Maximum must eventually compile to number. The future engine
// additionally requires the evaluated bounds to be finite, integer-valued,
// ordered such that the minimum does not exceed the maximum, and within
// configured numeric limits; invalid runtime bounds fail the entire
// containing transition atomically. This package adds no separate signed,
// unsigned, or floating-point random generator — a decimal random value, if
// ever needed, should be derived from an integer draw through explicit
// pure arithmetic.
type RandomIntegerGenerator struct {
	Minimum Expression
	Maximum Expression
}

func (RandomIntegerGenerator) isRandomGenerator() {}

// RandomElementGenerator selects one element of Collection uniformly by
// list position.
//
// Collection is evaluated exactly once when the operation executes and
// must eventually compile to a finite list; the generator's result type is
// that list's element type (for example, list<ParticipantId> yields a
// ParticipantId). Duplicate values occupy distinct positions and are
// therefore weighted by their number of occurrences — a list [A, A, B]
// gives A twice the positional probability of B. An empty collection is a
// future execution error that fails the entire containing transition
// atomically; this package never wraps the result in an optional, so a
// game that may encounter an empty collection must check it before
// drawing.
type RandomElementGenerator struct {
	Collection Expression
}

func (RandomElementGenerator) isRandomGenerator() {}

// RandomShuffleGenerator produces a uniformly randomized permutation of
// Collection as a new list value, preserving every input element exactly
// once and the input's element type.
//
// Collection is evaluated exactly once, must eventually compile to a
// finite list, and is never mutated by this operation — the shuffled
// result is a new value that must be assigned to authoritative state
// explicitly (for example with SetOperation) if it needs to persist. An
// empty or single-element collection is valid and yields a list with the
// same elements. This package adds no in-place shuffle.
type RandomShuffleGenerator struct {
	Collection Expression
}

func (RandomShuffleGenerator) isRandomGenerator() {}
