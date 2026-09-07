# ADR-0007: Session Lobby Lifecycle Contract

Status: ACCEPTED
Created: 2026-09-06
Last status change: 2026-09-06
Supersedes: None
Superseded by: None

## Context

ADR-0002 established that Session Runtime obtains and pins immutable Game definition/version data through a narrow Game Management read capability. ADR-0003 established Session Runtime as the owner of durable authoritative session/runtime state. ADR-0004 established Host/Participant separation, SessionActor identity, `LOBBY -> RUNNING -> TERMINAL`, and `lobby_expires_at`. ADR-0006 established `Identity.User` and `UserUUID`.

The lobby lifecycle now needs accepted operation semantics before implementation work can be planned. The main pressures are preventing stale concurrent roster/lifecycle decisions, pinning the correct Game definition, treating JoinCode as durable Session Runtime state, and making retries safe.

## Decision

All mutations that can alter lobby membership or cross the lobby/start boundary for one Session must serialize against that Session.

This includes at least:

- Join;
- Leave;
- Start;
- materialization of lobby expiration when performed by one of these operations or housekeeping.

For a single Session, these operations must not concurrently observe stale roster/lifecycle state and both commit conflicting results. A Session-row lock such as `SELECT ... FOR UPDATE` is a natural V1 implementation, but the accepted architecture is the serialization guarantee, not one specific SQL syntax. This decision applies to the LOBBY lifecycle only and does not decide the high-frequency RUNNING runtime concurrency strategy.

The create-lobby operation receives conceptually:

- the Game public UUID identifying what the host wants to run;
- authenticated host `UserUUID`;
- an opaque idempotency key.

Session Runtime does not accept an externally-precompiled `engine.Program`. It resolves the currently playable immutable Game definition/version through the narrow Game Management read capability, pins the Session to that immutable definition/version, and may compile/validate outside the Session transaction because the pinned definition is immutable.

Within one Session-owned transaction, create conceptually:

- Session in `LOBBY`;
- host SessionActor correlated through `user_uuid`;
- Session host relationship pointing to that internal SessionActor;
- active JoinCode;
- `lobby_expires_at`.

Creating the Session does not automatically make the host a gameplay Participant.

`lobby_expires_at` is Session Runtime policy for V1, not arbitrary caller-provided state, unless another accepted product decision says otherwise.

The public response does not expose `SessionActorID`. It may expose public/application-level results such as Session UUID, JoinCode, lobby expiration, and other accepted public Session information. QR codes and join URLs are projection/application concerns derived from JoinCode, not authoritative Session-domain state.

JoinCode is authoritative Session Runtime state, not cache-owned truth. The accepted JoinCode semantics are:

- one active JoinCode for a lobby unless a future accepted feature requires rotation/multiple codes;
- valid only while its Session remains admissible for joining;
- revoked when the Session exits `LOBBY`;
- historical records may be retained for traceability;
- a textual code value need not be permanently reserved forever after revocation, although no aggressive recycling algorithm is required now.

QR and URL representations do not change the underlying JoinCode identity/semantics.

Join receives conceptually:

- JoinCode;
- authenticated `UserUUID`;
- display-name snapshot supplied by the trusted application/integration layer;
- opaque idempotency key.

Session Runtime resolves the JoinCode to its Session, obtains the pinned Game definition needed for lobby constraints, enters the Session-serialized mutation, and revalidates JoinCode admissibility, `LOBBY` phase, and that current time is before `lobby_expires_at`.

Join finds or creates the Session-owned SessionActor for `(Session, UserUUID)`. If already an active Participant, repeated Join is the same logical admission rather than consuming another slot. Otherwise Join counts active Participants, enforces `players.max` from the pinned Game definition, activates Participant membership, and commits atomically.

The Game definition remains the source of truth for authored player-count constraints. Do not copy `players.min/max` into Session as a second source of truth merely for correctness.

Leave is a `LOBBY` participation operation. Its public/application-facing caller identity is `UserUUID`, not `SessionActorID`. Session Runtime resolves `(Session, UserUUID)` to its internal SessionActor.

Leaving deactivates logical Participant membership, releases a lobby player slot, does not delete the SessionActor, does not remove host authority, and does not automatically cancel the Session. A host who is also participating may leave gameplay participation while remaining host. If the same User rejoins the same admissible lobby, Session Runtime reuses the same SessionActor rather than creating a new actor identity.

Do not yet freeze the final historical storage representation of repeated join/leave/rejoin episodes; completed-session history remains a later concern.

`lobby_expires_at` is authoritative. Join/Start correctness does not depend on a sweeper having already changed the stored phase. When an operation discovers:

```text
phase == LOBBY && now >= lobby_expires_at
```

it may atomically materialize:

- `phase = TERMINAL`;
- terminal reason corresponding to lobby expiration;
- terminal timestamp representing the semantic expiration instant;
- JoinCode revocation.

The accepted semantic terminal time for this case is the lobby deadline itself, not merely the later instant when a request or sweeper noticed it. A background sweeper is housekeeping/materialization support, not the source of business correctness. This decision does not invent the exhaustive terminal-reason enum.

Start receives conceptually:

- Session UUID;
- authenticated `UserUUID`;
- opaque idempotency key.

Session Runtime resolves `(SessionUUID, UserUUID) -> SessionActorID` internally and uses that internal identity to verify host authority.

Start must use the Session's pinned immutable Game definition/version, compile/load that exact pinned definition rather than the latest current Game version, validate `LOBBY` phase, validate `lobby_expires_at`, validate that the resolved SessionActor is the Session host, load active Participants, validate `players.min`, and defensively validate `players.max`.

Start initializes the engine/runtime state, processes the engine's required initial signal/first Commit according to the accepted engine contract, persists the initial authoritative runtime state and durable consequences, transitions Session atomically to `RUNNING`, sets `started_at`, revokes JoinCode, and commits before any effects/outputs are delivered outside the transaction.

If initialization/first authoritative execution fails, the transaction must not partially expose a `RUNNING` Session. No Coordinator/client-visible output is delivered before successful durable commit.

Mutating external Session commands in this lobby lifecycle accept an opaque idempotency key, including conceptually Create, Join, Leave, and Start. Session Runtime must guarantee that retrying the same logical command does not repeat its effect. Natural/domain idempotency, such as repeated Join when already actively joined, is useful additional protection but is not a substitute for the explicit idempotency contract. This decision does not invent the final idempotency table/schema/storage algorithm.

## Rationale

Lobby operations are low-frequency but correctness-sensitive. Serializing per-Session lobby mutations prevents last-slot races, Join/Start races, Leave/Start races, duplicate Start exposure, and stale expiration decisions without deciding the later RUNNING runtime strategy.

Pinning the immutable Game definition at create time preserves the ADR-0002 boundary and ensures Join/Start evaluate constraints against the Session's execution definition, not a newer Game version. Keeping `players.min/max` in the Game definition avoids a second correctness source.

Treating JoinCode as durable Session Runtime state preserves recovery and prevents cache from becoming authoritative. Keeping QR and URL values as derived projections avoids confusing presentation with domain identity.

Explicit idempotency is required because public/application commands may be retried after network failures. Natural domain idempotency alone does not cover all cases.

## Alternatives Considered

### Let concurrent lobby operations race with optimistic checks only

Rejected. Without a per-Session serialization guarantee, two operations can observe stale roster/lifecycle state and both commit conflicting results.

### Make cache the authoritative JoinCode source

Rejected. JoinCode determines admission to a durable Session and must be recoverable with Session Runtime state.

### Copy player-count limits into Session for correctness

Rejected. The pinned Game definition already owns authored player-count constraints. A copied value may be useful later for diagnostics/history, but it is not needed as a second source of truth for correctness.

### Deliver outputs before Start commits

Rejected. Client-visible delivery before durable commit can expose a running session that the database did not commit.

### Rely only on natural idempotency

Rejected. Some retries cannot be resolved safely from domain state alone without an explicit command idempotency contract.

## Consequences

- Future lobby implementation must serialize Join/Leave/Start/expiration materialization per Session.
- Create must resolve and pin a playable immutable Game definition through Game Management rather than accepting an externally compiled Program.
- JoinCode persistence and revocation belong to Session Runtime.
- Join/Leave rejoin behavior must preserve SessionActor identity for the same `(Session, UserUUID)`.
- Start must atomically initialize durable runtime state and transition to `RUNNING` before outputs are delivered.
- Runtime Turn concurrency, output delivery after commit, timer-obligation lifecycle during RUNNING, and final idempotency storage design remain future architecture/implementation work.

## Canonical Knowledge Impact

- `game/README.md` - adds accepted lobby serialization, Create/Join/Leave/Start, JoinCode, lobby expiration, and command idempotency semantics.

## Implementation Impact

Future implementation work must align Session Runtime lobby APIs, persistence, transactions, and tests with this contract. No production code, migration, or WORK is authorized by this record.
