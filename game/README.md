# Game Management

Status: CANONICAL DOMAIN MODEL

## Responsibility

Game Management owns the authored lifecycle of Playhoot games.

## Owns

- Authored games and their definitions/versions.
- Game metadata and visibility/publication state.

## Does Not Own

- Transport/network connections.
- Identity/profile ownership.
- Public discovery/search experience.
- Live game sessions, session runtime state, or the execution lifecycle of a game session (owned by Session Runtime, `session/README.md`).
- Game Language / any script execution engine - Game Management does not import `program`/`gameservice`/`engine` for any call site Session Runtime's `Create` still reaches (`docs/projects/active/js-runtime-migration/works/WORK-0034-session-owned-executable-script-artifact-and-package-restructuring.md`); it still does for the six other Session Runtime operations that continue to read its pinned-definition capability, unchanged by that WORK.

## Internal Structure

Game Management is an independent top-level package/business bounded context, a sibling to Session Runtime (`session/`), `identity/`, `composer/`, and `orchestrator/` - not a capability nested under a shared "Game" bounded context (`docs/decisions/architecture/ADR-0014-management-session-domain-split.md`). Its own package/directory is `game/` (`package game`), the name the old shared "Game" bounded context previously used - freed for reuse by Game Management alone once that bounded context was dissolved into Game Management and Session Runtime as independent domains. `game/language/v1/...` (Game Language) also currently lives inside this same `game/` directory tree, but that placement does not mean Game Management owns it: Game Language remains Session Runtime's own execution dependency (`ADR-0015`), parked here only because it has not yet been physically relocated into Session Runtime's own package tree, and both domains still depend on it in the meantime. `game/docs/` similarly holds specialized/historical documentation (decision records, the Session Runtime persistence model, the Game Version Artifact model) that predates this rename and has not been relocated.

Session Runtime owns its own persisted copy of the executable Game Version Artifact it runs, keyed by the same immutable definition/version identity Game Management's own `game_definitions` table uses (`session/README.md`'s own Capability Persistence and Transaction Boundary section). No database transaction spans Game Management-owned and Session-Runtime-owned tables, and no cross-domain database foreign key exists between them (`ARCHITECTURE.md -> State and Transaction Boundaries`, `-> Cross-Domain Public Entity References`).

Rationale and alternatives for the domain-split decision are recorded in `docs/decisions/architecture/ADR-0014-management-session-domain-split.md`.
