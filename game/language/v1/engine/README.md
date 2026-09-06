Wrote by AI, human dev notes added as `Dev note:`

# game/language/v1/engine

`engine` compiles a `program.Definition` into an immutable, executable `Program`, then runs it: it turns one runtime `Signal` plus one `Snapshot` into a new `Snapshot`, as one atomic `Commit`. It is a pure, deterministic simulation core — no database, no network, no real clock, no OS randomness. Everything it needs comes in as an explicit argument; everything it produces comes out as plain data.

If you just want to *use* the engine, this document is for you. If you're going to modify `engine` itself, read `IMPLEMENTATION.md` instead.

`Dev note: this is one of the implementations of an engine — "program" defines a game-agnostic language, and there could be other engines built on top of it besides this one. engine only knows how to execute a program.Definition; it does not decide how you deliver that to players (HTTP, WebSockets, a CLI, tests, ...) — that's a session/application layer you build on top of engineservice.`

## Where to import from

```go
import (
    "github.com/diegobermudez03/playhoot/game/language/v1/engine"
    "github.com/diegobermudez03/playhoot/game/language/v1/engine/engineservice"
    "github.com/diegobermudez03/playhoot/game/language/v1/program"
)
```

`engine` itself is a pure data package — `Program`, `Snapshot`, `Signal`, `Commit`, `Output`, `Value`, and everything else you read or construct. `engineservice` is where every actual operation lives: `Compile`, `NewSnapshot`, `Step`, `Evaluate`, plus `Snapshot` persistence. You will `import` both in any real caller, exactly the same relationship as `program`/`gameservice`.

The three internal packages behind `engineservice` (`internal/compiler`, `internal/runtime`, `internal/codec`) are not importable from outside `game/language/v1/engine` — Go's own `internal/` visibility rule enforces this. `engineservice` is the only supported way in.

## The three operations

```
program.Definition                     -> engineservice.Compile        -> engine.Program, engineservice.Diagnostics
engine.Program + InitializationInput   -> engineservice.NewSnapshot     -> engine.Snapshot, engine.Signal, error
engine.Program + Snapshot + Signal     -> engineservice.Step            -> engine.Commit, error
```

A typical caller's lifecycle:

```go
p, diags := engineservice.Compile(def)
if diags.HasErrors() {
    // def has at least one SeverityError diagnostic — p must not be executed.
    return diags
}

snap, startSignal, err := engineservice.NewSnapshot(p, engine.InitializationInput{
    RootParameters: map[string]engine.Value{ /* ... */ },
    Seed:           mySessionSeed, // your own source of real unpredictability, drawn once
})
if err != nil {
    return err
}

commit, err := engineservice.Step(p, snap, startSignal, engine.DefaultLimits())
if err != nil {
    // snap is untouched; no Commit was produced; nothing was published.
    return err
}
snap = commit.Snapshot
// persist snap, deliver commit.Outputs, and loop: Step again with the next Signal.
```

### `Compile(def program.Definition) (engine.Program, engineservice.Diagnostics)`

Validates `def` and produces its immutable, executable representation. `Compile` never panics and never stops at the first problem — it collects every `Diagnostic` it can find and returns them alongside the result.

- `Diagnostics` is an ordered `[]Diagnostic`; each has `Severity` (`SeverityError`, `SeverityWarning`, `SeverityInfo`), a `Path` (dotted/bracketed, like `"$.workflows[0].states[2].transitions[0]"`, matching `program/gameservice`'s own error format), and a `Message`.
- `diags.HasErrors()` reports whether any entry is `SeverityError`. **If it returns true, the returned `Program` must not be executed** — treat it as unusable, not as "mostly fine."
- `Compile` does **not** assume `def` already passed `gameservice.Validate` — it re-derives everything it needs (type registration, name resolution, duplicate detection) independently. Running `gameservice.Validate` first is still worthwhile for faster, narrower-scoped feedback during authoring, but it is never required before calling `Compile`.
- A `Program` is immutable and safe to share — one compiled `Program` can back any number of concurrent `Snapshot`s:

  ```
  Parques Program v1
  ├── Snapshot A  (table 1)
  ├── Snapshot B  (table 2)
  └── Snapshot C  (table 3)
  ```

### `NewSnapshot(p engine.Program, input engine.InitializationInput) (engine.Snapshot, engine.Signal, error)`

Creates the initial `Snapshot` for one new game instance of `p`: binds and validates `input.RootParameters` against the root workflow's declared parameters, evaluates its local state and declared slots (all empty), evaluates every global-state field, and checks every compiled invariant against the result — atomically. If anything fails (a bad parameter, a violated invariant), no `Snapshot` is returned at all.

`input.Seed` seeds the instance's deterministic random state. The engine never reads OS randomness — if your game uses `DrawRandomOperation` anywhere, draw a real seed from your own legitimate entropy source once, when the session starts, and pass it here. Every random value the engine ever produces for that instance afterward is a deterministic function of that one seed plus every signal it's given.

The returned `Signal` is the mandatory first input to `Step` — it is how the root workflow instance actually starts running (matching a `WorkflowStarted` transition, if the root workflow declares one). Don't discard it.

### `Step(p engine.Program, snapshot engine.Snapshot, signal engine.Signal, limits engine.Limits) (engine.Commit, error)`

Applies exactly one `Signal` to `snapshot` and returns the result as one atomic `Commit`. `Step` never mutates `snapshot` in place — on success, the new state is `commit.Snapshot`; on failure, `snapshot` is guaranteed unchanged, no `Commit` was produced, and nothing in it should be treated as published.

Internally, one successful `Step` call: resolves the target instance from `signal.Path`, selects one matching transition, evaluates its guard, runs its operations (bounded by `limits`), applies its control result, checks every invariant, recomputes affected presentations, and returns everything as one `Commit`. The engine never runs more than one transition per `Step` call — it does not recursively chain transitions internally, even when a transition's own operations produce further signals (see `Commit.InternalSignals` below).

`limits` bounds one `Step` call's work — `engine.DefaultLimits()` is generous enough for ordinary turn-based logic while still failing a runaway transition (an unbounded loop, an unbounded recursive spawn) deterministically instead of hanging. Pass your own `engine.Limits` if you need tighter or looser bounds.

#### Reading a `Commit`

```go
type Commit struct {
    Snapshot        engine.Snapshot
    Outputs         []engine.Output
    InternalSignals []engine.Signal
    Trace           engine.Trace
    ConsumedSignal  engine.Signal
}
```

- **`Snapshot`** — persist this; it's the new authoritative state.
- **`Outputs`** — declarative, external actions to actually perform: open a question, schedule a timer, activate/update/remove a presentation, emit a client effect, report a workflow's completion. The engine never performs any of these itself (see "Outputs" below) — that's your job.
- **`InternalSignals`** — signals the engine itself needs applied next, in a *separate* `Step` call (for example, the `WorkflowStarted` signal a freshly spawned child workflow needs). Feed each one back through `Step` yourself; the engine will never do this for you within the same call.
- **`Trace`** — a debugging/explanation record of exactly what this one `Step` call did: which transition ran, its guard result, the state change, any terminal outcome, and how many operations it ran. Not consumed by anything downstream — it's for logs, replay verification, and tooling.
- **`ConsumedSignal`** — the `signal` you passed in, echoed back for convenience.

#### When `Step` returns an error instead

Two of `Step`'s possible errors are *expected, non-bug outcomes*, not something to alert on: a stale or unmatched signal simply produced no `Commit`.

- **`engineservice.ErrSignalRejected`** — nothing in the target instance's compiled workflow was willing to react to this signal at all (no transition matched, or the one that did had a false guard) — including a `signal.Path` that no longer names a running instance.
- **`engineservice.ErrInputRejected`** — something *was* willing to react to a signal of this shape, but its concrete payload failed authoritative validation (a stale/duplicate answer, an unauthorized respondent, an answer that doesn't satisfy the question's response type or `Validation` expression, an expired timer that was already cancelled, and so on).

Use `errors.Is(err, engineservice.ErrSignalRejected)` / `errors.Is(err, engineservice.ErrInputRejected)` to distinguish these from a real problem. Anything else is an `*engineservice.ExecutionError`, with a stable `.Code` (`engineservice.ExecutionErrorCode`) you can switch on or log — invariant violations, budget/limit overruns, division by zero, an occupied slot, and so on. See `internal/runtime/step.go`'s `ExecutionErrorCode` constants for the full, documented set; new codes are only ever appended, never renumbered or reused for a different meaning.

### `Evaluate(p engine.Program, expr engine.Expression, scope engine.Scope) (engine.Value, error)`

Evaluates a single compiled `Expression` against an arbitrary `Scope`, using the exact same pure-expression semantics `Step` uses internally for guards, operation values, and workflow control. Ordinary game execution never needs this directly — it exists for tooling, diagnostics, or anything that needs to evaluate an expression pulled out of a `Program` outside of a `Step` call.

## Outputs

`engine.Output` is a closed set of declarative actions — the engine describes what *should* happen without ever doing it itself:

| Output | Meaning |
| --- | --- |
| `OpenQuestionOutput` | a question was opened for one recipient in a named slot |
| `CloseQuestionOutput` | a pending question in a named slot was closed |
| `ScheduleTimerOutput` | a timer should fire after `DelayMilliseconds` — you own real scheduling and must deliver the matching `TimerExpiredSignalSource` signal back through `Step` when it fires |
| `CancelTimerOutput` | a pending timer was cancelled |
| `EmitEffectOutput` | a presentation-only client effect (animation, sound) fired for one or more recipients — losing this changes nothing about authoritative state |
| `ActivatePresentationOutput` | a presentation was newly mounted for one recipient, with its view name and computed model |
| `UpdatePresentationOutput` | an already-active presentation's computed model changed |
| `RemovePresentationOutput` | an active presentation was unmounted |
| `WorkflowCompletedOutput` | a workflow instance reached a terminal outcome (`Completed`/`Failed`/`Cancelled`) — for the root instance (`Path` empty), this is the only way to observe the whole game instance ending, since there's no parent to notify through a signal |

Type-switch over `engine.Output` exhaustively; the set is closed the same way `program.Expression`/`program.Operation` are — you can't add your own variant from outside the package.

## Determinism

Given the same compiled `Program`, `Snapshot`, `Signal`, and `Limits`, `Step` always returns the same `Commit` (or the same error). The engine never reads the system clock, the network, environment variables, or OS randomness — time enters only through explicit signal data (e.g. a `TimerExpiredSignalSource` you deliver), and randomness only through the deterministic `RandomState` carried inside `Snapshot`, seeded once via `InitializationInput.Seed`. This is what makes replay, simulation, and debugging possible: the same recorded sequence of signals against the same initial snapshot always reaches the same final state.

## Concurrency

A compiled `Program` is safe to share and read concurrently across any number of game instances. `engine`/`engineservice` do **not** serialize calls against the same `Snapshot` sequence for you — if two `Step` calls could race against the same game instance, your own session layer owns that ordering (locking, an actor per instance, optimistic concurrency, whatever fits). The engine only defines the deterministic result of one step in isolation.

## Persisting a `Snapshot`

```go
data, err := engineservice.EncodeSnapshot(snap)
// ... persist data ...
restored, err := engineservice.DecodeSnapshot(data)
if err := engineservice.CheckSnapshotCompatibility(p, restored); err != nil {
    // restored references a workflow p doesn't compile — e.g. resuming
    // against a newer Program version that dropped or renamed one.
}
```

- **`EncodeSnapshot`/`DecodeSnapshot`** — compact JSON, structurally strict on decode (`*engineservice.DecodeError` carries a path to exactly where decoding failed, same style as `Diagnostic.Path`).
- **`CheckSnapshotCompatibility`** — call this after `DecodeSnapshot` (or after recompiling a `program.Definition` to a newer `Program` version) before resuming a persisted `Snapshot` against it. It's the same check `Step` itself makes internally, exposed standalone so you can validate once, up front, instead of discovering the mismatch mid-step.

There is deliberately no codec for `Program` itself. A compiled `Program` is a pure, deterministic function of the `program.Definition` `Compile` was given — persist (or version-reference) the `Definition` through `program/gameservice`'s own codec, and recompile on load. Recompiling is cheap, deterministic, and avoids maintaining a second wire format for the same information.

## What you get back from a compiled `Program`

`Program` (and the types reachable from it — `Workflow`, `Type`, `Value`, ...) is read-only, plain data — inspect it freely, but never construct or mutate the compiled forms of these by hand outside of tests. The fields you'll actually reach for:

- `Program.Metadata` — the game version's identity (carried over unchanged from `program.Definition.Metadata`).
- `Program.Types`, `.Functions`, `.Resources`, `.Questions`, `.Effects`, `.Projections`, `.Views`, `.Workflows` — every one of `def`'s catalogs, compiled and keyed by declared name.
- `Program.RootWorkflow` — the workflow name `NewSnapshot` starts.
- `Snapshot.GlobalState`, `Snapshot.Root` (a `WorkflowInstance`, and through its `ChildSlots`/`TaskGroupSlots`, the whole child-workflow tree) — inspect these to build your own read models, admin tooling, or debugging views, using the exported `Value` variants (`BoolValue`, `NumberValue`, `RecordValue`, ...) and `Value.Equal`/`.Validate`.

You will not typically construct `engine.Program`/`engine.Workflow`/`engine.Expression`/... values by hand in real code — those come from `Compile`. Building them directly (as the engine's own tests do, to exercise runtime behavior independently of the compiler) is a testing technique, not the intended integration path.

## What this package does not do

No database, no session/room management, no HTTP/WebSocket/gRPC delivery, no real timer scheduling, no output publication, no signal ordering across concurrent requests, no retries. Every one of those belongs to an application/session layer you build on top of `engineservice` — the engine only ever defines the deterministic result of one `Compile`, one `NewSnapshot`, or one `Step` call.
