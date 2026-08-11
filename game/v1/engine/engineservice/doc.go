// Package engineservice is the engine's public API contract: a thin
// façade exposing exactly the operations LOGICAL_CONTRACT.md describes
// (compile, initialize, step) plus Snapshot persistence, and nothing
// else.
//
// engineservice is the direct analog, for engine, of gameservice for
// program: engine itself holds no behavior and imports nothing but the
// standard library, the same way program holds no behavior beyond its
// own data shapes. engineservice is what actually compiles a
// program.Definition into an engine.Program, creates an engine.Program's
// initial engine.Snapshot, applies an engine.Signal to an
// engine.Snapshot, and persists/restores a Snapshot.
//
// # Where the implementation actually lives
//
// This package intentionally contains almost no logic of its own. Every
// file at this level — compile.go, runtime.go, codec.go — is a short
// wrapper (or, for shared types like Diagnostic and ExecutionError, a
// type alias) around one of three sibling internal packages:
//
//   - engine/internal/compiler owns the whole Compile pipeline: symbol
//     resolution, type checking, expression/operation/control
//     compilation, structured-concurrency and UI validation, and
//     diagnostic collection.
//   - engine/internal/runtime owns NewSnapshot, Step, and expression
//     evaluation: transition selection, operation execution, workflow
//     control, invariant checking, presentation derivation, and
//     execution-error reporting.
//   - engine/internal/codec owns Snapshot's JSON wire format.
//
// A caller depending only on engineservice sees a small, stable set of
// functions and types — Compile, NewSnapshot, Step, Evaluate,
// EncodeSnapshot, DecodeSnapshot, CheckSnapshotCompatibility, Diagnostic
// and its kin, ExecutionError and its kin — without needing to read, or
// be affected by changes to, any of the three internal packages behind
// them. Nothing outside game/v1/engine can import those internal
// packages directly, by Go's own internal-package visibility rule; this
// package is deliberately the only supported way in.
package engineservice
