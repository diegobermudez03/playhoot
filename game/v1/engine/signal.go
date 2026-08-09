package engine

// Signal is one runtime input to engineservice.Step: a platform or
// lifecycle event, a user intent, a question answer, a timer expiration,
// or a child/ask-group/task-group completion, matching the shape a
// compiled Program's workflows declared they react to (see
// program.SignalSource).
//
// A Signal is always consumed by exactly one Step call; Step never
// applies more than one Signal, and the engine does not recursively
// generate and apply further transitions inside one Step.
//
// This is currently an empty placeholder: this version does not yet
// define concrete Signal variants or expose constructors for them. A
// later step adds those, mirroring program.SignalSource's variants
// (platform/lifecycle signals, user intents, question answers, timer
// expirations, and child/ask-group/task-group completions) with actual
// runtime payload data.
type Signal struct{}
