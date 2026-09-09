# Session Runtime Failure Classification And Fatal Diagnostic Persistence Checkpoint

Process: Architecture Discussion

Status: RESOLVED - accepted 2026-09-08. No checkpoint is currently pending; this file is retained as the human-facing record of the resolved checkpoint until the next milestone (final Operational Lifecycle completeness review) produces its own checkpoint.

## What Was Approved

Runtime/internal failure semantics - the previously explicit next milestone left open after GAME-ADR-0016 - was reviewed and accepted as GAME-ADR-0017.

### Four failure classes

- **A. Expected operation/runtime rejection.** `engineservice.ErrSignalRejected`/`ErrInputRejected` (and an unhandled `UserDisconnected`/`UserReconnected`, GAME-ADR-0011) reject/discard the current operation with no Snapshot mutation, no RuntimeTurn, and the Session stays `RUNNING`.
- **B. Deterministic runtime/game execution failure.** A non-rejection `ExecutionError` (invariant violation, division by zero, occupied timer/interaction slot, invalid quorum/runtime operation, or a safety-limit violation such as execution budget/loop limit/workflow depth) aborts the attempted RuntimeTurn - no commit, no partial gameplay effect, previous Turn/Snapshot remains authoritative - and terminalizes the Session with internal terminal reason `RUNTIME_EXECUTION_FAILED`. V1 treats safety-limit failures as fatal to that execution rather than endlessly retryable; concrete configured limits remain deferred.
- **C. Durable runtime-state invalidity.** A corrupt/unreadable Snapshot, a Snapshot incompatible with its pinned Program, or an impossible/missing durable runtime reference terminalizes the Session with internal terminal reason `RUNTIME_STATE_INVALID`, kept operationally distinguishable from `RUNTIME_EXECUTION_FAILED`.
- **D. Transient infrastructure failure.** Postgres unavailability, deadlock/serialization retry, transient network/dependency failure, transaction timeout, or process crash where the transaction did not commit does not terminalize the Session; last committed state remains authoritative and normal infrastructure retry policy applies.

### RuntimeTurn failure boundary

A fatal (B/C) attempted execution is never persisted as a `session_runtime_turns` row. Example: current authoritative Turn 41, an attempted Turn 42 fails deterministically at step 2 - `current_turn_id` remains 41, no Turn 42 exists, and no gameplay consequence from any earlier in-memory-successful step persists. Session Runtime does not fabricate a Turn to represent a failure.

### Separate diagnostic entity: `session_runtime_failures`

A new conceptual entity, separate from RuntimeTurn and owning no authoritative Snapshot, was accepted with fields: `id`, `session_id`, `failure_kind` (`RUNTIME_EXECUTION`|`RUNTIME_STATE_INVALID`), `error_code` (the engine's existing `ExecutionErrorCode` where available), `error_message`, `base_turn_id` (last successfully committed Turn before the fatal attempt), `attempted_sequence` (diagnostic only - never consumes the authoritative sequence or represents committed state), `failed_step_index`, `source_kind`, `source_interaction_id`/`source_timer_obligation_id` (nullable), `actor_id` (nullable), `diagnostic_payload` (versioned, schema not yet frozen), `created_at`. Nothing captured in `diagnostic_payload` becomes authoritative gameplay history merely because it was captured; successful intermediate Steps from an ultimately failed Turn remain non-committed.

### Atomic fatal materialization

For classes B/C, Session Runtime performs one Session transaction rather than a two-step "rollback, then separately terminalize" design: attempt execution in memory, encounter the fatal failure, persist no attempted Turn/Steps, persist the `SessionRuntimeFailure`, set `phase = TERMINAL` + internal `terminal_reason` + `terminal_at`, commit. Either the Session remains at its previous valid state because the transaction itself failed, or the Session is `TERMINAL` with its diagnostic record committed together with it - never a durable state where a known fatal error materialized while the Session remains `RUNNING`. If infrastructure fails before this transaction commits (class D), the prior state remains authoritative and no fatal terminalization/diagnostic record is considered to have happened.

### Expected rejections and infra failures never create fatal records

`session_runtime_failures` is not created for `ErrSignalRejected`/`ErrInputRejected`, expected stale-signal rejection, ordinary invalid user operations, or transient infrastructure/database errors.

### Internal vs. public terminal reasons

`RUNTIME_EXECUTION_FAILED` and `RUNTIME_STATE_INVALID` remain distinct internal reasons (one commonly a game-definition/authored-logic problem, the other a runtime/persistence integrity problem) but both project publicly as a generic `INTERNAL_ERROR` category (exact enum/DTO naming not frozen). Clients never receive raw technical detail - invariant names, division-by-zero, snapshot-mismatch specifics, engine paths/slots, stack traces, `diagnostic_payload`, raw `ExecutionErrorCode`, or internal DB/runtime identifiers. A valid user input that triggers a downstream authored-script failure (for example, a valid answer causing an authored script to divide by zero) is never reported back as input invalidity - the platform, not the player, is at fault, and the public message communicates an internal Session/runtime problem instead.

### Delivery and resync ordering

Live clients may be notified of the terminal failure only after `Session TERMINAL` + `SessionRuntimeFailure` commit durably - fatal attempt, then durable commit, then post-commit Coordinator delivery/fanout. A client that misses the live notification still discovers `phase = TERMINAL`/`INTERNAL_ERROR` through the already-accepted resync capability (GAME-ADR-0010); correctness must not depend on delivery success.

### Archival and queryability

Failed Sessions remain normal terminal Sessions for archival eligibility (GAME-ADR-0009). `session_runtime_failures` is explicitly excluded from the automatic hard-delete-after-archival policy applied to the heavy runtime-history tables: lightweight failure metadata remains relationally queryable in PostgreSQL after heavy archival (for example, failure counts by error code or by game definition), since a Session normally produces at most one such record. Large diagnostic-payload retention/compaction strategy remains a later, undesigned optimization.

## Where This Was Persisted

- `game/docs/decisions/GAME-ADR-0017-session-runtime-failure-classification-and-diagnostic-persistence.md`
- `game/docs/decisions/INDEX.md` (GAME-ADR-0017 row added; next Game ADR is now `GAME-ADR-0018`)
- `game/README.md` (new "Session Runtime Failure Classification And Diagnostic Persistence" section between Session Runtime Process-Agnostic Recovery and Session Runtime Durable Inactivity Expiration; an archival-exception sentence added to Session Runtime History Archival Direction)
- `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` (new "Session Runtime Failure Diagnostics" section with the `session_runtime_failures` ER diagram and cardinality notes; an archival-exception paragraph in Archival And Hard-Delete Policy; a new rationale link and a new Not-Yet-Decided entry)
- `docs/ai/workspaces/active/session-runtime-v1/AI_CONTEXT.md` (new accepted-decision section, updated deferred topics, updated next-milestone questions, updated drift/explicitly-not-done notes)

## Explicitly Not Authorized By This Checkpoint

No production code, migrations, engine/compiler changes, Coordinator behavior, tests, observability pipelines, archival workers, or WORK were created or changed. No `session_runtime_failures` table, `SessionRuntimeFailure` type, or terminal-reason enum was implemented anywhere in Go code. Concrete execution-budget/loop-limit/workflow-depth values, the exhaustive `source_kind`/interaction-kind enums, and the exact public DTO/HTTP/WebSocket status-code contract for `INTERNAL_ERROR` remain deferred, not designed here. GAME-ADR-0007, GAME-ADR-0009, GAME-ADR-0011, GAME-ADR-0013, GAME-ADR-0014, and GAME-ADR-0016's historical rationale were not rewritten; this checkpoint is additive.

## Next Milestone (Not Designed Here)

A final Operational Lifecycle completeness review before Session Runtime V1 architecture can be declared closed. The next Conversational AI should inspect the already-accepted design and identify only materially unresolved V1 architecture topics, in particular: (1) execution/runaway protection configuration (concrete limits); (2) platform abuse/resource limits; (3) maximum per-Turn Steps/internal-signal chain; (4) maximum Session runtime/inactivity (already partly addressed by GAME-ADR-0014); (5) maximum pending interactions/timers; (6) per-user/session request abuse/rate boundaries where architecture ownership matters; (7) corrupted/invalid Game Definition handling before Start; (8) any terminal reasons still missing; (9) cleanup semantics for outstanding interactions/timer obligations after terminalization. The goal is to determine whether any remaining architecture decision must be made before Session Runtime V1 moves from Architecture Discussion into Implementation Planning; if no material gap remains, recommend closing Architecture Discussion and moving this workspace into Initiative Implementation Planning. Do not automatically introduce solutions in that review.
