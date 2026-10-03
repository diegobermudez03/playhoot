# Game Management - Current State

Status: CURRENT IMPLEMENTATION

Session Runtime's own current-state doc is `session/CURRENT_STATE.md` - Game Management and Session Runtime are independent top-level packages (`docs/decisions/architecture/ADR-0014-management-session-domain-split.md`). Game Management currently has no cross-domain caller: Orchestrator's former `CreateSession` visibility gate was removed with the previous Session Runtime implementation, and `game/usecases/checkvisibility` is kept, tested, and unwired.

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
