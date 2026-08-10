package engine

// RandomState is the engine's deterministic random state.
//
// Per LOGICAL_CONTRACT.md, the engine never reads operating-system
// randomness: RandomState's initial State comes from
// InitializationInput.Seed, supplied by a caller that does have a
// legitimate randomness source (for example, when a session is
// created). From then on, every random value the engine ever produces
// is a pure function of the current RandomState — see Next — advanced
// only by a committed DrawRandomOperation, never by a fresh external
// read. A step that fails for any reason does not advance RandomState;
// see engineservice.Step's atomicity contract.
type RandomState struct {
	State uint64
}

// Next deterministically advances s and returns the new state together
// with the raw 64-bit value drawn, using splitmix64 — a standard,
// well-mixed bit generator whose entire state is one uint64. This
// package does not implement rejection sampling for a bounded range
// (see engineservice's random-generator execution): a random draw's
// bias from a plain modulo reduction is negligible for any range a real
// game would use, and avoiding it would trade a meaningful amount of
// simplicity for a benefit no realistic caller needs.
func (s RandomState) Next() (RandomState, uint64) {
	state := s.State + 0x9E3779B97F4A7C15
	z := state
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	z = z ^ (z >> 31)
	return RandomState{State: state}, z
}

// RandomGenerator is the compiled representation of one
// program.RandomGenerator: how a DrawRandomOperation produces its value.
//
// RandomGenerator is a closed interface, mirroring program's own
// closed-interface pattern.
type RandomGenerator interface {
	isRandomGenerator()
}

// RandomIntegerGenerator produces one uniformly selected integer from
// the inclusive range [Minimum, Maximum].
type RandomIntegerGenerator struct {
	Minimum Expression
	Maximum Expression
}

func (RandomIntegerGenerator) isRandomGenerator() {}

// RandomElementGenerator selects one element of Collection uniformly by
// list position.
type RandomElementGenerator struct {
	Collection Expression
}

func (RandomElementGenerator) isRandomGenerator() {}

// RandomShuffleGenerator produces a uniformly randomized permutation of
// Collection as a new list value.
type RandomShuffleGenerator struct {
	Collection Expression
}

func (RandomShuffleGenerator) isRandomGenerator() {}
