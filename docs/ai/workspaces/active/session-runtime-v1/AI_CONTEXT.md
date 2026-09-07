Process: Architecture Discussion
Topic: Session Runtime lifecycle/runtime design
Current stage: awaiting human review of first architectural milestone
Current execution surface: CONVERSATIONAL AI
Related durable artifacts: `game/README.md`, `game/CURRENT_STATE.md`, `game/docs/DATA_MODEL.md`, `game/docs/FLOWS.md`, `docs/decisions/architecture/ADR-0002-game-capability-persistence-transaction-boundary.md`
Blocked by: human decisions on Session Runtime ownership/state/coordinator/concurrency boundary before detailed lobby/runtime design
Next action: Conversational AI should review `HUMAN_REVIEW.md` with the human, resolve the first material architecture decisions, then route accepted outcomes to ADR/canonical docs or continue design
Last durable checkpoint: initial initiative workspace persisted for Session Runtime v1 architecture discussion; no WORK created
Last updated: 2026-09-06

# Resume Context

This workspace preserves the active initiative `session-runtime-v1`.

Goal: design and then incrementally implement the complete Session Runtime lifecycle. This is prioritized because playable multiplayer sessions are a core initial product capability.

Current internal process: Architecture Discussion.

This workspace was created by a Codebase Agent handoff from an ongoing Conversational AI design discussion. The Codebase Agent persisted state only. It did not approve architecture, create WORK, edit implementation code, or change canonical documentation.

## Source References Loaded

- `docs/ai/OPERATING_MODEL.md` - execution-surface authority, drift handling, decision boundaries.
- `docs/ai/KNOWLEDGE_MAP.md` - routing map used to load Game/domain and process context.
- `docs/ai/protocols/CONVERSATIONAL_ORCHESTRATOR.md` - initiative workspace model and natural continuation behavior.
- `docs/ai/workspaces/README.md` - required active workspace semantics and resume header.
- `docs/ai/processes/ARCHITECTURE_DISCUSSION.md` and `docs/ai/protocols/ARCHITECTURE_DISCUSSION.md` - current process guidance.
- `docs/decisions/architecture/ADR-0002-game-capability-persistence-transaction-boundary.md` - accepted Game Management / Session Runtime persistence and transaction boundary.
- `game/README.md` - canonical Game bounded-context and capability responsibilities.
- `game/CURRENT_STATE.md`, `game/docs/DATA_MODEL.md`, and `game/docs/FLOWS.md` - current implementation state references.
- `game/session/workflows/sessionlifecycle/step_create_room.go` - current `CreateRoom` stub accepts externally supplied `engine.Program`.
- `game/session/workflows/sessionlifecycle/step_join_room.go` - current `JoinRoom` stub.
- `game/session/workflows/sessionlifecycle/internal/repo/step_create_room.go` - incomplete repo scaffold.

## Accepted Constraints

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

Recommended resolution: do not fix this in the architecture checkpoint. Route it into later Feature Development/WORK after the first lifecycle/runtime architecture decisions are accepted.

Related documentation note: `game/CURRENT_STATE.md` currently says no known drift, while ADR-0002 and the inspected `CreateRoom` stub reveal this drift. This workspace records the drift but does not update current-state documentation because canonical/current-state docs were excluded from this handoff.

## Proposed Architecture For Human Review

Status: proposed, not accepted.

- Treat Session Runtime as process-stateless and reconstructible, but as the owner of durable authoritative session/runtime state.
- Keep transport/network connections outside Session Runtime.
- Explore a logical Live Session Coordinator between the WebSocket/application edge and Session Runtime. It would own ephemeral connection bindings, actual delivery, and real scheduling mechanisms, but not authoritative business/session truth.
- Do not make server/session affinity or sticky routing a correctness invariant. Initial implementation should exploit the current modular-monolith/single-process simplicity. Distributed routing, Redis, separate process deployment, or similar machinery is deferred until an actual scaling/deployment requirement exists.
- Durable timer obligations may need to belong to Session Runtime even if physical timers are scheduled by the coordinator, so process crashes do not silently change game semantics.
- Challenge adding a cross-session Player master entity inside Session Runtime. Preferred direction is session-scoped Participant records containing an opaque external actor/user UUID plus a display-name snapshot; Identity/Profile ownership remains external.
- Do not assume the session creator/host is necessarily a playing participant.
- `program.Definition.Players` is already the source of truth for authored min/max player constraints; avoid duplicating those values as a second source of truth without a later concrete reason.
- Likely lifecycle shape to explore: lobby/running/terminal plus a terminal reason, rather than only pending/running/finished or a large status explosion.
- Session expiration should be enforceable from durable timestamps even if asynchronous cleanup has not run.
- Runtime execution must define session-level serialization/concurrency because the Game Language engine intentionally does not serialize concurrent `Step` calls.
- A Runtime Turn must eventually define how an external signal, engine `Step`, `InternalSignals`, persisted Snapshot, ordered outputs, idempotency, and transaction boundaries interact.
- Completed-session history/archive ownership is still unresolved canonically and should be routed to Domain Design later rather than silently decided now.
- History/archive, disconnect/reconnect semantics, abuse/runaway-session policy, crash recovery, and distributed deployment are part of the initiative but should be sequenced rather than all implemented in the first slice.

## Suggested Design Sequence

1. Fundamental runtime ownership/state/failure/concurrency model.
2. Lobby lifecycle: create, join, leave, expiration, start, participants.
3. Runtime-turn contract: incoming signals -> engine execution -> durable state -> outputs.
4. Disconnect/reconnect, interruption/crash behavior, and abuse limits.
5. Completed-session history/archive ownership and persistence strategy, entering Domain Design when appropriate.
6. Initiative-level implementation planning followed by just-in-time Feature Development/WORK slices.

## Immediate Human Decisions Needed

- Whether to approve the boundary model: Session Runtime owns durable authoritative session/runtime state, while transport/network connections and ephemeral delivery/scheduling state live outside it.
- Whether to introduce the logical Live Session Coordinator concept now as a design boundary while keeping the first implementation in-process and simple.
- Whether to approve the default scaling stance: no sticky routing, distributed routing, Redis, or independent Session Runtime deployment as a correctness requirement for the initial implementation.
- Whether to approve session-scoped Participants with opaque external actor/user IDs and display-name snapshots, while rejecting a cross-session Player master entity inside Session Runtime for now.
- Whether lobby/running/terminal plus terminal reason is the right lifecycle direction to explore next.
- Whether durable timer obligations belong to Session Runtime even when physical scheduling is delegated.
- Whether to require an explicit session-level serialization/concurrency model before any runtime-turn implementation.

## Deferred / Out Of Scope For First Checkpoint

- Creating WORK.
- Implementation code changes.
- Migration/schema changes.
- Canonical ADR/domain/current-state documentation changes.
- Detailed lobby API contracts.
- Detailed runtime-turn persistence contract.
- Completed-session archive ownership decision.
- Distributed deployment/routing design.
- Disconnect/reconnect and abuse policy details.
