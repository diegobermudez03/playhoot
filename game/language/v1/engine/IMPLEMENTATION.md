Wrote by AI, human dev notes added as `Dev note:`

# Implementation notes: game/v1/engine

This document is for people modifying `engine`, `engineservice`, or any of the three internal packages behind it. If you're just consuming the engine, read `README.md` instead.

## Package layout and dependency rules

```
game/v1/engine/                    plain data: Program, Snapshot, Signal, Commit, Output, Value, Type, ... Stdlib only.
game/v1/engine/engineservice/      public API: Compile, NewSnapshot, Step, Evaluate, snapshot codec. Imports engine + the three internal packages below.
game/v1/engine/internal/compiler/  the whole Compile pipeline. Imports engine, program, and internal/runtime (for Evaluate).
game/v1/engine/internal/runtime/   NewSnapshot, Step, Evaluate, and everything execution-time. Imports engine only.
game/v1/engine/internal/codec/     Snapshot's JSON wire format. Imports engine only.
```

`engine` never imports `program` (see its own `doc.go`) — this is the same isolation `program` itself follows: a pure data package, stdlib only. `engineservice` is what actually depends on `program` (it has to, since `Compile` takes a `program.Definition`), plus `engine` and the three internal packages.

`internal/compiler` is the only one of the three internal packages that imports another: it depends on `internal/runtime` for one reason — evaluating a compiled resource's initializer expression once, at compile time (`compile_resources.go`), needs the exact same expression-evaluation semantics `Step` uses at runtime. This is one-directional; `internal/runtime` never imports `internal/compiler`, so there's no cycle. `internal/codec` depends on neither.

Being under `internal/` means only code rooted at `game/v1/engine/` can import `compiler`, `runtime`, or `codec` — that's what lets `engineservice` reach all three while keeping them invisible (and free to change shape) to every other consumer. This was a deliberate move: these three used to live directly inside `engineservice` itself (in an even earlier layout, before that, `engineservice` had no internal split at all), and they were relocated to `engine/internal/...` — not `engineservice/internal/...` — specifically so that `engineservice`'s own package boundary could become a thin façade with nothing behind it that isn't equally invisible to a caller.

## Why `engineservice` is almost entirely one-line wrappers

`compile.go`, `runtime.go`, and most of `codec.go` in `engineservice` contain (almost) no logic — each exported function is a direct call-through to the matching internal package function, and each exported type (`Diagnostic`, `Severity`, `Diagnostics`, `ExecutionError`, `ExecutionErrorCode`) is a Go type alias (`type X = internalpkg.X`), not a redeclaration. This is intentional, for the same reason `program`'s encode/decode/validate aren't methods on `Definition`: it lets the real implementation live in packages that can each depend on whatever they need (`compiler` depends on `runtime`; both depend on `engine`) without `engineservice` itself needing to know or care, and it means a caller depending only on `engineservice` is insulated from internal refactors as long as the façade's own signatures don't change.

The one function in `engineservice` that is **not** a pure wrapper is `CheckSnapshotCompatibility` (`codec.go`) — it has real logic of its own (a recursive walk of the snapshot's whole instance tree, building its own `*ExecutionError`), which is why it's tested directly in `engineservice`'s own test suite rather than in `internal/codec`'s.

### Where each package's tests actually live

This split is also why the test suite is organized by *what's actually being tested*, not by *which file happens to declare the wrapper*:

- **`internal/compiler`**'s own tests (`compiler_test` package) call `compiler.Compile` directly and assert on `Diagnostics`/`engine.Program` shape — no `engineservice` involved.
- **`internal/runtime`**'s own tests (`runtime_test` package) call `runtime.Step`/`NewSnapshot`/`Evaluate` directly (using either a hand-built `engine.Program`, bypassing the compiler entirely, or `compiler.Compile` as a fixture when a real compiled program is more convenient to set up).
- **`engineservice`**'s own tests (`engineservice_test` package) are deliberately few: `CheckSnapshotCompatibility` (real logic), and the full `Compile -> NewSnapshot -> Step` integration tests, which exist to verify the *composition* — that a `compiler`-produced `Program` actually plugs into `runtime`'s `NewSnapshot`/`Step` correctly end to end — not to re-test either package's internals in isolation.

If you add a test that only exercises one wrapped call's logic (e.g. "does `Compile` reject a duplicate type name"), it belongs in `internal/compiler`'s test package, calling `compiler.Compile` directly — not in `engineservice`, even though `engineservice.Compile` would technically also work. Reserve `engineservice`'s own tests for genuine cross-package composition or for `CheckSnapshotCompatibility`.

## The compiler (`internal/compiler`)

`compiler.Compile` walks a `program.Definition` roughly in dependency order: types (so later phases can resolve type references), functions, resources (evaluated once via `runtime.Evaluate`, hence the one dependency edge above), global state, invariants, then the four "simple named catalog" declarations — questions, effects, projections, views — and finally workflows (the most involved phase: slots, presentations, states, transitions, operations, expressions, control, structured-concurrency and UI-binding validation). It never stops at the first problem; every phase keeps going and reports every `Diagnostic` it can, via `(c *compiler) addf(path, format, args...)`.

### File organization

Files are grouped by what they compile, one file (or a small merged group) per closed-interface family or catalog:

- `compile.go` — the `compiler` struct, `Compile`'s top-level orchestration, `Diagnostic`/`Severity`/`Diagnostics`.
- `compile_symbols.go`, `compile_types.go` — type declaration registration and resolution.
- `compile_functions.go` — functions, with cycle-safe memoized resolution (a function may call another function; recursion, direct or through a cycle, is a diagnostic, not a compile-time hang).
- `compile_resources.go`, `compile_state.go` — resources (evaluated once, via `runtime.Evaluate`) and global state.
- `compile_declarations.go` — questions, effects, projections, and views: four catalogs that, unlike functions, need no cycle detection or cross-referencing between entries, so they're compiled by four sibling functions in one file (mirroring `engine/declarations.go`'s own grouping of the compiled `Function`/`Question`/`Projection`/`Effect` types).
- `compile_workflows.go`, `compile_slots.go`, `compile_control.go`, `compile_ask_groups.go`, `compile_task_groups.go`, `compile_signals.go`, `compile_calls.go`, `compile_match.go`, `compile_operations.go`, `compile_expressions.go`, `compile_ui.go` — the workflow-compilation phase, split by concern (slot declarations, control-flow/structured-concurrency validation, the two group kinds, signal-pattern matching, call-argument binding, match patterns, operations, expressions, UI/projection bindings).
- `compile_presentations.go` — kept separate from `compile_declarations.go` deliberately: a `Presentation` is owned by a `Workflow`/`WorkflowState`, not a standalone `Program`-level catalog the way the other four are, so it doesn't share their shape or their compile-order constraints.

`compile_functions.go` is likewise kept out of `compile_declarations.go` for the same reason in reverse: it's the one catalog with cross-referencing and cycle-detection logic the other four don't need.

### `exprScope`

Every expression-compiling function threads an `exprScope` (a `map[string]engine.Type` keyed by reserved root name or parameter name) that mirrors, at compile time, exactly what `engine.Scope` will hold at runtime for the same expression — this is what lets the compiler statically reject a reference to a name that won't exist in scope, or a type mismatch, before `runtime.Evaluate` ever runs. `engine.GlobalScopeRootName`/`engine.ResourcesScopeRootName` (in `engine/scope.go`) are the two reserved root names both `compiler` and `runtime` need to agree on by exact string value; they're public `engine` constants specifically so the two internal packages — which cannot see each other's private constants — stay in sync without duplicating the literal string in two places.

## The runtime (`internal/runtime`)

### File organization

- `step.go` — `ExecutionErrorCode`/`ExecutionError`, `NewSnapshot`, `Step` itself (transition selection, guard evaluation, control application, invariant checking, commit assembly), and the instance-tree helpers `Step` needs (`resolveInstance`/`applyInstancePath` walk a `Signal.Path` down through `ChildSlots`/`TaskGroupSlots`).
- `execute.go` — `execContext` (one `Step` call's mutable working state: the target instance's slots, local state, accumulated outputs/internal signals, operation/budget counters) and every `exec*` method that runs one compiled `Operation` against it.
- `evaluate.go` — `Evaluate` and every `eval*` method: the pure expression-evaluation half, used both by `Step` (guards, operation values, control conditions) and directly by `engineservice.Evaluate`.
- `ask_group.go`, `task_group.go` — the two structured-concurrency group kinds, each with their own operations, completion-policy resolution, and slot bookkeeping. Kept as two separate, structurally parallel files rather than merged: they represent genuinely different capabilities (one ask, many possible respondents, vs. many dynamically spawned homogeneous children), even though their file shapes look similar.
- `presentation.go` — `deriveActivePresentations`/`diffPresentations`: recomputing which presentations should be active after a transition and turning the diff into `Activate`/`Update`/`RemovePresentationOutput` values.

### `Step`'s commit sequence, and where structured-concurrency cleanup happens

Inside `Step` (`step.go`), once a transition's guard passes and its operations have run without exceeding any `Limits`, every compiled `Invariant` is checked against the resulting global state — atomically; a violation aborts the whole `Step` with `ExecutionErrorInvariantViolation` and no `Commit` is produced. Only after that does the new target instance get built, and — the one non-obvious piece of runtime behavior worth knowing before you touch this code — if this instance's own control just produced a terminal `WorkflowOutcome`, every child slot, ask-group slot, and task-group slot it owns (running or terminal-awaiting-join) is discarded wholesale via `clearedChildSlots`/`clearedAskGroupSlots`/`clearedTaskGroupSlots`, and exactly one `engine.WorkflowCompletedOutput` is appended to the commit's outputs. This is a deliberate design choice, not a missing feature: per `program.ChildWorkflowSlotDeclaration`'s own doc comment, a child "disappears when the parent workflow terminates" — nothing can ever join a slot belonging to an instance that will never run another transition, so there is no "must join or cancel children before completing" rule to enforce; completing a parent simply, silently, sweeps its whole owned subtree.

`WorkflowCompletedOutput` is produced exactly once per `Step` call, for the one instance whose control just terminated — never separately for a descendant swept away along with it, since those were never independently observed to reach their own outcome; the parent (if any) already observes a child's outcome the ordinary way, through `ChildCompletedSignalSource`/`ChildFailedSignalSource`/`ChildCancelledSignalSource`/the two group-completed signal sources, ahead of ever discarding it.

### Two error sentinels vs. one error type

`ErrSignalRejected` and `ErrInputRejected` (`step.go`) are pre-built `*ExecutionError` values meant to be compared via `errors.Is`, not constructed fresh per call site — they represent two structurally different "this signal produced nothing" outcomes (see `README.md`'s explanation of each) that callers are expected to actually distinguish, unlike every other `ExecutionErrorCode`, which is typically just logged or switched on for diagnostics. `(*ExecutionError).Is` compares by `Code`, not pointer identity, so a freshly constructed `*ExecutionError{Code: ExecutionErrorSignalRejected, ...}` still satisfies `errors.Is(err, ErrSignalRejected)`.

`ExecutionErrorCode`'s own doc comment states the append-only contract explicitly: a new code always goes at the end of the `const` block, an existing one's name is never reused for a different meaning, and `(ExecutionErrorCode) String()` must grow a matching `case` for every addition — `internal/runtime/limits_trace_test.go`'s `TestExec_ExecutionErrorCodeStringIsExhaustiveAndStable` enumerates every current code and will fail to compile-and-pass if a new one is added without a matching `String()` case, so keep that test's list in sync when you add a code.

## Closed interfaces: the `isX()` marker pattern

Exactly the same "sealed interface" pattern as `program` (see `program/IMPLEMENTATION.md`): every `engine` interface representing a fixed variant set (`Value`, `Type`, `Expression`, `Operation`, `WorkflowControl`, `SignalSource`, `Output`, `UIElement`, `UILayout`, `UIAction`, `MatchPattern`, `AskGroupCompletionPolicy`, `TaskGroupCompletionPolicy`) has a single unexported marker method, implemented with a value receiver by every concrete variant. `engine`'s own switches over these are compiled once, in `internal/compiler` (compile-time validation) and `internal/runtime` (`execute.go`/`evaluate.go`, runtime dispatch) — both are exhaustive, and both `default` branches return a real `*ExecutionError{Code: ExecutionErrorUnknown, ...}` rather than silently no-op-ing, so a missing case surfaces as a runtime error immediately rather than as quietly wrong behavior. When you add a new variant to one of these interfaces, you must update every one of: the compiler's validating switch, the runtime's executing/evaluating switch, and (if the variant round-trips through a persisted `Snapshot`, which so far only `Value` does) `internal/codec`.

`engine`'s own root-level files are organized by entity, the same convention `program` uses (see `program/IMPLEMENTATION.md`'s "File organization"): one file per concept, holding every type and marker-method implementation for that concept together (`ask_group.go`, `task_group.go`, `control.go`, `signal_pattern.go`, and so on). `declarations.go` holds `Function`, `Question`, `Projection`, and `Effect` together deliberately — they're the engine-side mirror of `internal/compiler`'s `compile_declarations.go` grouping (see above): each is compiled exactly once, keyed by name, into one of `Program`'s own catalog maps, with no cycle detection needed among them.

## `engine/internal/codec`: what it actually persists

`internal/codec` only knows how to encode/decode `Snapshot` and everything reachable from it (`WorkflowInstance` and its slots, `Value` and every variant, `RandomState`). It does **not** know about `Program`, `Diagnostic`, or `ExecutionError` — there is no wire format for any of those, by design (see `README.md`'s "Persisting a Snapshot"). If you add a new field to `Snapshot` or a new `Value` variant, it needs a matching change here; if you add a new `Output` variant, a new `ExecutionErrorCode`, or anything to `Program`, it does not — those are never persisted, only ever returned transiently from one `Step`/`Compile` call.

## Testing conventions

- `engine`'s own root package has no test files: it is plain data, no logic worth testing — the same reasoning `program`'s own near-empty test suite follows (test behavior, not structs that just hold fields).
- `internal/compiler`, `internal/runtime`, and `engineservice` each use the external `_test` package convention (`compiler_test`, `runtime_test`, `engineservice_test`) — this is the same pattern `program/gameservice` already established: tests can only reach the exported surface of the package under test, which keeps them honest about what's actually part of the contract.
- A handful of small fixture helpers (`numberType()`/`boolType()`/`withMinimalRootWorkflow()`, and similar) are intentionally duplicated in more than one test package rather than shared through a non-test helper package — Go test files can't share unexported code across package/directory boundaries, and introducing a shared helper package just for tests was judged not worth the indirection for a handful of few-line functions. If you need a new one in a third place, duplicate it rather than trying to unify all three.
- Replay/determinism tests (see `internal/runtime/limits_trace_test.go`'s `TestExec_RetryAfterFailureIsSafeAndReproducible`) verify that retrying an identical `Step` call after a failure reproduces the identical error — this is the concrete check behind the determinism guarantee in `LOGICAL_CONTRACT.md` and `README.md`.

## Adding a new execution concept, end to end

1. Add the compiled shape to `engine` (a new or existing entity file), with an `isX()` marker method if it's a new variant of a closed interface.
2. Add compile-time validation to the matching `internal/compiler` file (or a new one, following the existing grouping conventions above), and extend any exhaustive switch that now needs a new case.
3. Add runtime handling to `internal/runtime` (`execute.go` for a new `Operation`, `evaluate.go` for a new `Expression`, or a new file if it's a big enough concern — see `ask_group.go`/`task_group.go` as the precedent for "big enough to deserve its own file").
4. If the concept produces a new `Output` variant or affects `Snapshot`'s shape, update `internal/codec` accordingly — and if it's an `Output`, remember codec does *not* need to know about it (outputs aren't persisted).
5. Add tests in whichever internal package's test suite actually exercises the new logic — not in `engineservice`, unless the change is to genuine cross-package composition (see "Where each package's tests actually live" above).
6. Update `README.md` if callers need to know about it (most things do); update this file if the change affects package layout, dependency direction, or a convention documented here.
