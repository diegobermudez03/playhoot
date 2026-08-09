package program

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
