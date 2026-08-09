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

// DrawRandomOperation draws one value from Generator and introduces it as
// an immutable lexical binding named Name.
//
// The binding behaves lexically like the one introduced by LetOperation:
// it is immutable, visible to later operations in the same block and to
// nested blocks, does not escape the block it was declared in, and can
// never be assigned through SetOperation. Its type is inferred entirely
// from Generator (see RandomIntegerGenerator, RandomElementGenerator, and
// RandomShuffleGenerator) — this operation has no explicit Type field. The
// future compiler validates non-empty and non-duplicate binding names,
// shadowing rules, generator expression types, use-before-declaration, and
// lexical scope; this package performs none of that validation.
//
// # Deterministic, snapshot-owned random state
//
// Every game snapshot conceptually owns one authoritative random stream
// that every DrawRandomOperation in the game draws from — there are no
// per-client, per-workflow, per-task, or otherwise manually named random
// streams, and child workflows and task-group children draw from the same
// stream as their parent rather than an independent one. Given the same
// compiled program, input snapshot, input signal, and execution limits,
// the future engine must always generate the same random values and the
// same resulting commit; randomness must never depend on wall-clock time,
// operating-system randomness observed during a step, goroutine
// scheduling, process identity, network or database timing, or a
// client-supplied value. The source language exposes no random seed,
// generator state, or stream position — seed creation belongs to future
// engine or session initialization.
//
// Draw order follows execution order: operations run in source order
// within a block, a draw inside ForEachOperation consumes randomness once
// per iteration in list order, and a draw inside one branch of an
// IfOperation consumes randomness only when that branch executes. Because
// one game instance's transitions are eventually processed serially, the
// persisted signal and commit order fixes the authoritative draw sequence.
//
// # Atomicity, retry, and replay
//
// Random-state advancement is part of the candidate transition: if the
// transition later fails for any reason (an invalid operation, an
// occupied slot, an invalid integer range, an empty random-element
// collection, an execution-budget violation, an invariant violation, or
// any other engine execution error), no state mutation, output, or
// workflow-control change commits, and authoritative random state does
// not advance either. Retrying the same program, snapshot, and signal
// must therefore reproduce the same random values — this is what makes
// deterministic retries, replay, simulation, and debugging possible.
//
// # Server authority
//
// Random results are always server-authoritative: a client may request an
// action (for example "roll the dice"), but the workflow transition that
// handles the request is what executes DrawRandomOperation, and the
// client never supplies the authoritative random values themselves. The
// resulting value may then be stored in authoritative state, exposed
// through a projection, emitted through a transient effect, passed to a
// child workflow, or returned as a workflow result.
//
// # Prohibited contexts
//
// Because a draw advances authoritative state, it is only valid inside a
// workflow transition's synchronous operation Block — never inside a
// resource initializer, a state field initializer, a pure function body, an
// invariant condition, a projection body, a question's Validation
// expression, a view expression, a presentation's Targets or
// ProjectionArguments, or a UIAction. If one of those contexts needs a
// random value, an authoritative transition must generate it first and
// pass or store it explicitly; this package adds no random Expression
// variant and no random pure function.
type DrawRandomOperation struct {
	Name      string
	Generator RandomGenerator
}

func (DrawRandomOperation) isOperation() {}
