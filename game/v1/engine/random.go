package engine

// RandomState is the engine's deterministic random state.
//
// Per LOGICAL_CONTRACT.md, the engine never reads operating-system
// randomness: RandomState's initial Seed comes from
// InitializationInput, supplied by a caller that does have a legitimate
// randomness source (for example, when a session is created), and every
// random value the engine ever produces is meant to be a pure function
// of this state — the state itself only ever changing through a future
// random-drawing operation's own deterministic advancement, never
// through a fresh external read.
//
// This is currently just the seed: the actual generator algorithm and
// its advancing logic are added once a random-drawing operation is
// compiled.
type RandomState struct {
	Seed uint64
}
