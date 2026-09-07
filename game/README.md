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

Session Runtime conceptually owns a session-scoped identity such as `SessionActorID`. Host and Participant relationships refer to this local identity. Runtime/domain operations should primarily operate using Session-owned identity rather than propagating `UserUUID` throughout engine/runtime behavior.

A SessionActor may persist `user_uuid` as a cross-domain reference to `Identity.User`. This is a logical domain reference, not ownership of User/Profile, not permission to read Identity persistence, not dependency on Identity table layout, and not a cross-domain database foreign key.

Session Runtime owns its participation-time display-name snapshot associated with SessionActor/Participant representation. Mutable global display/profile ownership remains unresolved outside Game.

User-originated operations crossing the public/application boundary of Session Runtime identify the caller through authenticated `UserUUID`. The external client is not the trusted source of `UserUUID`; a trusted authentication/application layer resolves credentials into an authenticated `UserUUID`.

Session Runtime owns the translation from `(Session UUID, UserUUID)` to `SessionActorID`. `SessionActorID` is internal Session-owned identity, not part of Session Runtime's public/application-facing contract. Coordinator, Orchestrator, HTTP/WebSocket adapters, and clients do not own or maintain the mapping from `UserUUID` to `SessionActorID`.

Game Language / engine user identity represents the Session-local actor and must not receive `Identity.UserUUID`. When engine/runtime behavior targets a Session-local actor, Session Runtime resolves that internal actor back to `SessionActor.user_uuid` / `UserUUID` before crossing the Session Runtime boundary. Identity/Auth proves who the caller is; Session Runtime decides whether that User may act as host, active Participant, or another Session-owned relationship in the current lifecycle/runtime state.

Use the conceptual Session lifecycle:

```text
LOBBY -> RUNNING -> TERMINAL
```

`TERMINAL` is irreversible. Terminal cause/reason is modeled separately from lifecycle phase; the exhaustive terminal-reason enum is not yet accepted.

The current expiration concern is specifically `lobby_expires_at`, not a generic whole-session lifetime. Join/Start correctness must enforce this durable deadline even if no asynchronous cleanup/sweeper has materialized the Session as terminal yet. A later max-runtime/runaway-session policy is a different concern.

Connection state is not Participant state. Logical participation is durable Session Runtime state. Physical connection presence belongs to the Live Session Coordinator and must not be persisted as authoritative Participant membership through fields such as `is_connected`.

Disconnect/reconnect semantics remain deferred. Physical connection state is not logical session participation state. Future design must decide reconnect conditions, grace periods, inactive/forfeited/removed semantics, whether play waits or continues, what is generic Session Runtime policy versus authored Game Language behavior, and how reconnect is represented without making Session Runtime own TCP/WebSocket connections.

Rationale and alternatives are recorded in `docs/decisions/architecture/ADR-0004-session-runtime-actor-and-lifecycle-foundations.md`.

The public/internal identity boundary is refined by `docs/decisions/architecture/ADR-0008-session-public-and-internal-identity-boundary.md`.

## Session Runtime Lobby Lifecycle Contract

All mutations that can alter lobby membership or cross the lobby/start boundary for one Session must serialize against that Session. This includes Join, Leave, Start, and materialization of lobby expiration when performed by one of these operations or housekeeping. The accepted architecture is the per-Session serialization guarantee, not one specific SQL syntax. This rule applies to the LOBBY lifecycle only; high-frequency RUNNING runtime concurrency remains a later architecture topic.

CreateSession/CreateRoom receives conceptually a Game public UUID, authenticated host `UserUUID`, and opaque idempotency key. Session Runtime resolves the currently playable immutable Game definition/version through the narrow Game Management read capability, pins the Session to that immutable definition/version, and does not accept an externally-precompiled `engine.Program`.

Within one Session-owned transaction, Create creates a Session in `LOBBY`, a host SessionActor correlated through `user_uuid`, the Session host relationship pointing to that SessionActor, an active JoinCode, and `lobby_expires_at`. Creating the Session does not automatically make the host a gameplay Participant. `lobby_expires_at` is V1 Session Runtime policy unless another accepted product decision says otherwise. Public responses do not expose `SessionActorID`; QR codes and join URLs are projection/application concerns derived from JoinCode.

JoinCode is authoritative Session Runtime state, not cache-owned truth. V1 has one active JoinCode for a lobby unless a future accepted feature requires rotation/multiple codes. A JoinCode is valid only while its Session remains admissible for joining, is revoked when the Session exits `LOBBY`, and may retain historical records for traceability. A textual code value need not be permanently reserved forever after revocation.

Join receives conceptually a JoinCode, authenticated `UserUUID`, display-name snapshot supplied by the trusted application/integration layer, and opaque idempotency key. Session Runtime resolves the JoinCode to its Session, obtains the pinned Game definition needed for lobby constraints, enters the Session-serialized mutation, and revalidates JoinCode admissibility, `LOBBY` phase, and `lobby_expires_at`.

Join finds or creates the SessionActor for `(Session, UserUUID)`. If that actor is already an active Participant, repeated Join is the same logical admission rather than consuming another slot. Otherwise Join counts active Participants, enforces `players.max` from the pinned Game definition, activates Participant membership, and commits atomically. The Game definition remains the source of truth for authored `players.min/max`; Session does not copy those values as a second correctness source.

Leave is a `LOBBY` participation operation. Public/application caller identity is `UserUUID`; Session Runtime resolves `(Session, UserUUID)` to its internal SessionActor. Leaving deactivates logical Participant membership, releases a lobby slot, does not delete the SessionActor, does not remove host authority, and does not automatically cancel the Session. A host who is also participating may leave gameplay participation while remaining host. Rejoining the same admissible lobby reuses the same SessionActor.

Join/Start correctness does not depend on a sweeper having already changed stored phase. If an operation discovers `phase == LOBBY && now >= lobby_expires_at`, it may atomically materialize `TERMINAL`, a terminal reason corresponding to lobby expiration, a terminal timestamp equal to the lobby deadline, and JoinCode revocation. A background sweeper is housekeeping/materialization support, not the source of business correctness. The exhaustive terminal-reason enum is not yet accepted.

Start receives conceptually a Session UUID, authenticated `UserUUID`, and opaque idempotency key. Session Runtime resolves `(Session UUID, UserUUID)` to `SessionActorID` and uses that internal identity to verify host authority. Start uses the Session's pinned immutable Game definition/version, not the latest current Game version; validates `LOBBY`, `lobby_expires_at`, host authority, active Participants, `players.min`, and defensively `players.max`; initializes engine/runtime state; processes the engine's required initial signal/first Commit; persists initial authoritative runtime state and durable consequences; transitions Session atomically to `RUNNING`; sets `started_at`; revokes JoinCode; and commits before any Coordinator/client-visible outputs are delivered.

If initialization or first authoritative execution fails, the transaction must not partially expose a `RUNNING` Session. No Coordinator/client-visible output is delivered before successful durable commit.

Mutating external Session commands in this lobby lifecycle accept an opaque idempotency key, including Create, Join, Leave, and Start. Session Runtime must guarantee that retrying the same logical command does not repeat its effect. Natural/domain idempotency is useful additional protection but is not a substitute for the explicit idempotency contract. The final idempotency storage algorithm is not yet accepted.

Start uses the accepted Game Language root roster contract `players: list<user>`, where each `user` is the Session-local runtime identity derived from `SessionActorID`; `Identity.UserUUID` is never exposed to authored Game Language.

Rationale and alternatives are recorded in `docs/decisions/architecture/ADR-0007-session-lobby-lifecycle-contract.md`, `docs/decisions/architecture/ADR-0008-session-public-and-internal-identity-boundary.md`, and `docs/decisions/architecture/ADR-0009-game-language-root-player-roster-contract.md`.

## Boundary Notes

Completed-session history/archive ownership remains unresolved and is not defined by this document.
