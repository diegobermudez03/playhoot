# WORK-0017: Archival

Status: PLANNED
Created: 2026-09-20
Last status change: 2026-09-20 (Part L reconciliation, same day: rewritten from GCS/object-storage archival to same-database PostgreSQL JSONB compaction - see "Rewrite Record (Part L Reconciliation, 2026-09-20)" below; the original GCS-direction Outcome/Context/Scope this WORK was created with are superseded, not merely amended, since the storage destination itself changed)

Related decisions:
- GAME-ADR-0009 (superseded in part - GCS archive-location decision only; archive-metadata-entity concept and verified-hard-delete policy remain and are restated by GAME-ADR-0024)
- GAME-ADR-0024 (Replay-First Session Runtime Persistence - the accepted direction this WORK now implements: PostgreSQL JSONB compaction, not GCS)

Canonical context:
- `game/docs/decisions/GAME-ADR-0024-replay-first-session-runtime-persistence.md` (the accepted archival model this WORK implements)
- `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` (revised "Archive Metadata"/"Archival And Hard-Delete Policy" sections)
- `docs/projects/active/session-runtime-v1/works/WORK-0019-replay-first-session-runtime-persistence-migration.md` (the replay-input log this WORK compacts; this WORK depends on that model being stable enough to compact, not necessarily fully implemented end-to-end first - see Blockers)
- `docs/projects/active/session-runtime-v1/works/WORK-0006-broaden-live-fanout-effects-presentations.md` (historical - originally added Start's `Seed` persistence toward this WORK's replay goal; that responsibility moved to WORK-0019)

## Outcome

A `TERMINAL` Session's normalized replay-input/operational-history rows (`session_runtime_turns`, `session_interactions`, `session_timer_obligations`, and `session_runtime_steps` if WORK-0019 retains it) are compacted into one durable, versioned JSON archive record in the same PostgreSQL database, after which the normalized rows that are no longer required are removed. The archive is self-sufficient for the intended historical replay/inspection contract - pinned Game semantic/version identity, Start's Seed/RootParameters, the ordered replay-input history, interactions/responses, terminal metadata, and any timer/intent/disconnect/cancellation inputs that occurred - without requiring any deleted row to still exist.

No archival code or table exists anywhere in `game/session/internal/storage/migrations/` today.

## Context

**This WORK's original direction (GCS object storage, `session_history_archives`, a checksum/provider/storage-key metadata shape) is superseded.** GAME-ADR-0009 accepted that direction when full per-Turn Snapshot persistence made runtime history's volume a real concern worth moving to a separate storage system. GAME-ADR-0024 removes that volume driver at its source (no more full-Snapshot persistence - see WORK-0019) and, having done so, no longer wants the GCS integration surface (object-storage client, checksum verification, provider/key metadata) for Session Runtime V1's actual scale. This WORK's Outcome/Scope below describe the new PostgreSQL-resident direction; the original GCS-direction text is not preserved inline in this file (unlike a DONE WORK, this WORK was never implemented under its original direction, so there is no implemented behavior to preserve as history here) - the original direction's rationale and its supersession are preserved in `game/docs/decisions/GAME-ADR-0009-session-runtime-history-archival-and-hard-delete.md` and `game/docs/decisions/GAME-ADR-0024-replay-first-session-runtime-persistence.md` respectively.

This WORK is lowest architectural risk among the Project's remaining WORK - archival only matters once real Sessions have produced history worth compacting - but is now more directly coupled to WORK-0019 than the original GCS direction was: the whole point of compacting into a self-sufficient archive record is that the archive can stand in for the deleted normalized rows, which requires WORK-0019's replay-input model to already define what "self-sufficient" means for a given Session (which causes are durably captured, in what shape). This WORK does not require WORK-0019 to be fully DONE before its own DRAFT design can start, but its Acceptance Criteria cannot be finalized independently of WORK-0019's own resolved Blockers (see WORK-0019's Blockers 1-3).

## Scope

### In Scope

- `session_archives`-equivalent persistence (exact naming is this WORK's/WORK-0019's implementation-planning detail): `id`, `session_id` (unique), `format_version`, `payload` (`JSONB`), `archived_at`, `created_at`, `updated_at` - no `storage_provider`/`storage_key`/`checksum`, since the payload lives in this row, not an external object.
- The compaction transaction: build the archive payload from the still-normalized rows, insert/mark the archive row, delete the removable source rows, commit - one transaction, idempotent under retry, no source-row deletion before the archive row's own insert commits in the same transaction.
- Deciding the exact archive JSON payload schema (pinned Game semantic/version identity, Start's Seed/RootParameters, ordered replay-input history, interactions/responses, terminal metadata, and any other replay-critical metadata WORK-0019's model defines) - self-sufficient for the intended historical replay/inspection contract without requiring any deleted row.
- Auditing whether any lightweight relational metadata should remain queryable without opening the archive (Session identity, host, participants, game definition identity, terminal reason/timestamps, lightweight failure metadata) - per the already-accepted exclusion of `sessions`/`session_actors`/`session_participants`/`session_runtime_failures` from deletion.

### Out of Scope

- Building an actual replay/rewatch feature - only archival/compaction itself; replay-capability-in-principle is WORK-0019's own concern, this WORK only compacts what WORK-0019's model already makes replayable.
- Any GCS/object-storage integration, checksum verification, or provider/key metadata - explicitly superseded, not merely deprioritized.
- `session_runtime_steps`'s fate as a live/operational table - decided by WORK-0019, not this WORK; this WORK only compacts/removes whatever WORK-0019 leaves as a live table, if any.
- Any change to live/operational read paths for a still-`RUNNING` Session - this WORK only ever touches `TERMINAL` Sessions.

## Approved Design

Not yet fully designed - the transactional compaction shape and the general PostgreSQL-JSONB direction are settled by GAME-ADR-0024; the exact archive JSON schema, exact `session_archives` table shape, and exact set of removable rows (contingent on WORK-0019's `session_runtime_steps` decision) remain for this WORK's own DRAFT phase.

## Constraints and Invariants

- No hard delete of a removable row occurs unless the same transaction that deletes it also durably persists the archive row it is being compacted into - both happen together, in one transaction, or neither happens (this replaces the original GCS design's "verify checksum before delete" step with a stronger, same-transaction guarantee that removes the need for a separate verification step at all).
- `sessions`, `session_actors`, `session_participants`, and `session_runtime_failures` are never deleted by this WORK's compaction, consistent with the already-accepted exclusion (unchanged from the original GAME-ADR-0009 policy, restated by GAME-ADR-0024).
- The archive payload never includes a derived Snapshot, Presentation, or Effect - only durable replay-input/terminal-metadata content, consistent with WORK-0019's own model.
- Repeated/retried archival must be idempotent (safe to invoke again against an already-archived Session without error or duplicate archive rows).

## Acceptance Criteria

Not yet fully defined - finalized once WORK-0019's own Blockers 1-3 resolve (they determine exactly what content the archive payload must include). The following are already clear from GAME-ADR-0024 and expected to survive into the READY version:

- A `TERMINAL` Session's compaction produces exactly one `session_archives` row and removes the designated normalized rows, verified by a test that compacts a Session with at least one answered interaction and one Turn, then confirms the archive payload alone (with the normalized rows actually deleted) is sufficient to reconstruct that Session's final state via WORK-0019's replay-reconstruction capability.
- A failed/interrupted compaction attempt (simulated transaction rollback) leaves every normalized row intact and no partial/incomplete archive row.
- Retrying compaction against an already-archived Session is a no-op (or an explicitly defined idempotent outcome), not a duplicate archive row or an error.
- `sessions`/`session_actors`/`session_participants`/`session_runtime_failures` remain queryable and unmodified by compaction.
- `go build ./...`, `go vet ./...`, `go test ./... -count=1` pass, with no new failure beyond the already-recorded out-of-scope `getgame` JSONB-comparison defect.

## Blockers

- Depends on WORK-0019's own Blockers 1-3 (Seed/RootParameters shape, `session_runtime_steps` disposition, per-cause durable representation) resolving enough to define a stable archive payload schema - this WORK's own DRAFT design should track WORK-0019's progress rather than design the payload schema independently and risk mismatch.
- Exact archive JSON schema and `session_archives` table shape are this WORK's own open design questions for DRAFT, not resolved here.

## Documentation Impact

- `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` - already revised by this reconciliation pass ("Archive Metadata"/"Archival And Hard-Delete Policy" sections); this WORK finalizes the exact schema once implemented.
- `game/session/README.md`, `game/README.md` - update the archival direction reference once implemented (both currently describe, or are silent pending, the now-superseded GCS direction).
- `game/docs/DATA_MODEL.md` - add `session_archives` once implemented.

## Rewrite Record (Part L Reconciliation, 2026-09-20)

This WORK was originally created (2026-09-20, same day) describing GCS object-storage archival with `session_history_archives` (status/storage_provider/storage_key/checksum). A broader reconciliation session, in the same overall pass, accepted GAME-ADR-0024 (Replay-First Session Runtime Persistence), which explicitly supersedes that direction: PostgreSQL remains the archive's home, not GCS, once full-Snapshot persistence (the original volume driver) is removed at its source. This WORK was never implemented under its original direction (it was PLANNED, not DRAFT/READY/IMPLEMENTING), so this is a rewrite of an unimplemented WORK's design, not a rewrite of completed implementation history - `docs/work/README.md`'s "Completed Work Immutability" section does not apply, since this WORK never reached DONE or CANCELLED. No production code, migration, or GCS integration was ever implemented against the original direction, and none is implemented against this rewritten direction by this reconciliation pass either. This WORK's Status remains PLANNED.

## Completion Record

Not started. PLANNED.
