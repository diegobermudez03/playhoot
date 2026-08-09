// Package engine declares the plain data shapes that describe one
// compiled game version and one step of its execution: Program,
// Snapshot, Signal, Commit, Output, and Trace.
//
// Like program, engine is a pure data model: it has no behavior of its
// own, performs no I/O, and imports nothing but the standard library.
// Compiling a program.Definition into a Program, creating a Program's
// initial Snapshot, and applying a Signal to a Snapshot are all
// operations, not data — they belong to the sibling package
// engineservice, built on top of these types, the same way encoding,
// decoding, and validating a program.Definition belong to
// program/gameservice rather than to program itself.
//
// # Core operations (owned by engineservice)
//
//	program.Definition                    -> engineservice.Compile     -> engine.Program, engineservice.Diagnostics
//	engine.Program + InitializationInput  -> engineservice.NewSnapshot -> engine.Snapshot, error
//	engine.Program + Snapshot + Signal    -> engineservice.Step        -> engine.Commit, error
//
// This file, and the package as a whole, currently only establish the
// engine's public boundary. It intentionally does not yet implement real
// language semantics — symbol resolution, type checking, control-flow
// validation, expression evaluation, or workflow execution. Those are
// added incrementally, inside engineservice and a future private
// engine/internal package, without changing the shape of this boundary.
// See LOGICAL_CONTRACT.md for the constraints that work must preserve.
//
// # Determinism and non-responsibilities
//
// The package performs no I/O. It does not read the system clock, the
// network, environment variables, or operating-system randomness, and it
// does not manage database transactions, sessions, rooms, WebSocket or
// HTTP delivery, or persistence. Those belong to a future application
// layer built on top of engineservice.
//
// # Dependency rules
//
// engine imports nothing but the standard library — the same rule
// program follows. engineservice may import both engine and program (and,
// once real compilation/execution logic exists, its own private
// engine/internal package). engine and program must never import
// engineservice, or each other.
package engine
