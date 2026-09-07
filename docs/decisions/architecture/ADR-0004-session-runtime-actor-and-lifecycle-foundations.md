# ADR-0004: Session Runtime Actor and Lifecycle Foundations

Status: ACCEPTED
Created: 2026-09-06
Last status change: 2026-09-06
Supersedes: None
Superseded by: None

## Context

Session Runtime needs enough accepted model foundation to design the lobby lifecycle and later runtime operations. The current implementation has `sessions.owner_uuid` and `session_players.player_uuid`, but current schema is not implementation authority for the new design. Game also explicitly does not own Identity/Profile.

The design discussion needed to avoid three premature commitments: making the session creator automatically a gameplay participant, introducing a cross-session Player/Profile entity inside Session Runtime, and freezing Identity-domain terminology before Identity itself has been designed.

## Decision

Host and Participant are independent concepts. Creating a Session establishes a host but does not automatically make that host a gameplay participant. The same Session-owned actor may be both host and Participant, but those are separate relationships.

Do not introduce separate mutable `creator` and `host` concepts for V1 unless an already-accepted repository decision requires them. Host transfer is not currently designed.

Participant is session-scoped. Do not introduce a cross-session Player/Profile master entity inside Session Runtime. Participant belongs to one Session and contains Session-owned participation/lifecycle state plus a display-name snapshot.

Do not add a generic persisted `role` field in V1 merely to anticipate future spectators, judges, controllers, or gameplay roles. Gameplay roles belong primarily to game definition/runtime state unless Session Runtime later has a concrete cross-game reason to own such a distinction.

Session-local uniqueness is the accepted invariant: an actor may occupy at most one active logical participation for the same Session. Do not establish a platform-wide invariant that one User may belong to only one active Session.

Introduce conceptually a Session-owned, session-scoped identity such as `SessionActorID`. Host and Participant relationships refer to this local identity. Runtime/domain operations should primarily operate using Session-owned identity rather than propagating Identity-domain UUIDs throughout the engine/runtime.

A SessionActor may persist a cross-domain reference to the stable public identity entity exported by the future Identity domain. This is a logical domain reference, not ownership and not permission to read Identity persistence. The exact public Identity entity name/semantics are not decided here; Domain Design must resolve whether the public concept is `User`, `Principal`, or another accepted term.

Do not prematurely freeze `user_uuid` into Session canonical domain documentation while the exported Identity entity itself has not yet been accepted through Domain Design.

Use the conceptual Session lifecycle:

```text
LOBBY -> RUNNING -> TERMINAL
```

`TERMINAL` is irreversible. Terminal cause/reason is modeled separately from lifecycle phase instead of expanding every terminal cause into a top-level state. This ADR does not invent the exhaustive terminal-reason enum.

The current expiration concern is specifically `lobby_expires_at`, not a generic whole-session lifetime. Join/Start correctness must enforce this durable deadline even if no asynchronous cleanup/sweeper has materialized the Session as terminal yet. A later max-runtime/runaway-session policy is a different concern.

Connection state is not Participant state. Logical participation is durable Session Runtime state. Physical connection presence belongs to the Live Session Coordinator and must not be persisted as authoritative Participant membership through fields such as `is_connected`.

Disconnect/reconnect semantics remain deferred.

## Rationale

The model needs stable local identities for game/runtime operations without pulling an eventual Identity domain into every Session Runtime decision. A session-local actor identity gives Session Runtime something it owns and can use consistently for host and participant relationships, while still allowing correlation to a public Identity entity later.

Keeping Host separate from Participant avoids assuming all session creators play. Keeping Participant session-scoped avoids creating an accidental Profile or Player master record inside Game. Avoiding a generic role field prevents speculative schema/design around future interaction modes before there is a concrete cross-game Session Runtime responsibility.

The compact lifecycle keeps top-level state understandable while preserving room for terminal reasons. Treating lobby expiration as a durable deadline, rather than only a sweeper effect, keeps Join/Start behavior correct even when cleanup is delayed.

## Alternatives Considered

### Creator is always a participant

Rejected. Some sessions may be created or hosted by an actor who is not part of gameplay. The model should not force that product behavior without an accepted requirement.

### Add separate creator and mutable host in V1

Rejected for now. Host transfer and creator semantics are not designed, and V1 does not need both concepts unless another accepted decision requires them.

### Cross-session Player/Profile entity inside Session Runtime

Rejected. Identity/Profile ownership is outside Game. Session Runtime needs session participation state, not a profile master record.

### Generic Participant role field in V1

Rejected. It anticipates future roles before a concrete Session Runtime-owned reason exists. Authored game roles should primarily live in game definition/runtime state.

### Platform-wide one-active-session-per-user invariant

Rejected. The accepted uniqueness rule is session-local only. Platform-wide concurrency limits would be a product/domain policy decision not made here.

### Persist connection state as participant membership

Rejected. Physical connection presence is ephemeral Coordinator state. Logical participation is durable Session Runtime state.

## Consequences

- Future Session Runtime design should use a Session-owned local actor identity for host and participant relationships.
- Session Runtime may persist a cross-domain reference to a future public Identity entity, but Identity-domain naming and semantics remain unresolved.
- Lobby operation design must enforce session-local participation uniqueness.
- Detailed terminal reasons, host transfer, reconnect semantics, and max-runtime/runaway-session policy remain future design topics.
- Existing schema names such as `owner_uuid` and `player_uuid` are current implementation facts, not accepted future model names.

## Canonical Knowledge Impact

- `game/README.md` - adds accepted Session Runtime actor, participant, lifecycle, lobby expiration, and connection-state boundaries.

## Implementation Impact

Future implementation work will need to align Session Runtime types, persistence, and operations with the accepted actor/lifecycle model. No implementation, migration, schema change, or WORK is authorized by this record.
