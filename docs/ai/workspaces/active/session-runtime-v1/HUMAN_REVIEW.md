# Session Runtime Turn Architecture Checkpoint

Process: Architecture Discussion

Status: next checkpoint pending.

## Accepted Context

The detailed LOBBY lifecycle architecture is now HUMAN-APPROVED and canonically promoted.

Accepted context for the next checkpoint:

- Session Runtime owns durable authoritative session/runtime state.
- `Live Session Coordinator` owns ephemeral connections, delivery/fan-out, physical disconnect detection, and physical timer/scheduling mechanisms.
- V1 remains a single-process modular monolith; sticky-session correctness, Redis, distributed routing, distributed locks, and other multi-instance mechanisms are not V1 requirements.
- Timer obligations that can affect game semantics are durable Session Runtime state.
- Identity owns stable public `Identity.User` / `UserUUID`.
- Public/app Session Runtime boundaries use authenticated `UserUUID`; clients are not trusted sources of `UserUUID`.
- Session Runtime owns `(SessionUUID, UserUUID) -> SessionActorID`.
- `SessionActorID` is internal-only and is the basis for engine runtime identity.
- Identity.UserUUID is not exposed to Game Language or engine semantics.
- Outbound identity translation maps engine user identity / SessionActorID back through `SessionActor.user_uuid` before crossing public boundaries.
- Identity/Auth proves who the caller is; Session Runtime decides Session business authorization.
- Host and Participant are independent relationships to SessionActor.
- LOBBY operations that materialize Session meaning serialize per Session.
- Create pins the current playable immutable Game definition through the narrow Game Management read capability; callers do not provide `engine.Program`.
- JoinCode is authoritative Session Runtime state, with one active code per join-admissible lobby.
- Join/Leave/rejoin behavior is defined for LOBBY and preserves SessionActor identity.
- `lobby_expires_at` is authoritative and must be enforced by Join/Start even without a sweeper.
- Start atomically validates host, expiration, participant counts, initializes engine state, persists runtime state/durable consequences, transitions to RUNNING, and revokes JoinCode before external outputs.
- Mutating external Session commands Create, Join, Leave, and Start accept opaque idempotency keys.
- Session Runtime supplies standardized root input `players: list<user>` at Start, built from active Participants.
- Arbitrary external game-specific root params are deferred until Session Configuration is designed.

Canonical references:

- `ARCHITECTURE.md`
- `game/README.md`
- `identity/README.md`
- `docs/decisions/architecture/ADR-0003-session-runtime-durable-boundary.md`
- `docs/decisions/architecture/ADR-0004-session-runtime-actor-and-lifecycle-foundations.md`
- `docs/decisions/architecture/ADR-0005-cross-domain-public-entity-references.md`
- `docs/decisions/architecture/ADR-0006-identity-user-public-identity-boundary.md`
- `docs/decisions/architecture/ADR-0007-session-lobby-lifecycle-contract.md`
- `docs/decisions/architecture/ADR-0008-session-public-and-internal-identity-boundary.md`
- `docs/decisions/architecture/ADR-0009-game-language-root-player-roster-contract.md`
- `game/language/v1/program/README.md`
- `game/language/v1/engine/README.md`
- `game/language/v1/engine/LOGICAL_CONTRACT.md`

## Runtime Turn Questions To Decide

Before implementation planning, decide:

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

## Deferred Topics To Preserve

Disconnect/reconnect semantics remain deferred and are not an accepted feature design.

Physical connection state is not logical session participation state. Future design must decide reconnect conditions, grace periods, inactive/forfeited/removed semantics, whether games wait or continue, what is generic Session Runtime policy versus authored Game Language behavior, and how reconnect is represented without making Session Runtime own TCP/WebSocket connections.

Current human preference/intuition, not an accepted design: some reconnect/inactivity semantics may need to be expressible by the game itself because different games can require different behavior.

Other deferred topics:

- RUNNING-phase concurrency and turn processing are not yet accepted.
- Exhaustive terminal-reason enum is not yet accepted.
- Max-runtime/runaway-session policy is deferred.
- Completed-session history/archive ownership is unresolved.
- Mutable global display/profile ownership is unresolved.
- Identity reconciliation/merge/alias semantics are deferred.
- Session Configuration and arbitrary external game-specific root params are deferred.
- Final idempotency storage schema/algorithm is deferred.

## Explicit Non-Implementation Note

Do not create WORK yet. Do not modify production code, migrations, tests, or current-state docs to pretend implementation exists.

Current implementation remains behind the accepted architecture: Session Runtime scaffolding still accepts externally supplied `engine.Program`, has owner/player UUID fields inconsistent with the accepted `UserUUID`/`SessionActorID` boundary, lacks Join/Leave/Start behavior, and does not implement the standardized `players: list<user>` root roster contract.
