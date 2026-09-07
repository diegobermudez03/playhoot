# Game

Status: CANONICAL DOMAIN MODEL

## Responsibility

Game owns the authored lifecycle and runtime execution of Playhoot games.

## Owns

- Authored games and their definitions/versions.
- Game metadata and visibility/publication state.
- Live game sessions.
- Session runtime state and participation.
- Execution lifecycle of a game session.

## Does Not Own

- Transport/network connections.
- Identity/profile ownership.
- Public discovery/search experience.

## Internal Structure

### Business Capabilities

- Game Management: owns authored game lifecycle concerns.
- Session Runtime: owns execution/session lifecycle concerns.

### Supporting Subsystems

- Game Language: defines, compiles, and executes game behavior for Game; it is not a business domain.

## Capability Persistence and Transaction Boundary

Game Management and Session Runtime share one business domain but not one persistence or transaction boundary.

- Game Management owns its persisted state: authored games, definitions/versions, publication/visibility state, images, and related authored-game history.
- Session Runtime owns its persisted state: sessions, session state, participants, join codes, and other live-runtime persistence.
- Both may currently share the same physical PostgreSQL database. Physical co-location does not authorize either capability to mutate the other's tables, and does not require separate schemas/namespaces to simulate future separation.
- Their migrations and persistence implementations remain independently owned.
- No database transaction spans both Game Management-owned and Session Runtime-owned state, and a transaction handle must not be propagated from one capability into the other.

Session Runtime may require Game Management information — in particular, the game definition/version that defines runtime behavior — but depends on a narrow Game Management definition/read capability contract rather than on Game Management's repository as its architectural API. Concrete infrastructure may be reused underneath that contract, but persistence-implementation sharing must not erase capability ownership.

A session must be pinned to a concrete execution definition/version, and the definition observed by an existing session must remain semantically stable for that session's lifetime; version immutability for existing sessions is the preferred model. Changing a game's current definition affects only newly created sessions.

This boundary is Game-specific and preserves an inexpensive future path toward independently deploying Session Runtime; it is not a general rule that every pair of internal capabilities must have independent transaction boundaries. Rationale and alternatives are recorded in `docs/decisions/architecture/ADR-0002-game-capability-persistence-transaction-boundary.md`.

## Boundary Notes

Completed-session history/archive ownership remains unresolved and is not defined by this document.
