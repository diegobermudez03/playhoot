# WORK-0017: Archival

Status: PLANNED
Created: 2026-09-20
Last status change: 2026-09-20

Related decisions:
- GAME-ADR-0009 (archival direction accepted; exact schema/storage implementation deferred)

Canonical context:
- `docs/projects/active/session-runtime-v1/PROJECT.md`
- `game/session/README.md` ("Completed-session history/archive ownership remains unresolved and is not defined here")
- `game/README.md` (deferred direction: `session_history_archives`, GCS artifact)
- `docs/projects/active/session-runtime-v1/works/WORK-0006-broaden-live-fanout-effects-presentations.md` (Start's persisted `Seed` - the one addition that WORK made specifically to keep future replay/archival theoretically possible)

## Outcome

Historical runtime material for a completed Session moves to durable archival/object storage, and operational storage is cleaned up afterward according to the accepted architecture, once a Session has actually produced history worth archiving/deleting.

No archival code or table exists anywhere in `game/session/internal/storage/migrations/` today.

## Context

Lowest architectural risk, purely additive, and not required for an initial end-to-end playable Session - archival only matters once real Sessions have produced history worth archiving. Any replay/versioning/random-seed concern that is not needed for live execution belongs here rather than being pulled into Presentation (WORK-0006 deliberately avoided this).

## Scope

### In Scope (known required outcome; design not yet started)

- `session_history_archives` persistence.
- JSON archive serialization to object storage (for example GCS).
- Verified hard-delete of heavy runtime-history tables after successful archival.

### Out of Scope

- Building an actual replay/rewatch feature - only archival/cleanup itself; replay is a separate, later concern (WORK-0006 already closed the one persistence gap - Start's `Seed` - blocking a hypothetical future replay feature, without building one).
- Reducing existing full-Snapshot persistence in the live/operational path - tracked separately (`docs/product/IDEAS.md -> Minimize Persisted RuntimeTurn History`), not this WORK's concern.

## Approved Design

Not yet designed. Exact archive JSON schema and storage backend/implementation details remain deferred per GAME-ADR-0009.

## Constraints and Invariants

- Hard-delete of operational storage must only occur after archival is verified successful.

## Acceptance Criteria

Not yet defined - to be written when this WORK moves to DRAFT.

## Blockers

- None yet beyond the deferred schema/storage-backend design questions above.

## Documentation Impact

Not yet assessed in detail; expected to touch `game/session/README.md`, `game/README.md`, `game/docs/DATA_MODEL.md` once designed.

## Completion Record

Not started. PLANNED.
