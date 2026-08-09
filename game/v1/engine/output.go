package engine

// Output is one declarative, external action produced by a committed
// Step: the engine describes what should happen — open a question,
// schedule a timer, activate a presentation, publish a projection
// update, report a workflow completion, and so on — without performing
// it. The engine never performs external side effects itself; an
// application layer built on top of engineservice decides how to
// persist, schedule, or deliver each Output.
//
// README.md's "Outputs" section documents the variants this package
// intends to produce. Output variants form a closed set controlled by
// this package, the same way program's Expression, Operation, and
// similar concepts are closed; callers outside the package are expected
// to eventually type-switch over them exhaustively.
//
// This is currently an empty placeholder: this version does not yet
// define any concrete Output variant. Concrete variants are added in a
// later step, once the execution semantics that produce them exist.
type Output struct{}
