Wrote by AI, human dev notes added as `Dev note:`

# game/language/v1/engine

`engine` compiles a `program.Definition` into an immutable, executable `Program`, then runs it, one Turn at a time: given the durable log of every signal already applied plus one new signal, it returns that new Turn's declarative `Output`s. It is a pure, deterministic simulation core — no database, no network, no real clock, no OS randomness. Everything it needs comes in as an explicit argument; everything it produces comes out as plain data. A caller never holds or persists a `Snapshot` — replaying the durable log to reconstruct current state, and chaining any internal signal a Step produces, are both `engineservice`'s own concern, never the caller's (see `game/docs/decisions/GAME-ADR-0027-engine-owned-turn-execution-and-replay.md`).

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

`engine` itself is a pure data package — `Program`, `Signal`, `Output`, `Value`, `InitializationInput`, `Limits`, and everything else you read or construct. `engineservice` is where every actual operation lives: `Compile`, `StartTurn`, `AdvanceTurn`, `Evaluate`. `Snapshot` also lives in `engine`, but it is an internal implementation detail of `StartTurn`/`AdvanceTurn` — a real caller never constructs, holds, or reads one. You will `import` both `engine` and `engineservice` in any real caller, exactly the same relationship as `program`/`gameservice`.

The three internal packages behind `engineservice` (`internal/compiler`, `internal/runtime`, `internal/codec`) are not importable from outside `game/language/v1/engine` — Go's own `internal/` visibility rule enforces this. `engineservice` is the only supported way in.

## The three operations

```
program.Definition                                                          -> engineservice.Compile      -> engine.Program, engineservice.Diagnostics
engine.Program + InitializationInput                                        -> engineservice.StartTurn    -> []engine.Output, error
engine.Program + InitializationInput + []Signal (prior) + Signal (new)      -> engineservice.AdvanceTurn  -> []engine.Output, error
```

A typical caller's lifecycle:

```go
p, diags := engineservice.Compile(def)
if diags.HasErrors() {
    // def has at least one SeverityError diagnostic — p must not be executed.
    return diags
}

start := engine.InitializationInput{
    RootParameters: map[string]engine.Value{ /* ... */ },
    Seed:           mySessionSeed, // your own source of real unpredictability, drawn once
}

outputs, err := engineservice.StartTurn(p, start, engine.DefaultLimits())
if err != nil {
    return err
}
// deliver outputs; durably append start's own record; no engine.Signal is
// yours to construct or store for this first turn.

// Later, for every subsequent turn: load start and every already-committed
// signal back from your own durable storage, in order, then:
outputs, err = engineservice.AdvanceTurn(p, start, priorSignals, newSignal, engine.DefaultLimits())
if err != nil {
    // nothing was published; if replaying priorSignals is what failed, err
    // wraps engineservice.ErrReplayDivergence — a data-integrity condition,
    // never an ordinary decline.
    return err
}
// durably append newSignal to your own signal log, deliver outputs, and
// loop: AdvanceTurn again with the next Signal.
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

### `StartTurn(p engine.Program, start engine.InitializationInput, limits engine.Limits) ([]engine.Output, error)`

Performs a new game instance's mandatory first turn, entirely internally: binds and validates `start.RootParameters` against the root workflow's declared parameters, evaluates its local state and declared slots (all empty), evaluates every global-state field, checks every compiled invariant against the result, and applies the engine's own synthesized first lifecycle signal (matching a `WorkflowStarted` transition, if the root workflow declares one) to quiescence — atomically. If anything fails (a bad parameter, a violated invariant, the first signal itself failing), no `Output`s are returned at all.

You never construct or see the synthesized first signal — it is a fixed, deterministic value with nothing for a caller to decide. Nor do you ever see the `Snapshot` this creates; it exists only inside this call.

`start.Seed` seeds the instance's deterministic random state. The engine never reads OS randomness — if your game uses `DrawRandomOperation` anywhere, draw a real seed from your own legitimate entropy source once, when the session starts, and pass it here. Every random value the engine ever produces for that instance afterward is a deterministic function of that one seed plus every signal it's given. Durably record `start` yourself (you will need it again for every `AdvanceTurn` call) — the engine does not persist anything.

### `AdvanceTurn(p engine.Program, start engine.InitializationInput, priorSignals []engine.Signal, newSignal engine.Signal, limits engine.Limits) ([]engine.Output, error)`

Processes one new turn for an already-started game instance. Internally, `AdvanceTurn` reconstructs current state by replaying `start`'s own initialization and every signal in `priorSignals`, in order, against a freshly initialized instance (discarding their `Output`s — you already durably recorded those the first time each one happened), then applies `newSignal` to quiescence exactly as a single internal transition would. It returns only `newSignal`'s own `Output`s.

`priorSignals` is exactly the ordered log of every signal you have already durably recorded for this instance since it started (never including the implicit first signal `StartTurn` handles for you). You never construct, hold, or read a `Snapshot` to get this log — you already have it, because you recorded each `newSignal` yourself after every prior successful `AdvanceTurn` call.

If replaying `start` or an element of `priorSignals` fails, the returned error wraps `engineservice.ErrReplayDivergence` — every one of those signals already succeeded once (that is why it is durable), so failing to reproduce it identically means your recorded log, the pinned `Program`, or the engine itself no longer agree with what actually happened; this is a data-integrity condition to alert on, never an ordinary decline. A failure of `newSignal` itself surfaces exactly as described below, structurally distinct from a replay divergence.

### Accepted Session Runtime Root Roster Contract

Status: ACCEPTED DESIGN, NOT YET IMPLEMENTED AS A COMPLETE VALIDATED CONTRACT.

When Session Runtime starts a lobby, it supplies the root workflow with:

```text
players: list<user>
```

Session Runtime builds this value from active Participants at Start. Each `user` is the Session-local runtime identity derived from `SessionActorID`, not `Identity.UserUUID`.

Session Runtime must initialize/load the pinned immutable Game definition/version, call `StartTurn`, and persist the initial authoritative runtime state and durable consequences before delivering outputs outside its transaction.

Rationale and alternatives are recorded in `game/docs/decisions/GAME-ADR-0006-game-language-root-player-roster-contract.md`.

### Accepted Disconnect/Reconnect Delivery And Offline-Interaction Invariants

Status: ACCEPTED DESIGN, NOT YET IMPLEMENTED. `UserDisconnected`/`UserReconnected` are accepted as standard `NamedSignalSource` platform/lifecycle signals exposing only `user: user`; nothing in this package's current `AdvanceTurn`/`Signal` handling implements them today. Session Runtime is accepted to deliver both to the one workflow instance a Session runs — there is no other instance to broadcast to, since the engine executes a Session's entire game logic as a single flat workflow instance (see `game/docs/decisions/GAME-ADR-0026-flat-workflow-execution-model-and-keyed-interaction-slots.md`). An authored game may declare no matching transition for either signal; that is an ordinary `ErrSignalRejected` outcome, not an error condition, and Session Runtime must not create a `RuntimeTurn` or infer any gameplay consequence from it.

Independently, Session Runtime opening an interaction/question for a SessionActor with no live transport connection must still produce the engine's normal `OpenQuestionOutput`/interaction behavior unconditionally — engine execution itself has no notion of connectivity, and this accepted invariant constrains the Session Runtime caller, not this package.

Rationale and alternatives are recorded in `game/docs/decisions/GAME-ADR-0011-game-language-disconnect-reconnect-authored-semantics.md`.

### Keyed Interaction Slots

Question, Ask Group, and Timer each have a compiled keyed counterpart — `KeyedQuestionSlot`/`KeyedAskGroupSlot`/`KeyedTimerSlot`, with runtime occupancy tracked in `KeyedQuestionSlotInstance`/`KeyedAskGroupSlotInstance`/`KeyedTimerSlotInstance` — holding several independent, simultaneously pending occurrences per slot at once, addressed by an authored key (any compiled `Type`, exactly like a `MapType` key) instead of the ordinary families' one-occurrence-per-slot limit.

Occupancy identity is `(slot, key)`: opening/scheduling into an already-occupied tuple is an atomic `ExecutionErrorSlotOccupied`, exactly like the ordinary families' occupied-slot behavior, scoped to one key — a different key under the same slot is entirely unaffected. Internally, an authored transition still reacts to a specific slot's `KeyedQuestionAnsweredSignalSource`/`KeyedTimerExpiredSignalSource`/`KeyedAskGroupCompletedSignalSource`, and the occurrence's key is exposed as the bound `"key"` schema field — but see "Answering an Interaction" below for how a *caller* (outside the compiled workflow) actually addresses one of these occurrences; it is never by `Slot`+`Key` directly, except for Timer.

`OpenKeyedQuestionOutput`/`CloseKeyedQuestionOutput`/`ScheduleKeyedTimerOutput`/`CancelKeyedTimerOutput` are the keyed counterparts of their ordinary `Output` equivalents, each carrying an additional `Key` field — see "Outputs" below. A keyed ask group's per-recipient opened questions reuse `OpenKeyedQuestionOutput`/`CloseKeyedQuestionOutput`, exactly like an ordinary ask group already reuses `OpenQuestionOutput`/`CloseQuestionOutput`.

Presentation has no keyed family — deliberately deferred pending a concrete demonstrated need, since its fully declarative, no-open/close-operation shape would need a materially different mechanism than the other three.

Rationale and alternatives for the original Timer case are recorded in `game/docs/decisions/GAME-ADR-0012-game-language-keyed-timer-slots.md`; the generalization to Question/Ask Group/Presentation (and the decision to defer Presentation) is recorded in `game/docs/decisions/GAME-ADR-0026-flat-workflow-execution-model-and-keyed-interaction-slots.md`.

### Answering an Interaction

Every opened Question or Ask Group occurrence (ordinary or keyed) is assigned an `InteractionID` — a unique, monotonically increasing identity the engine assigns itself as part of `Snapshot`'s own deterministic state, carried on the `OpenQuestionOutput`/`OpenKeyedQuestionOutput` that opened it (and on the matching `Close*Output` that closes it) alongside an explicit `Kind` (`InteractionKindQuestion` or `InteractionKindAskGroup`), so a caller never needs the compiled `Program` to classify what it just received.

Answering one is always `Signal{Kind: SignalKindInteractionAnswered, InteractionID: id, Respondent: ..., Answer: ...}` — never a `Slot`, a `Key`, or the caller's own classification of what kind of interaction it is. The engine resolves `InteractionID` alone to the underlying slot (ordinary or keyed), its occurrence `Key` if any, and whether it behaves as a Question (selects and runs a matching transition directly) or an Ask Group (never selects a transition per individual answer — it only records the answer and re-evaluates the group's completion policy, exactly like the ordinary/keyed families already did). Once an Ask Group occurrence is completed-awaiting-join, joining it is `Signal{Kind: SignalKindInteractionCompleted, InteractionID: id}` — Question occurrences never produce this signal, since answering one already selects its own transition directly.

`InteractionID` values are never reused for the lifetime of a `Snapshot`, even after their occurrence closes — a stale, unknown, or already-answered `InteractionID` is rejected the same way a stale `Slot`(+`Key`) submission always was; see `ErrInputRejected`. Timer is not part of this scheme — `SignalKindTimerExpired`/`SignalKindKeyedTimerExpired` keep `Signal.Slot`(+`Key`) addressing unchanged, since a caller does not "answer" a timer the way it answers a Question.

### How a Turn is actually processed, internally

Neither `StartTurn` nor `AdvanceTurn` exposes this — it's here so you understand what one call actually does, not because you need to drive it yourself. Internally, applying one signal ("one Step") to the instance: selects one matching transition, evaluates its guard, runs its operations (bounded by `limits`), applies its control result, checks every invariant, and recomputes affected presentations. A Turn may require more than one such internally-chained Step if a transition's own operations produce further signals for the engine itself to apply (currently never happens in practice — the engine runs a Session's game logic as one flat workflow instance, and nothing in it produces such a signal — but the mechanism exists and is bounded regardless: see `engine.Limits.MaxStepsPerTurn`, exceeding which returns `engineservice.ExecutionErrorStepChainExceeded`). `limits` also bounds the work any one internal Step may do — `engine.DefaultLimits()` is generous enough for ordinary turn-based logic while still failing a runaway transition (an unbounded loop, too many active interaction slots) deterministically instead of hanging.

#### When `StartTurn`/`AdvanceTurn` return an error instead

Two possible errors on `newSignal` (or `StartTurn`'s own first signal) are *expected, non-bug outcomes*, not something to alert on: a stale or unmatched signal simply produced no `Output`s.

- **`engineservice.ErrSignalRejected`** — nothing in the compiled workflow was willing to react to this signal at all (no transition matched, or the one that did had a false guard).
- **`engineservice.ErrInputRejected`** — something *was* willing to react to a signal of this shape, but its concrete payload failed authoritative validation (a stale/duplicate answer, an unauthorized respondent, an answer that doesn't satisfy the question's response type or `Validation` expression, an expired timer that was already cancelled, and so on).

Use `errors.Is(err, engineservice.ErrSignalRejected)` / `errors.Is(err, engineservice.ErrInputRejected)` to distinguish these from a real problem. Anything else is an `*engineservice.ExecutionError`, with a stable `.Code` (`engineservice.ExecutionErrorCode`) you can switch on or log — invariant violations, budget/limit overruns, division by zero, an occupied slot, and so on. See `internal/runtime/step.go`'s `ExecutionErrorCode` constants for the full, documented set; new codes are only ever appended, never renumbered or reused for a different meaning.

`AdvanceTurn` has one more error category, structurally distinct from both of the above: `errors.Is(err, engineservice.ErrReplayDivergence)` means an already-committed element of `priorSignals` (or `start`'s own initialization) failed to reproduce its original result — always a data-integrity condition to alert on, never an ordinary decline, since every one of those signals already succeeded once.

### `Evaluate(p engine.Program, expr engine.Expression, scope engine.Scope) (engine.Value, error)`

Evaluates a single compiled `Expression` against an arbitrary `Scope`, using the exact same pure-expression semantics `StartTurn`/`AdvanceTurn` use internally for guards, operation values, and workflow control. Ordinary game execution never needs this directly — it exists for tooling, diagnostics, or anything that needs to evaluate an expression pulled out of a `Program` outside of Turn processing.

## Outputs

`engine.Output` is a closed set of declarative actions — the engine describes what *should* happen without ever doing it itself:

| Output | Meaning |
| --- | --- |
| `OpenQuestionOutput` | a question was opened for one recipient in a named slot, carrying its `InteractionID`/`Kind` |
| `CloseQuestionOutput` | a pending question in a named slot was closed, carrying the same `InteractionID` it opened with |
| `ScheduleTimerOutput` | a timer should fire after `DelayMilliseconds` — you own real scheduling and must deliver the matching `TimerExpiredSignalSource` signal back through `AdvanceTurn` when it fires |
| `CancelTimerOutput` | a pending timer was cancelled |
| `OpenKeyedQuestionOutput` | a question was opened for one recipient at a named keyed slot's `Key` occurrence, carrying its `InteractionID`/`Kind` |
| `CloseKeyedQuestionOutput` | a pending question at a named keyed slot's `Key` occurrence was closed, carrying the same `InteractionID` it opened with |
| `ScheduleKeyedTimerOutput` | a timer at a named keyed slot's `Key` occurrence should fire after `DelayMilliseconds` |
| `CancelKeyedTimerOutput` | a pending timer at a named keyed slot's `Key` occurrence was cancelled |
| `EmitEffectOutput` | a presentation-only client effect (animation, sound) fired for one or more recipients — losing this changes nothing about authoritative state |
| `ActivatePresentationOutput` | a presentation was newly mounted for one recipient, with its view name and computed model |
| `UpdatePresentationOutput` | an already-active presentation's computed model changed |
| `RemovePresentationOutput` | an active presentation was unmounted |
| `RunCompletedOutput` | the one instance a Session runs reached a terminal outcome (`Completed`/`Failed`/`Cancelled`) — this is the only way to observe the whole game instance ending, since there's no parent to notify through a signal |

Type-switch over `engine.Output` exhaustively; the set is closed the same way `program.Expression`/`program.Operation` are — you can't add your own variant from outside the package.

## Determinism

Given the same compiled `Program`, `InitializationInput`, prior signal log, new `Signal`, and `Limits`, `AdvanceTurn` always returns the same `Output`s (or the same error) — because it always reconstructs current state the same way, by replaying the same inputs. The engine never reads the system clock, the network, environment variables, or OS randomness — time enters only through explicit signal data (e.g. a `TimerExpiredSignalSource` you deliver), and randomness only through the engine's own deterministic random state, seeded once via `InitializationInput.Seed`. This is what makes replay, simulation, and debugging possible: the same recorded sequence of signals against the same initial input always reaches the same final state — and it is exactly what lets `AdvanceTurn` reconstruct that state from your durable log instead of you having to persist it directly.

## Concurrency

A compiled `Program` is safe to share and read concurrently across any number of game instances. `engine`/`engineservice` do **not** serialize calls against the same game instance for you — if two `AdvanceTurn` calls could race against the same instance's signal log, your own session layer owns that ordering (locking, an actor per instance, optimistic concurrency, whatever fits). The engine only defines the deterministic result of one turn in isolation.

## Persisting durable state

There is no `Snapshot` to persist — `StartTurn`/`AdvanceTurn` never return one. What you persist is exactly what you already pass in on the next call: `InitializationInput` (once, at Start) and every `newSignal` you have successfully processed since, in order. Append each `newSignal` to your own durable log right after `AdvanceTurn` accepts it, and load that whole log back (in order) as `priorSignals` on every later call. There is deliberately no codec needed for this — `InitializationInput` and `Signal` are already plain data your own storage format already knows how to represent, since you construct every `Signal` yourself from your own request data.

`engineservice.EncodeSnapshot`/`DecodeSnapshot`/`CheckSnapshotCompatibility` still exist, but they operate on the internal `Snapshot` type `StartTurn`/`AdvanceTurn` never expose — they exist for advanced/tooling use (an admin inspection view, or a future opt-in performance cache sitting in front of `AdvanceTurn`'s own internal replay), not for ordinary game execution.

There is deliberately no codec for `Program` itself. A compiled `Program` is a pure, deterministic function of the `program.Definition` `Compile` was given — persist (or version-reference) the `Definition` through `program/gameservice`'s own codec, and recompile on load. Recompiling is cheap, deterministic, and avoids maintaining a second wire format for the same information.

## What you get back from a compiled `Program`

`Program` (and the types reachable from it — `Workflow`, `Type`, `Value`, ...) is read-only, plain data — inspect it freely, but never construct or mutate the compiled forms of these by hand outside of tests. The fields you'll actually reach for:

- `Program.Metadata` — the game version's identity (carried over unchanged from `program.Definition.Metadata`).
- `Program.Types`, `.Functions`, `.Resources`, `.Questions`, `.Effects`, `.Projections`, `.Views`, `.Workflows` — every one of `def`'s catalogs, compiled and keyed by declared name.
- `Program.RootWorkflow` — the workflow name `StartTurn` starts.

You will not typically construct `engine.Program`/`engine.Workflow`/`engine.Expression`/... values by hand in real code — those come from `Compile`. Building them directly (as the engine's own tests do, to exercise runtime behavior independently of the compiler) is a testing technique, not the intended integration path.

## What this package does not do

No database, no session/room management, no HTTP/WebSocket/gRPC delivery, no real timer scheduling, no output publication, no signal ordering across concurrent requests, no retries. Every one of those belongs to an application/session layer you build on top of `engineservice` — the engine only ever defines the deterministic result of one `Compile`, one `StartTurn`, or one `AdvanceTurn` call.
