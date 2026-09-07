Process: Architecture Discussion
Topic: Session Runtime lifecycle/runtime design
Current stage: lobby lifecycle architecture accepted; ready for Runtime Turn architecture
Current execution surface: CONVERSATIONAL AI
Related durable artifacts: `ARCHITECTURE.md`, `game/README.md`, `identity/README.md`, `docs/ai/KNOWLEDGE_MAP.md`, `docs/decisions/architecture/ADR-0002-game-capability-persistence-transaction-boundary.md`, `docs/decisions/architecture/ADR-0003-session-runtime-durable-boundary.md`, `docs/decisions/architecture/ADR-0004-session-runtime-actor-and-lifecycle-foundations.md`, `docs/decisions/architecture/ADR-0005-cross-domain-public-entity-references.md`, `docs/decisions/architecture/ADR-0006-identity-user-public-identity-boundary.md`, `docs/decisions/architecture/ADR-0007-session-lobby-lifecycle-contract.md`, `docs/decisions/architecture/ADR-0008-session-public-and-internal-identity-boundary.md`, `docs/decisions/architecture/ADR-0009-game-language-root-player-roster-contract.md`, `game/language/v1/program/README.md`, `game/language/v1/engine/README.md`, `game/language/v1/engine/LOGICAL_CONTRACT.md`, `docs/engineering/standards/cross-domain-reference-naming.md`
Blocked by: Runtime Turn architecture decisions
Next action: Conversational AI should design Runtime Turn architecture; do not enter implementation planning or create WORK yet
Last durable checkpoint: accepted detailed Session lobby lifecycle, public/internal identity boundary, and Game Language root roster contract were promoted to ADR-0007 through ADR-0009 and canonical docs; no WORK created
Last updated: 2026-09-06

# Resume Context

This workspace preserves the active initiative `session-runtime-v1`.

Goal: design and then incrementally implement the complete Session Runtime lifecycle. This is prioritized because playable multiplayer sessions are a core initial product capability.

The first architecture checkpoints are accepted and promoted to canonical owners. The initiative is still in Architecture Discussion. No Feature Development process or WORK has been opened.

This workspace records process continuity only. Accepted architecture/domain facts were promoted to canonical owners and ADRs where required. This file is not canonical implementation authority.

## Accepted Foundation Decisions

Status: HUMAN-APPROVED and canonically promoted.

Session Runtime owns all durable authoritative state that determines the meaning of a session and must be reconstructible after loss or restart of the process. `Stateless` means process-stateless/reconstructible, not that the domain itself has no state.

`Live Session Coordinator` is the conceptual responsibility boundary outside Session Runtime. The Coordinator owns ephemeral/runtime mechanisms such as live connection bindings, delivery/fan-out to connected clients, detection of physical disconnects, and physical timer/scheduling mechanisms. It does not own authoritative session/game truth or business consequences. This is a responsibility boundary, not a requirement to create another deployed service. V1 may keep it in the same Go process.

V1 does not introduce sticky-session correctness requirements, Redis, distributed session routing, distributed locks, or other multi-instance mechanisms. The initial single-process modular-monolith deployment may use local maps, channels, and Go timers for ephemeral mechanisms. Correctness of a session must not depend exclusively on those ephemeral objects.

Timer obligations that can affect game semantics are durable Session Runtime state. Coordinator owns the physical timer and knowledge that wall-clock time elapsed; Session Runtime owns the durable timer obligation and decides the semantic consequence. When a timer expires, Coordinator calls Session Runtime with the corresponding expiration signal/event. Pending or overdue obligations must be reconstructible/recoverable from durable state.

Identity owns stable `User` identity and public `UserUUID`. A guest is already a `User`; normal guest-to-registered conversion preserves the same `UserUUID`. Session Runtime owns local `SessionActorID`. A SessionActor persistently correlates to `Identity.User` through `user_uuid`.

Public/app Session Runtime boundaries use an authenticated `UserUUID`; clients are not trusted sources of `UserUUID`. A trusted auth/application layer resolves credentials to `UserUUID`. Identity/Auth proves who the caller is; Session Runtime decides what the caller may do inside a Session.

`SessionActorID` is internal-only and is not a public/app contract. Coordinator, Orchestrator, adapters, and clients do not own the mapping. Host and Participant are independent relationships to Session-owned actor identity.

Canonical references: ADR-0003 through ADR-0006, ADR-0008, `ARCHITECTURE.md`, `identity/README.md`, `game/README.md`, and `docs/engineering/standards/cross-domain-reference-naming.md`.

## Accepted Lobby Lifecycle Contract

Status: HUMAN-APPROVED and canonically promoted.

Session-scoped lobby operations that materialize Session meaning serialize per Session. Join, Leave, Start, and expiration materialization are mutually serialized while the Session is in LOBBY. A database row lock is the natural V1 mechanism but not the canonical requirement. This accepted serialization statement applies to LOBBY only, not RUNNING.

Create Session/Room takes Game public UUID, authenticated host UserUUID, and opaque idempotency key. The caller does not provide an external `engine.Program`. Session Runtime resolves the current playable immutable Game definition through the narrow Game Management read capability, pins it, and may compile/validate outside the Session transaction. In the Session transaction it creates the LOBBY Session, host SessionActor correlated to `user_uuid`, host relationship, active JoinCode, and `lobby_expires_at`. The host is not automatically a Participant. QR codes and URLs are projections over JoinCode state.

JoinCode is authoritative Session Runtime state, not cache state. One active JoinCode exists per join-admissible lobby. It is revoked when the Session leaves LOBBY. Historical codes may be retained and textual code values are not forever reserved.

Join takes JoinCode, authenticated UserUUID, display-name snapshot from a trusted app/integration boundary, and opaque idempotency key. It resolves the JoinCode, obtains the pinned Game definition for constraints, enters serialized mutation, revalidates LOBBY and expiration, finds or creates the SessionActor for `(Session, UserUUID)`, and creates or reactivates Participant state. Repeated active Join has the same admission effect. Active Participants must not exceed `players.max` from the pinned Game definition.

Leave is a LOBBY participation operation. The public caller is `UserUUID`, resolved to SessionActor. It deactivates Participant and releases the slot, but does not delete SessionActor, remove host, or cancel the Session. A participating host may leave gameplay participation but remains host. Rejoin to an admissible lobby reuses the same SessionActor.

`lobby_expires_at` is authoritative. Join and Start correctness do not depend on a sweeper. If phase is LOBBY and now is at or after the deadline, an operation may atomically materialize TERMINAL with a lobby-expiration terminal reason, terminal timestamp equal to the deadline, and JoinCode revocation.

Start takes Session UUID, authenticated UserUUID, and opaque idempotency key. It resolves `(SessionUUID, UserUUID) -> SessionActorID`, verifies host authority, uses the pinned immutable definition/version, validates LOBBY, expiration, host, active Participants, `players.min`, and defensively `players.max`, initializes engine state, processes the engine-required initial signal/first Commit per engine contract, persists authoritative runtime state and durable consequences, atomically transitions to RUNNING, sets `started_at`, revokes JoinCode, and commits before external outputs. Failure must not produce partial RUNNING. Coordinator/client outputs occur after durable commit.

External mutating Session commands Create, Join, Leave, and Start accept an opaque idempotency key. Retries of the same logical command must not repeat effects. The final idempotency schema/algorithm remains undesigned.

Canonical references: ADR-0007, ADR-0008, and `game/README.md`.

## Accepted Game Language Root Roster Contract

Status: HUMAN-APPROVED and canonically promoted; not yet implemented as a complete validated contract.

Session Runtime supplies this standardized root input at Start:

```text
players: list<user>
```

Session Runtime builds `players` from active Participants at Start. Each `user` corresponds to Session-local runtime identity derived from SessionActorID. Identity.UserUUID is never exposed to authored Game Language or engine semantics.

Authored games should use the standard `players` root parameter instead of inventing custom root parameter names for the participant roster.

For V1, arbitrary external game-specific root parameters are deferred until Session Configuration is designed.

Canonical references: ADR-0009, `game/language/v1/program/README.md`, `game/language/v1/engine/README.md`, and `game/language/v1/engine/LOGICAL_CONTRACT.md`.

## Deferred Design Topics

Disconnect/reconnect semantics remain DEFERRED, not a decided feature.

Physical connection state is not logical session participation state. Future design must determine reconnect conditions, grace periods, inactive/forfeited/removed semantics, whether the game continues/waits/substitutes/removes/terminates, what is generic Session Runtime policy versus authored Game Language behavior, and how reconnect is represented without making Session Runtime own TCP/WebSocket connections.

Current human preference/intuition, not an accepted design: some reconnect/inactivity semantics may need to be expressible by the game itself because different games can require different behavior.

Other deferred topics: RUNNING-phase concurrency/turn processing, exhaustive terminal reasons, host transfer, max-runtime/runaway-session policy, completed-session history/archive ownership, mutable global display/profile ownership, Identity reconciliation/merge/alias semantics, Session Configuration/arbitrary external game-specific root params, and final idempotency storage schema/algorithm.

## Next Architecture Milestone

Design Runtime Turn architecture before implementation planning.

The next Conversational AI should decide:

1. What exactly is one externally triggered runtime transaction/turn?
2. How is concurrent input for the same RUNNING Session serialized?
3. How are external signals distinguished from engine InternalSignals?
4. Are InternalSignals drained in one domain transaction, and what bounded-loop protection applies?
5. What Snapshot/history records are persisted per turn?
6. How are engine outputs transformed into durable obligations versus post-commit delivery effects?
7. How are timer obligations created, cancelled, and consumed atomically?
8. What idempotency/versioning prevents duplicate or stale runtime inputs?
9. What happens if database commit succeeds but Coordinator delivery fails?
10. What recoverable state exists after process crash at each boundary?

Do not enter Feature Development, create WORK, or implement production code until the Runtime Turn contract is accepted and the repository workflow authorizes implementation.

## Current Implementation Facts And Drift To Carry Forward

- Identity has an accepted domain model but no implementation.
- `game/CURRENT_STATE.md` records Session Runtime as partial: session, session-state, session-player, and join-code schema exists; lifecycle scaffolding exists; `CreateRoom` and `JoinRoom` are stubs; no session execution flow was found implemented.
- Current Session Runtime scaffolding accepts externally supplied `engine.Program`, which conflicts with accepted architecture requiring Session Runtime to resolve/pin the playable immutable Game definition through the narrow Game Management read capability.
- Existing owner/player UUID fields are not aligned with the accepted `UserUUID` public boundary and internal `SessionActorID` runtime boundary.
- Join, Leave, Start, idempotency, and the standardized `players: list<user>` root roster contract are not implemented.
- `game/CURRENT_STATE.md` previously reported no known drift. This workspace records drift but does not update current-state documentation because current-state docs were excluded from this checkpoint.

## Explicitly Not Done

- No WORK was created.
- No production code, tests, migrations, authentication code, or Session Runtime implementation were changed.
- No current-state docs were modified to pretend implementation exists.
- No final reconnect contract was designed.
- No Runtime Turn contract was designed.
