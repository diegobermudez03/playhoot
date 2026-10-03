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
- Any script execution runtime - Game Management imports none (`session/README.md`).

## Internal Structure

Game Management is an independent top-level package/business bounded context, a sibling to Session Runtime (`session/`), `identity/`, `composer/`, and `orchestrator/` - not a capability nested under a shared "Game" bounded context (`docs/decisions/architecture/ADR-0014-management-session-domain-split.md`). Its own package/directory is `game/` (`package game`), the name the old shared "Game" bounded context previously used - freed for reuse by Game Management alone once that bounded context was dissolved into Game Management and Session Runtime as independent domains. `game/docs/` holds this domain's decision records, data model and flows.

No database transaction spans Game Management-owned and Session-Runtime-owned tables, and no cross-domain database foreign key exists between them (`ARCHITECTURE.md -> State and Transaction Boundaries`, `-> Cross-Domain Public Entity References`).

Rationale and alternatives for the domain-split decision are recorded in `docs/decisions/architecture/ADR-0014-management-session-domain-split.md`.
