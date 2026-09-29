# Game Management - Current State

Status: CURRENT IMPLEMENTATION

Session Runtime's own current-state doc is `session/CURRENT_STATE.md` - Game Management and Session Runtime are independent top-level packages (`docs/decisions/architecture/ADR-0014-management-session-domain-split.md`). Session Runtime depends on Game Management for nothing directly: `Create` resolves entirely from its own tables (`WORK-0034`), and its five other pinned-artifact-reading operations do too. Game Management's only current cross-domain caller is Orchestrator's `CreateSession` visibility gate (`docs/projects/active/js-runtime-migration/works/WORK-0032-composer-mediated-session-creation-visibility-check.md`).

## Capability Status

| Area | Status | Notes |
| --- | --- | --- |
| Game Management | PARTIAL | Persisted authored game/image/history schema exists (no version/definition concept - see `game/docs/DATA_MODEL.md`); checking whether a Game is currently playable/visible by UUID is implemented and tested (`game/usecases/checkvisibility`). Create/edit/publish lifecycle operations were not found in the inspected code. |

## Current Gaps

- Game Management lifecycle operations (create/edit/publish a Game) were not found implemented anywhere in this repository.

## Known Drift

- None currently identified.

## Evidence

- Game Management storage and migrations: `game/internal/storage/`.
- Game visibility check: `game/usecases/checkvisibility/`.
