# WORK-0012: Timer Obligations (Ordinary + Keyed)

Status: IMPLEMENTING
Created: 2026-09-20
Last status change: 2026-09-25 (READY -> IMPLEMENTING: implementation pass completed against this WORK's own Approved Design - see "Implementation Report" below. Prior status change, same day: DRAFT -> READY: Blockers 1-2 resolved by explicit human decision - see "Blockers" below. Prior status change, same day: PLANNED -> DRAFT: Approved Design drafted, folding WORK-0013's remaining scope in per explicit human direction - see "Scope Merge (2026-09-25, Human-Directed)" below)

Related decisions:
- GAME-ADR-0007 (Session Runtime Turn And Persistence Model - RuntimeTurn as the historical/transactional unit, the Turn/Interaction/Timer relationship rules this WORK extends)
- GAME-ADR-0008 (no durable due-at, full-configured-delay recovery tradeoff)
- GAME-ADR-0012 (Game Language Keyed Timer Slots - accepts the `KeyedTimerSlot<Key>` capability and the `session_timer_obligations.engine_key` persistence consequence this WORK implements; its own "Implemented by" note already confirms the compiler/engine/program portion is DONE via WORK-0025)
- GAME-ADR-0013 (process-agnostic timer recovery)
- GAME-ADR-0018 (RUNNING serialization - timer expiration contends for the same boundary as interaction responses)
- GAME-ADR-0019 (no-ACTIVE-obligations-after-terminalization invariant, closure provenance fields, state vocabulary reuse)
- GAME-ADR-0024 (Replay-First Session Runtime Persistence - `session_timer_obligations` already gives a TimerExpired occurrence a durable home; this WORK confirms, does not newly design, that fit)

Canonical context:
- `docs/projects/active/session-runtime-v1/PROJECT.md`
- `game/language/v1/program/timer.go` (`TimerSlotDeclaration`/`ScheduleTimerOperation`/`CancelTimerOperation`, `KeyedTimerSlotDeclaration`/`ScheduleKeyedTimerOperation`/`CancelKeyedTimerOperation` - all already implemented)
- `game/language/v1/engine/output.go` (`ScheduleTimerOutput`/`CancelTimerOutput`/`ScheduleKeyedTimerOutput`/`CancelKeyedTimerOutput` - already produced by the engine, currently unhandled anywhere in Session Runtime)
- `game/language/v1/engine/signal.go` (`SignalKindTimerExpired`/`SignalKindKeyedTimerExpired`)
- `game/session/workflows/sessionlifecycle/internal/interactions/capture.go` (the sibling durable-capture package this WORK's own `timers` package mirrors)
- `game/session/workflows/sessionlifecycle/internal/replay/replay.go` (`loadReplaySignal` - this WORK adds the `TIMER_EXPIRED` case)
- `docs/projects/active/session-runtime-v1/works/WORK-0013-keyed-timers.md` (CANCELLED, superseded - its remaining scope is folded into this WORK; see "Scope Merge" below)
- `docs/projects/active/session-runtime-v1/works/WORK-0020-role-aware-live-connections.md` (owns rebuilding the `play` Coordinator this WORK's physical scheduling extends; `play` does not currently exist - deleted in full by WORK-0005's Blocker 11)

## Outcome

An authored Game Language `TimerSlot` **or** `KeyedTimerSlot<Key>` obligation is durably scheduled, survives process loss/restart, and its expiration re-enters Session Runtime as a RuntimeTurn cause exactly like an interaction response does today.

Today, `ScheduleTimerOutput`/`CancelTimerOutput`/`ScheduleKeyedTimerOutput`/`CancelKeyedTimerOutput` are real engine outputs but `grep "Timer"` across `game/session/` returns zero matches - nothing schedules, cancels, or expires a timer obligation anywhere in Session Runtime.

## Context

This is required before any authored game that uses a timer (a countdown per question, a turn-time-limit, an independent per-player disconnect-grace timeout, etc.) can actually run to completion in Session Runtime - without it, a `ScheduleTimerOutput`/`ScheduleKeyedTimerOutput` the engine produces is simply dropped, and the game logically waiting on that timer can never receive its expiration.

## Scope Merge (2026-09-25, Human-Directed)

While resequencing this Project after WORK-0006/0007/0029 (all DONE), a reconciliation pass found `WORK-0013-keyed-timers.md`'s premise stale: it states `KeyedTimerSlot` "does not exist in code anywhere today," but `game/language/v1/program/timer.go`'s `KeyedTimerSlotDeclaration`/`ScheduleKeyedTimerOperation`/`CancelKeyedTimerOperation`, `engine.ScheduleKeyedTimerOutput`/`CancelKeyedTimerOutput`, and `SignalKindKeyedTimerExpired` are all fully implemented today (compiler, runtime, engine) - landed by the now-completed `WORK-0025-keyed-interaction-slots.md`. `GAME-ADR-0012` itself already carries an "Implemented by" note confirming this and stating the **only** remaining piece is the Session Runtime persistence consequence (`session_timer_obligations.engine_key`) - which is this WORK's own territory, not a separate compiler/engine design effort.

Per explicit human direction (2026-09-25): WORK-0013 is folded into this WORK rather than kept as a separate design effort. This WORK's scope now covers ordinary **and** keyed timer persistence/expiration together, designed and implemented once. `WORK-0013-keyed-timers.md` is marked `CANCELLED` as superseded (see its own file for the cross-reference), preserved as closed historical record per `docs/work/README.md`. `PROJECT.md`'s Phase 1 table is updated accordingly (see its own change).

A second, smaller drift was found in the same pass: `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md`'s `session_timer_obligations` schema (and `GAME-ADR-0007`/`GAME-ADR-0012`'s own text) list an `engine_path` column, mirroring how `session_interactions` originally carried `engine_path`/`engine_slot`. But `engine.ScheduleTimerOutput`/`CancelTimerOutput`/`ScheduleKeyedTimerOutput`/`CancelKeyedTimerOutput` and `Signal`'s timer-addressing fields (`Slot`, `Key`) have never carried a `Path` field, in this repository's entire history (confirmed via `git log -p` on `output.go`) - unlike questions, which did carry `engine_path`/`engine_slot` before `WORK-0026`/`WORK-0027` replaced that addressing with `engine_interaction_id`. `engine_path` for timers was therefore never grounded in an actual engine field; it appears to have been copied from the interaction pattern when `GAME-ADR-0007`/`GAME-ADR-0012` were written, without verifying `ScheduleTimerOutput`'s actual shape. This WORK's persisted schema (below) accordingly has **no `engine_path` column** - only `engine_slot`/`engine_key`. `SESSION_RUNTIME_PERSISTENCE_MODEL.md`'s "Keyed Timer Discriminator" section and mermaid diagram are corrected to match (see this WORK's own Documentation Impact).

## Scope

### In Scope

- Durable persistence for a pending timer obligation - both the existing single-pending-timer `TimerSlot` shape and the `KeyedTimerSlot<Key>` shape - in a new `session_timer_obligations` table (`engine_slot`, nullable `engine_key`, `delay_ms`, `state`, Turn-relationship/closure-provenance fields mirroring `session_interactions`). Needs no `play`/transport code.
- A new shared `internal/timers` package capturing `ScheduleTimerOutput`/`CancelTimerOutput`/`ScheduleKeyedTimerOutput`/`CancelKeyedTimerOutput` from every Turn-producing step's outputs, mirroring the already-established `internal/interactions` package's role for questions.
- `session_runtime_turns.source_timer_obligation_id` (new nullable column, mirroring the existing `source_interaction_id`) and the corresponding `CreateRuntimeTurn` parameter.
- A new `Manager.ExpireTimer` step: given a still-ACTIVE obligation's UUID, drives a `SignalKindTimerExpired`/`SignalKindKeyedTimerExpired` signal through `engineservice.AdvanceTurn` under the same per-Session serialization/reload-after-lock discipline `AnswerInteraction` already uses (GAME-ADR-0018), durably transitions the obligation to `CONSUMED`, and returns the same `Outputs`/`TerminalReason` shape `StartResult`/`AnswerInteractionResult` already return.
- `internal/replay.loadReplaySignal`'s new `TIMER_EXPIRED` case, reconstructing the correct ordinary/keyed `engine.Signal` from a durable obligation, mirroring the existing `INTERACTION_RESPONSE` case.
- Cancellation semantics matching `CancelTimerOperation`/`CancelKeyedTimerOperation`'s authored meaning: the matching ACTIVE row transitions to `CANCELLED`; cancelling an already-empty slot/key is a no-op (the engine itself produces no Output for it).
- Extending the no-ACTIVE-obligations-after-terminalization invariant (GAME-ADR-0019), today enforced only for interactions, to timer obligations: every currently-existing terminal path (`Start`'s completion-terminated branch, `AnswerInteraction`'s completion-terminated branch and fatal path) additionally cancels every still-ACTIVE obligation for the Session, and the new `ExpireTimer` step's own completion-terminated/fatal paths do the same from the start.
- Correctness after process loss/restart, using the accepted full-configured-delay recovery tradeoff (GAME-ADR-0008/0013) rather than a durable due-at timestamp: `delay_ms` is durably recorded per obligation specifically so a future recovery pass can reschedule from it.

### Out of Scope

- Physical scheduling/wakeup - added to the `play` Coordinator WORK-0020 (re)builds, not a mechanism this WORK invents independently of WORK-0020's own registry/lifecycle design. This WORK never calls a clock, a timer library, or `time.AfterFunc` anywhere in its own code.
- How the future Coordinator learns about a newly-scheduled/newly-cancelled obligation (synchronously from a mutation's own result, vs. re-querying ACTIVE obligations) - left to WORK-0020's own design; see Blocker 2.
- Disconnect-driven timer usage (WORK-0015) - a consumer of this mechanism, not this WORK's own scope.

## Approved Design

### Persisted schema: `session_timer_obligations`

```sql
CREATE TABLE session_timer_obligations (
    id BIGSERIAL PRIMARY KEY,
    uuid TEXT NOT NULL UNIQUE,
    session_id BIGINT NOT NULL,
    engine_slot TEXT NOT NULL,
    engine_key JSONB NULL,
    delay_ms BIGINT NOT NULL,
    state TEXT NOT NULL,
    created_by_turn_id BIGINT NOT NULL,
    closed_by_turn_id BIGINT NULL,
    closure_reason TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX session_timer_obligations_active_key
ON session_timer_obligations (session_id, engine_slot, (COALESCE(engine_key, 'null'::jsonb)))
WHERE state = 'ACTIVE';
```

- `uuid` is the obligation's own external identity - the handle a future Coordinator correlates a physical wall-clock timer against, and the parameter `Manager.ExpireTimer` takes (mirroring `session_interactions.uuid`/`InteractionUUID`).
- No `engine_path` column - see "Scope Merge" above.
- `engine_key` is `NULL` for an ordinary `TimerSlot` timer, and the authored key encoded via `engineservice.EncodeValue` (the same wire shape `session_interactions.response_payload`/`session_runtime_starts.root_parameters` already use) for a `KeyedTimerSlot` timer.
- `state` values: `ACTIVE`, `CONSUMED` (closed by its own expiration), `CANCELLED` (closed by an authored cancel, or by terminal cleanup) - reusing the existing state vocabulary GAME-ADR-0019 already establishes for this table.
- The partial unique index is defense-in-depth mirroring `session_interactions_active_interaction_key`'s precedent, not the sole correctness mechanism - the engine itself already atomically fails the whole transition when scheduling into an occupied `(slot[, key])` tuple, so no durable row for a rejected schedule is ever written in the first place. The `COALESCE`-based expression handles ordinary timers' `NULL` key (Postgres treats two `NULL`s in a unique index as distinct, which would otherwise let two ordinary timers coexist on the same slot); see Implementation Freedom for the residual key-encoding edge case.

### `session_runtime_turns.source_timer_obligation_id`

A new nullable column, mirroring the existing `source_interaction_id` exactly (added by this WORK's own migration, alongside the `session_timer_obligations` table). `CreateRuntimeTurn` gains a corresponding `sourceTimerObligationID *uint` parameter; every existing call site (`Start`, `AnswerInteraction`) passes `nil`; the new `ExpireTimer` step passes the causing obligation's id and `nil` for `sourceInteractionID`. `actorID` is `nil` for a timer-caused Turn, the same way it already is for `Start`'s own Turn - a timer expiration is a Coordinator-reported wall-clock event, not an actor's own action.

### `internal/timers` package (new, mirrors `internal/interactions`)

```go
func Capture(ctx context.Context, tx *gorm.DB, repo CaptureRepo, sessionID uint, turnID uint, outputs []engine.Output) error
```

Iterates `outputs`, handling:

- `engine.ScheduleTimerOutput` / `engine.ScheduleKeyedTimerOutput` -> `repo.CreateTimerObligation(ctx, tx, sessionID, slot, encodedKeyOrNil, delayMs, turnID)`.
- `engine.CancelTimerOutput` / `engine.CancelKeyedTimerOutput` -> `repo.CancelActiveTimerObligation(ctx, tx, sessionID, slot, encodedKeyOrNil, turnID)` (matches the currently-ACTIVE row for that `(slot[, key])`; a no-op if none exists, mirroring `CloseActiveInteraction`'s already-accepted no-op precedent).

Called from every Turn-producing step exactly where `interactions.Capture` already is: `step_start.go`, `step_answer_interaction.go`, and the new `step_expire_timer.go` - a Turn that both closes one timer and schedules another (or completes the game instance) must have both effects captured atomically in the same transaction as everything else that Turn produced.

### `internal/replay.loadReplaySignal`: new `TIMER_EXPIRED` case

```go
const TimerExpiredSourceKind = "TIMER_EXPIRED"
```

Given a `RuntimeTurnRecord` with `SourceKind == TimerExpiredSourceKind`, loads the obligation by `turn.SourceTimerObligationID` (new `Repo.GetTimerObligationByID`) and rebuilds:

- `engine.Signal{Kind: engine.SignalKindTimerExpired, Slot: obligation.EngineSlot}` when `EngineKey` is `NULL`;
- `engine.Signal{Kind: engine.SignalKindKeyedTimerExpired, Slot: obligation.EngineSlot, Key: decodedKey}` otherwise (decoded via `engineservice.DecodeValue`).

Mirrors `buildAnswerSignal`'s existing shape/error-handling exactly (an unresolvable obligation or undecodable key is a `monitoring.Alert` + data-integrity error, never an ordinary decline - the same standard already applied to an unresolvable interaction).

### `Manager.ExpireTimer` (new step, `step_expire_timer.go`)

```go
func (m *Manager) ExpireTimer(ctx context.Context, timerObligationUUID TimerObligationUUID) (ExpireTimerResult, error)
```

Structurally mirrors `AnswerInteraction` exactly, substituting the interaction lookup/response-decoding with an obligation lookup and a `SignalKindTimerExpired`/`SignalKindKeyedTimerExpired` signal:

1. `ResolveSessionForTimerObligation(ctx, timerObligationUUID)` - unlocked pre-lookup (mirrors `ResolveSessionForInteraction`).
2. `sessionlock.LockByID`, then re-read the obligation under lock (mirrors `FindInteractionByUUID`'s re-read pattern).
3. If the obligation is not found: `session.ErrTimerObligationNotFound` (new sentinel, mirrors `ErrInteractionNotFound`).
4. If `obligation.State != ACTIVE`: return `ExpireTimerOutcomeStale` - an ordinary declined outcome (already-consumed or already-cancelled obligation; a duplicate/late physical-timer delivery). No RuntimeTurn, no engine call.
5. Otherwise: `replay.LoadPriorSignals`, build the `engine.Signal` (ordinary or keyed, from the obligation's own `engine_slot`/`engine_key`), `engineservice.AdvanceTurn`. `ErrReplayDivergence` is a data-integrity alert exactly like `AnswerInteraction`'s. `ErrSignalRejected`/`ErrInputRejected` (the engine's own stale/cancelled-delivery backstop, per `program/timer.go`'s documented contract) is `ExpireTimerOutcomeRejected`, an ordinary decline. Any other engine failure is a fatal path (`terminalizeExpireTimerFatal`, mirroring `terminalizeAnswerInteractionFatal`, additionally cancelling active obligations - see below).
6. On success: `CreateRuntimeTurn(..., timerExpiredSourceKind, sourceInteractionID: nil, actorID: nil)` with the new `sourceTimerObligationID` set; `repo.CloseTimerObligation(obligationID, CONSUMED, turnID)`; `interactions.Capture` + `timers.Capture` for this Turn's own outputs (a timer's expiration may itself open a question, schedule another timer, etc.); `SetCurrentTurn`; `completion.Detect` + terminal handling identical to `AnswerInteraction`'s, extended to also cancel active timer obligations (see next section); `mapOutputs` for the client-facing subset; return `ExpireTimerResult{Outcome: ExpireTimerOutcomeExpired, SessionUUID, Outputs, TerminalReason}`.

No idempotency key is required - identical reasoning to `AnswerInteraction`: the obligation's own persisted state is the dedup identity. A duplicate physical-timer delivery against an already-`CONSUMED` obligation replays `ExpireTimerOutcomeStale` without a second engine effect.

### Terminal cleanup: extending the no-ACTIVE-obligations invariant to timers

A new `CancelAllActiveTimerObligationsForSession(ctx, tx, sessionID, reason)` repo method, mirroring `CloseAllActiveInteractionsForSession` exactly (bulk `UPDATE ... SET state = 'CANCELLED', closure_reason = ? WHERE session_id = ? AND state = 'ACTIVE'`), is called alongside every existing `CloseAllActiveInteractionsForSession` call site:

- `answerInteractionInTx`'s completion-terminated branch (`step_answer_interaction.go`).
- `terminalizeAnswerInteractionFatal` (`step_answer_interaction.go`).
- The new `ExpireTimer`'s own completion-terminated branch and `terminalizeExpireTimerFatal`.

`startSessionInTx`'s completion-terminated branch already calls `CloseAllActiveInteractionsForSession` and gains the same sibling call, even though a first Turn scheduling and then immediately completing within the same Turn is an unusual authored pattern - the invariant must hold unconditionally, not only for the paths expected to exercise it in practice. `terminalizeStartFatal` needs no change: it terminalizes before any Turn (and therefore any obligation) can exist.

## Constraints and Invariants

- No durable due-at timestamp is required to be exact (GAME-ADR-0008's accepted tradeoff); recovery uses the full configured delay (`delay_ms`), not a resumed countdown. This WORK never computes or persists an absolute deadline.
- Timer expiration must serialize correctly against a concurrent interaction response (or another timer expiration) for the same Session (GAME-ADR-0018) - `ExpireTimer` acquires the same per-Session row lock and reloads current state after acquiring it, exactly like `AnswerInteraction`.
- No physical clock, timer library, or scheduling loop is introduced by this WORK anywhere in `game/session` - see Out of Scope.
- `engine_path` is not part of the persisted schema - see "Scope Merge" above.
- A `TERMINAL` Session must never retain an `ACTIVE` `session_timer_obligations` row (GAME-ADR-0019, extended by this WORK from interactions-only to also cover timer obligations).

## Acceptance Criteria

1. Scheduling an ordinary `TimerSlot` timer during `Start` or `AnswerInteraction` durably persists an `ACTIVE` `session_timer_obligations` row with the correct `engine_slot`/`delay_ms`/`created_by_turn_id`, `engine_key` NULL.
2. Scheduling a `KeyedTimerSlot` timer persists `engine_key` alongside `engine_slot`; two different keys under the same slot may be simultaneously `ACTIVE`.
3. Scheduling into an already-occupied `(slot[, key])` tuple fails the whole transition atomically (engine-enforced); no `session_timer_obligations` row is left behind and no other output of that attempted Turn is persisted.
4. `CancelTimerOperation`/`CancelKeyedTimerOperation` durably transitions the matching `ACTIVE` row to `CANCELLED` with the causing Turn as `closed_by_turn_id`; cancelling an already-empty slot/key changes no row (idempotent no-op).
5. `Manager.ExpireTimer` given a still-`ACTIVE` obligation's UUID drives a new RuntimeTurn via `SignalKindTimerExpired`/`SignalKindKeyedTimerExpired`, durably transitions the obligation to `CONSUMED`, and returns the same `Outputs`/`TerminalReason` shape `Start`/`AnswerInteraction` already return.
6. `Manager.ExpireTimer` given an already-`CONSUMED`/`CANCELLED` obligation UUID (a stale/duplicate physical delivery) returns `ExpireTimerOutcomeStale` without creating a RuntimeTurn or mutating any obligation.
7. `Manager.ExpireTimer` for an unknown obligation UUID returns `session.ErrTimerObligationNotFound`.
8. Timer expiration correctly serializes against a concurrent interaction response for the same Session (a concurrency integration test analogous to the existing `AnswerInteraction`/`Start` ones, exercising the GAME-ADR-0018 lock boundary).
9. `session_runtime_turns.source_timer_obligation_id` correctly identifies the causing obligation, and `internal/replay.loadReplaySignal` reconstructs the correct ordinary/keyed `engine.Signal` from it, proven by a replay-reconstruction test mirroring the existing interaction-response case (including a multi-Turn Session mixing interaction responses and timer expirations, replayed end-to-end).
10. A Session terminalizing via game completion or a fatal failure leaves no `ACTIVE` `session_timer_obligations` row - every still-`ACTIVE` row is atomically `CANCELLED` with `closure_reason = SESSION_TERMINATED`.
11. No production code path in this WORK schedules, delays, or delivers a physical timer - `Manager.ExpireTimer` is only ever invoked directly by a caller (a test today).

## Blockers

All resolved. Preserved below as a record of the decisions, per this Project's own convention (compare WORK-0003/WORK-0029's own resolved-Blockers sections).

1. **Cross-session `ListActiveTimerObligations` read API shape/placement - RESOLVED (2026-09-25, human-directed): internal only for now.** No `Manager`-level public method is added by this WORK. A repo-level query capability exists where this WORK's own implementation/tests need it (e.g. to prove recovery-relevant facts), but its public shape/placement for a future recovery pass or WORK-0020's Coordinator is designed later, by whichever WORK actually consumes it - not guessed at here ahead of a real consumer.
2. **How WORK-0020's future Coordinator learns about a newly-scheduled/newly-cancelled obligation - RESOLVED (2026-09-25, human-directed): deferred entirely to WORK-0020.** This WORK adds no schedule/cancel notice to `StartResult`/`AnswerInteractionResult`/`ExpireTimerResult`. WORK-0020 decides its own mechanism (push vs. query) when it designs the Coordinator, matching this WORK's original scope boundary (physical scheduling is WORK-0020's own concern).
3. **`engine_key` uniqueness relies on `COALESCE`-wrapped JSONB equality**, which is reliable for primitive key types (string/number/bool) but could, for a structurally-complex authored key type (a record/list key), admit two encodings of the same semantic value with different byte layout evading the partial unique index. The engine's own atomic occupied-tuple check remains the actual correctness backstop (see Approved Design), so this is a defense-in-depth gap, not a correctness gap - left as an Implementation Freedom item, not a blocker to READY.

## Documentation Impact

- `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` - "Keyed Timer Discriminator" section and both mermaid diagrams' `session_timer_obligations` class corrected to drop `engine_path` (see "Scope Merge" above), and the Turn/Timer relationship/worked example sections confirmed against the actually-implemented `source_timer_obligation_id` column.
- `game/docs/DATA_MODEL.md` - `session_timer_obligations` table and its relationships to `sessions`/`session_runtime_turns` added, matching the pattern already used for `session_interactions`.
- `game/CURRENT_STATE.md` / `game/docs/FLOWS.md` - Timer scheduling/cancellation/expiration flow added alongside the existing Interaction flow, once implemented.
- `game/README.md` - Session Runtime Turn And Persistence Model section's timer-obligation mention updated from "not yet implemented" to reference this WORK.

## Implementation Report (2026-09-25)

Implemented against this WORK's own Approved Design, exactly as designed - no material deviation.

- **Migrations**: `20260925000000_session_timer_obligations.go` (creates `session_timer_obligations`: `engine_slot`/`engine_key` (JSONB, nullable)/`delay_ms`/`state`/`created_by_turn_id`/`closed_by_turn_id`/`closure_reason`, no `engine_path`; a `COALESCE`-normalized partial unique index on `(session_id, engine_slot, engine_key)` where `state = 'ACTIVE'`) and `20260925000001_...source_timer_obligation_id.go` (adds `session_runtime_turns.source_timer_obligation_id`), both registered in `migration.go`. The stale `engine_path` comment in `20260924000000_session_interactions_engine_interaction_id.go` was corrected in place.
- **`internal/repo/timer_obligation.go`** (new): `TimerObligation`, `ResolveSessionForTimerObligation`, `FindTimerObligationByUUID`, `GetTimerObligationByID`, `CreateTimerObligation`, `CancelActiveTimerObligation`, `CloseTimerObligation`, `CancelAllActiveTimerObligationsForSession` - mirroring `internal/repo/interaction.go`'s method shapes throughout.
- **`internal/repo/runtime_turn.go`**: `source_timer_obligation_id` added to `runtimeTurnInsert`/`RuntimeTurnRecord`; `CreateRuntimeTurn` gained a `sourceTimerObligationID *uint` parameter (every existing call site updated).
- **`internal/timers` package** (new): `Capture` (handles `ScheduleTimerOutput`/`CancelTimerOutput`/`ScheduleKeyedTimerOutput`/`CancelKeyedTimerOutput`), mirroring `internal/interactions.Capture`; called from `step_start.go`, `step_answer_interaction.go`, and the new `step_expire_timer.go`.
- **`internal/replay/replay.go`**: `TimerExpiredSourceKind` constant, `Repo.GetTimerObligationByID`, `loadReplaySignal`'s new `TIMER_EXPIRED` case, and `buildTimerExpiredSignal` (ordinary vs. keyed signal reconstruction from a durable obligation).
- **`step_expire_timer.go`** (new): `Manager.ExpireTimer`, `expireTimerRepoAPI`, `terminalizeExpireTimerFatal` - structurally mirrors `AnswerInteraction`/`terminalizeAnswerInteractionFatal` throughout (same lock/reload/replay/fatal-path discipline).
- **Terminal cleanup**: `CancelAllActiveTimerObligationsForSession` wired into every existing terminal-materialization call site (`step_start.go`'s completion-terminated branch, `step_answer_interaction.go`'s completion-terminated branch and fatal path) plus `ExpireTimer`'s own two.
- **`session.go`**: `TimerObligationStateActive/Consumed/Cancelled`, `TimerObligationClosureReasonSessionTerminated`, `ErrTimerObligationNotFound`.
- **`types.go`**: `TimerObligationUUID`, `ExpireTimerOutcome` (`Expired`/`Stale`/`Rejected`/`RuntimeExecutionFailed`), `ExpireTimerResult`.
- **`manager.go`**: `expireTimerRepo` field wired in `New`; `mockgen` directive extended and mocks regenerated.

**Tests added**: `internal/timers/capture_test.go` (pure, mirrors `internal/interactions/capture_test.go` - schedule/cancel, ordinary/keyed, no-op, invalid-delay cases); `step_expire_timer_test.go` (mocked pre-transaction path, mirrors `TestManagerAnswerInteraction`); `step_expire_timer_integration_test.go` (real-Postgres: AC1-AC8, AC10 - scheduling persistence, expiration/consumption, stale replay, unknown UUID, keyed-timer independence, cancellation via an authored operation, atomic double-schedule failure, terminal cleanup); `replay_timer_integration_test.go` (real-Postgres: AC9 - ordinary and keyed `TIMER_EXPIRED` replay reconstruction, mirroring `TestReconstructCurrentSnapshot_Integration`'s process-loss-recovery proof style). New fixtures added to `testutil_test.go`: `timerDefinition`, `timerCancelledOnAnswerDefinition`, `doubleScheduleTimerDefinition`, `keyedTimerDefinition`, `timerActiveAtTerminationDefinition`.

**Verification**: `go build ./...`, `go vet ./...`, and `gofmt -l` (changed files) all clean repository-wide. `go test ./... -count=1` - every mocked-collaborator/pure-function test passes, including all new `internal/timers`/`step_expire_timer_test.go` cases; every real-Postgres repository-integration test (including all of this WORK's own new `_Integration` tests) self-reports `SKIP` - this sandbox has no reachable Docker/Postgres engine, the same known limitation recorded by every prior checkpoint across this Project's history, not a new gap introduced here.

**Open gates before DONE** (per `docs/ai/protocols/IMPLEMENTATION_REVIEW.md`/`docs/work/README.md`): real-Postgres execution of this WORK's new integration tests, and independent implementation review. Neither has happened yet - `Status` is `IMPLEMENTING`, not `DONE`.

## Completion Record

Not yet DONE. Implementation complete against the Approved Design (see "Implementation Report" above); real-Postgres verification and independent review remain outstanding.
