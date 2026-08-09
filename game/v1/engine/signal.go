package engine

// Signal is one runtime input to engineservice.Step: something that
// happened, identified by Name, together with whatever payload Fields
// its schema exposes for binding — see engine.SignalPattern and
// engine.SignalBinding.
//
// A Signal is always consumed by exactly one Step call; Step never
// applies more than one Signal, and the engine does not recursively
// generate and apply further transitions inside one Step.
//
// This is currently narrower than program.SignalSource's full variant
// set: Name only ever identifies a named platform or lifecycle signal
// (see engineservice's named lifecycle signal catalog) — the only kind
// engineservice.NewSnapshot produces, as the "first lifecycle signal"
// LOGICAL_CONTRACT.md requires. A user intent, a question answer, a
// timer expiration, and a child/ask-group/task-group completion are
// added once a future step makes engineservice.Step actually dispatch a
// Signal against a compiled SignalPattern.
type Signal struct {
	Name   string
	Fields map[string]Value
}
