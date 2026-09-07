# Minimum Identity Concept For Session Runtime

Process: Domain Design

Parent process: Session Runtime Architecture Discussion

Status: next checkpoint pending.

## Why This Is The Next Question

Session Runtime now has accepted architecture foundations for durable state, the Live Session Coordinator boundary, timer durability, V1 scaling, SessionActor identity, Participant scoping, and lobby lifecycle foundations.

The remaining blocker before detailed Session Runtime operation design is the minimum Identity-domain concept that a SessionActor may reference. Session Runtime must not freeze a persisted field such as `user_uuid` until the producing Identity domain has accepted what public entity it exports.

Keep this discussion deliberately small: define only what Session Runtime needs in order to correlate a session-local actor to an external public identity.

## Accepted Architecture Context

- Session Runtime owns durable authoritative session/runtime state.
- `Live Session Coordinator` owns ephemeral connection, delivery/fan-out, disconnect detection, and physical scheduling mechanisms.
- Host and Participant are independent relationships to a Session-owned actor.
- Participant is session-scoped and is not a cross-session Player/Profile entity.
- Session Runtime conceptually owns a local `SessionActorID`.
- A SessionActor may persist a cross-domain reference to a stable public identity entity exported by the future Identity domain.
- Cross-domain public entity references use stable public UUIDs that identify logical entities, not storage rows.
- Consumers of public UUID references must not create cross-domain database foreign keys or depend on producer table layout.

Canonical references:

- `game/README.md`
- `ARCHITECTURE.md`
- `docs/decisions/architecture/ADR-0003-session-runtime-durable-boundary.md`
- `docs/decisions/architecture/ADR-0004-session-runtime-actor-and-lifecycle-foundations.md`
- `docs/decisions/architecture/ADR-0005-cross-domain-public-entity-references.md`
- `docs/engineering/standards/cross-domain-reference-naming.md`

## Boundary Question

Define the minimum public Identity-domain concept needed by Session Runtime.

The next discussion should determine:

1. What is the stable public Identity entity: `User`, `Principal`, or another concept?
2. What business thing does that entity represent?
3. Does the same public identity exist for guests and registered people?
4. If a guest later registers, does the same public UUID survive that transition?
5. Which state belongs to that public entity versus Account/Profile/Auth details?
6. What public capabilities are necessary to create/resolve it?
7. What does Identity explicitly not own?
8. Is `display_name` Identity-owned mutable profile data while Session keeps its own participation-time snapshot?

## Current Direction To Preserve

Session Runtime should continue to own:

- session-local `SessionActorID`;
- Host and Participant relationships to that local actor;
- session-owned participation/lifecycle state;
- display-name snapshot captured for session participation;
- durable session/runtime state and semantic consequences.

Identity, if accepted, should own only the public identity concept and capabilities needed for correlation/resolution. The discussion should not assume the public concept is `User` until Domain Design accepts that name and meaning.

## Out Of Scope

- Login providers.
- OAuth/social sign-in.
- Profile pages.
- Account settings.
- Full authentication architecture.
- Identity persistence schema.
- Session Runtime implementation.
- Database migrations.
- WORK creation.
- Final disconnect/reconnect semantics.

## Deferred Design Topic

Disconnect/reconnect semantics remain deferred and are not an accepted feature design.

Physical connection state is not logical session participation state.

Future design must decide reconnect conditions, grace periods, inactive/forfeited/removed semantics, whether games wait or continue, what is generic Session Runtime policy versus authored Game Language behavior, and how reconnect is represented without making Session Runtime own TCP/WebSocket connections.

Current human preference/intuition, not an accepted design: some reconnect/inactivity semantics may need to be expressible by the game itself because different games can require different behavior.

## Explicit Non-Implementation Note

No implementation exists for these newly accepted architecture decisions yet. Current Session Runtime remains scaffolding, and `CreateRoom`/`JoinRoom` are still stubs.
