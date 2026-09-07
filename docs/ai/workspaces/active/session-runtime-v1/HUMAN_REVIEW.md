# Session Runtime V1 Architecture Checkpoint

Process: Architecture Discussion

Checkpoint: first architecture milestone before detailed lobby/runtime design.

Status: proposed, not accepted.

## Problem

Playable multiplayer sessions are a core initial product capability, but Session Runtime is currently scaffolding. The next design step should set the runtime ownership, state, failure, and concurrency boundaries before turning lobby or execution behavior into WORK.

The checkpoint should stay deliberately narrow: approve the architectural boundary and sequencing, then continue into detailed lobby/runtime design. It should not become a full implementation specification yet.

## Current Accepted Constraints

- Game is one bounded context containing Game Management and Session Runtime as internal capabilities.
- Game Management and Session Runtime have independent persistence and transaction ownership.
- Session Runtime obtains immutable definition/version information through a narrow Game Management read capability.
- Sessions are pinned to a stable execution definition/version.
- Transport/network connections are outside Game ownership.
- Identity/profile ownership is outside Game ownership.
- Completed-session history/archive ownership is unresolved.

Source: `game/README.md` and `docs/decisions/architecture/ADR-0002-game-capability-persistence-transaction-boundary.md`.

## Current Implementation Facts

- Session Runtime has persisted schema and lifecycle scaffolding, but no implemented create/join/start/execution flow.
- `CreateRoom` and `JoinRoom` are stubs.
- Current `CreateRoom` still accepts an externally supplied `engine.Program`.

DRIFT DETECTED: ADR-0002 says the externally supplied `engine.Program` is no longer the accepted Session Runtime contract. This should be fixed later through governed implementation work, not during this checkpoint.

## Recommended Boundary

Treat Session Runtime as process-stateless/reconstructible but owner of durable authoritative session/runtime state.

Keep transport/network connections outside Session Runtime. Introduce a logical Live Session Coordinator between the WebSocket/application edge and Session Runtime:

- owns ephemeral connection bindings;
- performs actual message delivery;
- performs physical scheduling of timers or wakeups;
- does not own authoritative business/session truth.

Session Runtime should own durable facts that affect game semantics:

- session lifecycle state;
- participants;
- pinned game definition/version;
- persisted runtime snapshot/state;
- durable timer obligations if timers affect game meaning;
- ordering/concurrency guarantees for a session.

## Scaling Stance

Do not make server/session affinity or sticky routing a correctness invariant.

For the initial modular-monolith/single-process implementation, exploit the simple in-process shape. Defer distributed routing, Redis, separate process deployment, or similar machinery until there is an actual scaling or deployment requirement.

This keeps the model honest: durable state and ordering rules provide correctness; process-local helpers provide convenience.

## Participant Model

Challenge a cross-session Player master entity inside Session Runtime.

Preferred direction: session-scoped Participant records containing:

- an opaque external actor/user UUID;
- a display-name snapshot;
- session participation/lifecycle fields.

Identity/Profile ownership remains external. Do not assume the session creator/host is necessarily a playing participant.

## Lifecycle Direction

Explore a compact lifecycle:

```text
lobby -> running -> terminal
```

Use a terminal reason rather than expanding the top-level status into many finished/cancelled/expired/abandoned variants too early.

Session expiration should be enforceable from durable timestamps even if asynchronous cleanup has not run.

## Runtime Turn Direction

Before implementing game execution, define the Runtime Turn contract:

```text
incoming external signal
-> session-level ordering/idempotency check
-> engine Step
-> internal signals handling
-> persisted Snapshot/state transition
-> ordered outputs for delivery
```

The Game Language engine intentionally defines one deterministic `Step` in isolation; it does not serialize concurrent calls for a live session. Session Runtime must own that ordering/concurrency model.

## Alternatives

Alternative: let WebSocket handlers own session state directly.

Tradeoff: simple for a prototype, but it mixes transport concerns with authoritative session truth and makes crash recovery/reconnect/distributed routing harder to reason about.

Alternative: add Redis/distributed routing now.

Tradeoff: prepares for scale, but adds operational complexity before the deployment problem exists. It should remain deferred.

Alternative: put a Player master entity inside Session Runtime.

Tradeoff: tempting for queries and reuse, but it pulls identity/profile ownership into Game. Session-scoped participants keep the boundary cleaner.

Alternative: persist authored min/max player limits onto the session.

Tradeoff: may help historical/debug display later, but `program.Definition.Players` is already the authored source of truth. Avoid creating a second source of truth without a concrete reason.

## Decisions Requested

1. Approve or revise the boundary: Session Runtime owns durable authoritative session/runtime state; transport connections and ephemeral delivery/scheduling state live outside it.
2. Decide whether to use the logical Live Session Coordinator concept now, while keeping implementation in-process initially.
3. Approve or revise the scaling stance: no sticky routing, distributed routing, Redis, or independent Session Runtime deployment as a correctness requirement for the first implementation.
4. Approve or revise the participant direction: session-scoped Participants with opaque external actor/user UUIDs and display-name snapshots; no cross-session Player master entity inside Session Runtime for now.
5. Decide whether `lobby/running/terminal + terminal reason` is the lifecycle shape to explore in the next design step.
6. Decide whether durable timer obligations belong to Session Runtime when timers affect game semantics.
7. Confirm that session-level serialization/concurrency must be explicitly designed before runtime-turn implementation.

## Proposed Sequence After This Checkpoint

1. Fundamental runtime ownership/state/failure/concurrency model.
2. Lobby lifecycle: create, join, leave, expiration, start, participants.
3. Runtime-turn contract: incoming signals -> engine execution -> durable state -> outputs.
4. Disconnect/reconnect, interruption/crash behavior, and abuse limits.
5. Completed-session history/archive ownership and persistence strategy, routed to Domain Design when appropriate.
6. Initiative-level implementation planning, then just-in-time Feature Development/WORK slices.

## Out Of Scope For This Checkpoint

- Creating WORK.
- Implementation code changes.
- Database migrations.
- Canonical ADR/domain/current-state documentation updates.
- Detailed endpoint/use-case contracts.
- Full runtime-turn persistence schema.
- Completed-session archive ownership.
- Distributed deployment/routing design.
