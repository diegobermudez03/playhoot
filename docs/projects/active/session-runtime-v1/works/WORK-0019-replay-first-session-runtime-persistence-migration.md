# WORK-0019: Replay-First Session Runtime Persistence Migration

Status: READY
Created: 2026-09-20
Last status change: 2026-09-22 (HUMAN-AUTHORIZED DRAFT -> READY; all 5 Blockers resolved HUMAN-APPROVED, spec sections filled in - see Blockers section for full resolution history)

Related decisions:
- GAME-ADR-0024 (Replay-First Session Runtime Persistence - the accepted direction this WORK implements)
- GAME-ADR-0007 (superseded in part - Snapshot-persistence portion only; RuntimeTurn/RuntimeStep conceptual model and Turn/Interaction/Timer relationship rules remain and are restated by GAME-ADR-0024)
- GAME-ADR-0009 (superseded in part - GCS archive-location portion only; archive-metadata-entity concept and verified-hard-delete policy remain and are restated by GAME-ADR-0024)
- GAME-ADR-0013 (process-agnostic recovery - extended, not contradicted, by replay-based reconstruction)
- GAME-ADR-0018 (RUNNING mutation serialization - authoritative Turn-sequence ordering is what already orders the replay-input log)
- GAME-ADR-0019 (RuntimeTurn execution bound and terminal cleanup - unaffected)
- GAME-ADR-0020 (post-commit client delivery semantics - unaffected; this WORK changes durable persistence, not delivery)
- GAME-ADR-0023 (current-turn pointer lives on `sessions` - unaffected in mechanism, reinterpreted in meaning per GAME-ADR-0024)

Canonical context:
- `game/docs/decisions/GAME-ADR-0024-replay-first-session-runtime-persistence.md` (the accepted model this WORK implements, including the audited replay-input catalog)
- `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` ("Replay-First Persistence Model" section, and the revised "Archive Metadata"/"Archival And Hard-Delete Policy"/"Process-Agnostic Recovery" sections)
- `game/docs/DATA_MODEL.md` (current implemented schema this WORK migrates away from - `session_runtime_turns.snapshot_payload`/`snapshot_format_version`)
- `game/language/v1/engine/random.go`, `game/language/v1/engine/snapshot.go` (`InitializationInput{RootParameters, Seed}`, `RandomState` - the confirmed-deterministic initialization this WORK's replay reconstruction depends on)
- `game/language/v1/engine/signal.go`, `game/language/v1/program/signal.go` (the complete `SignalKind`/`SignalSource` catalog GAME-ADR-0024 audited to produce the replay-input list)
- `game/session/workflows/sessionlifecycle/step_start.go` (`drawSeed()` - today's one-time Seed draw, currently unpersisted; WORK-0006's own Seed-persistence addition is reassigned to this WORK, see below)
- `docs/projects/active/session-runtime-v1/works/WORK-0006-broaden-live-fanout-effects-presentations.md` (its "persist Start's Seed" scope item moves here)
- `docs/projects/active/session-runtime-v1/works/WORK-0017-archival.md` (the downstream compaction WORK this migration's replay-input log makes viable in PostgreSQL)

## Outcome

Session Runtime's durable persistence stops treating the per-Turn `engine.Snapshot` as authoritative history. Instead, it durably persists exactly what GAME-ADR-0024 requires - pinned Game semantics (already true), Start's deterministic initialization (`Seed`/`RootParameters`), and the ordered, durably-reconstructible driving-input history for every RuntimeTurn-producing cause - and derives current/historical Runtime state, Presentations, and Effects by deterministic replay rather than by loading a stored payload. `session_runtime_turns.snapshot_payload`/`snapshot_format_version` are removed. `session_runtime_steps`'s fate is decided (removed, bounded, or retained) against the needed-for-live-correctness/needed-as-replay-input/needed-for-diagnostics test GAME-ADR-0024 establishes. A `RUNNING` Session survives process loss by replay-based reconstruction, not by reloading a durable checkpoint.

## Context

This WORK exists because GAME-ADR-0024 materially changes already-implemented persisted schema (`session_runtime_turns.snapshot_payload`/`snapshot_format_version`, added by WORK-0003/DONE) and an already-accepted-but-unimplemented archival direction (WORK-0017/GAME-ADR-0009, GCS). WORK-0003 is not reopened or rewritten - it correctly describes what was implemented at the time, under the then-accepted GAME-ADR-0007 model. This WORK owns the forward migration.

Every currently-implemented RuntimeTurn-producing path (`Start`, `AnswerInteraction`) already has its cause's content normalized in an existing table (`session_interactions`), independent of the Snapshot column - removing the Snapshot column does not remove any information those paths need for their own already-correct behavior (open-interaction identity, response recording). What removing the Snapshot column *does* remove is the one thing every current read of "what is the current game state" relies on today: a stored payload to deserialize. This WORK must replace that read path with replay-based reconstruction before (or atomically with) removing the column, not after - there must be no window where current Runtime state is unreconstructible.

## Scope

### In Scope

- Remove `session_runtime_turns.snapshot_payload`/`snapshot_format_version` (edited directly into the existing WORK-0003 migration - see Blocker 5 Resolution; no `session_runtime_state` table is introduced, consistent with GAME-ADR-0023's already-accepted "thin pointer" stance).
- Remove `session_runtime_steps` in full: the table (edited directly into its existing WORK-0003 migration), `CreateRuntimeStep` and every call site (`step_start.go`, `step_answer_interaction.go`), and `runtimeturn.Drain`'s `StepTrace`/`Steps` accumulation to the extent it existed only to feed that table - see Blocker 2 Resolution. `Drain`'s own `MaxSteps` bound and step-counting loop are unaffected; only the persisted trace is removed.
- Add durable persistence for Start's `Seed` and `RootParameters`: a dedicated one-row-per-Session table (conceptually `session_id`, `seed`, `root_parameters` JSONB via `engineservice.EncodeValue`/`DecodeValue`) - see Blocker 1 Resolution. This absorbs WORK-0006's original "persist Start's Seed" scope item, which WORK-0006 itself no longer owns.
- Add a generic `session_cause_events` satellite table (`id`, `session_id`, `runtime_turn_id`, `cause_kind`, `actor_id` nullable, `payload` JSONB, `created_at`) plus `session_runtime_turns.source_cause_event_id` (nullable FK) - see Blocker 3 Resolution. This WORK creates the table and column; it does not populate them, since no cause using them (UserIntent/SessionCancelled/UserDisconnected/UserReconnected) is implemented yet - WORK-0010/WORK-0011/WORK-0015 are each required to write into this table rather than introduce their own.
- Build the replay-reconstruction capability: given a Session's pinned `game_definitions.script`, Start's `Seed`/`RootParameters`, and the ordered `session_runtime_turns` log (with each row's cause resolved against its normalized content - `session_interactions`, `session_timer_obligations`, `session_cause_events`), deterministically reproduce the current authoritative `engine.Snapshot` in memory. No ephemeral cache is built alongside it - see Blocker 4 Resolution and Out of Scope.
- Update every existing read path that currently loads `session_runtime_turns.snapshot_payload` (`Start`'s idempotent-replay path, `AnswerInteraction`'s load-current-state path) to instead call the new replay-reconstruction capability, with no behavior change to any already-accepted outcome (idempotent replay, interaction resolution, etc.).
- Update `game/docs/DATA_MODEL.md`, `game/CURRENT_STATE.md`, `game/docs/FLOWS.md` to reflect the migrated schema once implemented.

### Out of Scope

- Any new RuntimeTurn-producing cause's own design (WORK-0010 UserIntent, WORK-0011 SessionCancelled, WORK-0012/WORK-0013 TimerExpired, WORK-0015/WORK-0018 UserDisconnected/UserReconnected) - this WORK defines the *model* those causes must integrate with (`session_cause_events`, a durable, ordered, replayable representation) but does not itself design or populate any of their specific `payload` shapes. Each owning WORK is responsible for satisfying this WORK's model when it lands.
- The ephemeral in-memory current-state cache - explicitly deferred (Blocker 4 Resolution), conditioned on WORK-0023 (Session Runtime Observability Metrics) first measuring actual replay cost. Revisiting this decision is a future follow-up, not this WORK's own scope.
- WORK-0022 (Session/Platform Abuse and Resource-Rate Limits) and WORK-0023 (Session Runtime Observability Metrics) themselves - both identified while resolving this WORK's Blockers, but each is its own separately-tracked WORK with its own design task.
- The archive/compaction implementation itself (WORK-0017) - this WORK makes a PostgreSQL-resident archive viable by bounding what needs to be archived, but does not implement the archive table/compaction transaction.
- Any change to `sessionlifecycle.Manager`'s existing public method signatures/business outcomes for `Start`/`AnswerInteraction` beyond what is strictly required to swap the internal state-loading mechanism.
- Any change to live delivery/fan-out (`play`/`api`) - this WORK is a durable-persistence migration, not a transport change.
- Replay compatibility across a hypothetical future `game/language/v2` beyond relying on the already-existing pinned `program.Metadata.LanguageVersion` (see GAME-ADR-0024's still-open versioning question) - if a concrete need for finer-grained compiler-build tracking emerges, that is a new, separately-escalated decision, not silently invented here.

## Approved Design

GAME-ADR-0024 settles the overall direction (replay-first, no durable Snapshot, PostgreSQL-resident archive). This WORK's own 5 Blockers resolve the concrete implementation shape, all HUMAN-APPROVED 2026-09-22 (full rationale in each Blocker's own Resolution below):

1. **Start's `Seed`/`RootParameters`** persist in a dedicated one-row-per-Session table, captured atomically in the same transaction as Start's Turn 1 - never derived from `session_participants`' current (mutable, rejoin-overwritten) state.
2. **`session_runtime_steps` is removed entirely** - no bounded retention, no replacement table. Conditioned on `session_runtime_failures` (WORK-0014) remaining the durable home for unexpected engine outcomes once implemented, and on successful outputs remaining recoverable only via replay, never via a persisted per-Step trace.
3. **Future RuntimeTurn causes without an existing normalized home** (UserIntent, SessionCancelled, UserDisconnected/UserReconnected) share one generic satellite table, `session_cause_events`, discriminated by `cause_kind` - not a bespoke table per future WORK. `session_interactions`/`session_timer_obligations` remain excluded and unchanged, since they carry independent mutable lifecycle state that a write-once occurrence table does not fit.
4. **No ephemeral current-state cache for V1.** Replay-on-every-read is accepted until WORK-0023 supplies real duration data.
5. **Migration mechanics**: given zero deployed or locally-persisted Session data anywhere today, this WORK edits the existing WORK-0003 migrations in place rather than adding new forward-only ones, and both schema changes (new Start-init/`session_cause_events` tables, removed Snapshot/Steps columns/table) land together. This is a one-time exception; ordinary expand/contract discipline applies again the moment any environment holds Session data worth preserving.

## Constraints and Invariants

- No durable full-Snapshot persistence is reintroduced under any name.
- No durable checkpoint/recovery table is introduced as a substitute for replay - an ephemeral in-memory cache is the only accepted performance optimization, and its loss must only cause replay, never data loss or a correctness gap.
- Every RuntimeTurn-producing cause must have a durable, ordered, replayable representation before it is authorized to produce a RuntimeTurn - this WORK enforces that requirement on itself (for `Start`/`AnswerInteraction`, already true today) and states it as a binding constraint on every later WORK introducing a new cause.
- `sessions.current_turn_id` continues to exist and mean "the current authoritative position in the replay-input log" (GAME-ADR-0023, reinterpreted per GAME-ADR-0024) - this WORK does not remove or relocate that pointer.
- **Exception approved 2026-09-22 (see Blocker 5 Resolution below):** for this WORK specifically, the migrations this WORK changes (`20260919000000_session_runtime_turns.go`'s Snapshot columns, `20260919000001_session_runtime_steps.go` in full) may be edited/removed directly in place rather than added as new forward migrations, since nothing has ever been deployed and no environment (including local) holds any persisted data to preserve. This does not reopen WORK-0001/WORK-0003/WORK-0004's own historical accuracy as records of what was designed/implemented at the time - only the migration files themselves are edited. This exception does not generalize past this WORK's own implementation; see Blocker 5 Resolution for the ordinary expand/contract discipline that applies from the point real persisted state exists anywhere (including local).
- No behavior change to any already-accepted `Start`/`AnswerInteraction` outcome, idempotent-replay semantics, or interaction-resolution semantics - this is an internal persistence-mechanism swap, not a business-logic change.

## Acceptance Criteria

- `session_runtime_turns.snapshot_payload`/`snapshot_format_version` no longer exist in the schema after this WORK's migrations.
- `session_runtime_steps` no longer exists in the schema; `CreateRuntimeStep` and every call site are removed; no replacement persisted trace of any kind is introduced.
- A dedicated Start-initialization table durably persists `Seed`/`RootParameters` for every Session that completes Start, written atomically with Start's Turn 1.
- `session_cause_events` exists in the schema with the columns in Approved Design, and `session_runtime_turns.source_cause_event_id` exists as a nullable FK to it - unpopulated by this WORK, since no cause using it is implemented yet.
- A Session's current authoritative Runtime state is correctly reconstructable purely from pinned Game semantics + Start's Seed/RootParameters + the ordered replay-input log, verified by a test that reconstructs a multi-Turn Session's state via replay and compares it against the state produced by the original live execution.
- `Start`'s idempotent-replay path and `AnswerInteraction`'s current-state-load path both continue to produce identical outcomes to their pre-migration behavior for every existing test case.
- Process-loss recovery of a `RUNNING` Session (simulated by reconstructing purely from durable state, with no in-memory cache of any kind involved, per Blocker 4) produces the same current state as before the simulated loss.
- No ephemeral cache, checkpoint, or durable Snapshot-shaped persistence of any kind is introduced anywhere in this WORK's implementation.
- `go build ./...`, `go vet ./...`, `go test ./... -count=1` pass, with no new failure beyond the already-recorded out-of-scope `getgame` JSONB-comparison defect.

## Implementation Freedom

Exact table/column names beyond what Approved Design fixes conceptually (the Start-init table's name, `session_cause_events`'s exact name if a better one emerges), exact Go type/repository shapes, and internal replay-reconstruction function structure are ordinary Codebase Agent autonomy per the Operating Model - not material decisions requiring further escalation.

## Verification

- `go build ./...`, `go vet ./...`, `gofmt -l` (changed files) clean.
- `go test ./... -count=1` against real Postgres, no new failure beyond the already-recorded out-of-scope `getgame` JSONB-comparison defect.
- A new test reconstructing a multi-Turn Session's state via replay and comparing it against the state produced by the original live execution (see Acceptance Criteria).
- A new test simulating process-loss recovery (no in-memory cache involved) producing identical current state.

## Blockers

Status: **ALL RESOLVED, HUMAN-APPROVED (2026-09-22)** - see each item's own Resolution below. None of these were business/product-scope questions; all were implementation-shape decisions GAME-ADR-0024 deliberately left open for this WORK. This WORK is otherwise ready to move DRAFT -> READY once the remaining template sections (Approved Design, Acceptance Criteria, Verification, Documentation Impact) are filled in against these resolutions - see this WORK's own conversation record for that next step.

1. **Exact durable shape for Start's `Seed`/`RootParameters`.** A new column pair on `sessions` (parallel to how `host_actor_id` already lives there), a dedicated one-row-per-Session table, or fields on the Start `session_runtime_turns` row itself (Turn `sequence = 1` always being Start's own Turn). Recommend: a dedicated small table or columns on `sessions`, since `RootParameters` is Start-specific and does not recur on every Turn the way `source_kind`/`actor_id` do - open for human/DRAFT-review input.

   **Resolution (2026-09-22, HUMAN-APPROVED):** a dedicated one-row-per-Session table (exact name/columns is implementation-planning detail for the READY spec - conceptually `session_id`, `seed`, `root_parameters` JSONB, encoded via `engineservice.EncodeValue`/`DecodeValue` the same way `session_interactions` already encodes engine-typed payloads). Reinforced by inspecting `game/session/workflows/sessionlifecycle/internal/repo/participant.go`: `session_participants` is a current-value-only table (`CreateParticipant`'s rejoin path overwrites `joined_at` and resets `left_at` to `NULL`), so Start's actual roster at the serialized Start moment cannot be reliably re-derived from `session_participants`' current state after any later leave/rejoin - it must be captured independently and atomically in the same transaction that creates Start's Turn 1, not derived.

2. **`session_runtime_steps`'s final disposition.** Remove entirely, retain with a short bounded retention window (for near-term operational debugging only, explicitly excluded from any replay/archive role), or leave unchanged pending real operational experience. This is a genuine tradeoff (debuggability vs. the same unbounded-growth concern that motivated removing the Snapshot column) worth explicit human input rather than a unilateral implementation choice, since it revisits a decision GAME-ADR-0007 originally made deliberately.

   **Resolution (2026-09-22, HUMAN-APPROVED):** remove entirely. Confirmed by inspection that `session_runtime_steps`/`CreateRuntimeStep` is write-only in the current codebase - no read path exists anywhere in `game/session`. WORK-0014 (Runtime Failure Diagnostics), the most plausible future consumer, explicitly designs its own `diagnostic_payload` to be self-contained (captured directly from in-memory state at the moment of failure) and explicitly does not depend on a persisted Steps trace. Approved on the condition (stated by the human approving this) that: (a) any *unexpected* engine outcome - any non-rejection execution failure or state-invalidity failure (GAME-ADR-0017 classes B/C) - remains durably captured elsewhere (it does: `session_runtime_failures`, owned by WORK-0014, unaffected by this removal since it is a wholly separate entity from `session_runtime_steps`); (b) *successful* engine outputs remain debuggable by deterministic replay of the durable input log (guaranteed by this WORK's own replay-reconstruction capability), not by inspecting a persisted per-Step trace. Both conditions hold under the accepted design. Note: WORK-0014 itself is not yet implemented (still PLANNED) - today, a deterministic execution failure sets only a coarse `terminal_reason`, with no `diagnostic_payload` yet (see `step_start.go`'s `terminalizeStartFatal` doc comment: "no runtime-failure diagnostic entity exists yet"). The human explicitly chose to leave WORK-0014's position in the Phase 1 sequence unchanged rather than reprioritize it, on the understanding that its own scope already correctly commits to closing this gap when it is implemented.

3. **Exact durable representation for each future RuntimeTurn cause without an existing normalized home** (UserIntent's submitted arguments, SessionCancelled's issuing actor, the UserDisconnected/UserReconnected occurrence itself as an ordered event rather than only `session_actors.semantic_presence`'s current value). This WORK defines the *requirement* (every cause must be durably, order-preservingly reconstructible) but the concrete shape for each specific future cause is better decided by that cause's own owning WORK (WORK-0010/WORK-0011/WORK-0015) at the point it is actually designed, informed by this WORK's model - listed here so it is not silently assumed solved by this WORK alone.

   **Resolution (2026-09-22, HUMAN-APPROVED):** rather than leaving each future cause's satellite shape entirely to its own owning WORK (WORK-0010/0011/0015) to invent independently, this WORK fixes one shared generic satellite table for causes that are pure immutable occurrences - `session_cause_events` (exact name confirmable at READY): `id`, `session_id` (FK `sessions`), `runtime_turn_id` (FK `session_runtime_turns`, 1:1 - the Turn this cause produced), `cause_kind` (discriminator - `USER_INTENT`, `SESSION_CANCELLED`, `USER_DISCONNECTED`, `USER_RECONNECTED`, and any later addition), `actor_id` (nullable), `payload` (JSONB, shape specific to and documented per `cause_kind`), `created_at`. `session_runtime_turns` gains a third nullable pointer, `source_cause_event_id`, mirroring the already-established `source_interaction_id`/`source_timer_obligation_id` pattern.

   `session_interactions`/`session_timer_obligations` are explicitly excluded from this table and remain their own dedicated entities - they are not mere historical occurrence records: both carry independent mutable lifecycle state (`ACTIVE`/`CLOSED`/`TERMINATED`) that is read and updated while still open, which is a materially different access pattern from a write-once immutable fact. WORK-0010 (UserIntent), WORK-0011 (SessionCancelled), and WORK-0015 (UserDisconnected/UserReconnected) must each write into `session_cause_events` with their own `cause_kind`/`payload` shape rather than introducing a new bespoke table of their own.

4. **Ephemeral current-state cache mechanism, if adopted at all** - in-process only (per-Coordinator-process memory, lost on restart, rebuilt by replay), or deliberately not built for V1 (replay on every RUNNING-mutation read, accepted as adequate given the `MAX_STEPS_PER_RUNTIME_TURN = 20`-bounded, already-serialized-per-Session nature of RUNNING mutations). Recommend evaluating actual replay cost against a real authored Definition before committing to cache complexity that may not be needed yet - a NOW/SOON/LATER/NOT NEEDED classification per `docs/ai/OPERATING_MODEL.md`'s Anti-Overengineering guidance, not decided here.

   **Resolution (2026-09-22, HUMAN-APPROVED):** not built for V1 (NOT NEEDED / LATER classification) - replay on every RUNNING-mutation read is accepted as adequate for now, deferred pending real measurement. Explicitly conditioned on adding real observability first: replay/reconstruction duration must be measured (logging/metrics) so a future cache decision is data-driven rather than speculative. A new WORK (see Documentation Impact / Consequences) is being opened for this observability need, since no metrics capability exists in the repository today beyond the `monitoring` package's own `Alert` stub (`monitoring/alerts.go`) - confirmed by inspection; `monitoring/README.md` already scaffolds `monitoring` as the intended home for "metrics, alerts, etc." but only `Alert` exists so far.

5. **Exact migration sequencing** - whether the Seed/RootParameters-persistence migration and the Snapshot-column-removal migration can land together or must land in an order that avoids ever having a Session whose current state cannot be reconstructed by either mechanism (relevant only for already-`RUNNING` Sessions at deploy time, if any exist when this ships - a Session Runtime V1 environment realistically has none yet, but the design should not assume that).

   **Resolution (2026-09-22, HUMAN-APPROVED):** confirmed by direct inspection (no tables exist in the local Postgres instance at all; nothing has ever been deployed anywhere) that zero `RUNNING` Sessions of any kind currently exist in any environment, including local. Given that, the underlying risk this Blocker exists to prevent - a `RUNNING` Session whose `Seed` was never durably captured becoming unreconstructible once the Snapshot column is gone - cannot occur right now, since there is no persisted state anywhere to protect. The human explicitly authorized editing the existing WORK-0003 migrations in place (`20260919000000_session_runtime_turns.go`'s Snapshot columns, `20260919000001_session_runtime_steps.go` in full) rather than requiring new additive forward migrations, and both this WORK's schema changes may land together in one pass - see the corresponding Constraints and Invariants exception recorded above.

   This is an explicit, narrow, one-time exception, not a new general practice. The ordinary rule - restated here for when it starts to matter - is standard expand/contract: (1) add the new durable Seed/RootParameters persistence and the code that writes/reads it, without touching the Snapshot column yet; (2) confirm no Session that predates that change remains `RUNNING` (such a Session's `Seed` was never captured and can never become replay-reconstructible after the fact - this is an accepted, permanent limitation, not something migration ordering can fix); (3) only then remove the Snapshot column. This applies again the moment any environment - including a developer's own local database - holds Session data worth preserving across a schema change, not only once this ships to a real deployment.

## Documentation Impact

### Accepted / Canonical Knowledge

- Already updated by this reconciliation pass: `game/docs/decisions/GAME-ADR-0024-replay-first-session-runtime-persistence.md` (new), `game/docs/decisions/INDEX.md`, `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` ("Replay-First Persistence Model" and revised Archive/Recovery sections), `game/docs/decisions/GAME-ADR-0007-*.md`/`GAME-ADR-0009-*.md` (`Superseded by` pointer only).

### Current-State Documentation After Implementation

- `game/docs/DATA_MODEL.md` - remove `session_runtime_turns.snapshot_payload`/`snapshot_format_version` and the entire `session_runtime_steps` class/relationships; add the new Start-init table and `session_cause_events` (with `session_runtime_turns.source_cause_event_id`).
- `game/CURRENT_STATE.md`, `game/docs/FLOWS.md` - describe replay-based state reconstruction as now-implemented; remove any remaining reference to persisted per-Turn Snapshots or `session_runtime_steps`.
- `docs/projects/active/session-runtime-v1/PROJECT.md` - already updated during this WORK's Blocker resolution (WORK-0022, WORK-0023 added); no further change expected from implementation alone.

### Intentionally Unchanged

- WORK-0001, WORK-0003, WORK-0004 (completed historical WORKs) - not reopened or rewritten as records of what was designed/implemented at the time; only their migration *files* are edited in place per Blocker 5's approved exception, which does not alter what those WORK documents themselves claim happened.
- `sessionlifecycle.Manager`'s public method signatures for `Start`/`AnswerInteraction` (no change to their outcome shape, only to their internal state-loading mechanism).

## Completion Record

Not yet DONE. Status: READY (human-authorized 2026-09-22). All 5 Blockers are HUMAN-APPROVED (see Blockers section) and this spec's template sections are filled in accordingly. Implementation has not started - next step is the Codebase Agent implementation/review handoff per `docs/ai/protocols/IMPLEMENTATION_REVIEW.md`.
