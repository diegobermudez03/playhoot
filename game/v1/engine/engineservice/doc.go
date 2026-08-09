// Package engineservice implements the engine's three operations —
// compile, initialize, and step — on top of the plain data types
// declared in engine.
//
// engineservice is the direct analog, for engine, of gameservice for
// program: engine itself holds no behavior and imports nothing but the
// standard library, the same way program holds no behavior beyond its
// own data shapes. engineservice is what actually compiles a
// program.Definition into an engine.Program, creates an engine.Program's
// initial engine.Snapshot, and applies an engine.Signal to an
// engine.Snapshot. It is free to import both engine and program, and,
// once real compilation/execution logic exists, a private
// engine/internal package for the parts that should not be part of this
// package's own public surface.
//
// This package currently only establishes the shape of that behavior —
// see engine/doc.go and LOGICAL_CONTRACT.md for what is deliberately not
// implemented yet.
package engineservice
