# Game Management - Current State

Status: CURRENT IMPLEMENTATION

Session Runtime's own current-state doc is `session/CURRENT_STATE.md` - Game Management and Session Runtime are independent top-level packages (`docs/decisions/architecture/ADR-0014-management-session-domain-split.md`). Game Management currently has no cross-domain caller: Orchestrator's former `CreateSession` visibility gate was removed with the previous Session Runtime implementation, and `game/usecases/checkvisibility` is kept, tested, and unwired.

## Capability Status

| Area | Status | Notes |
| --- | --- | --- |
| Game Management | PARTIAL | Persisted authored game/image/history schema exists (see `game/docs/DATA_MODEL.md`); checking whether a Game is currently playable/visible by UUID is implemented and tested (`game/usecases/checkvisibility`). |
| Card | IMPLEMENTED | `game/workflows/gamecard` (2026-11-02): `ReadCard`, `UpdateCard` (name ≤ 80, description ≤ 2000, `game_histories`), `UploadImage` (logo or photo through the backend: cut at 2/5 MiB, PNG/JPEG/WebP by bytes, intent row in `game_card_objects` before storing at `games/card-images/<sha256>`, idempotent per key in `game_card_image_requests`), `RemoveImage`; at most 8 photos. |
| Publication | IMPLEMENTED | `gamelifecycle` (2026-11-02): `BeginPublication` (owner, open edition, card rules, one release per key, one pending per game), `FindPendingPublication`, `CompletePublication` (edition PUBLISHED, `published_at` once, visibility, history), `AbandonPublication`, `OpenEdition` (published game, no open edition nor pending release). `checkplayability.IsPlayable` is real: published, not deleted, public or owner. Seed games are recorded published/public by `seed_20261103000000_game_play_seed_games`. |
| Game lifecycle | PARTIAL | Game creation project (2026-11-01). `game/workflows/gamelifecycle.Manager.CreateGame(ownerUUID, name, idempotencyKey)`: one transaction creates an unpublished private Game named `name` (the preliminary card name, required since 2026-11-02) and its first edition `EDITING`, whose uuid is the Play version it is built in; idempotent per (owner, key), concurrent retries create one Game; metric `game_create_total{outcome}`. `game/usecases/findeditableversion.Service.FindEditableVersion(gameUUID, userUUID)`: the open edition's version when the user owns the Game, else `not_found` (also for someone else's Game) or `not_editable`; it authorizes edits and, for now, reads of the version being built (owner only). Publishing and opening a new edition of a published Game are not implemented yet. Not wired to the API or the orchestrator yet. Unit, repository and integration tests (Postgres). |

## Placeholders (wired, but not real behavior)

- None.

## Current Gaps

- None in the game creation flow; catalog/discovery of public games is a later project.

## Known Drift

- None currently identified.

## Evidence

- Game Management storage and migrations: `game/internal/storage/`.
- Game visibility check: `game/usecases/checkvisibility/`.
- Game lifecycle: `game/workflows/gamelifecycle/`, `game/usecases/findeditableversion/`, migration `game/internal/storage/migrations/20261101000000_game_editions.go`.
