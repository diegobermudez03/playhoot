# WORK-0004: Interaction Response Processing

Status: READY
Created: 2026-09-19
Last status change: 2026-09-19

Related decisions:
- GAME-ADR-0007 (RuntimeTurn/Interaction persistence model)
- GAME-ADR-0018 (RUNNING mutation serialization)
- GAME-ADR-0019 (RuntimeTurn Step bound, terminal cleanup)
- GAME-ADR-0017 (failure classification)
- GAME-ADR-0022 (business declines as workflow outcomes)

Canonical context:
- `game/README.md` (Session Runtime Turn And Persistence Model, RUNNING Mutation Serialization, RuntimeTurn Execution Bound And Terminal Cleanup sections)
- `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` (`session_interactions` shape, Turn/Interaction relationships)
- `game/language/v1/engine/README.md` (Step contract, Outputs table, Signal kinds)
- `docs/ai/workspaces/active/session-runtime-v1/PLAN.md` (Slice 3)
- `game/session/workflows/sessionlifecycle/step_start.go` (existing RuntimeTurn execution/persistence pattern this WORK extends)

## Outcome

A player can answer an open interaction (a `Question` or an `AskGroup` question) against a `RUNNING` Session, entirely at the Go-API level (no transport yet - Slice 4 exposes this live). Session Runtime obtains RUNNING-phase per-Session serialization (GAME-ADR-0018), reloads current authoritative state, applies the accepted response through the Game Language engine, persists the resulting RuntimeTurn, and durably resolves the `session_interactions` row the response answered.

This WORK also closes a gap discovered while designing it: no Turn-producing path today persists `session_interactions` at all - `step_start.go`'s `drainRuntimeTurn` discards `commit.Outputs` entirely, so even a game whose root workflow opens a question immediately at Start would leave that interaction with no durable record for anyone to ever answer. This WORK is therefore also where `OpenQuestionOutput`/`CloseQuestionOutput` capture is introduced, retrofitted onto Start and shared with the new interaction-answering step.

## Context

Slice 1 (WORK-0001) and Slice 2 (WORK-0003) are DONE: `sessionlifecycle.Manager` exposes `Create`/`Join`/`Leave`/`Start`; `Start` executes the engine's first RuntimeTurn and commits `phase = RUNNING`, `sessions.current_turn_id`, per GAME-ADR-0023. No RUNNING-phase mutation exists yet - `sessions.current_turn_id` is written once by Start and never read/advanced again. No `session_interactions` table exists yet (not yet migrated). GAME-ADR-0018 (RUNNING serialization) and GAME-ADR-0019 (Step bound, terminal cleanup) are accepted architecture but have zero implementation surface outside what Slice 2 already needed for Start's own initialization chain.

The engine (`game/language/v1/engine`) exposes two ways a workflow can request player input - `OpenQuestionOperation` (single recipient) and `OpenAskGroupOperation` (many recipients, with a completion policy) - both surfacing as one `OpenQuestionOutput` per recipient when opened, and closing via `CloseQuestionOutput`. A submitted answer re-enters the engine as `engine.Signal{Kind: SignalKindQuestionAnswered, ...}` or `SignalKind AskGroupAnswered, ...}` (same `Slot`/`Respondent`/`Answer` shape, distinguished only by `Kind`) through `engineservice.Step`. `OpenQuestionOutput` itself carries no discriminator saying which of the two originated it.

Per PLAN.md, Slice 3 is deliberately the simplest RUNNING-phase mutation (no physical scheduling, unlike timers) specifically to validate GAME-ADR-0018's serialization mechanism with the fewest moving parts, and to complete the minimal create->join->start->answer vertical slice Slice 4's thin Coordinator will later expose live.

## Scope

### In Scope

- A new durable `session_interactions` table (migration), per the accepted shape in `SESSION_RUNTIME_PERSISTENCE_MODEL.md`.
- Capturing `OpenQuestionOutput`/`CloseQuestionOutput` from any committed RuntimeTurn's `Commit.Outputs` and persisting/closing the corresponding `session_interactions` row - applied to **both** the new interaction-answering step and a retrofit of `step_start.go` (see Blockers).
- A new `AnswerInteraction` step directly on the existing `sessionlifecycle.Manager` (alongside `Create`/`Join`/`Leave`/`Start`, not a new sibling workflow package - see Blockers) that: resolves an interaction by its public UUID, obtains RUNNING-phase per-Session serialization (extending the existing `sessionlock` mechanism to a `RUNNING` Session - no new locking mechanism), reloads `sessions.current_turn_id` and its Snapshot, validates the interaction is still `ACTIVE` and the caller is its recipient, constructs the correct `engine.Signal` for the stored interaction `kind`, drains the resulting RuntimeTurn to quiescence under the existing `MAX_STEPS_PER_RUNTIME_TURN = 20` bound, and atomically persists the new RuntimeTurn/Steps, advances `sessions.current_turn_id`, and resolves the answered `session_interactions` row (`response_payload`, `state`, `closed_by_turn_id`).
- Expected-rejection handling: an engine `ErrSignalRejected`/`ErrInputRejected` (stale/duplicate/unauthorized/invalid answer) is an ordinary declined outcome - no RuntimeTurn, no Snapshot mutation, Session stays `RUNNING` (GAME-ADR-0017 class A).
- Fatal-failure handling reusing Start's existing pattern: a deterministic `ExecutionError` terminalizes the Session (`RUNTIME_EXECUTION_FAILED`) via the same atomic fatal-materialization transaction shape Start already uses, extended with the narrow terminal-cleanup this slice's own path needs (see Blockers) - not the full Slice 6 diagnostics entity.
- Extracting the pure Step-draining/bound execution logic (`step_start.go`'s `drainRuntimeTurn`) into a shared mechanism scoped to the `sessionlifecycle` workflow itself, now that a second, independent caller genuinely needs it (see Blockers) - never a session-level shared package.

### Out of Scope

- Timer obligations/expiration (Slice 5).
- Disconnect/reconnect/semantic presence (Slice 7).
- The thin live Coordinator/WebSocket transport and any wire/DTO format for the interaction answer payload (Slice 4) - this WORK's public method accepts an already-typed `engine.Value`, exactly as `Start` already accepts typed `engine.Value`/`engine.UserID` rather than a raw wire payload.
- The general-purpose terminal-cleanup sweep and the `session_runtime_failures` diagnostic entity (Slice 6) beyond the narrow path-scoped cleanup this slice's own fatal path requires (see Blockers).
- `AskGroup`-specific aggregate semantics beyond routing one individual accepted answer (per-recipient completion policy evaluation, `AskGroupCompletedSignalSource` join handling) - already the engine's own responsibility inside one `Step` call; this WORK does not add any Session-Runtime-side aggregate bookkeeping beyond the one `session_interactions` row per recipient every `OpenQuestionOutput` already implies.
- Any change to `Create`/`Join`/`Leave` behavior.

## Approved Design

Beyond canonical context, this WORK settles:

- **RUNNING-phase locking reuses the existing mechanism unchanged.** `game/session/internal/sessionlock.LockByUUID`/`LockByID` already lock a `sessions` row regardless of `phase`; GAME-ADR-0018 requires no new lock primitive, only that a RUNNING-phase mutation use it the same way LOBBY mutations already do, then reload `current_turn_id`/Snapshot after acquiring it (never trusting a pre-lock read).
- **No `session_requests` idempotency record for interaction responses.** Per GAME-ADR-0007's already-accepted "Interaction response semantics": the interaction UUID plus its persisted `state`/`response_payload` is the natural dedup identity. A first valid response resolves the interaction; a retry submitting an equivalent response is idempotent (same outcome, no second engine effect); a conflicting different response against an already-resolved interaction is rejected as a workflow outcome (GAME-ADR-0022 pattern), never a second engine effect.
- **Public method accepts a typed `engine.Value` answer directly**, not a wire payload - decoding a transport-level answer into `engine.Value` against the interaction's recorded question/response type is Coordinator/transport-layer work (Slice 4), consistent with GAME-ADR-0002's boundary and PLAN.md's explicit "Go-API level" framing for Slices 1-3.
- **Respondent authorization**: the caller-supplied `UserUUID` must resolve to the `SessionActorID` recorded as the interaction's `session_actor_id` (the recipient at open time); a mismatch is rejected the same way `engineservice.Step` would reject an unauthorized `Respondent`, without needing to reach the engine at all.

## Constraints and Invariants

- Every RUNNING-phase mutation must acquire per-Session serialization before executing, and must reload current state after acquiring it (GAME-ADR-0018) - never execute against a Snapshot read before the lock.
- One RuntimeTurn's Step-chain execution (initial signal plus every internal-signal-caused Step) must never exceed `MAX_STEPS_PER_RUNTIME_TURN = 20` (GAME-ADR-0019); exceeding it is a fatal `RUNTIME_EXECUTION_FAILED` termination, identical in kind to Start's own existing enforcement.
- A fatal (non-rejection) failure must never leave a partially-committed RuntimeTurn; `current_turn_id` remains at the last committed Turn (GAME-ADR-0017).
- `session_interactions.opened_by_turn_id`/`closed_by_turn_id`/the causing Turn's `source_interaction_id` are Turn-level references only, never Step-level (GAME-ADR-0007).
- Authoritative ordering of concurrent causes is defined purely by which cause wins serialization and commits first - never by arrival/wall-clock timestamps (GAME-ADR-0018).

## Acceptance Criteria

- A `Question`/`AskGroup` interaction opened by any committed RuntimeTurn (including Start's own first Turn) is durably persisted as an `ACTIVE` `session_interactions` row before that Turn's transaction commits.
- A valid accepted-recipient response to an `ACTIVE` interaction commits a new RuntimeTurn, advances `sessions.current_turn_id`, and resolves the interaction (`state`, `response_payload`, `closed_by_turn_id` set to the causing Turn).
- A stale, duplicate, unauthorized-respondent, or type/validation-invalid answer is rejected as an ordinary declined outcome: no RuntimeTurn is created, `current_turn_id` is unchanged, and the interaction remains `ACTIVE`.
- A retried, semantically equivalent response to an already-resolved interaction replays the same outcome without a second engine execution; a conflicting different response to an already-resolved interaction is rejected without engine execution.
- A deterministic engine execution failure (including Step-bound overflow) while processing a response terminalizes the Session (`RUNTIME_EXECUTION_FAILED`) atomically, with no partial RuntimeTurn persisted and `current_turn_id` unchanged.
- Two concurrent responses/causes targeting the same Session never both execute a RuntimeTurn concurrently; exactly one wins serialization and commits first, and the other observes the resulting committed state.
- `go build ./...`, `go vet ./...`, and `go test ./... -count=1` (including real-Postgres repository-integration/concurrency tests) pass, with no new failure introduced beyond the already-recorded out-of-scope `getgame` JSONB-comparison test defect.

## Implementation Freedom

- Exact Go type/method/function names for the new `AnswerInteraction` step and its narrow repository contract, subject to `repositories.md`/`domain-logic-placement.md`/`function-signatures.md`.
- Whether the extracted shared RuntimeTurn-execution logic takes the shape of a plain unexported helper shared across `sessionlifecycle`'s own files, or a workflow-scoped `game/session/workflows/sessionlifecycle/internal/runtimeturn` package - either is acceptable per Blocker 2's resolution, so long as it stays inside the `sessionlifecycle` workflow package tree and never at `game/session/internal/...`.
- Exact SQL for the `session_interactions` migration (column types, indexes), consistent with the existing migration style and the accepted column list in `SESSION_RUNTIME_PERSISTENCE_MODEL.md`.
- Exact mechanism for resolving `session_interactions.kind` from the compiled `engine.Program`'s slot declarations, per Blocker 4's resolution below.
- Test structure/fixtures, following `testing.md` and the existing Slice 1/2 test layout.

## Verification

- `go build ./...`, `go vet ./...`, `gofmt -l` on changed files.
- `go test ./... -count=1` against a real PostgreSQL instance (`TEST_DATABASE_*`) - new repository-integration tests for interaction persistence/resolution, and concurrency tests for two competing causes against the same Session (mirroring `TestManagerStart_Integration`'s concurrency subtest).
- Mocked-collaborator unit tests for the new step's pre-transaction/in-transaction business logic (authorization, stale/duplicate/conflict handling, outcome selection).
- Engine-level tests proving Step-bound enforcement/atomicity reused correctly from the extracted shared package, and `OpenQuestionOutput`/`CloseQuestionOutput` capture correctness, without needing a real Postgres connection - mirroring `TestDrainRuntimeTurn`'s existing style.

## Documentation Impact

### Accepted / Canonical Knowledge

- None expected. `game/README.md`, GAME-ADR-0007/0017/0018/0019/0022, and `SESSION_RUNTIME_PERSISTENCE_MODEL.md` already describe this design; this WORK implements it without changing it - unless Blocker resolution below reveals a genuine gap in accepted architecture, which would route through the appropriate decision process instead of being silently decided here.

### Current-State Documentation After Implementation

- `game/CURRENT_STATE.md` - describe interaction response processing as implemented, and Start's retrofitted interaction-capture behavior.
- `game/docs/DATA_MODEL.md` - add `session_interactions` as a now-implemented table.
- `game/docs/FLOWS.md` - add the interaction-response sequence, and update Start's sequence to show interaction capture.

### Intentionally Unchanged

- `docs/work/completed/WORK-0001-session-lobby-foundation.md`, `WORK-0002-rename-game-management-package.md`, `WORK-0003-session-start-first-runtimeturn.md` (completed work, historical).

## Blockers

Status: **RESOLVED (2026-09-19, HUMAN-APPROVED)**. Each item below shaped either this WORK's own durable schema/behavior or a pattern later slices (5, 6, 7) inherit, so none was silently decided within existing accepted architecture without human confirmation. All five are now resolved and this WORK is READY.

1. **Workflow classification: new sibling workflow package, or a step on the existing `sessionlifecycle.Manager`?** Proposed: a new `game/session/workflows/runtimeexecution` package. **Resolution: REJECTED as proposed, resolved with the stated alternative instead** - `AnswerInteraction` is a new step directly on the existing `sessionlifecycle.Manager` (`step_answer_interaction.go`, alongside `step_create.go`/`step_join.go`/`step_leave.go`/`step_start.go`), following the same Manager/narrow-repo-contract/idempotency pattern those steps already use. Whether `TimerExpired` (Slice 5) or disconnect-reconnect (Slice 7) processing also lands on `sessionlifecycle.Manager` or eventually warrants a separate package is explicitly left undecided here - each of those slices decides for itself, just-in-time, when it materializes, the same way Blocker 2 below is itself a just-in-time extraction decision rather than one made preemptively at Slice 2.
2. **Extract the shared RuntimeTurn-execution mechanism now.** Proposed: extract `step_start.go`'s pure `drainRuntimeTurn`/Step-bound logic into `game/session/internal/runtimeturn` (a session-level shared package, sibling to `sessionlock`/`idempotency`/`pgerrs`). **Resolution: REJECTED as proposed at that location.** The extracted mechanism must stay scoped to the `sessionlifecycle` workflow itself, not promoted to session level - this is still workflow execution logic two steps of the *same* workflow now share, not a horizontal capability every Session Runtime workflow needs (unlike `sessionlock`/`idempotency`, which genuinely are). Acceptable shapes: a plain unexported helper shared across `sessionlifecycle`'s own files (e.g. a `runtime_turn.go` alongside `manager.go`), or, if a separate package is warranted, `game/session/workflows/sessionlifecycle/internal/runtimeturn` (sibling to the existing `sessionlifecycle/internal/repo`) - never `game/session/internal/runtimeturn`. The exact choice between the two is Implementation Freedom. `step_start.go` is refactored to call the extracted function; nothing about its behavior changes. Each step still independently persists its own RuntimeTurn/Steps/Interactions via its own local repository, per `repositories.md`'s Sharing Rule (mechanism is shared, entity persistence is not).
3. **Retrofit `step_start.go` to capture interactions opened by Start's own first Turn.** **Resolution: APPROVED as proposed** - in this WORK's scope, so a game whose root workflow opens a question immediately at Start actually produces a durable `session_interactions` row `AnswerInteraction` can resolve.
4. **How `session_interactions.kind` (`QUESTION` vs `ASK_GROUP`) is determined.** **Resolution: APPROVED as proposed** - resolved from the compiled `engine.Program`'s own slot declaration for `(workflow, engine_slot)` at the moment the interaction is captured, and persisted on the `session_interactions` row so response processing never needs to re-derive it. Still not deeply verified against `internal/compiler`'s actual shape - if implementation finds the compiled `Program` doesn't actually expose this cleanly, that is new information routed back through this WORK rather than silently worked around.
5. **Narrow terminal-cleanup scope for this slice's own fatal path.** **Resolution: APPROVED as proposed, now** - this WORK implements the narrow case: when this slice's own fatal-termination transaction commits, it atomically closes every currently-`ACTIVE` `session_interactions` row for that Session in the same transaction (`closed_by_turn_id = NULL`, a `SESSION_TERMINATED`-equivalent closure reason), leaving the fully general sweep (covering future entities like timers) to Slice 6.

Local implementation choices (exact SQL/column types for the `session_interactions` migration, test structure/fixtures) remain Implementation Freedom and were never blockers.

## Completion Record

Not yet DONE. Status transitioned DRAFT -> READY on 2026-09-19 per the human's resolution of all five Blockers above (see this file's Blockers section and `docs/ai/workspaces/active/session-runtime-v1/HUMAN_REVIEW.md`).
