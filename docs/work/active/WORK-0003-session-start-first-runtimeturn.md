# WORK-0003: Session Start + First RuntimeTurn

Status: DRAFT
Created: 2026-09-19
Last status change: 2026-09-19

Related decisions:
- `game/docs/decisions/GAME-ADR-0004-session-lobby-lifecycle-contract.md` (Start's contract, host authority, players.min/max, JoinCode revocation)
- `game/docs/decisions/GAME-ADR-0005-session-public-and-internal-identity-boundary.md` (UserUUID -> SessionActorID resolution)
- `game/docs/decisions/GAME-ADR-0006-game-language-root-player-roster-contract.md` (`players: list<user>` root roster)
- `game/docs/decisions/GAME-ADR-0007-session-runtime-turn-and-persistence-model.md` (RuntimeTurn/RuntimeStep/session_runtime_state model)
- `game/docs/decisions/GAME-ADR-0017-session-runtime-failure-classification-and-diagnostic-persistence.md` (failure taxonomy; this WORK only reaches the fatal-terminalization consequence, not the deferred `session_runtime_failures` diagnostic entity)
- `game/docs/decisions/GAME-ADR-0018-session-running-mutation-serialization.md` (RUNNING reuses the same per-Session DB-locking mechanism as LOBBY)
- `game/docs/decisions/GAME-ADR-0019-runtimeturn-execution-bound-and-terminal-cleanup.md` (`MAX_STEPS_PER_RUNTIME_TURN = 20`, pre-first-Turn fatal Start semantics, `base_turn_id` nullability)
- `game/docs/decisions/GAME-ADR-0021-session-lobby-command-idempotency-token-semantics.md`, `GAME-ADR-0022-session-lobby-business-declines-as-workflow-outcomes.md` (extended here to Start's own declines, not superseded)

No new ADR is proposed by this WORK. Everything below operationalizes already-ACCEPTED architecture; see Blockers for the handful of implementation-planning gaps that fill in details those ADRs left open, which this DRAFT resolves with a proposed reading rather than inventing new architecture.

Canonical context:
- `game/README.md` (Session Runtime Lobby Lifecycle Contract - Start; RUNNING Mutation Serialization; Turn And Persistence Model; Failure Classification And Diagnostic Persistence; Terminal Cleanup)
- `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` (Runtime History Tables ER diagram, Turn/Interaction/Timer worked example)
- `game/language/v1/engine/LOGICAL_CONTRACT.md`, `game/language/v1/engine/engineservice/{compile.go,runtime.go,codec.go}` (Compile, NewSnapshot, Step, EncodeSnapshot)
- `docs/work/completed/WORK-0001-session-lobby-foundation.md` (the Manager/repo/sessionlock/idempotency pattern this WORK extends, not replaces)
- `docs/engineering/standards/{domain-logic-placement.md,repositories.md,error-handling.md,idempotency.md,data-integrity.md,function-signatures.md,testing.md}`

## Outcome

A host can start a `LOBBY` Session: Session Runtime resolves/pins/compiles the Session's already-pinned Game Definition, initializes the Game Language engine with the root `players: list<user>` roster built from active Participants, executes the first `RuntimeTurn` (draining `Commit.InternalSignals` to quiescence under a hard `MAX_STEPS_PER_RUNTIME_TURN = 20` bound), and atomically persists Turn 1 / `session_runtime_state` / `session_runtime_steps` together with `phase = RUNNING`, `started_at`, and JoinCode revocation. This is the first slice that ever executes the Game Language engine against a real Session, and it introduces the shared `RuntimeTurn` execution mechanism (Step-draining + the 20-Step bound + Turn/Step/State persistence) that Slice 3 (interaction responses) and Slice 5 (timer expirations) will reuse unchanged, per the initiative's approved sequencing (`docs/ai/workspaces/active/session-runtime-v1/PLAN.md`).

A deterministic failure anywhere in this initialization chain (including exceeding the Step bound) must terminalize the Session directly from `LOBBY` to `TERMINAL` - never leave it partially `RUNNING`, and never return it to `LOBBY` for a retry that would deterministically fail again the same way. A transient infrastructure failure must leave the Session unchanged and retryable.

## Context

Slice 1 (`WORK-0001`, DONE) delivered Create/Join/Leave against the accepted lobby schema, the per-Session `sessionlock` DB-locking mechanism, and the `session_requests` idempotency mechanism - all under one `sessionlifecycle.Manager` in `game/session/workflows/sessionlifecycle/`. `docs/engineering/standards/domain-logic-placement.md`'s own worked example of the "Preferred Workflow Package Shape" already anticipates this WORK's home: `step_start.go` alongside `step_create.go`/`step_join.go`/`step_leave.go` in the same package/Manager.

The Game Language engine (`game/language/v1/engine`, `engineservice`) is fully implemented and tested: `Compile`, `NewSnapshot`, `Step`, `EncodeSnapshot`/`DecodeSnapshot` all exist and work today. Session Runtime has never called `NewSnapshot`/`Step` before this WORK - Slice 1 only ever compiled a Definition to validate it compiles (Create) or to read `Players.Max` (Join). No `session_runtime_turns`/`session_runtime_steps`/`session_runtime_state` table exists yet.

The Game Management pinned-definition read capability (`getgamedefinition.GetGameDefinition(ctx, gameDefinitionUUID) (*program.Definition, error)`) already exists (built by Slice 1 for Join) and is reused here unchanged - no new Game Management capability is required.

## Scope

### In Scope

- A `Start` step on the existing `sessionlifecycle.Manager` (`game/session/workflows/sessionlifecycle/step_start.go`), following the same Manager/step/narrow-repo-contract/idempotency pattern as Create/Join/Leave.
- A new shared internal mechanism package, `game/session/internal/runtimeturn`, implementing the common RuntimeTurn executor: given a compiled `engine.Program`, a `engine.Snapshot`, an initiating `engine.Signal`, and a `Cause` (source metadata), it drains `Commit.InternalSignals` to quiescence, enforces `MAX_STEPS_PER_RUNTIME_TURN = 20` across the whole Turn, and - only when the Turn is not fatal - persists one `session_runtime_turns` row (the final Snapshot only), the Turn's `session_runtime_steps` rows, and creates/advances `session_runtime_state.current_turn_id`, all against the caller-supplied transaction. This is the mechanism Slice 3/5 will call unchanged for their own causes.
- New migrations for `session_runtime_state`, `session_runtime_turns`, `session_runtime_steps`, using the full accepted column shape from `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md`'s Runtime History Tables diagram (including `source_interaction_id`/`source_timer_obligation_id`, left unused/NULL until Slices 3/5 populate them) - creating the complete accepted shape once, the same way Slice 1 created the complete `session_requests` shape even though it only used the CREATE/JOIN/LEAVE operation subset.
- Start's own business/lifecycle policy: resolving `(SessionUUID, UserUUID) -> SessionActorID` and verifying host authority; validating `LOBBY` phase and `lobby_expires_at` (reusing Slice 1's existing lazy-expiration materialization); loading the pinned Definition and validating active-Participant count against `players.min` (and defensively `players.max`); building the `players: list<user>` root roster from active Participants; compiling the pinned Definition; calling `engineservice.NewSnapshot`; invoking the shared RuntimeTurn executor; and, only on a successful (non-fatal) Turn, atomically transitioning `phase = RUNNING`, setting `started_at`, and revoking the active JoinCode (`RevokeActiveJoinCode`, already implemented, unused until now) - all in the one transaction the executor's persistence also participates in.
- The pre-first-Turn deterministic fatal-failure path (GAME-ADR-0019): on a deterministic failure (a non-rejection `ExecutionError`, a rejection of Start's own initial/internal signal chain, or the Step-bound overflow) reported by the executor, Start's same transaction instead sets `phase = TERMINAL`, `terminal_at = now`, an internal terminal reason (`RUNTIME_EXECUTION_FAILED`, or `RUNTIME_STATE_INVALID` for the one narrow pinned-Definition-recompile-failure case - see Approved Design), leaves `started_at` at its canonical `NULL`, and writes no Turn/Step/State row - reusing the existing `SetSessionTerminal` repository method built for lobby expiration in Slice 1. No `session_runtime_failures` row is created (that diagnostic entity is Slice 6's scope, per the initiative plan).
- Start's own request/command idempotency (`(user_uuid, "START", idempotency_key)`), following the same `session_requests` claim/replay/conflict pattern as Create/Join/Leave, including replay of a previously recorded deterministic decline (including the fatal `RUNTIME_INIT_FAILED` case - see Approved Design).
- Distinguishing deterministic runtime failure (a committed, replayable decline outcome; Session becomes `TERMINAL`) from transient infrastructure failure (an ordinary Go `error`; transaction rolls back; Session remains `LOBBY`, retryable) throughout this whole path.
- Current-state documentation updates required by this implementation (`game/CURRENT_STATE.md`, `game/docs/DATA_MODEL.md`, `game/docs/FLOWS.md`).

### Out of Scope

- Everything Slice 3 onward owns: interaction response processing, `session_interactions` persistence, the thin live Coordinator/WebSocket, timer obligations, the `session_runtime_failures` diagnostic entity and its stable error codes, terminal-cleanup-of-still-open-obligations (there are none yet - Start's own Turn cannot itself open an interaction/timer that later needs cleanup, since this WORK does not implement interaction/timer creation), disconnect/reconnect/resync, `activity_expires_at`/inactivity expiration/Reaper, keyed timers, and archival. None of these are needed for Start to correctly execute and persist Turn 1.
- Any change to Create/Join/Leave's existing approved design, schema, or behavior beyond the new `RevokeActiveJoinCode` call this WORK adds to the Start path (the method itself already exists, unused, from Slice 1).
- Any change to the Game Language engine/compiler/`program` package. Start only calls already-existing `engineservice` entry points.
- Identity/Auth implementation. Start receives an already-trusted `UserUUID`, per the same accepted boundary Slice 1 already established (GAME-ADR-0005).
- Configurability of `MAX_STEPS_PER_RUNTIME_TURN`, `engine.Limits`, or the random `Seed` source beyond a single code-level constant/reasonable default - no configuration surface is introduced.
- Player-roster ordering as a designed product decision (see Approved Design's default and Blockers).

## Approved Design

### Where Start Lives

`Start(ctx, sessionUUID SessionUUID, userUUID UserUUID, idempotencyKey IdempotencyKey) (StartResult, error)` is added to the existing `sessionlifecycle.Manager` in `step_start.go`, with its own narrow `startRepoAPI` persistence contract (per `domain-logic-placement.md`'s Workflow Grouping Does Not Imply A Shared Repository Contract) even where it overlaps method shapes already used by Create/Join/Leave. Start reuses `m.pinnedGameReader` (already present on `Manager` for Join) to load the pinned Definition - no new Game Management dependency is introduced.

### Transaction and Serialization Boundary

Identical shape to Join/Leave: an unlocked pre-transaction read resolves the Session's pinned `game_definition_uuid` and `host_actor_id` are not safely readable before the lock (unlike Join's JoinCode resolution, Start's `SessionUUID` already identifies the Session directly, so there is no unlocked pre-lookup step other than loading the pinned Definition once the Session row is known to exist). `Manager.Start` calls `utils.RunInDBTransaction(ctx, m, callback)`; inside, `sessionlock.LockByID`/`LockByUUID` acquires the same per-Session `FOR UPDATE` lock GAME-ADR-0018 requires RUNNING mutations to reuse. `materializeExpirationIfDue` (already shared by Join/Leave) runs first, exactly as today. Everything else - host/roster/min-max validation, engine initialization, the RuntimeTurn executor call, the RUNNING transition, JoinCode revocation, and the idempotency claim's completion - happens inside this same single transaction and commits or rolls back together.

### The Shared RuntimeTurn Executor (`game/session/internal/runtimeturn`)

This is a new shared internal mechanism package, justified under `repositories.md -> Sharing Rule`: the Step-draining/20-Step-bound/Turn-Step-State persistence invariant is exactly the kind of "protocol where divergent implementations would be a bug" the Sharing Rule centralizes (parallel to `sessionlock`/`idempotency`), not a horizontal entity-CRUD package.

Conceptual contract (exact Go shape is implementation freedom):

```text
Execute(ctx, tx, sessionID, program, snapshot, initialSignal, cause, limits) -> Result
```

- `cause` carries `SourceKind` (a short string label - Start's call always passes a value meaning "session start"; the exhaustive `source_kind` enum remains undecided repository-wide, consistent with `SESSION_RUNTIME_PERSISTENCE_MODEL.md`'s "Not Yet Decided" list) and the optional `actor_id`/`source_interaction_id`/`source_timer_obligation_id` fields Slices 3/5 will populate (all left unset by this WORK's own call).
- `Execute` loads `session_runtime_state.current_turn_id` for `sessionID` (nullable - absent for Start, since this is the Session's first Turn) under the already-held per-Session lock, and computes the new Turn's `sequence` as `previous + 1` (or `1` when none exists).
- `Execute` calls `engineservice.Step` once with `initialSignal`, then repeatedly with each subsequently queued `InternalSignal` (FIFO) until the queue is empty (quiescence), counting every `Step` call (the initial one plus every internal-signal-triggered one) toward the 20-call bound - exactly GAME-ADR-0019's definition, applying equally to this initialization chain.
- On quiescence within the bound: persists exactly one `session_runtime_turns` row (only the final Snapshot, per GAME-ADR-0007 - intermediate Snapshots are never separately authoritative), one `session_runtime_steps` row per actual `Step` call (technical trace only), and creates (Start's case) or updates `session_runtime_state.current_turn_id`. Returns the committed Turn's id/sequence/final Snapshot/aggregated `Output`s to the caller.
- On a non-rejection `ExecutionError` from any `Step` call, or on needing a `Step` call beyond the 20th to reach quiescence: persists nothing (no Turn/Step/State row), and returns a typed fatal result (`StepLimitExceeded` or `ExecutionFailed{underlying *engineservice.ExecutionError}`) for the caller to decide the Session-lifecycle consequence - the executor itself never mutates `sessions.phase`/`terminal_reason`, since that consequence differs by caller/phase (Start's is LOBBY-direct-to-TERMINAL with `base_turn_id = NULL`; a later RUNNING-phase caller's is different) per `domain-logic-placement.md`'s Responsibility Categories.
- A rejection (`ErrSignalRejected`/`ErrInputRejected`) of the *initial* signal is also reported as a fatal result to Start specifically (see next subsection for why); a later RUNNING-phase caller (Slice 3+) may interpret the same executor-reported rejection differently for its own initial signal, since GAME-ADR-0017's "expected rejection, Session stays RUNNING" class applies once a Session has an established RuntimeTurn history to remain on - that interpretation is out of this WORK's scope to build, only to leave room for.

### Root Roster And Engine Initialization

Start builds `RootParameters["players"]` as an `engine.ListValue` of `engine.UserValue`, one per Participant currently `active = true` for the locked Session, ordered by `joined_at` ascending (ties broken by internal id) - a simple, deterministic default; see Blockers. Each `engine.UserValue.ID` is derived from that Participant's internal `session_actors.id` (an opaque string representation - `engine.UserID` is documented as "only ever compared for equality," never exposed publicly); `Identity.UserUUID` is never placed into engine state, per GAME-ADR-0005/GAME-ADR-0006.

Start compiles the pinned Definition (`engineservice.Compile`) fresh - Program is never persisted, by design (`engineservice/codec.go`'s own doc comment: recompiling is cheap, deterministic, and avoids a second wire format). Since Create already validated this exact Definition/version compiles, and Definitions are immutable once pinned, a compile failure discovered here is an unexpected data-integrity condition, not an ordinary deterministic game-execution failure - see next subsection for its classification. `InitializationInput.Seed` is drawn from a fresh unpredictable source at Start (not derived from anything guessable), once, per `InitializationInput.Seed`'s own contract.

### Fatal-Path Classification (Filling An Open Corner Of GAME-ADR-0017/0019)

Two distinct fatal cases both terminalize `LOBBY -> TERMINAL` with `started_at` left `NULL` and no Turn/Step/State row, but with different internal `terminal_reason` values:

- **`RUNTIME_STATE_INVALID`**: the pinned Definition unexpectedly fails to recompile (`engineservice.Compile` returns error diagnostics) despite having compiled successfully at Create. This is durable-state invalidity (GAME-ADR-0017's third class - "other durable state making safe reconstruction/execution impossible"), not a deterministic authored-game-logic failure; it also emits a `monitoring.Alert` (mirroring `getgamedefinition`'s existing missing-definition alert), per `data-integrity.md`.
- **`RUNTIME_EXECUTION_FAILED`**: everything downstream of a successful compile - `NewSnapshot` returning an `ExecutionError`, any `Step` call in the initial/internal-signal chain returning a non-rejection `ExecutionError`, a rejection (`ErrSignalRejected`/`ErrInputRejected`) of the chain's own signals, or the 20-Step bound being exceeded. GAME-ADR-0019 explicitly names Step-bound overflow and "a deterministic engine/runtime initialization failure" as this class; this WORK additionally reads an outright rejection of Start's own initialization chain the same way, since LOBBY has no notion of "an active interaction/RuntimeTurn the rejection leaves untouched" for the Session to safely remain on - there is nothing to stay `RUNNING` against yet (see Blockers).

### Start's Business Declines And Idempotency

Following GAME-ADR-0022's already-accepted pattern (deterministic declines are workflow outcomes, not errors), `Start` returns a `StartOutcome`:

- `StartOutcomeStarted` - success; Session is now `RUNNING`.
- `StartOutcomeLobbyExpired` - discovered/materialized expired (existing `materializeExpirationIfDue` path).
- `StartOutcomeNotHost` - the resolved `SessionActorID` is not `sessions.host_actor_id`.
- `StartOutcomeNotEnoughPlayers` - active Participant count is below the pinned Definition's `players.min`.
- `StartOutcomeRuntimeInitFailed` - the pre-first-Turn fatal path above was taken; the Session is now `TERMINAL`.

All five are recorded as the `START` idempotency claim's completed outcome and replayed identically on a same-token retry (including `StartOutcomeRuntimeInitFailed` - per `idempotency.md`'s Completed Outcomes vs. Transient Failures, a deterministic decline, even one that terminalized the Session, is a completed logical outcome and must not be silently re-executed against a Session that has since become `TERMINAL`; see Blockers for why this reading is called out explicitly rather than assumed).

A defensive `players.max` re-check (mirroring Join's) is included for robustness but is not expected to ever trigger in practice, since Join already enforces it; if it somehow does, it is treated the same as `StartOutcomeNotEnoughPlayers`'s sibling case (an ordinary LOBBY-phase decline, not fatal) - naming/grouping of this defensive case is implementation freedom.

## Constraints and Invariants

- No two RuntimeTurns may ever execute concurrently against the same Session (GAME-ADR-0018) - guaranteed here by the same `sessionlock` row lock Join/Leave already use.
- A Session becomes durably `RUNNING` only once `phase = RUNNING`, `started_at`, Turn 1/`session_runtime_state`, and JoinCode revocation all commit together in one transaction (GAME-ADR-0019). No intermediate state may be observed where some but not all of these are true.
- No partial RuntimeTurn, partial Step set, or intermediate Snapshot is ever persisted (GAME-ADR-0007/0019) - the executor writes either the complete committed Turn or nothing.
- `MAX_STEPS_PER_RUNTIME_TURN = 20` is a code-level constant, not per-game-authored, not a durable per-Session setting (GAME-ADR-0019).
- A transient infrastructure failure anywhere in this path must leave the Session unchanged (still `LOBBY`, un-terminalized, idempotency claim not completed) and safely retryable - never a permanently replayable "completed" outcome (`idempotency.md`).
- `Identity.UserUUID` never enters engine/runtime state; only Session-local identity derived from `SessionActorID` does (GAME-ADR-0005/0006).
- No `session_runtime_failures` row, table, or reference is introduced by this WORK (Slice 6's scope) - the fatal path uses only the already-existing `sessions.terminal_reason` column.

## Acceptance Criteria

- Starting a `LOBBY` Session with a valid host, an unexpired lobby, and active Participants between `players.min` and `players.max` results in: `phase = RUNNING`, `started_at` set, the active JoinCode revoked, exactly one `session_runtime_turns` row (`sequence = 1`) holding the final post-initialization Snapshot, one or more `session_runtime_steps` rows tracing every `Step` call made, and `session_runtime_state.current_turn_id` pointing at that Turn - all committed atomically.
- A pinned Definition whose root workflow requires more than 20 total `Step` calls to reach initial quiescence causes the Session to terminalize `LOBBY -> TERMINAL` (`RUNTIME_EXECUTION_FAILED`, `started_at` still `NULL`) with no Turn/Step/State row written, in the same transaction as the fatal `START` idempotency completion.
- A non-rejection deterministic `ExecutionError` during `NewSnapshot`/the initial `Step` chain produces the same `RUNTIME_EXECUTION_FAILED` outcome and durable shape as the Step-bound case above.
- A Start attempted by a non-host `UserUUID` is rejected as `StartOutcomeNotHost` without mutating `phase`/`started_at`/JoinCode/engine state.
- A Start attempted with fewer active Participants than `players.min` is rejected as `StartOutcomeNotEnoughPlayers` without mutating `phase`/`started_at`/JoinCode/engine state.
- A Start attempted against an already-expired lobby materializes/observes `LOBBY_EXPIRED` exactly as Join/Leave already do, and returns `StartOutcomeLobbyExpired`.
- A same-token retry of a previously completed Start (successful or any decline, including `RuntimeInitFailed`) replays the exact same recorded outcome without re-executing engine initialization or re-mutating durable state.
- A same-identity Start with a conflicting semantically-meaningful payload is rejected with the existing idempotency-conflict error, unchanged from Create/Join/Leave's pattern.
- A simulated transient infrastructure failure at any point before commit leaves the Session `LOBBY`, un-terminalized, with no idempotency claim completed, and safely retryable.
- Two concurrent Start attempts against the same Session never both execute a RuntimeTurn; exactly one produces Turn 1 and the other observes the resulting committed state (either replaying the same completed outcome under the same token, or observing `phase = RUNNING`/`TERMINAL` under a different token).
- `go build ./...`, `go vet ./...`, and `go test ./... -count=1` (including real-Postgres repository-integration/concurrency tests, per this repository's existing verification practice) pass, with no new failure introduced beyond the already-recorded, out-of-scope `getgame` JSONB-comparison test defect.

## Implementation Freedom

- Exact Go type/method names inside `game/session/internal/runtimeturn`, `step_start.go`, and any new `internal/repo` methods, subject to `repositories.md`/`domain-logic-placement.md`.
- Exact serialized shape of `session_runtime_steps.commit_payload` (technical trace only, never authoritative) and `session_runtime_turns.snapshot_format_version`'s starting value.
- Exact SQL lock/query statements, migration IDs/filenames, and column types, consistent with the existing migration style (`game/session/internal/storage/migrations/`) and the accepted column list in `SESSION_RUNTIME_PERSISTENCE_MODEL.md`.
- `players` roster ordering implementation detail (ascending `joined_at`, tie-broken by id, per Approved Design) unless the human directs otherwise (see Blockers).
- The engine `UserID` representation derived from `session_actors.id` (opaque internal string), and how `InitializationInput.Seed` is drawn.
- Naming/grouping of the defensive `players.max` re-check outcome at Start.
- Test structure/fixtures, following `testing.md` and the existing Slice 1 test layout (`*_test.go`, `*_integration_test.go`, `manager_*_concurrency_test.go` equivalents for Start).

## Verification

- `go build ./...`, `go vet ./...`, `gofmt -l` on changed files.
- `go test ./... -count=1` against a real PostgreSQL instance (`TEST_DATABASE_*`), per this repository's existing verification practice for Session Runtime work - including new repository-integration tests for the `runtimeturn` package and Start's own concurrency tests (two concurrent Starts against the same Session; Start racing Join/Leave/lazy-expiration under the existing shared lock).
- Mocked-collaborator unit tests for `Start`'s pre-transaction and in-transaction business logic (host/roster/min-max decisions, outcome selection), mirroring Create/Join/Leave's existing `manager_*_test.go` pattern.
- Engine-level tests proving the executor's Step-bound enforcement and atomicity (a pinned Definition authored specifically to require >20 Steps; a Definition whose root workflow deterministically fails on its first transition) without needing a real Postgres connection, mirroring `engineservice`'s own existing test style.

## Documentation Impact

### Accepted / Canonical Knowledge

- None. `game/README.md`, `GAME-ADR-0004/0007/0017-0019/0021/0022`, and `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` already describe this design; this WORK implements it without changing it.

### Current-State Documentation After Implementation

- `game/CURRENT_STATE.md` - describe Start's implementation and the new `runtimeturn` mechanism, and update any "RUNNING phase never reached" language left over from Slice 1.
- `game/docs/DATA_MODEL.md` - add `session_runtime_state`, `session_runtime_turns`, `session_runtime_steps` as now-implemented tables (the accepted shape already lives in `SESSION_RUNTIME_PERSISTENCE_MODEL.md`; this file tracks actually-persisted schema).
- `game/docs/FLOWS.md` - add Start's sequence, including the RuntimeTurn executor call and the fatal-path branch.

### Intentionally Unchanged

- `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` itself (accepted future-design record; already matches what this WORK implements).
- `docs/work/completed/WORK-0001-session-lobby-foundation.md` (completed work, historical).

## Blockers

The following are implementation-planning gaps this DRAFT resolves with a specific proposed reading, called out explicitly per this session's instructions rather than silently decided, because each shapes either this WORK's own durable schema/behavior or a pattern Slices 3/5/6 will inherit unchanged. None require new architecture - each is a direct, narrow extension of already-ACCEPTED GAME-ADRs. Pending explicit human confirmation before READY:

1. **Shared `runtimeturn` package placement** - introducing `game/session/internal/runtimeturn` now (rather than inlining the Step-loop into `step_start.go` and extracting a shared package only when Slice 3 needs it) commits Slices 3/5/6 to this shape. Confirm this is the right moment/place to introduce it.
2. **Fatal-path classification split** - a pinned-Definition recompile failure at Start classified as `RUNTIME_STATE_INVALID`, while every other Start-chain failure (including an outright rejection of Start's own initial signal) is classified as `RUNTIME_EXECUTION_FAILED`. Neither GAME-ADR-0017 nor GAME-ADR-0019 spells this split out explicitly for the rejection case.
3. **Replaying a fatal `StartOutcomeRuntimeInitFailed` via idempotency** - treating a Start that already fatally terminalized the Session as a normal replayable completed outcome (never re-attempted on retry), the same way `LOBBY_FULL`/`ALREADY_JOINED` already replay for Join.
4. **`players` roster ordering** - defaulting to ascending `joined_at` (a product-visible choice, e.g. "who is Player 1") with no existing ADR/product decision on record specifying it.

Local implementation choices (engine `UserID` representation, exact serialized `commit_payload`/`snapshot_format_version`, defensive `players.max` outcome naming, migration filenames) are Implementation Freedom and are not blockers.

## Completion Record

Not yet started.
