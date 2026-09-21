# WORK-0019: Replay-First Session Runtime Persistence Migration

Status: DRAFT
Created: 2026-09-20
Last status change: 2026-09-20

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

- Remove `session_runtime_turns.snapshot_payload`/`snapshot_format_version` (migration; no `session_runtime_state` table is introduced, consistent with GAME-ADR-0023's already-accepted "thin pointer" stance).
- Add durable persistence for Start's `Seed` and `RootParameters` (the initial `players` roster passed to `engineservice.NewSnapshot`) - exact column/table shape is this WORK's own design task. This absorbs WORK-0006's original "persist Start's Seed" scope item, which WORK-0006 itself no longer owns (see WORK-0006's revised Scope).
- Build the replay-reconstruction capability: given a Session's pinned `game_definitions.script`, Start's `Seed`/`RootParameters`, and the ordered `session_runtime_turns` log (with each row's cause resolved against its normalized content - `session_interactions`, `session_timer_obligations`, and future per-cause satellite data), deterministically reproduce the current authoritative `engine.Snapshot` in memory.
- Design and, if adopted, implement an ephemeral, non-durable in-memory current-state cache to avoid replaying from the beginning on every read - explicitly not authoritative, no durable-recovery obligation.
- Decide `session_runtime_steps`'s disposition (remove, bound/short-retain, or leave unchanged) against GAME-ADR-0024's justification test.
- Update every existing read path that currently loads `session_runtime_turns.snapshot_payload` (`Start`'s idempotent-replay path, `AnswerInteraction`'s load-current-state path, any diagnostic/debug read) to instead call the new replay-reconstruction capability, with no behavior change to any already-accepted outcome (idempotent replay, interaction resolution, etc.).
- Update `game/docs/DATA_MODEL.md`, `game/CURRENT_STATE.md`, `game/docs/FLOWS.md` to reflect the migrated schema once implemented.

### Out of Scope

- Any new RuntimeTurn-producing cause's own design (WORK-0010 UserIntent, WORK-0011 SessionCancelled, WORK-0012/WORK-0013 TimerExpired, WORK-0015/WORK-0018 UserDisconnected/UserReconnected) - this WORK defines the *model* those causes must integrate with (a durable, ordered, replayable representation) but does not itself design any of their specific payloads. Each owning WORK is responsible for satisfying this WORK's model when it lands.
- The archive/compaction implementation itself (WORK-0017) - this WORK makes a PostgreSQL-resident archive viable by bounding what needs to be archived, but does not implement the archive table/compaction transaction.
- Any change to `sessionlifecycle.Manager`'s existing public method signatures/business outcomes for `Start`/`AnswerInteraction` beyond what is strictly required to swap the internal state-loading mechanism.
- Any change to live delivery/fan-out (`play`/`api`) - this WORK is a durable-persistence migration, not a transport change.
- Replay compatibility across a hypothetical future `game/language/v2` beyond relying on the already-existing pinned `program.Metadata.LanguageVersion` (see GAME-ADR-0024's still-open versioning question) - if a concrete need for finer-grained compiler-build tracking emerges, that is a new, separately-escalated decision, not silently invented here.

## Approved Design

GAME-ADR-0024 already settles the direction (replay-first, no durable Snapshot, PostgreSQL-resident archive). What remains for this WORK's own DRAFT-to-READY progression, listed as Blockers below, is the concrete implementation shape.

## Constraints and Invariants

- No durable full-Snapshot persistence is reintroduced under any name.
- No durable checkpoint/recovery table is introduced as a substitute for replay - an ephemeral in-memory cache is the only accepted performance optimization, and its loss must only cause replay, never data loss or a correctness gap.
- Every RuntimeTurn-producing cause must have a durable, ordered, replayable representation before it is authorized to produce a RuntimeTurn - this WORK enforces that requirement on itself (for `Start`/`AnswerInteraction`, already true today) and states it as a binding constraint on every later WORK introducing a new cause.
- `sessions.current_turn_id` continues to exist and mean "the current authoritative position in the replay-input log" (GAME-ADR-0023, reinterpreted per GAME-ADR-0024) - this WORK does not remove or relocate that pointer.
- Migrations are additive/forward-only against already-shipped schema (per this Project's Decision 2: existing implemented behavior/migrations from WORK-0001/WORK-0003/WORK-0004 are not rewritten; this WORK adds new migrations that change schema going forward).
- No behavior change to any already-accepted `Start`/`AnswerInteraction` outcome, idempotent-replay semantics, or interaction-resolution semantics - this is an internal persistence-mechanism swap, not a business-logic change.

## Acceptance Criteria

Acceptance criteria are finalized once the Blockers below resolve; the following are already clear from GAME-ADR-0024 and are expected to survive into the READY version unchanged:

- `session_runtime_turns.snapshot_payload`/`snapshot_format_version` no longer exist in the schema after this WORK's migrations.
- A Session's current authoritative Runtime state is correctly reconstructable purely from pinned Game semantics + Start's Seed/RootParameters + the ordered replay-input log, verified by a test that reconstructs a multi-Turn Session's state via replay and compares it against the state produced by the original live execution.
- `Start`'s idempotent-replay path and `AnswerInteraction`'s current-state-load path both continue to produce identical outcomes to their pre-migration behavior for every existing test case.
- Process-loss recovery of a `RUNNING` Session (simulated by discarding any in-memory cache and reconstructing purely from durable state) produces the same current state as before the simulated loss.
- `go build ./...`, `go vet ./...`, `go test ./... -count=1` pass, with no new failure beyond the already-recorded out-of-scope `getgame` JSONB-comparison defect.

## Blockers

Status: **UNRESOLVED - DRAFT phase design questions**, not yet human-approved. None of these are business/product-scope questions; all are implementation-shape decisions GAME-ADR-0024 deliberately left open for this WORK.

1. **Exact durable shape for Start's `Seed`/`RootParameters`.** A new column pair on `sessions` (parallel to how `host_actor_id` already lives there), a dedicated one-row-per-Session table, or fields on the Start `session_runtime_turns` row itself (Turn `sequence = 1` always being Start's own Turn). Recommend: a dedicated small table or columns on `sessions`, since `RootParameters` is Start-specific and does not recur on every Turn the way `source_kind`/`actor_id` do - open for human/DRAFT-review input.
2. **`session_runtime_steps`'s final disposition.** Remove entirely, retain with a short bounded retention window (for near-term operational debugging only, explicitly excluded from any replay/archive role), or leave unchanged pending real operational experience. This is a genuine tradeoff (debuggability vs. the same unbounded-growth concern that motivated removing the Snapshot column) worth explicit human input rather than a unilateral implementation choice, since it revisits a decision GAME-ADR-0007 originally made deliberately.
3. **Exact durable representation for each future RuntimeTurn cause without an existing normalized home** (UserIntent's submitted arguments, SessionCancelled's issuing actor, the UserDisconnected/UserReconnected occurrence itself as an ordered event rather than only `session_actors.semantic_presence`'s current value). This WORK defines the *requirement* (every cause must be durably, order-preservingly reconstructible) but the concrete shape for each specific future cause is better decided by that cause's own owning WORK (WORK-0010/WORK-0011/WORK-0015) at the point it is actually designed, informed by this WORK's model - listed here so it is not silently assumed solved by this WORK alone.
4. **Ephemeral current-state cache mechanism, if adopted at all** - in-process only (per-Coordinator-process memory, lost on restart, rebuilt by replay), or deliberately not built for V1 (replay on every RUNNING-mutation read, accepted as adequate given the `MAX_STEPS_PER_RUNTIME_TURN = 20`-bounded, already-serialized-per-Session nature of RUNNING mutations). Recommend evaluating actual replay cost against a real authored Definition before committing to cache complexity that may not be needed yet - a NOW/SOON/LATER/NOT NEEDED classification per `docs/ai/OPERATING_MODEL.md`'s Anti-Overengineering guidance, not decided here.
5. **Exact migration sequencing** - whether the Seed/RootParameters-persistence migration and the Snapshot-column-removal migration can land together or must land in an order that avoids ever having a Session whose current state cannot be reconstructed by either mechanism (relevant only for already-`RUNNING` Sessions at deploy time, if any exist when this ships - a Session Runtime V1 environment realistically has none yet, but the design should not assume that).

## Documentation Impact

### Accepted / Canonical Knowledge

- Already updated by this reconciliation pass: `game/docs/decisions/GAME-ADR-0024-replay-first-session-runtime-persistence.md` (new), `game/docs/decisions/INDEX.md`, `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` ("Replay-First Persistence Model" and revised Archive/Recovery sections), `game/docs/decisions/GAME-ADR-0007-*.md`/`GAME-ADR-0009-*.md` (`Superseded by` pointer only).

### Current-State Documentation After Implementation

- `game/docs/DATA_MODEL.md` - remove `snapshot_payload`/`snapshot_format_version` from the implemented `session_runtime_turns` shape; add whatever new Seed/RootParameters columns/table this WORK's Blocker 1 resolves to; reflect `session_runtime_steps`'s resolved disposition.
- `game/CURRENT_STATE.md`, `game/docs/FLOWS.md` - describe replay-based state reconstruction as now-implemented.

### Intentionally Unchanged

- WORK-0001, WORK-0003, WORK-0004 (completed historical WORKs) - not reopened or rewritten; they correctly describe what was implemented under the then-accepted model.
- `sessionlifecycle.Manager`'s public method signatures for `Start`/`AnswerInteraction` (no change to their outcome shape, only to their internal state-loading mechanism).

## Completion Record

Not yet DONE. Status: DRAFT. Blockers 1-5 need human input/resolution before this WORK can move to READY.
