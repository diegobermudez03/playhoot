# GAME-ADR-0027: Engine-Owned Turn Execution — Replay and Step-Chain Draining Move Inside `engineservice`

Status: PROPOSED
Created: 2026-09-24
Last status change: 2026-09-24
Supersedes: None
Refines: GAME-ADR-0019 (RuntimeTurn Execution Bound and Terminal Cleanup) — only the *mechanical enforcement location* of the RuntimeTurn Step-chain bound changes; the bound's value, its status as Session Runtime platform policy (not an authored Game Language setting), its `RUNTIME_EXECUTION_FAILED` classification, its diagnostic-code requirement, and every terminal-cleanup/`base_turn_id` rule GAME-ADR-0019 establishes are unaffected and are restated here.
Generalizes: GAME-ADR-0024 (Replay-First Session Runtime Persistence) — extends its "derived state is not truth" principle one step further: not only is a Snapshot never persisted, the *mechanics of deriving it* (replaying durable causes) are no longer the caller's own code either.
Superseded by: None
Legacy ID: None

## Context

`game/language/v1/engine/engineservice` exposes exactly one *signal-driven execution* primitive: `Step(program, snapshot, signal, limits) (Commit, error)` — one signal in, one atomic commit out. (`NewSnapshot`/`EncodeSnapshot`/`DecodeSnapshot`/`CheckSnapshotCompatibility` also take or return `engine.Snapshot`, but none of them applies a `Signal` — see Decision 2's own note on why those four are unaffected by this record.) `Step` has no "replay this history" or "run to quiescence" call. Two consequences of this, both discovered while auditing `game/session/workflows/sessionlifecycle` after GAME-ADR-0026/WORK-0024–0027 landed:

1. **`game/session/workflows/sessionlifecycle/internal/runtimeturn` is Session Runtime's own reimplementation of the engine's step-chaining model.** `Drain` loops calling `engineservice.Step`, feeding `Commit.InternalSignals` back into itself, and enforces `MaxSteps` (GAME-ADR-0019's `MAX_STEPS_PER_RUNTIME_TURN`) itself, returning `ErrStepBoundExceeded` as its own sentinel. `StepTrace` — a caller-defined type describing one internal `Step` call's outcome — is threaded out to `captureInteractions`, which in practice only ever flattens every step's `Outputs` in order; it has no use for step boundaries at all (confirmed by inspection: no code anywhere reads a `StepTrace` field to distinguish one step from another).
2. **`reconstructCurrentSnapshot`/`replayTurn` (`sessionlifecycle/replay.go`) are Session Runtime's own reimplementation of GAME-ADR-0024's replay mechanism.** On every `AnswerInteraction` call, Session Runtime loads `session_runtime_starts` + every `session_runtime_turns` row, translates each into an `engine.Signal` (`loadReplaySignal`), and hand-loops `engineservice.NewSnapshot` + `runtimeturn.Drain` per turn to rebuild the current `engine.Snapshot` — the exact mechanism GAME-ADR-0024 accepted, but its *code* lives entirely in the caller, not in the engine.

Both exist because `Snapshot` is a caller-visible type at all: `sessionlifecycle` has to hold one, thread it through two hand-rolled loops, and understand enough about `InternalSignals`/step-counting/replay-divergence to do so correctly. This is exactly the shape of leak GAME-ADR-0026 already removed for *addressing* (`Signal.Path`/`Slot`/`Key` → `InteractionID`, so a caller never needs to understand the engine's internal instance-addressing model to answer an interaction) — but it was never applied to *execution*. Nothing in GAME-ADR-0026/WORK-0024–0027 was scoped to touch this; it was found afterward, specifically because the review process for those WORKs was scoped around "no removed identifier remains," not "does every caller-visible concept still make sense for a caller that should only know about Turns and interactions."

Confirmed by direct inspection that this is safe to change with **zero new persistence**: `session_runtime_turns` already carries only the ordered replay-input pointer (no Snapshot column, per GAME-ADR-0024), `session_runtime_starts` already carries `Seed`/`RootParameters`, and `session_interactions.response_payload`/`engine_interaction_id` already carry everything `loadReplaySignal`/`buildAnswerSignal` need to reconstruct each historical `engine.Signal`. Nothing this record proposes requires a new column, table, or migration — only where the *replay loop itself* executes changes.

## Decision

### 1. `engine.Snapshot` never crosses the `engineservice`/caller boundary again

`sessionlifecycle` (and any future consumer of `engineservice`) never holds, constructs, or reads a `engine.Snapshot` value. Snapshot remains exactly what it already is *inside* `engine`/`engineservice` — the complete internal runtime state `Step` operates on — but it becomes purely an implementation detail of the new entry points below, never a return value or parameter a caller supplies.

### 2. `engineservice` exposes only Turn-level vocabulary — "Step" is not a term a caller of `engineservice` ever sees

`engineservice.Step`/`NewSnapshot` are removed from `engineservice`'s public surface entirely, not merely superseded or left as an alternative. Confirmed by inspection that this loses nothing: both are one-line pass-throughs to `engine/internal/runtime.Step`/`NewSnapshot`, and the engine's own test suite already calls `internal/runtime.Step` directly (it is in the same module tree, so Go's own `internal/` visibility rule already permits this without needing an `engineservice`-level re-export). The only real external callers of the `engineservice`-level wrappers today are `sessionlifecycle` itself (removed by this record) and `engineservice`'s own black-box test suite / `game/language/v1/example.go` (migrated to the new Turn-level entry points below, since that is the contract they should demonstrate). "Step" — the internal one-signal/one-internally-chained-commit execution unit — is a concept that belongs entirely inside `engine`/`engine/internal/runtime`; a caller of `engineservice` should never need that word at all. `engineservice`'s only execution-facing exports become the two Turn-level entry points below.

`EncodeSnapshot`/`DecodeSnapshot`/`CheckSnapshotCompatibility` are unaffected by this record — confirmed by inspection that `sessionlifecycle` never calls any of them today, so they are not part of the leak this record addresses. They remain a legitimate, self-contained, opt-in capability (for admin/debug tooling, or a future performance cache per GAME-ADR-0024's own allowance) rather than something a caller is forced to touch to use `engineservice` correctly; nothing in this record requires or forbids their continued existence.

### `engineservice` gains two Turn-level entry points that internally own replay and step-chain draining

Conceptually (exact Go names/signatures are Implementation Freedom for the implementing WORK):

- **Start a session's first turn**: given a compiled `Program` and an `InitializationInput`, internally perform the engine's own `NewSnapshot` + apply its synthesized first lifecycle signal (`WorkflowStarted`) to quiescence, and return only that turn's `[]Output`. The caller never sees or supplies the synthesized first signal itself — it is a fixed, deterministic value with nothing for a caller to decide.
- **Advance by one new turn**: given a compiled `Program`, the same `InitializationInput`, the ordered list of every `Signal` already durably committed since Start (`priorSignals`), and the new `Signal` to process (`newSignal`), internally: replay `priorSignals` in order (silently, discarding their `Outputs` — already durably recorded when they originally happened) to reconstruct current internal state, then drain `newSignal` to quiescence exactly as `Step`/`Drain` do today, and return only `newSignal`'s own `[]Output`.

`priorSignals` is exactly the ordered replay-input log Session Runtime already persists (`session_runtime_turns`, translated per-row via the existing `loadReplaySignal`-equivalent logic, which **stays in `sessionlifecycle`** — reconstructing an `engine.Signal` from `sessionlifecycle`'s own DB rows is Session Runtime's own schema-translation responsibility, not an engine internal). No new column, table, or field is required to supply it.

### 3. The RuntimeTurn Step-chain bound (GAME-ADR-0019) is enforced inside `engineservice`, as part of `engine.Limits`

`engine.Limits` gains a new field bounding the number of internally-chained `Step` calls one Turn (one element of `priorSignals`, or `newSignal`) may require to reach quiescence — the same bound `runtimeturn.MaxSteps`/GAME-ADR-0019's `MAX_STEPS_PER_RUNTIME_TURN` already enforces today, relocated. `engine.DefaultLimits()` sets it to the same value GAME-ADR-0019 already accepted (20), so this record changes *where* the bound is enforced, never *what* it is. Exceeding it is a new `engine`-level `ExecutionErrorCode` (replacing `runtimeturn.ErrStepBoundExceeded`), returned through the same `*runtime.ExecutionError` mechanism every other engine execution failure already uses — Session Runtime classifies it exactly as it classifies any other `*ExecutionError` today, with no step-counting of its own.

GAME-ADR-0019's own policy content is unchanged by this: the bound remains Session Runtime's own configured value (passed into the new entry points via `Limits`, exactly as `MaxOperations`/`MaxLoopIterations`/`MaxActiveSlotsPerInstance` already are), not something an authored game can set — GAME-ADR-0019's Alternatives Considered already rejected making it author-controlled, and this record does not reopen that. The `RUNTIME_EXECUTION_FAILED` classification, the stable diagnostic error-code requirement, terminal-cleanup, and `base_turn_id` nullability are all restated, unmodified, by this record.

### 4. Replay divergence is distinguishable from an ordinary new-signal failure

If replaying an already-committed element of `priorSignals` fails, that is never an ordinary decline — every element of `priorSignals` succeeded once already (that is why it is durable), so failing to reproduce it identically is a data-integrity condition, not a rejected input. `Advance`'s error contract must let the caller tell this apart from `newSignal` itself failing/being rejected (`ErrSignalRejected`/`ErrInputRejected`/an ordinary `*ExecutionError` on `newSignal`), so Session Runtime can continue to `monitoring.Alert` on the former and treat the latter as an ordinary business decline — exactly the distinction `replayTurn`'s own error wrapping makes today, moved to live inside `engineservice`'s own returned error type instead of being built ad hoc by the caller.

### 5. `sessionlifecycle` deletes its own replay/step-chaining code

`internal/runtimeturn` (`Drain`, `StepTrace`, `MaxSteps`, `ErrStepBoundExceeded`) is removed in full. `reconstructCurrentSnapshot`/`replayTurn` (`replay.go`) are removed; `loadReplaySignal`/`buildAnswerSignal`/`encodeRootParameters`/`decodeRootParameters` (translating `sessionlifecycle`'s own persisted rows into plain `engine.Signal`/`InitializationInput` values, and back) remain — that translation is Session Runtime's own schema concern, not an engine internal, and stays exactly where it is. `captureInteractions` takes a flat `[]engine.Output` instead of `[]runtimeturn.StepTrace`, since no caller ever needed per-step grouping.

`Manager.Start`/`Manager.AnswerInteraction`'s own exported signatures and return types (`StartResult`/`AnswerInteractionResult`) are unaffected — this record changes only their internal implementation, not `sessionlifecycle`'s own public API or the wire protocol built on it (already confirmed clean of `Snapshot`/`Slot`/step concepts in `api/session/wire.go`).

## Rationale

This is the same principle GAME-ADR-0026 already applied to interaction addressing, applied one layer deeper: an external package's caller should only need to understand the vocabulary of *its own domain* (Sessions, Turns, interactions) to use it correctly, never the package's own execution model. A caller that has to write a step-chaining loop and a replay loop to use `engineservice` correctly is a caller that has been hired to reimplement part of the engine — exactly backwards from "engine is an external package with a narrow, clean contract."

Moving replay inside `engineservice` costs nothing new: Session Runtime already replays the full history on every `AnswerInteraction` call today (`reconstructCurrentSnapshot` is not a cache — GAME-ADR-0024 explicitly forbids a durable checkpoint), so relocating that loop does not change its complexity class, only which package's code executes it. If replay cost ever becomes a real, measured problem, GAME-ADR-0024 already permits an ephemeral, non-durable, best-effort cache without requiring a new decision — this record does not need to solve that preemptively, and does not.

Folding the step-chain bound into `engine.Limits` rather than inventing a second, caller-side bounding mechanism reuses machinery that already exists for the identical purpose (`MaxOperations`/`MaxLoopIterations`/`MaxActiveSlotsPerInstance` are already "Session-Runtime-configured, engine-enforced" values) and lets a step-chain-exceeded failure surface through the same `*runtime.ExecutionError`/`ExecutionErrorCode` mechanism every other execution failure already uses, rather than a bespoke sentinel (`ErrStepBoundExceeded`) a caller has to know about separately.

## Alternatives Considered

### Keep `runtimeturn`/`replay.go` as-is; only rename or lightly tidy them

Rejected. This was the state of the world this record identifies as the problem — a rename does not change that Session Runtime's own code implements the engine's step-chaining and replay mechanics.

### Introduce a caller-visible opaque "continuation" or "resumable session handle" instead of full replay-from-log every call

Considered, as a way to avoid the (already-accepted) full-history-replay cost. Rejected for now: it would introduce a new caller-visible concept (a handle a caller must store and pass back) precisely where this record is trying to remove one, and GAME-ADR-0024 already permits an ephemeral, non-durable cache for this exact performance concern without needing a new caller-facing type. If replay cost is ever measured as a real problem, an internal cache inside `engineservice`/`sessionlifecycle` — invisible to callers — is the accepted next step, not a new contract shape.

### Make the RuntimeTurn Step-chain bound part of `program.Definition` (an authored, per-game setting)

Not reopened here. GAME-ADR-0019 already considered and rejected this for V1, for reasons unrelated to this record's own concern (mechanical enforcement location); this record only relocates *where* the already-accepted platform-policy value is enforced.

### Keep `engineservice.Step`/`NewSnapshot` exported alongside the new Turn-level entry points, for tooling

Considered, then rejected once actually verified against the code rather than assumed: `engineservice.Step`/`NewSnapshot` are one-line pass-throughs to `internal/runtime.Step`/`NewSnapshot`, and the engine's own test suite already calls `internal/runtime.Step` directly — it never needed the `engineservice`-level wrapper. Keeping a public `Step` in the same package as the new Turn-level entry points would leave exactly the ambiguity this record exists to remove (which one should a caller use?), for zero actual capability gain. `internal/runtime.Step`/`NewSnapshot` remain exactly as they are — unexported outside `engine/`, already serving the engine's own test suite and now also the new Turn-level entry points' own internal implementation.

## Consequences

- `game/session/workflows/sessionlifecycle/internal/runtimeturn` is deleted in full.
- `reconstructCurrentSnapshot`/`replayTurn` are deleted from `sessionlifecycle/replay.go`; `loadReplaySignal`/`buildAnswerSignal`/`encodeRootParameters`/`decodeRootParameters` remain, now feeding the new `engineservice` entry points' `priorSignals`/`InitializationInput` parameters directly instead of a hand-rolled loop.
- `captureInteractions` takes `[]engine.Output` directly; nothing in `sessionlifecycle` needs a per-Step grouping concept ever again.
- `engineservice.Step`/`NewSnapshot` are deleted; "Step" is no longer a term anything outside `engine/` (including `engineservice`'s own public surface) ever needs to know. `engineservice`'s own test suite and `game/language/v1/example.go`/`users_setup_example.go` (the only other real callers of the removed wrappers) migrate to the new Turn-level entry points. `internal/runtime.Step`/`NewSnapshot` are unaffected and continue to serve the engine's own granular test suite directly.
- `engine.Limits` gains a new Step-chain-bound field, defaulted to GAME-ADR-0019's already-accepted value (20); a new `ExecutionErrorCode` replaces `runtimeturn.ErrStepBoundExceeded`.
- GAME-ADR-0019's policy ownership, classification, diagnostic-code requirement, and terminal-cleanup rules are restated and unaffected. WORK-0014 (Runtime Failure Diagnostics, still PLANNED) must, once implemented, source its step-chain-exceeded diagnostic fields from the new engine-level error, not from a Session-Runtime-side step counter — this record does not implement WORK-0014, only ensures the information it will need remains available through the new error contract.
- GAME-ADR-0024's replay-first principle (no persisted Snapshot, current state always a pure function of durable causes) is fully preserved and restated, not reversed — only which package's code performs the replay changes.
- No migration, production code, or implementation is authorized by this record; it is design/architecture acceptance only, per this repository's own ADR/WORK separation.

## Canonical Knowledge Impact

- `game/language/v1/engine/README.md`, `game/language/v1/engine/LOGICAL_CONTRACT.md` — document the new Turn-level `engineservice` entry points and the relocated Step-chain bound in `engine.Limits`.
- `game/README.md` — Session Runtime Turn And Persistence Model section updated to describe replay/step-chain draining as happening inside `engineservice`, not `sessionlifecycle`.
- `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` — updated to reflect that no caller-side replay loop exists anymore.
- `game/docs/decisions/GAME-ADR-0019-...md`, `GAME-ADR-0024-...md` — gain "Implemented by" cross-reference notes once the implementing WORK lands (historical decision text itself unedited, per this repository's Historical Immutability convention).

## Implementation Impact

Owned by a new WORK under `docs/projects/active/game-language-flat-execution-model/`. No migration, production code, or implementation is authorized directly by this record.
