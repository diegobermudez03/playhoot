Process: Architecture Discussion
Topic: Session Runtime lifecycle/runtime design
Current stage: minimum Identity Domain Design completed; ready to design detailed LOBBY lifecycle operations
Current execution surface: CONVERSATIONAL AI
Related durable artifacts: `ARCHITECTURE.md`, `game/README.md`, `identity/README.md`, `identity/CURRENT_STATE.md`, `docs/ai/KNOWLEDGE_MAP.md`, `docs/decisions/architecture/ADR-0002-game-capability-persistence-transaction-boundary.md`, `docs/decisions/architecture/ADR-0003-session-runtime-durable-boundary.md`, `docs/decisions/architecture/ADR-0004-session-runtime-actor-and-lifecycle-foundations.md`, `docs/decisions/architecture/ADR-0005-cross-domain-public-entity-references.md`, `docs/decisions/architecture/ADR-0006-identity-user-public-identity-boundary.md`, `docs/engineering/standards/cross-domain-reference-naming.md`
Blocked by: human decisions on detailed CreateSession/CreateRoom, Join, Leave, and Start lobby operation contracts
Next action: Conversational AI should design detailed LOBBY lifecycle operations over the accepted Session Runtime and Identity foundations; do not enter implementation planning until the lobby contract is sufficiently accepted
Last durable checkpoint: accepted minimum Identity boundary was promoted to ADR-0006 and canonical docs; initiative returned to Session Runtime Architecture Discussion; no WORK created
Last updated: 2026-09-06

# Resume Context

This workspace preserves the active initiative `session-runtime-v1`.

Goal: design and then incrementally implement the complete Session Runtime lifecycle. This is prioritized because playable multiplayer sessions are a core initial product capability.

The initiative temporarily entered Domain Design to define the minimum public Identity concept needed by Session Runtime. That Domain Design result is completed and accepted. The initiative has returned to Architecture Discussion for detailed Session Runtime LOBBY lifecycle operation design.

This workspace records process continuity only. Accepted architecture/domain facts were promoted to canonical owners and ADRs where required. No WORK or implementation authority exists yet.

## Source References Loaded

- `docs/ai/OPERATING_MODEL.md` - execution-surface authority, drift handling, decision boundaries.
- `docs/ai/KNOWLEDGE_MAP.md` - routing map used to load Game/Identity/domain and process context.
- `docs/ai/protocols/CONVERSATIONAL_ORCHESTRATOR.md` - initiative workspace model and process transition behavior.
- `docs/ai/workspaces/README.md` - required active workspace semantics and resume header.
- `docs/ai/processes/ARCHITECTURE_DISCUSSION.md` and `docs/ai/protocols/ARCHITECTURE_DISCUSSION.md` - architecture checkpoint and canonical-promotion behavior.
- `docs/ai/processes/DOMAIN_DESIGN.md` and `docs/ai/protocols/DOMAIN_DESIGN.md` - completed Identity boundary process.
- `docs/ai/templates/domain/` - domain documentation templates used for Identity README/current-state shape.
- `docs/decisions/README.md` and `docs/decisions/templates/ARCHITECTURE_DECISION.template.md` - ADR creation and accepted-decision synchronization rules.
- `ARCHITECTURE.md` - global architecture owner.
- `game/README.md` - Game domain model owner.
- `identity/README.md` and `identity/CURRENT_STATE.md` - Identity domain model and implementation state owners.
- `docs/engineering/standards/cross-domain-reference-naming.md` - cross-domain reference naming owner.
- `game/CURRENT_STATE.md`, `game/docs/DATA_MODEL.md`, and `game/docs/FLOWS.md` - current Game implementation state references; not modified.
- `game/session/workflows/sessionlifecycle/step_create_room.go` - current `CreateRoom` stub accepts externally supplied `engine.Program`.
- `game/session/workflows/sessionlifecycle/step_join_room.go` - current `JoinRoom` stub.
- `game/session/workflows/sessionlifecycle/internal/repo/step_create_room.go` - incomplete repo scaffold.

## Canonically Promoted Decisions

### ADR-0003

`docs/decisions/architecture/ADR-0003-session-runtime-durable-boundary.md`

Status: ACCEPTED.

Session Runtime owns durable authoritative session/runtime state; `Live Session Coordinator` owns ephemeral connection/delivery/disconnect/timer mechanisms; V1 remains single-process/modular-monolith without Redis, sticky-session correctness, distributed routing, distributed locks, or other multi-instance mechanisms; semantic timer obligations are durable Session Runtime state.

Canonical owner synchronized: `game/README.md`.

### ADR-0004

`docs/decisions/architecture/ADR-0004-session-runtime-actor-and-lifecycle-foundations.md`

Status: ACCEPTED.

Host and Participant are independent relationships to Session-owned actor identity; Participant is session-scoped; no cross-session Player/Profile master entity inside Session Runtime; no generic persisted Participant role in V1; uniqueness is session-local only; Session lifecycle is conceptually `LOBBY -> RUNNING -> TERMINAL`; `lobby_expires_at` is the current expiration concern; connection state is not Participant state; reconnect remains deferred.

Canonical owner synchronized: `game/README.md`.

### ADR-0005

`docs/decisions/architecture/ADR-0005-cross-domain-public-entity-references.md`

Status: ACCEPTED.

Domains may publish cross-domain referenceable logical entities identified by stable public UUIDs. Public logical entity identifiers are distinct from storage/table primary key contracts. Consumers may persist public UUID references without cross-domain database foreign keys, direct producer-persistence access, or dependence on producer table layout.

Canonical owners synchronized: `ARCHITECTURE.md`, `docs/engineering/standards/cross-domain-reference-naming.md`, and `docs/engineering/standards/INDEX.md`.

### ADR-0006

`docs/decisions/architecture/ADR-0006-identity-user-public-identity-boundary.md`

Status: ACCEPTED.

Identity is the bounded context responsible for stable identity of people interacting with Playhoot. `User` is the public cross-domain referenceable entity exported by Identity. `User` represents stable Playhoot identity for a real person and owns public `UserUUID`.

Guest and registered person are not separate public identities. A guest is already a `User`; normal guest-to-registered conversion preserves the same `UserUUID`.

`User` is distinct from authentication/account concerns and profile/presentation concerns. User existence does not require an authenticated account.

Session Runtime owns local `SessionActorID`. A SessionActor persistently correlates to `Identity.User` through:

```text
user_uuid -> Identity.User
```

Session Runtime uses `SessionActorID` as its primary local/runtime identity and avoids propagating `UserUUID` through engine semantics unnecessarily. The `user_uuid` reference is logical: no cross-domain database foreign key, no direct reads of Identity persistence, and no dependency on Identity table layout.

Mutable global display/profile ownership remains unresolved. Session Runtime owns its participation-time display-name snapshot. Identity reconciliation/merge/alias semantics remain deferred, and do not weaken Session's contract to persist the `UserUUID` it was given under the accepted workflow at the time.

Canonical owners synchronized: `ARCHITECTURE.md`, `identity/README.md`, `identity/CURRENT_STATE.md`, `game/README.md`, `docs/ai/KNOWLEDGE_MAP.md`, and `docs/engineering/standards/cross-domain-reference-naming.md`.

## Engineering Standard

`docs/engineering/standards/cross-domain-reference-naming.md`

Status: CANONICAL ENGINEERING STANDARD.

Accepted naming rule: for a persisted cross-domain reference, name the field/column after the public entity being referenced, not after the consumer's local role for it.

`Identity.User` is now accepted, so `user_uuid` is the correct persisted reference name when referencing `Identity.User`.

Migration strategy: FUTURE CODE ONLY. No repository-wide migration or renaming is authorized.

## Deferred Design Topics

### Disconnect / Reconnect Semantics

Status: DEFERRED DESIGN TOPIC, not a decided feature.

Physical connection state is not logical session participation state.

Future design must determine:

- under what conditions a disconnected participant may reconnect;
- whether there is a grace period;
- when a participant is considered inactive, forfeited, or removed;
- whether the game continues, waits, substitutes/removes the participant, or terminates;
- what part of this behavior is generic Session Runtime policy versus authored Game Language behavior;
- how reconnect is represented without making Session Runtime own TCP/WebSocket connections.

Current human preference/intuition, not an accepted design: some reconnect/inactivity semantics may need to be expressible by the game itself because different games can require different behavior.

Do not design the final reconnect contract yet. Preserve it for the later lifecycle-operational milestone.

### Identity Topics

- Mutable global display/profile ownership remains unresolved.
- Identity reconciliation/merge/alias semantics are deferred.
- Authentication/account/provider systems, authorization policy, and Identity persistence details are not designed.

### Session Runtime Topics

- Exhaustive terminal-reason enum is not yet accepted.
- Host transfer is not designed.
- Max-runtime/runaway-session policy is distinct from `lobby_expires_at` and remains deferred.
- Completed-session history/archive ownership remains unresolved and may require Domain Design later.

## Next Architecture Milestone

Design the detailed LOBBY lifecycle operations over the now-accepted foundations.

The next Conversational AI should specifically design:

1. `CreateSession` / `CreateRoom`

- inputs;
- host SessionActor creation;
- pinned Game definition/version;
- join-code creation;
- lobby expiration;
- transactional boundary;
- returned public/session-local identifiers.

2. `Join`

- resolution of join code vs SessionID responsibilities;
- creation/reactivation of SessionActor/Participant;
- `UserUUID` + display-name snapshot;
- player-count constraints from the pinned Game definition;
- concurrency when multiple players take the last available slots;
- idempotency/repeated Join.

3. `Leave`

- lobby-only semantics;
- participant lifecycle;
- rejoin behavior before Start;
- host behavior if relevant.

4. `Start`

- host authority;
- lobby expiration;
- minimum/maximum player constraints;
- atomic transition to RUNNING;
- construction/persistence of the initial runtime Snapshot;
- initial engine outputs/timer obligations;
- concurrency with Join/Leave/duplicate Start.

Do not yet design disconnect/reconnect semantics beyond preserving the deferred topic. Do not enter implementation planning unless the Conversational AI later determines the lobby contract is sufficiently accepted.

## Current Implementation Facts

- Identity has an accepted domain model but no implementation.
- `game/CURRENT_STATE.md` records Session Runtime as partial: session, session-state, session-player, and join-code schema exists; lifecycle scaffolding exists; `CreateRoom` and `JoinRoom` are stubs; no session execution flow was found implemented.
- `game/docs/DATA_MODEL.md` records current Session Runtime tables: `sessions`, `session_players`, `join_codes`, and `session_states`.
- `game/docs/FLOWS.md` records `CreateRoom` and `JoinRoom` as stubs.
- `game/session/workflows/sessionlifecycle/step_create_room.go` currently declares `CreateRoom(ctx, program engine.Program, gameVersionUUID, ownerUUID)`.
- `game/session/workflows/sessionlifecycle/step_join_room.go` currently returns nil without behavior.
- `game/session/workflows/sessionlifecycle/internal/repo/step_create_room.go` declares `func (r *Repo) CreateRoom(ctx context.Context)` without a function body.

## Drift To Carry Forward

DRIFT DETECTED

Canonical model: ADR-0002 says Session Runtime should no longer treat an externally supplied `engine.Program` as the accepted contract. It should obtain definition/version information itself through a narrow Game Management read capability and pin the session to a stable execution definition/version.

Current implementation: `game/session/workflows/sessionlifecycle/step_create_room.go` still accepts an externally supplied `engine.Program`, and the lifecycle/repo code is scaffolding.

Likely explanation: Session Runtime existed as a pre-decision scaffold before ADR-0002 clarified the capability boundary and definition-read contract.

Recommended resolution: route it into later Feature Development/WORK after the detailed lobby operation contract is accepted. Do not fix it during documentation persistence.

Related documentation note: `game/CURRENT_STATE.md` currently says no known drift, while ADR-0002 and the inspected `CreateRoom` stub reveal this drift. This workspace records the drift but does not update current-state documentation because current-state docs were excluded from this handoff.

## Explicitly Not Done

- No WORK was created.
- No production code, tests, migrations, authentication code, or Session Runtime implementation were changed.
- No Identity implementation, authentication/account system, profile system, or storage schema was designed or implemented.
- No final reconnect contract was designed.
