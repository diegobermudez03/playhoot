# Game Management - Current State

Status: CURRENT IMPLEMENTATION

Session Runtime's own current-state doc is `session/CURRENT_STATE.md` - Game Management and Session Runtime are independent top-level packages (`docs/decisions/architecture/ADR-0014-management-session-domain-split.md`). Game Management currently has no cross-domain caller: Orchestrator's former `CreateSession` visibility gate was removed with the previous Session Runtime implementation, and `game/usecases/checkvisibility` is kept, tested, and unwired.

## Capability Status

| Area | Status | Notes |
| --- | --- | --- |
| Game Management | PARTIAL | Persisted authored game/image/history schema exists (see `game/docs/DATA_MODEL.md`); checking whether a Game is currently playable/visible by UUID is implemented and tested (`game/usecases/checkvisibility`). |
| Game lifecycle | PARTIAL | Game creation project (2026-11-01). `game/workflows/gamelifecycle.Manager.CreateGame(ownerUUID, idempotencyKey)`: one transaction creates an unpublished private Game with an empty card and its first edition `EDITING`, whose uuid is the Play version it is built in; idempotent per (owner, key), concurrent retries create one Game; metric `game_create_total{outcome}`. `game/usecases/findeditableversion.Service.FindEditableVersion(gameUUID, userUUID)`: the open edition's version when the user owns the Game, else `not_found` (also for someone else's Game) or `not_editable`; it authorizes edits and, for now, reads of the version being built (owner only). Publishing and opening a new edition of a published Game are not implemented yet. Not wired to the API or the orchestrator yet. Unit, repository and integration tests (Postgres). |

## Placeholders (wired, but not real behavior)

- `game/usecases/checkplayability` (`IsPlayable`) is called by the play-lifecycle orchestration when a Session is created, but it ignores its arguments and always answers `true`. Real behavior (the Game exists, has a published version, and is visible to the user) is not implemented and is outside the current Session Runtime project.

## Current Gaps

- Publishing a Game, opening a new edition of a published one, and editing the card are not implemented yet (game creation project).

## Known Drift

- None currently identified.

## Evidence

- Game Management storage and migrations: `game/internal/storage/`.
- Game visibility check: `game/usecases/checkvisibility/`.
- Game lifecycle: `game/workflows/gamelifecycle/`, `game/usecases/findeditableversion/`, migration `game/internal/storage/migrations/20261101000000_game_editions.go`.
