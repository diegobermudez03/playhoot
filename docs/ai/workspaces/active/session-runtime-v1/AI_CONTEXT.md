Process: Architecture Discussion
Topic: Session Runtime lifecycle/runtime design
Current stage: first architecture checkpoint approved; preparing durable Session/Participant model and lobby lifecycle foundations
Current execution surface: CONVERSATIONAL AI
Related durable artifacts: `game/README.md`, `game/CURRENT_STATE.md`, `game/docs/DATA_MODEL.md`, `game/docs/FLOWS.md`, `docs/decisions/architecture/ADR-0002-game-capability-persistence-transaction-boundary.md`
Blocked by: next human decisions on participant identity/ownership, creator/host relationship, lifecycle phases, terminal reasons, and expiration semantics
Next action: Conversational AI should use `HUMAN_REVIEW.md` to define the durable Session/Participant model and lobby lifecycle foundations before detailed Create/Join/Leave/Start operation design
Last durable checkpoint: human approved first Session Runtime architecture milestone; no WORK created; canonical promotion may be required before implementation authority
Last updated: 2026-09-06

# Resume Context

This workspace preserves the active initiative `session-runtime-v1`.

Goal: design and then incrementally implement the complete Session Runtime lifecycle. This is prioritized because playable multiplayer sessions are a core initial product capability.

Current internal process: Architecture Discussion.

This workspace records human-approved architecture decisions from the first checkpoint and the next architecture milestone to pursue. It is temporary process context only. It is not canonical architecture documentation, not implementation authority, and not WORK.

## Source References Loaded

- `docs/ai/OPERATING_MODEL.md` - execution-surface authority, drift handling, decision boundaries.
- `docs/ai/KNOWLEDGE_MAP.md` - routing map used to load Game/domain and process context.
- `docs/ai/protocols/CONVERSATIONAL_ORCHESTRATOR.md` - initiative workspace model and natural continuation behavior.
- `docs/ai/workspaces/README.md` - required active workspace semantics and resume header.
- `docs/ai/processes/ARCHITECTURE_DISCUSSION.md` and `docs/ai/protocols/ARCHITECTURE_DISCUSSION.md` - architecture discussion checkpoint behavior.
- `docs/decisions/architecture/ADR-0002-game-capability-persistence-transaction-boundary.md` - accepted Game Management / Session Runtime persistence and transaction boundary.
- `game/README.md` - canonical Game bounded-context and capability responsibilities.
- `game/CURRENT_STATE.md`, `game/docs/DATA_MODEL.md`, and `game/docs/FLOWS.md` - current implementation state references.
- `game/session/workflows/sessionlifecycle/step_create_room.go` - current `CreateRoom` stub accepts externally supplied `engine.Program`.
- `game/session/workflows/sessionlifecycle/step_join_room.go` - current `JoinRoom` stub.
- `game/session/workflows/sessionlifecycle/internal/repo/step_create_room.go` - incomplete repo scaffold.

## Previously Accepted Constraints

- Game is one accepted bounded context containing Game Management and Session Runtime as internal capabilities.
- Game Language is a supporting subsystem of Game.
- Session Runtime is already an accepted internal capability of Game.
- Game Management and Session Runtime have independent persistence and transaction ownership, even while they share the same physical PostgreSQL database.
- Game Management owns authored games, definitions/versions, publication/visibility state, images, and authored-game history.
- Session Runtime owns sessions, session state, participants, join codes, and other live-runtime persistence.
- No transaction may span Game Management-owned and Session Runtime-owned state.
- Session Runtime obtains game definition/version data through a narrow Game Management definition/read capability contract, not through Game Management's repository as the architectural API.
- A session must be pinned to a concrete stable execution definition/version.
- Version immutability for existing sessions is the preferred accepted model. Snapshot ownership by Session Runtime is not decided by ADR-0002.
- Current communication remains in-process in the modular-monolith phase. RPC, events, separate databases, Redis, or service extraction are not current requirements.
- Transport/network connections are outside Game ownership per `game/README.md`.
- Identity/profile ownership is outside Game ownership per `game/README.md`.
- Completed-session history/archive ownership is explicitly unresolved in `game/README.md`.

## Human-Approved First Checkpoint Decisions

Status: HUMAN-APPROVED in the architecture discussion, pending any required canonical promotion.

### A. Durable Authoritative State

Session Runtime owns all durable authoritative state that determines the meaning of a session and must be reconstructible after loss or restart of the process.

"Stateless" means process-stateless/reconstructible. It does not mean the domain has no state.

### B. Live Session Coordinator Boundary

Introduce the conceptual responsibility boundary `Live Session Coordinator` outside Session Runtime.

The Coordinator owns ephemeral/runtime mechanisms such as:

- live connection bindings;
- delivery/fan-out to connected clients;
- detection of physical disconnects;
- physical timer/scheduling mechanisms.

The Coordinator does not own authoritative session/game truth or business consequences.

This is a responsibility boundary, not a requirement to create another deployed service. V1 may keep it in the same Go process.

### C. V1 Deployment And Scaling Stance

Do not introduce sticky-session correctness requirements, Redis, distributed session routing, distributed locks, or other multi-instance mechanisms in V1.

The initial single-process modular-monolith deployment may use local maps, channels, and Go timers for ephemeral mechanisms.

Correctness of a session must not depend exclusively on ephemeral objects. Durable Session Runtime state and recovery rules must keep the architecture evolvable toward multiple processes or instances later.

### D. Durable Timer Obligations

Timer obligations that can affect game semantics are durable Session Runtime state.

The Coordinator owns the physical timer and the knowledge that wall-clock time has elapsed. Session Runtime owns the durable timer obligation and decides the semantic consequence.

When a timer expires, the Coordinator calls Session Runtime with the corresponding expiration signal/event. Session Runtime decides what happens, normally through its authoritative runtime/engine flow.

A process crash may destroy physical timers but must not silently erase timer obligations or change game semantics. Pending or overdue obligations must be reconstructible/recoverable from durable state.

## Deferred Design Topics

### Disconnect / Reconnect Semantics

Status: DEFERRED DESIGN TOPIC, not a decided feature.

Important distinction: physical connection state is not the same thing as logical session participation state.

Future design must determine:

- under what conditions a disconnected participant may reconnect;
- whether there is a grace period;
- when a participant is considered inactive, forfeited, or removed;
- whether the game continues, waits, substitutes/removes the participant, or terminates;
- what part of this behavior is generic Session Runtime policy versus authored Game Language behavior;
- how reconnect is represented without making Session Runtime own TCP/WebSocket connections.

Current human preference/intuition, not an accepted design: some reconnect/inactivity semantics may need to be expressible by the game itself because different games can require different behavior.

Do not design the final reconnect contract yet. Preserve it for the later lifecycle-operational milestone.

## Current Implementation Facts

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

Recommended resolution: do not fix this during architecture persistence. Route it into later Feature Development/WORK after the required lifecycle/runtime architecture decisions are accepted and canonically promoted as needed.

Related documentation note: `game/CURRENT_STATE.md` currently says no known drift, while ADR-0002 and the inspected `CreateRoom` stub reveal this drift. This workspace records the drift but does not update current-state documentation because canonical/current-state docs were excluded from this handoff.

## Pending Canonical Promotion

The first checkpoint produced material architecture decisions. Per `docs/ai/protocols/ARCHITECTURE_DISCUSSION.md`, significant human-decided architecture rationale should be persisted as an ADR when it meets the decision-record threshold and synchronized to canonical architecture/domain documentation when practical.

This handoff was authorized to update only the active workspace. The next Conversational AI step should determine and route the required canonical promotion before these decisions become implementation authority for WORK.

## Next Architecture Milestone

Define the durable Session/Participant model and lobby lifecycle foundations before designing detailed Create/Join/Leave/Start operations.

Focus areas:

- participant identity/ownership model;
- creator/host relationship;
- lifecycle phases and terminal reasons;
- expiration semantics.

## Suggested Initiative Design Sequence

1. Fundamental runtime ownership/state/failure/concurrency model. First checkpoint approved; canonical promotion pending.
2. Lobby lifecycle foundations: participant model, creator/host relationship, lifecycle phases, terminal reasons, expiration semantics.
3. Lobby operations: create, join, leave, expiration handling, start.
4. Runtime-turn contract: incoming signals -> engine execution -> durable state -> outputs.
5. Disconnect/reconnect, interruption/crash behavior, and abuse limits.
6. Completed-session history/archive ownership and persistence strategy, entering Domain Design when appropriate.
7. Initiative-level implementation planning followed by just-in-time Feature Development/WORK slices.

## Explicitly Not Done

- No WORK was created.
- No implementation code was changed.
- No migrations or schema were changed.
- No canonical ADR, architecture, domain, engineering-standard, or current-state documentation was changed.
- No final reconnect contract was designed.
