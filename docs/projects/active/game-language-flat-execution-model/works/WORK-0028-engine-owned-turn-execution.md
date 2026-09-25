# WORK-0028: Engine-Owned Turn Execution (Replay and Step-Chain Draining)

Status: IMPLEMENTING
Created: 2026-09-24
Last status change: 2026-09-24 (implementation complete, self-reported ready for independent review - see Completion Record; READY -> IMPLEMENTING; DRAFT -> READY, GAME-ADR-0027 ACCEPTED, human-approved "Approved, proceed"; PLANNED -> DRAFT, design filled in)

Related decisions:
- GAME-ADR-0027 (Engine-Owned Turn Execution — Replay and Step-Chain Draining Move Inside `engineservice`) — PROPOSED. This WORK cannot move DRAFT -> READY until GAME-ADR-0027 is ACCEPTED.
- GAME-ADR-0019 (RuntimeTurn Execution Bound and Terminal Cleanup) — refined, not superseded, by GAME-ADR-0027; this WORK relocates the Step-chain bound's enforcement, not its value or classification.
- GAME-ADR-0024 (Replay-First Session Runtime Persistence) — generalized, not superseded; this WORK relocates the replay loop, not the no-persisted-Snapshot principle.

Canonical context:
- `game/docs/decisions/GAME-ADR-0027-engine-owned-turn-execution-and-replay.md`
- `game/language/v1/engine/engineservice/runtime.go` (current `Step`/`NewSnapshot`), `game/language/v1/engine/limits.go`, `game/language/v1/engine/internal/runtime/step.go` (`ExecutionErrorCode`)
- `game/session/workflows/sessionlifecycle/internal/runtimeturn/runtimeturn.go` (to be deleted), `game/session/workflows/sessionlifecycle/replay.go`, `interaction_capture.go`, `step_answer_interaction.go`, `step_start.go` — every current caller of the code this WORK removes, read in full while drafting this WORK

## Outcome

`game/session/workflows/sessionlifecycle` never holds, constructs, or reads an `engine.Snapshot`, never implements its own step-chaining loop, and never implements its own replay loop. It supplies `engineservice` with exactly what it already durably persists — the pinned `program.Definition`, `Start`'s `Seed`/`RootParameters`, the ordered list of already-committed `engine.Signal`s, and the new one to process — and receives back only that new turn's `[]engine.Output`. `internal/runtimeturn` is deleted in full; `reconstructCurrentSnapshot`/`replayTurn` are deleted from `replay.go`. `engineservice.Step`/`NewSnapshot` are deleted entirely — "Step" is not a term any caller of `engineservice` (including its own test suite) needs to know anymore; `engineservice`'s only execution-facing vocabulary is Turn-level.

## Context

See GAME-ADR-0027's own Context in full. In short: `engineservice.Step` is a one-signal-in/one-commit-out primitive, so Session Runtime built its own step-chaining loop (`runtimeturn.Drain`, enforcing GAME-ADR-0019's `MaxSteps` itself) and its own replay loop (`reconstructCurrentSnapshot`/`replayTurn`, implementing GAME-ADR-0024's accepted replay-first model) on top of it. Both loops are pure, deterministic, and require no new persisted data to relocate — confirmed by reading `session_runtime_starts`/`session_runtime_turns`/`session_interactions`'s actual schema and every current caller of `loadReplaySignal`/`buildAnswerSignal`.

## Scope

### In Scope

- Two new `engineservice` entry points (see Approved Design) replacing `sessionlifecycle`'s direct use of `NewSnapshot`/`Step`/`runtimeturn.Drain` for its two real call sites (`Start`, `AnswerInteraction`).
- `engine.Limits` gains a Step-chain-bound field (name/shape per Approved Design); `engine.DefaultLimits()` sets it to 20 (GAME-ADR-0019's already-accepted value, unchanged).
- A new `engine`/`internal/runtime` `ExecutionErrorCode` for the Step-chain-exceeded case, replacing `runtimeturn.ErrStepBoundExceeded`.
- A distinguishable `engineservice`-level error (or wrapped error) for "an already-committed prior signal failed to replay identically," distinct from an ordinary failure/rejection of the new signal.
- `sessionlifecycle` rework: `step_start.go`/`step_answer_interaction.go` call the new `engineservice` entry points instead of `NewSnapshot`/`Step`/`runtimeturn.Drain`/`reconstructCurrentSnapshot`; `internal/runtimeturn` deleted in full; `replay.go`'s `reconstructCurrentSnapshot`/`replayTurn` deleted (`loadReplaySignal`/`buildAnswerSignal`/`encodeRootParameters`/`decodeRootParameters` kept, now feeding the new entry points directly); `interaction_capture.go`'s `captureInteractions` takes `[]engine.Output` instead of `[]runtimeturn.StepTrace`.
- **`engineservice.Step`/`NewSnapshot` deleted** (confirmed one-line pass-throughs to `internal/runtime.Step`/`NewSnapshot`, with no caller needing them once `sessionlifecycle` no longer does — `internal/runtime`'s own test suite already calls `internal/runtime.Step` directly and is unaffected). `engineservice`'s own black-box test suite (`codec_test.go`, `helpers_test.go`, `integration_features_test.go`, `integration_keyed_question_test.go`, `integration_test.go`, `keyed_codec_test.go`) and `game/language/v1/example.go`/`users_setup_example.go` — the only other real callers — migrate to the new Turn-level entry points.
- Updating every test that currently exercises the removed code paths (`runtimeturn_test.go` deleted with the package; `replay_fixture_test.go`/`replay_integration_test.go`/`interaction_capture_test.go` adapted to the new call shape, preserving the same behavioral coverage — in particular, the existing multi-turn live-vs-replay agreement proof and the replay-divergence-is-distinguishable-from-rejection proof).

### Out of Scope

- Any change to `Manager.Start`/`Manager.AnswerInteraction`'s exported signatures, `StartResult`/`AnswerInteractionResult`, or the wire protocol in `api/session` — this WORK is an internal implementation change only, already confirmed not to require any change at that layer.
- Any change to GAME-ADR-0019's Step-chain-bound *value*, its status as Session-Runtime-configured (not authored-game) policy, its `RUNTIME_EXECUTION_FAILED` classification, its diagnostic-error-code requirement, or its terminal-cleanup/`base_turn_id` rules — only the mechanical enforcement location changes.
- WORK-0014 (Runtime Failure Diagnostics) itself — still PLANNED, not implemented by this WORK. This WORK only ensures the new Step-chain-exceeded error carries at least as much diagnostic information as `*runtime.ExecutionError` already carries for every other execution failure, so WORK-0014 can consume it later without needing a Session-Runtime-side step counter.
- Caching reconstructed runtime state for performance — explicitly deferred by GAME-ADR-0024 and GAME-ADR-0027 alike; this WORK replays the full durable log on every call, exactly as `reconstructCurrentSnapshot` already does today (no new cost introduced, none removed).
- Any new `RuntimeTurn`-producing cause (UserIntent, Timer, SessionCancelled, disconnect/reconnect) — this WORK only needs to carry forward the replay shapes Session Runtime already supports (`answerInteractionSourceKind`), matching `loadReplaySignal`'s current switch.

## Approved Design

### New `engineservice` entry points

```go
// StartTurn performs a Session's mandatory first turn: initializes new
// runtime state from start, applies the engine's own synthesized first
// lifecycle signal to quiescence entirely internally, and returns that
// turn's Outputs. The caller never constructs or sees the synthesized
// signal itself, or any Snapshot.
func StartTurn(p engine.Program, start engine.InitializationInput, limits engine.Limits) ([]engine.Output, error)

// AdvanceTurn reconstructs current runtime state by internally replaying
// every signal in priorSignals, in order, against a freshly initialized
// instance (discarding their Outputs - already durably recorded when they
// first happened), then drains newSignal to quiescence exactly as Step
// does today. Returns only newSignal's own Outputs. The caller never
// constructs, holds, or reads a Snapshot.
//
// If replaying an element of priorSignals fails, that is ErrReplayDivergence
// (wrapping the underlying cause) - a data-integrity condition, never an
// ordinary decline, since every element of priorSignals already succeeded
// once (that is why it is durable). A failure of newSignal itself surfaces
// exactly as Step already surfaces it today (ErrSignalRejected,
// ErrInputRejected, or another *runtime.ExecutionError).
func AdvanceTurn(p engine.Program, start engine.InitializationInput, priorSignals []engine.Signal, newSignal engine.Signal, limits engine.Limits) ([]engine.Output, error)

var ErrReplayDivergence = errors.New("engineservice: replaying an already-committed signal did not reproduce its original commit")
```

`engineservice.Step`/`NewSnapshot` are deleted. `internal/runtime.Step`/`NewSnapshot` (already `internal`-scoped, already what the engine's own test suite calls directly) are unaffected and back these two new entry points internally. `engineservice`'s public surface has no "Step" vocabulary left anywhere.

Exact naming is Implementation Freedom; the conceptual shape above (two entry points, `priorSignals`/`newSignal` split, `ErrReplayDivergence` distinguishable from an ordinary `newSignal` failure, no `Step`/`NewSnapshot` left in `engineservice`) is not.

### `engine.Limits`

```go
type Limits struct {
    MaxOperations     int
    MaxLoopIterations int
    MaxActiveSlotsPerInstance int

    // MaxStepsPerTurn bounds the number of internally-chained Step calls
    // one Turn (one priorSignals element, or newSignal) may require to
    // reach quiescence, draining Commit.InternalSignals - see
    // ExecutionErrorStepChainExceeded. This is GAME-ADR-0019's
    // MAX_STEPS_PER_RUNTIME_TURN, enforced here instead of by a caller's
    // own loop.
    MaxStepsPerTurn int
}
```

`DefaultLimits()` sets `MaxStepsPerTurn: 20` (GAME-ADR-0019's already-accepted value). A new `ExecutionErrorCode` (e.g. `ExecutionErrorStepChainExceeded`) is added to `internal/runtime/step.go`'s existing enum (appended, never renumbered, per that file's own stability contract) and returned via the same `*runtime.ExecutionError` mechanism as every other execution failure. `runtimeturn.MaxSteps`/`ErrStepBoundExceeded` are deleted.

**Bound scope.** The bound applies per signal processed (once for each `priorSignals[i]`, once for `newSignal`) — not as one combined counter across an entire replay. Each `priorSignals[i]` already succeeded within this same bound when it was originally committed (GAME-ADR-0019 already required this of every RuntimeTurn), so replaying it deterministically reproduces that success; the bound only has real teeth against `newSignal`, exactly as today.

### `sessionlifecycle` call sites

- `step_start.go`: builds `engine.InitializationInput` as today, calls `engineservice.StartTurn(compiledProgram, input, engine.DefaultLimits())`, passes the returned `[]engine.Output` to `captureInteractions`.
- `step_answer_interaction.go`: loads `start`/`turns` as today (unchanged repo calls), builds `priorSignals` by mapping every turn through the existing `loadReplaySignal`, builds `newSignal` exactly as today, calls `engineservice.AdvanceTurn(compiledProgram, input, priorSignals, newSignal, engine.DefaultLimits())`. On `errors.Is(err, engineservice.ErrReplayDivergence)`, `monitoring.Alert` exactly as `replayTurn`'s own divergence path does today. On an ordinary `newSignal` failure, the existing `ErrSignalRejected`/`ErrInputRejected`/fatal-materialization branching is unchanged.
- `replay.go`: `reconstructCurrentSnapshot`/`replayTurn` deleted. `loadReplaySignal`/`buildAnswerSignal`/`encodeRootParameters`/`decodeRootParameters` kept as-is (they translate `sessionlifecycle`'s own persisted rows into plain `engine.Signal`/`InitializationInput` values — Session Runtime's own schema concern, not an engine internal).
- `interaction_capture.go`: `captureInteractions(ctx, tx, repo, sessionID, turnID uint, outputs []engine.Output) error` — drops the outer per-step loop, iterates `outputs` directly.
- `internal/runtimeturn`: package deleted in full (`runtimeturn.go`, `runtimeturn_test.go`).

## Constraints and Invariants

- `sessionlifecycle` production code never imports/constructs/reads a `engine.Snapshot` value anywhere, after this WORK.
- `engineservice.Step`/`NewSnapshot` no longer exist; nothing outside `engine/` needs the word "Step" to use `engineservice` correctly.
- GAME-ADR-0019's Step-chain-bound value (20), its Session-Runtime-configured (not authored) status, its `RUNTIME_EXECUTION_FAILED` classification, and its terminal-cleanup/`base_turn_id` rules are unchanged — only enforcement location moves.
- GAME-ADR-0024's no-persisted-Snapshot, replay-from-durable-causes principle is unchanged — only which package's code performs the replay moves. No new column, table, or migration is introduced by this WORK.
- `Manager.Start`/`Manager.AnswerInteraction`'s exported signatures and result types are byte-for-byte unchanged.
- No dead scaffolding: if any part of this design is deferred, it is left for a clearly-tracked future WORK, not stubbed out with an unused type or a `// not implemented` path, per this Project's own established practice (WORK-0024/25/26/27).

## Acceptance Criteria

- `game/session/workflows/sessionlifecycle/internal/runtimeturn` no longer exists (directory removed).
- `reconstructCurrentSnapshot`/`replayTurn` no longer exist anywhere in `game/session`.
- A repository-wide search confirms `sessionlifecycle` production code contains no reference to `engine.Snapshot`.
- `engineservice.Step`/`NewSnapshot` no longer exist (repository-wide search confirms zero remaining references outside historical documentation); `engineservice`'s own test suite and `game/language/v1/example.go`/`users_setup_example.go` are migrated to the new Turn-level entry points, not left broken or skipped.
- `engineservice.StartTurn`/`AdvanceTurn` (or their approved final names) exist, are covered by unit tests exercising: a fresh Start; a multi-turn Advance sequence whose live-execution result matches replaying the same signals from scratch (the existing live-vs-replay agreement proof, adapted rather than dropped); a Step-chain-exceeded case surfacing the new `ExecutionErrorCode`; a simulated replay divergence surfacing `ErrReplayDivergence` distinguishably from an ordinary `newSignal` rejection.
- `go build ./...`, `go vet ./...` clean repository-wide.
- `go test ./game/language/v1/... -count=1` and `go test ./game/session/... -count=1` (against real Postgres) pass, with `Manager.Start`/`Manager.AnswerInteraction`'s own observable behavior unchanged from `sessionlifecycle`'s external callers' perspective.
- `go test ./... -count=1` against real Postgres: no new failure beyond the already-known, pre-existing, out-of-scope `getgame` JSONB-whitespace defect.

## Implementation Freedom

- Exact naming of `StartTurn`/`AdvanceTurn`/`MaxStepsPerTurn`/`ExecutionErrorStepChainExceeded`/`ErrReplayDivergence`, and whether they live in `runtime.go` or a new file within `engineservice`/`internal/runtime`, is ordinary Codebase Agent autonomy.
- Whether `AdvanceTurn` replays `priorSignals` via an internal helper shared with `StartTurn`'s own quiescence-draining logic (a natural, expected refactor) is ordinary internal implementation choice.
- Exact wording/fields of the diagnostic information carried by the new Step-chain-exceeded `*runtime.ExecutionError` (beyond matching the existing `Code`/`Message` shape every other `ExecutionErrorCode` already provides) is Implementation Freedom, provided it is at least as informative as what WORK-0014 will eventually need per GAME-ADR-0019's own diagnostic-code requirement.

## Verification

- `go build ./...`, `go vet ./...`, `gofmt -l` (changed files).
- `go test ./game/language/v1/... -count=1` — full suite, including new tests for `StartTurn`/`AdvanceTurn`.
- `go test ./game/... -count=1` against real Postgres (`playhoot-postgres-1`), confirming `game/session/...` passes with `sessionlifecycle`'s external behavior unchanged.
- A repository-wide search confirming zero remaining reference to `runtimeturn`/`reconstructCurrentSnapshot`/`replayTurn`/`engine.Snapshot` within `game/session` production code.

## Documentation Impact

### Accepted / Canonical Knowledge

- `game/docs/decisions/GAME-ADR-0027-...md` — remains ACCEPTED (once approved) and historically unedited; gains an "Implemented by" cross-reference to this WORK.
- `game/docs/decisions/GAME-ADR-0019-...md`, `GAME-ADR-0024-...md` — historically unedited; each gains an "Implemented by" cross-reference noting the enforcement-location/replay-ownership relocation this WORK performs.

### Current-State Documentation After Implementation

- `game/language/v1/engine/README.md`, `game/language/v1/engine/LOGICAL_CONTRACT.md` — document `StartTurn`/`AdvanceTurn` and the relocated `MaxStepsPerTurn` in `engine.Limits`.
- `game/README.md` — Session Runtime Turn And Persistence Model section updated: replay and step-chain draining are performed by `engineservice`, not `sessionlifecycle`.
- `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` — updated to remove any remaining description of a caller-side replay loop.

### Intentionally Unchanged

- `Manager.Start`/`Manager.AnswerInteraction`'s public contract and the `api/session` wire protocol — unaffected, already confirmed clean of the concepts this WORK removes.
- Every other completed Session Runtime WORK — unaffected; this WORK touches only `sessionlifecycle`'s own internal replay/step-chaining implementation.

## Blockers

1. ~~GAME-ADR-0027 must be ACCEPTED before this WORK can move DRAFT -> READY.~~ **Resolved (2026-09-24, human-approved "Approved, proceed")** - GAME-ADR-0027 is ACCEPTED.

## Completion Record

Not yet DONE. Independent review has not yet occurred.

### Implementation Report (2026-09-24)

Work: `docs/projects/active/game-language-flat-execution-model/works/WORK-0028-engine-owned-turn-execution.md`

Work status: IMPLEMENTING

Implemented:
- `engine/limits.go`: `Limits` gains `MaxStepsPerTurn int`; `DefaultLimits()` sets it to 20 (GAME-ADR-0019's already-accepted value, relocated not changed).
- `engine/internal/runtime/step.go`: new `ExecutionErrorStepChainExceeded` code appended to the existing enum (with its `String()` case); new exported `DrainSignal(p, snapshot, signal, limits) (Snapshot, []Output, error)` - applies `signal` via `Step`, then repeatedly drains `Commit.InternalSignals` in FIFO order until quiescence, counting every actual `Step` call toward `limits.MaxStepsPerTurn` and returning `ExecutionErrorStepChainExceeded` if exceeded, accumulating every chained call's `Outputs` in order.
- `engine/engineservice/runtime.go`: `Step`/`NewSnapshot` **removed** (confirmed one-line pass-throughs to `internal/runtime`, needed by no external caller once `sessionlifecycle` stopped calling them - `internal/runtime`'s own test suite already calls `internal/runtime.Step` directly and was unaffected). New `ExecutionErrorStepChainExceeded` re-export; new `ErrReplayDivergence` sentinel; new `StartTurn(p, start, limits) ([]Output, error)` (internally: `NewSnapshot` + `DrainSignal` on the engine's own synthesized first signal) and `AdvanceTurn(p, start, priorSignals, newSignal, limits) ([]Output, error)` (internally: replays `start`'s initialization then every element of `priorSignals` via `DrainSignal`, wrapping any failure in `ErrReplayDivergence`; then drains `newSignal` the same way and returns only its `Outputs`, surfacing its failure unwrapped).
- `game/session/workflows/sessionlifecycle`: `internal/runtimeturn` package deleted in full (`runtimeturn.go`, `runtimeturn_test.go`). `replay.go`'s `reconstructCurrentSnapshot`/`replayTurn` deleted; new `loadPriorSignals` returns `(engine.InitializationInput, []engine.Signal, error)` from the same durable rows the deleted functions read, reusing `loadReplaySignal`/`buildAnswerSignal`/`encodeRootParameters`/`decodeRootParameters` unchanged. `step_start.go` calls `engineservice.StartTurn`; `step_answer_interaction.go` calls `engineservice.AdvanceTurn` and branches on `errors.Is(err, engineservice.ErrReplayDivergence)` (alert + fatal-path-equivalent plain error, matching the prior `reconstructCurrentSnapshot` error path exactly) before its existing `ErrSignalRejected`/`ErrInputRejected`/fatal-materialization branching. `interaction_capture.go`'s `captureInteractions` now takes `[]engine.Output` directly, iterating it without any per-step grouping. `manager.go`'s doc comment updated.
- `game/language/v1/example.go`/`users_setup_example.go` (scratch/draft files, not production code) rewritten to demonstrate the new usage pattern: no `Snapshot` held across calls, `s.signals []engine.Signal` held instead, `applyTurn` calling `AdvanceTurn`.
- New tests: `engine/engineservice/turn_test.go` - `TestStartTurn_MatchesLiveNewSnapshotAndStep`, `TestAdvanceTurn_MatchesLiveExecutionAcrossMultipleTurns` (proves `AdvanceTurn`'s internal replay reproduces exactly what live, in-memory continued execution produces, at every turn of a multi-turn sequence), `TestAdvanceTurn_StepChainExceededSurfacesExecutionError`, `TestAdvanceTurn_ReplayDivergenceDistinguishableFromNewSignalRejection`.
- Every existing `engineservice_test` file that called the removed `engineservice.Step`/`NewSnapshot` (`codec_test.go`, `helpers_test.go`, `integration_test.go`, `integration_features_test.go`, `integration_keyed_question_test.go`, `keyed_codec_test.go`) was migrated to call `internal/runtime.Step`/`NewSnapshot` directly instead - these tests hand-construct/inspect raw `engine.Snapshot` values for codec/engine-behavior verification unrelated to the Turn-level API's own contract, which `internal/runtime` (already legitimately reachable under Go's own `internal/` visibility rule from anywhere under `engine/`) serves directly, exactly as the engine's own test suite already does.
- `sessionlifecycle` test files updated to match: `interaction_capture_test.go` rewritten (`[]engine.Output` directly); `replay_fixture_test.go` rewritten around `StartTurn`/`AdvanceTurn`; `replay_integration_test.go`'s `TestReconstructCurrentSnapshot_Integration` rewritten around two independent `loadPriorSignals` loads plus an in-memory-only `AdvanceTurn` call with a shared hypothetical next signal, asserting byte-identical results (`Snapshot` is never compared directly anymore, since neither this package nor `engineservice` exposes one); `testutil_test.go`'s `replayObservableDefinition` fixture extended with a third question (`Q3`) exposing the second answer as its own argument, so the fixture proxy for "did replay correctly thread through" is observable via `Output` alone.

Local implementation decisions:
- `DrainSignal` lives in `internal/runtime` (not `engineservice`) so it can reuse `newExecutionError`/`ExecutionErrorCode` directly and stays the same "primitive engine mechanism" `Step`/`NewSnapshot` already were; `engineservice.StartTurn`/`AdvanceTurn` are thin orchestration on top, mirroring the existing `engineservice`-wraps-`internal/runtime` pattern.
- `AdvanceTurn`'s `ErrReplayDivergence` wraps the underlying cause with `%s`, not `%w` - deliberately, so `errors.Is(err, engineservice.ErrSignalRejected)` is never simultaneously true for a replay-divergence error, keeping the two failure categories structurally exclusive per this WORK's own Approved Design.
- The `FailedOnInitialSignal`-style distinction the deleted `runtimeturn.Result` used (was a rejection the caller's own initial signal, or an engine-internally-generated one) is not preserved in `AdvanceTurn`'s contract: `AdvanceTurn` already isolates `newSignal` into its own final `DrainSignal` call, so any failure from that call is unambiguously about `newSignal`'s own processing (whether on its literal first internal Step or a later chained one) - a distinction the caller no longer needs, consistent with this WORK's own goal of not exposing internal step-shaped granularity.
- `replayObservableDefinition`'s fixture gained a third question/global field (`Q3`/`c`) rather than being left at two, since "answering Q2 must not open a further question" (the old assertion) was only ever a fixture convenience, not a load-bearing property - extending it let the "did replay thread the second answer through" proof stay purely Output-observable instead of needing `Snapshot.GlobalState` access no longer available to this package's own tests.

Deviations from the approved WORK:
- None.

Discoveries:
- None requiring escalation. One local, non-blocking finding, corrected in the same pass: three doc-comment edits made while implementing (in `runtime.go`, `turn_test.go` x2, plus one in `example.go`/`replay_integration_test.go`/`step_start_test.go`/`testutil_test.go`) initially cited GAME-ADR-0027/0019/0024 as justification inside Go source comments, caught by this repository's own `TestNoInternalDocCitationsInComments` - reworded to state the reasoning directly, per the Code Comment Standard (citation is fine in `.md` documentation, per this repository's own convention, but not inside Go source comments).

Verification performed:
- `go build ./...`, `go vet ./...` - clean, repository-wide, iterated to a clean state via the compiler/`go vet` error list.
- `go test . -run TestNoInternalDocCitationsInComments` - passes (after the fix above).
- `go test ./game/language/v1/... -count=1` - all pass, including the four new `StartTurn`/`AdvanceTurn` tests.
- `go test ./... -count=1` against real Postgres (`playhoot-postgres-1`) - every package passes except the already-known, pre-existing, out-of-scope `getgame` JSONB-whitespace defect, run multiple times across this pass with identical results each time. `TestReconstructCurrentSnapshot_Integration`/`TestManagerAnswerInteraction_Integration`/`TestManagerStart_Integration` (including their fatal-execution and two-concurrent-calls-never-both-execute cases) all pass, confirming `Manager.Start`/`Manager.AnswerInteraction`'s externally observable behavior is unchanged.
- A repository-wide search confirms zero remaining reference to `engineservice.Step`/`NewSnapshot`, `runtimeturn`, or `reconstructCurrentSnapshot`/`replayTurn` anywhere (production code, tests, or non-historical documentation).

Documentation synchronized:
- `game/language/v1/engine/README.md` - substantially rewritten: `StartTurn`/`AdvanceTurn` replace `NewSnapshot`/`Step` throughout (the three operations, the typical caller lifecycle example, the dedicated subsections, the error-handling section, the `Outputs` table's `Step` reference), `Determinism`/`Concurrency` reworded around Turn-level calls, `Persisting a Snapshot` replaced with `Persisting durable state` (persist `InitializationInput` + the signal log, not a `Snapshot`; `EncodeSnapshot`/`DecodeSnapshot`/`CheckSnapshotCompatibility` reframed as advanced/tooling-only), `What you get back from a compiled Program` drops the `Snapshot.GlobalState`/`Root` bullet (no longer caller-reachable).
- `game/language/v1/engine/LOGICAL_CONTRACT.md` - new paragraph clarifying `Snapshot`/`Commit`/`step` are `engine`'s own internal logical model, with `engineservice.StartTurn`/`AdvanceTurn` as the actual Turn-level public entry points.
- `game/README.md` - Session Runtime Turn And Persistence Model section rewritten: the Step-chain bound is now `engine.Limits.MaxStepsPerTurn`, enforced inside `engineservice`, not `sessionlifecycle`; new paragraph stating `sessionlifecycle` never reconstructs a `Snapshot` or drains a Step chain itself.
- `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` - `internal/runtimeturn.MaxSteps` reference corrected.
- `game/CURRENT_STATE.md`, `game/docs/DATA_MODEL.md`, `game/docs/FLOWS.md` - every `internal/runtimeturn`/`reconstructCurrentSnapshot`/`Drain` reference corrected to describe `engineservice.StartTurn`/`AdvanceTurn` ownership; `FLOWS.md`'s two sequence diagrams (Start, AnswerInteraction) redrawn around the new calls, and one pre-existing, unrelated stale `Path, Slot` signal-construction mention (predating this WORK, from before WORK-0026/0027's own addressing rework) corrected in passing since it was on a line already being rewritten for this WORK's own reasons.
- `game/docs/decisions/GAME-ADR-0019-...md`, `GAME-ADR-0024-...md`, `GAME-ADR-0027-...md` - each gained an "Implemented by" cross-reference (historical decision text itself unedited, per this repository's Historical Immutability convention).

Known limitations:
- None beyond what GAME-ADR-0027/this WORK's own Approved Design already scoped out (caching, new RuntimeTurn causes beyond `answerInteractionSourceKind`).

Ready for independent review:
YES.
