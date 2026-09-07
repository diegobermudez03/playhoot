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

## Session Runtime Durable Boundary

Session Runtime owns all durable authoritative state that determines the meaning of a session and must be reconstructible after loss or restart of the process.

"Stateless" for Session Runtime means process-stateless/reconstructible. It does not mean the capability has no durable domain state.

`Live Session Coordinator` is the accepted conceptual responsibility boundary outside Session Runtime for ephemeral/runtime mechanisms:

- live connection bindings;
- delivery/fan-out to connected clients;
- detection of physical disconnects;
- physical timer/scheduling mechanisms.

The Coordinator does not own authoritative session/game truth or business consequences. This boundary does not require another deployed service; V1 may keep the Coordinator in the same Go process.

V1 does not introduce sticky-session correctness requirements, Redis, distributed session routing, distributed locks, or other multi-instance mechanisms. The initial single-process modular-monolith deployment may use local maps, channels, and Go timers for ephemeral mechanisms, but session correctness must not depend exclusively on those ephemeral objects.

Timer obligations that can affect game semantics are durable Session Runtime state. The Coordinator owns the physical timer and detects elapsed wall-clock time; Session Runtime owns the durable timer obligation and decides the semantic consequence, normally through its authoritative runtime/engine flow. A process crash may destroy physical timers but must not silently erase timer obligations or change game semantics. Pending or overdue obligations must be reconstructible/recoverable from durable state.

Rationale and alternatives are recorded in `docs/decisions/architecture/ADR-0003-session-runtime-durable-boundary.md`.

## Session Runtime Actor and Lifecycle Model

Host and Participant are independent concepts. Creating a Session establishes a host but does not automatically make that host a gameplay participant. The same Session-owned actor may be both host and Participant, but those are separate relationships.

V1 does not introduce separate mutable `creator` and `host` concepts unless another accepted decision requires them. Host transfer is not currently designed.

Participant is session-scoped. Session Runtime does not own a cross-session Player/Profile master entity. A Participant belongs to one Session and contains Session-owned participation/lifecycle state plus a display-name snapshot.

V1 does not include a generic persisted `role` field merely to anticipate spectators, judges, controllers, or gameplay roles. Gameplay roles belong primarily to game definition/runtime state unless Session Runtime later has a concrete cross-game reason to own such a distinction.

Session-local uniqueness is the accepted invariant: an actor may occupy at most one active logical participation for the same Session. Game does not establish a platform-wide invariant that one User may belong to only one active Session.

Session Runtime conceptually owns a session-scoped identity such as `SessionActorID`. Host and Participant relationships refer to this local identity. Runtime/domain operations should primarily operate using Session-owned identity rather than propagating external Identity-domain UUIDs throughout engine/runtime behavior.

A SessionActor may persist a cross-domain reference to the stable public identity entity exported by the future Identity domain. This is a logical domain reference, not ownership and not permission to read Identity persistence. The exact public Identity entity name/semantics are unresolved; Domain Design must decide whether the public concept is `User`, `Principal`, or another accepted term before Session canonical documentation freezes a persisted field name such as `user_uuid`.

Use the conceptual Session lifecycle:

```text
LOBBY -> RUNNING -> TERMINAL
```

`TERMINAL` is irreversible. Terminal cause/reason is modeled separately from lifecycle phase; the exhaustive terminal-reason enum is not yet accepted.

The current expiration concern is specifically `lobby_expires_at`, not a generic whole-session lifetime. Join/Start correctness must enforce this durable deadline even if no asynchronous cleanup/sweeper has materialized the Session as terminal yet. A later max-runtime/runaway-session policy is a different concern.

Connection state is not Participant state. Logical participation is durable Session Runtime state. Physical connection presence belongs to the Live Session Coordinator and must not be persisted as authoritative Participant membership through fields such as `is_connected`.

Disconnect/reconnect semantics remain deferred. Physical connection state is not logical session participation state. Future design must decide reconnect conditions, grace periods, inactive/forfeited/removed semantics, whether play waits or continues, what is generic Session Runtime policy versus authored Game Language behavior, and how reconnect is represented without making Session Runtime own TCP/WebSocket connections.

Rationale and alternatives are recorded in `docs/decisions/architecture/ADR-0004-session-runtime-actor-and-lifecycle-foundations.md`.

## Boundary Notes

Completed-session history/archive ownership remains unresolved and is not defined by this document.
