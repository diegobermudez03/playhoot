# Session Runtime Lobby Operations Checkpoint

Process: Architecture Discussion

Status: next checkpoint pending.

## Why This Is The Next Question

Session Runtime now has accepted foundations for durable authoritative state, the Live Session Coordinator boundary, V1 scaling, timer durability, SessionActor identity, Host/Participant separation, lifecycle shape, lobby expiration, and the minimum Identity public entity needed for cross-domain correlation.

The next architecture milestone is the detailed LOBBY lifecycle operation contract. Do not enter implementation planning until this contract is sufficiently accepted.

## Accepted Context

- Session Runtime owns durable authoritative session/runtime state.
- `Live Session Coordinator` owns ephemeral connection, delivery/fan-out, disconnect detection, and physical scheduling mechanisms.
- V1 remains single-process/modular-monolith and does not require Redis, distributed locks, distributed session routing, sticky-session correctness, or multi-instance mechanisms.
- Host and Participant are independent relationships to Session-owned actor identity.
- Session Runtime owns local `SessionActorID`.
- Identity owns stable `User` identity.
- `Identity.User` owns stable public `UserUUID`.
- A SessionActor persistently correlates to `Identity.User` through `user_uuid`.
- A guest is already a `User`; normal guest-to-registered conversion preserves the same `UserUUID`.
- Session Runtime owns its participation-time display-name snapshot.
- Mutable global display/profile ownership remains unresolved.
- Conceptual Session lifecycle is `LOBBY -> RUNNING -> TERMINAL`; `TERMINAL` is irreversible.
- Current expiration concern is `lobby_expires_at`, not a generic whole-session lifetime.
- Join/Start correctness must enforce `lobby_expires_at` even if asynchronous cleanup has not materialized terminal state.
- Connection state is not Participant state.
- Disconnect/reconnect semantics remain deferred.

Canonical references:

- `ARCHITECTURE.md`
- `game/README.md`
- `identity/README.md`
- `docs/decisions/architecture/ADR-0003-session-runtime-durable-boundary.md`
- `docs/decisions/architecture/ADR-0004-session-runtime-actor-and-lifecycle-foundations.md`
- `docs/decisions/architecture/ADR-0005-cross-domain-public-entity-references.md`
- `docs/decisions/architecture/ADR-0006-identity-user-public-identity-boundary.md`
- `docs/engineering/standards/cross-domain-reference-naming.md`

## Operation Contracts To Design

### 1. `CreateSession` / `CreateRoom`

Decide:

- inputs;
- host SessionActor creation;
- pinned Game definition/version;
- join-code creation;
- lobby expiration;
- transactional boundary;
- returned public/session-local identifiers.

### 2. `Join`

Decide:

- resolution of join code vs SessionID responsibilities;
- creation/reactivation of SessionActor/Participant;
- `UserUUID` + display-name snapshot;
- player-count constraints from the pinned Game definition;
- concurrency when multiple players take the last available slots;
- idempotency/repeated Join.

### 3. `Leave`

Decide:

- lobby-only semantics;
- participant lifecycle;
- rejoin behavior before Start;
- host behavior if relevant.

### 4. `Start`

Decide:

- host authority;
- lobby expiration;
- minimum/maximum player constraints;
- atomic transition to RUNNING;
- construction/persistence of the initial runtime Snapshot;
- initial engine outputs/timer obligations;
- concurrency with Join/Leave/duplicate Start.

## Deferred Topics To Preserve

Disconnect/reconnect semantics remain deferred and are not an accepted feature design.

Physical connection state is not logical session participation state. Future design must decide reconnect conditions, grace periods, inactive/forfeited/removed semantics, whether games wait or continue, what is generic Session Runtime policy versus authored Game Language behavior, and how reconnect is represented without making Session Runtime own TCP/WebSocket connections.

Mutable global display/profile ownership remains unresolved. Identity owns stable User identity; Session Runtime owns participation-time display-name snapshots.

Identity reconciliation/merge/alias semantics are deferred. This does not weaken Session's contract to persist the `UserUUID` it was given under the accepted workflow at the time.

## Explicit Non-Implementation Note

No implementation exists for the newly accepted Session Runtime or Identity decisions yet. Current Session Runtime remains scaffolding, and `CreateRoom`/`JoinRoom` are still stubs. Identity has no implementation.
