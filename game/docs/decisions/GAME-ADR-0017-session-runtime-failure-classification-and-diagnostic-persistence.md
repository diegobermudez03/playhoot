# GAME-ADR-0017: Session Runtime Failure Classification and Fatal Diagnostic Persistence

Status: ACCEPTED
Created: 2026-09-08
Last status change: 2026-09-08
Supersedes: None
Superseded by: None

## Context

GAME-ADR-0007 accepts `RuntimeTurn` as the atomic historical/transactional unit for RUNNING-phase execution and GAME-ADR-0013 accepts that an uncommitted RuntimeTurn transaction rolls back entirely on process death, with no durable "in-progress" state. Neither record classifies *why* a RuntimeTurn attempt might not reach a normal successful Commit, nor what Session Runtime should do about it. Distinct situations were being talked about as if they were one thing:

- the engine's own accepted rejection outcomes (`engineservice.ErrSignalRejected`, `engineservice.ErrInputRejected`), which GAME-ADR-0011 already treats as ordinary no-op outcomes for the specific case of an unhandled `UserDisconnected`/`UserReconnected` signal;
- a deterministic engine execution error - an invariant violation, division by zero, an occupied timer/interaction slot the runtime contract treats as an error, an invalid quorum/runtime operation, or a safety-limit violation (execution budget, loop limit, workflow depth) - where the attempted execution cannot safely continue;
- durable runtime state that is itself invalid or unreconstructible (a corrupt/unreadable Snapshot, a Snapshot that no longer matches its pinned Program, an impossible/missing durable runtime reference);
- a transient infrastructure failure (Postgres unavailability, deadlock/serialization retry, network/dependency failure, transaction timeout, process crash) where the authoritative transaction simply did not commit.

Without an accepted taxonomy, an implementation could plausibly do any of: terminalize a Session merely because an ordinary rejection occurred; silently continue a Session past a deterministic invariant violation; persist a fake RuntimeTurn to "record" a failure; or terminalize a Session because of a transient database hiccup that never actually committed anything. GAME-ADR-0009's archival direction and GAME-ADR-0014's `terminal_reason` precedent already establish that Session Runtime distinguishes terminal causes and retains lightweight lifecycle/runtime metadata relationally; this record extends that pattern to internal/fatal runtime failure specifically, and introduces the diagnostic entity needed to investigate it later without polluting authoritative gameplay history.

## Decision

### Four failure classes

Session Runtime distinguishes four conceptually different failure classes:

**A. Expected operation/runtime rejection.** `engineservice.ErrSignalRejected` and `engineservice.ErrInputRejected` (see `game/language/v1/engine/README.md`) are expected rejection outcomes, not Session failure. The current operation is rejected/discarded; the authoritative Snapshot is not mutated; no RuntimeTurn is created; the Session remains `RUNNING`; an active interaction may remain ACTIVE when the response itself was rejected. An unhandled lifecycle signal such as `UserDisconnected`/`UserReconnected` (GAME-ADR-0011) is compatible with these semantics: no authored gameplay effect, no no-op RuntimeTurn, and any Session-level semantic-presence edge may still commit according to its own already-accepted atomic rules (GAME-ADR-0016).

**B. Deterministic runtime/game execution failure.** A non-rejection `ExecutionError` - an invariant violation, division by zero, an occupied timer/interaction slot the runtime contract treats as an execution error, an invalid quorum/runtime operation, an execution-budget/loop-limit/workflow-depth violation, or another deterministic engine execution error meaning the current execution cannot safely continue - is not "bad input, try again." The attempted RuntimeTurn does not commit; no partial gameplay effect becomes authoritative; the previous RuntimeTurn/Snapshot remains the last valid state; the Session becomes `TERMINAL` with internal terminal reason `RUNTIME_EXECUTION_FAILED`. For V1, deterministic safety-limit failures (runaway/budget/depth/loop violations) are fatal to that Session's execution rather than endlessly retryable; the concrete configured limits remain a separate, still-deferred topic.

**C. Durable runtime-state invalidity.** A corrupt/unreadable persisted Snapshot, a Snapshot incompatible with its pinned Program, an impossible/missing durable runtime reference, a `snapshot_program_mismatch`, or other durable state making safe reconstruction/execution impossible is different from an authored game execution error: the Session becomes `TERMINAL` with internal terminal reason `RUNTIME_STATE_INVALID`, kept operationally distinguishable from `RUNTIME_EXECUTION_FAILED`.

**D. Transient infrastructure failure.** Temporary Postgres unavailability, transaction/deadlock/serialization retry, transient network/dependency failure, transaction timeout, or process crash, where the authoritative Session transaction did not commit and durable state remains valid, does not terminalize the Session merely because infrastructure temporarily failed. The last committed authoritative state is preserved; the operation may fail/retry under normal infrastructure policy; if no transaction committed, the attempted mutation did not happen from the Session-domain perspective (consistent with GAME-ADR-0013's existing crash-rollback semantics).

### A fatal attempted execution is not a RuntimeTurn

A fatal attempted execution (class B or C) does not become a `session_runtime_turns` row. Example: current authoritative Turn is 41; the next attempt (would-be Turn 42) succeeds in memory for steps 0 and 1, then fails deterministically at step 2. The authoritative result is that `current_turn_id` remains Turn 41, no Turn 42 exists, no intermediate Snapshot becomes authoritative, and no gameplay consequence from steps 0/1 persists; the Session is terminalized separately, as part of the same atomic materialization described below - not by fabricating a Turn to represent the failure.

### `session_runtime_failures`: a separate durable fatal-diagnostic record

The accepted future persistence model gains a conceptual `session_runtime_failures` entity, separate from `RuntimeTurn` and not owning an authoritative Snapshot, with conceptual fields: `id`, `session_id`, `failure_kind`, `error_code`, `error_message`, `base_turn_id`, `attempted_sequence`, `failed_step_index`, `source_kind`, `source_interaction_id` (nullable), `source_timer_obligation_id` (nullable), `actor_id` (nullable), `diagnostic_payload`, `created_at`. Exact SQL types/names follow repository convention when implemented; none of this is migrated or coded by this record.

Field meaning:

- **`failure_kind`** is the stable Session/operator classification (`RUNTIME_EXECUTION` or `RUNTIME_STATE_INVALID`), distinct from the raw engine error code.
- **`error_code`** persists a stable technical code where available - the engine's existing `ExecutionErrorCode` values are suitable for this - for operator/debug diagnostics only, never exposed directly to players.
- **`error_message`** is technical diagnostic text, operator/debug only, never part of the player-facing API contract.
- **`base_turn_id`** references the last successfully committed authoritative RuntimeTurn before the fatal attempt - "what valid Session state were we executing from?"
- **`attempted_sequence`** may record the sequence number the failed Turn would have had. It is diagnostic only: it does not consume the authoritative sequence, does not create a historical gap clients must interpret, and does not represent committed state. (Example: last committed sequence is 41; a fatal attempt is annotated `attempted_sequence = 42`; the Session terminalizes; there is still no committed RuntimeTurn sequence 42.)
- **`failed_step_index`** identifies which internal Step failed when known, as technical/debug metadata.

### Diagnostic payload

`diagnostic_payload` is a versioned internal diagnostic structure whose exact JSON/schema is not frozen by this record. It may eventually preserve the attempted external/runtime cause, the failed signal, traces/metadata from successful in-memory Steps before the fatal Step, the failing Step index, technical error code/message, and other debugging context Session Runtime has available. Nothing stored in `diagnostic_payload` becomes authoritative gameplay history merely because it was captured: successful intermediate Steps from an ultimately failed RuntimeTurn remain non-committed, and their traces are diagnostics only. Session-owned diagnostic capture may need to preserve its own execution context, since the current engine contract exposes technical `ExecutionError` information on failure but is not assumed to emit a normal successful `Commit.Trace` for the Step that failed - only for Steps that produced successful in-memory commits earlier in the same attempt. This record does not change engine behavior to provide such a trace.

### Atomic fatal materialization

For class B/C failures, Session Runtime avoids a design equivalent to "(1) rollback the attempted Turn; (2) later, in a separate unrelated operation, terminalize the Session." Because engine execution is in-memory/pure relative to authoritative persistence, the fatal path is instead conceptually one Session transaction: validate/serialize the Session; attempt runtime execution in memory; encounter the fatal deterministic/state-invalid failure; persist no attempted RuntimeTurn and no partial RuntimeSteps/gameplay state; persist the `SessionRuntimeFailure`; set `phase = TERMINAL`, the corresponding internal `terminal_reason`, and `terminal_at` to the fatal materialization instant; commit. The durable result is therefore atomic: either the Session remains at its previous valid state because the transaction itself failed, or the Session is `TERMINAL` and its fatal diagnostic record exists together with it. Session Runtime must never leave a durable state where a known deterministic fatal error was materialized but the Session remains `RUNNING`.

If Postgres/infrastructure fails before this fatal-terminalization transaction commits (class D), the previous Session state remains authoritative, no fatal terminalization is considered to have happened, and no `SessionRuntimeFailure` is considered committed - this remains an infrastructure failure/retry problem, and Session Runtime must not force `TERMINAL` based only on an uncommitted local observation, consistent with GAME-ADR-0013.

### Expected rejections and transient infrastructure failures do not create fatal records

`session_runtime_failures` is not created for `ErrSignalRejected`, `ErrInputRejected`, expected stale-signal rejection, duplicate/invalid normal user operations that do not indicate corrupt runtime, or transient infrastructure/database errors. The table represents terminal/internal runtime failure diagnostics, not every failed request.

### Internal terminal reasons and public projection

`RUNTIME_EXECUTION_FAILED` and `RUNTIME_STATE_INVALID` remain distinct internal Session/operator lifecycle reasons and are not collapsed internally - one commonly indicates deterministic execution/game-definition behavior, the other indicates authoritative runtime reconstruction/state-integrity problems. Both project to clients as a generic public terminal category equivalent to `INTERNAL_ERROR` (exact enum/DTO naming not frozen). Clients must not receive raw technical terminal details: `division_by_zero`, invariant names, snapshot-mismatch details, engine paths, internal slots, stack traces, `diagnostic_payload`, raw `ExecutionErrorCode`, or internal DB/runtime identifiers. Player-facing UX communicates only that the Session ended because of an internal error; exact message copy is not frozen. If a valid user interaction triggers authored execution that then fails deterministically (for example, a valid answer causes an authored script to divide by zero), the public result must not blame that input ("your input was invalid") - it communicates an internal Session/runtime problem instead, keeping input rejection and internal runtime failure clearly distinct.

### Delivery and resync

Live clients may be notified of the terminal failure only after `Session TERMINAL` + `SessionRuntimeFailure` have committed durably: fatal runtime attempt, then durable terminal commit, then post-commit Coordinator delivery/fanout. Clients must not be told the Session definitively failed before the database says so. A client that misses the live terminal notification must still discover `Session.phase = TERMINAL` and public reason `INTERNAL_ERROR` through the already-accepted resync/current-state capability (GAME-ADR-0010) - correctness must not depend on successful WebSocket delivery.

### Archival and queryability

Failed Sessions remain normal terminal Sessions for archival eligibility (GAME-ADR-0009); the long-term archive should eventually preserve enough failure diagnosis for historical investigation, though the exact archive JSON contract remains unfrozen. `session_runtime_failures` is not automatically classified as hard-deletable heavy history: lightweight runtime-failure metadata should remain queryable relationally in PostgreSQL after heavy runtime-history archival, supporting future queries such as how many Sessions failed by error code, which game definitions produced execution-budget failures, or which Sessions encountered runtime-state invalidity. Large diagnostic-payload retention/compaction strategy may be optimized later and is not designed here.

## Rationale

Collapsing rejection, deterministic execution failure, state invalidity, and transient infrastructure failure into one bucket would force an implementation to guess at correct behavior case by case, risking either over-terminalizing a Session for an ordinary rejected/stale operation, or under-terminalizing past a genuine invariant violation whose continued execution cannot be trusted. Keeping expected rejection (class A) untouched from GAME-ADR-0011's already-accepted no-op semantics avoids re-litigating that decision. Distinguishing deterministic execution failure (class B) from durable state invalidity (class C) matters operationally: one is normally a game-definition/authored-logic problem worth surfacing to whoever owns that game's content; the other is a runtime/persistence integrity problem worth surfacing to platform operators - conflating them under one terminal reason would make both harder to triage later. Treating transient infrastructure failure (class D) as a non-Session-domain event follows directly from the already-accepted process-agnostic/transactional-atomicity model (GAME-ADR-0013): a transaction that never committed is, from Session Runtime's perspective, as if nothing happened, so infrastructure retry policy - not Session lifecycle - is the correct owner.

Not persisting a fake RuntimeTurn for a fatal attempt preserves the integrity of GAME-ADR-0007's accepted invariant that one committed RuntimeTurn corresponds to one authoritative Session-state sequence; inventing a Turn to represent absence-of-progress would corrupt that invariant and force every future reader of RuntimeTurn history to special-case "empty"/"failed" Turns. A separate `session_runtime_failures` entity gives fatal diagnostics a durable home without overloading RuntimeTurn's meaning, mirroring the existing precedent of keeping Interaction/Timer entities separate from Turn while still referencing it (GAME-ADR-0007).

Atomic fatal materialization (terminalization + diagnostic persistence in one transaction) is chosen over a two-step "rollback, then separately terminalize" design because the two-step version durably risks leaving a Session `RUNNING` after a known-fatal deterministic failure if the second step is ever skipped, delayed, or fails independently - exactly the inconsistency this record must prevent. Because the engine executes in-memory relative to Session Runtime's own transaction, there is no technical reason the two outcomes cannot be one commit.

Generic public `INTERNAL_ERROR` projection follows the same reasoning already applied to `RUNTIME_INACTIVITY_EXPIRED` (GAME-ADR-0014): internal terminal-reason granularity serves operators and future engineers, not players, and raw technical detail (invariant names, engine paths, error codes) has no legitimate player-facing use and a real risk of leaking implementation internals. Not blaming the triggering user input when an authored bug causes a deterministic failure is a correctness/fairness concern distinct from technical detail suppression: an `ErrInputRejected` genuinely means "your input didn't validate," while a downstream authored-script failure after a valid input means the platform/game, not the player, is at fault, and conflating the two would mislead players about what happened.

Keeping `session_runtime_failures` queryable relationally rather than folding it into the same hard-delete-after-archival policy as `session_runtime_turns`/`session_runtime_steps`/etc. (GAME-ADR-0009) follows the same reasoning that already exempts `session_actors`/`session_participants`: lightweight, low-volume metadata supporting ongoing product/operational queries (failure-rate-by-error-code, failure-rate-by-game-definition) has different retention economics than heavy per-Turn/per-Step runtime history, and forcing an operator to rehydrate the archive merely to answer "how many Sessions failed last week" would defeat the purpose of keeping any relational failure signal at all.

## Alternatives Considered

### Treat every non-rejection engine error as an immediate Session-terminalizing failure, with no separate deterministic/state-invalid distinction

Rejected. Collapsing class B and class C into one terminal reason would make it impossible to tell, without inspecting `diagnostic_payload`, whether a wave of failures is caused by a buggy authored game definition (class B, actionable by content owners) or a runtime/persistence integrity problem (class C, actionable by platform engineers) - exactly the operational distinction GAME-ADR-0014's `terminal_reason` precedent already values.

### Persist a synthetic/placeholder RuntimeTurn to represent the failed attempt

Rejected. This would corrupt GAME-ADR-0007's accepted invariant that one committed RuntimeTurn equals one authoritative Session-state sequence and one authoritative Snapshot, forcing every future consumer of RuntimeTurn history (recovery, archival, audit/replay tooling) to special-case an empty/failed Turn that never actually executed to a valid state.

### Roll back the attempted Turn and terminalize the Session as two separate operations

Rejected. A crash or failure between the two steps could durably leave a Session `RUNNING` after a known-fatal deterministic failure, which this record explicitly must prevent; atomic materialization in one transaction removes that window entirely.

### Terminalize the Session immediately on any transient infrastructure error (Postgres timeout, deadlock, transient network failure)

Rejected. This would make ordinary, recoverable infrastructure hiccups destroy otherwise-healthy Sessions, contradicting the already-accepted process-agnostic/transactional-atomicity model (GAME-ADR-0013) under which an uncommitted transaction is equivalent to nothing having happened from the Session-domain perspective.

### Expose raw `ExecutionErrorCode`/technical terminal detail directly to players for transparency

Rejected. Raw engine paths, invariant names, and internal identifiers have no legitimate player-facing purpose, risk leaking implementation internals (and, in principle, information useful for probing the runtime), and provide no actionable information a player could act on; a generic `INTERNAL_ERROR` plus operator-facing diagnostics serves both audiences appropriately, mirroring the existing `RUNTIME_INACTIVITY_EXPIRED` -> generic-reason precedent.

### Fold `session_runtime_failures` into the same hard-delete-after-archival policy as the heavy runtime-history tables

Rejected. Fatal-failure metadata is low-volume relative to per-Turn/per-Step history and has durable operational/product query value (failure-rate-by-error-code, failure-rate-by-game-definition) that heavy archival compaction would otherwise hide behind a rehydration step, mirroring why `session_actors`/`session_participants` are already excluded from that same policy (GAME-ADR-0009).

## Consequences

- Future Session Runtime implementation must classify every non-successful execution/persistence outcome into exactly one of the four classes before deciding Session-lifecycle consequence; it must not invent a fifth ad hoc category without a new decision record.
- A fatal attempted execution (class B/C) must never be persisted as a `session_runtime_turns` row; `current_turn_id` must remain at the last successfully committed Turn.
- Session Runtime gains a `session_runtime_failures` conceptual entity (fields listed above) separate from RuntimeTurn, to be added to the persistence model and eventually migrated; this record does not migrate or implement it.
- Fatal-class terminalization (`RUNTIME_EXECUTION_FAILED`/`RUNTIME_STATE_INVALID`) and its `SessionRuntimeFailure` record must be persisted atomically in one transaction together with the Session's `TERMINAL` phase/`terminal_reason`/`terminal_at`.
- Transient infrastructure failures (class D) must never, by themselves, terminalize a Session or create a `SessionRuntimeFailure`.
- Player-facing terminal projection for both fatal internal reasons must remain the generic `INTERNAL_ERROR` category; no raw technical/internal detail may reach clients through any current or future public contract.
- Live terminal delivery must occur only after durable commit; resync must independently allow a client to discover the same terminal truth.
- `session_runtime_failures` must be excluded from the automatic hard-delete-after-archival policy that GAME-ADR-0009 applies to heavy runtime-history tables, pending a later, separately designed retention/compaction strategy if payload volume ever warrants one.

## Canonical Knowledge Impact

- `game/README.md` - adds a Session Runtime Failure Classification And Diagnostic Persistence section referencing this ADR.
- `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` - adds the conceptual `SESSION_RUNTIME_FAILURE` entity/diagram and updates the archival/hard-delete section to exclude it.

## Implementation Impact

No migration, production code, engine/compiler change, Coordinator behavior, observability pipeline, archival worker, or WORK is authorized by this record. Concrete configured execution-budget/loop/depth limits, the exhaustive `source_kind`/interaction-kind enums, the exact public DTO/HTTP/WebSocket status-code contract for `INTERNAL_ERROR`, and interaction/timer closure semantics at fatal termination remain separately deferred design topics.
