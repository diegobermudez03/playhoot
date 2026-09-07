# Session Runtime V1 Architecture Checkpoint

Process: Architecture Discussion

Status: first checkpoint approved; next checkpoint pending.

## Approved First Checkpoint Decisions

The human approved these architecture decisions for the Session Runtime initiative:

1. Session Runtime owns all durable authoritative state that determines the meaning of a session and must be reconstructible after loss or restart of the process.
2. "Stateless" means process-stateless/reconstructible, not that Session Runtime has no durable domain state.
3. `Live Session Coordinator` is a conceptual responsibility boundary outside Session Runtime.
4. The Coordinator owns ephemeral/runtime mechanisms: live connection bindings, delivery/fan-out, physical disconnect detection, and physical timer/scheduling mechanisms.
5. The Coordinator does not own authoritative session/game truth or business consequences.
6. The Coordinator boundary does not require another deployed service. V1 may keep it in the same Go process.
7. V1 should not introduce sticky-session correctness requirements, Redis, distributed session routing, distributed locks, or other multi-instance mechanisms.
8. The initial single-process modular-monolith deployment may use local maps, channels, and Go timers for ephemeral mechanisms.
9. Session correctness must not depend exclusively on ephemeral objects, so the architecture remains evolvable toward multiple processes or instances later.
10. Timer obligations that can affect game semantics are durable Session Runtime state.
11. The Coordinator owns the physical timer and notices elapsed wall-clock time; Session Runtime owns the durable timer obligation and decides the semantic consequence.
12. On timer expiry, the Coordinator calls Session Runtime with the corresponding expiration signal/event. Session Runtime decides what happens, normally through its authoritative runtime/engine flow.
13. A process crash may destroy physical timers but must not silently erase timer obligations or change game semantics. Pending or overdue obligations must be reconstructible/recoverable from durable state.

## Canonical Promotion Note

These are material architecture decisions. This workspace records the approved checkpoint, but it is not canonical architecture documentation or implementation authority.

Per the Architecture Discussion protocol, the next Conversational AI step should determine and route any required ADR/canonical documentation promotion before implementation WORK relies on these decisions.

No canonical ADR, architecture, domain, engineering-standard, current-state, implementation, migration, or WORK files were updated by this checkpoint persistence step.

## Deferred Design Topic

Disconnect / reconnect semantics remain deferred. They are not an accepted feature design yet.

Important distinction:

```text
physical connection state != logical session participation state
```

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

- Session Runtime has persisted schema and lifecycle scaffolding, but no implemented create/join/start/execution flow.
- `CreateRoom` and `JoinRoom` are stubs.
- Current `CreateRoom` still accepts an externally supplied `engine.Program`.

DRIFT DETECTED: ADR-0002 says the externally supplied `engine.Program` is no longer the accepted Session Runtime contract. This should be fixed later through governed implementation work, not during this architecture checkpoint persistence step.

## Next Architecture Milestone

Define the durable Session/Participant model and lobby lifecycle foundations before designing detailed Create/Join/Leave/Start operations.

Focus especially on:

- participant identity/ownership model;
- creator/host relationship;
- lifecycle phases and terminal reasons;
- expiration semantics.

## Questions For The Next Conversation

1. What is the durable identity model for a session participant, given that Identity/Profile ownership stays outside Game?
2. Is the creator/host always a participant, optionally a participant, or a separate role from participation?
3. What lifecycle phases does Session Runtime need at the durable model level?
4. Which terminal reasons are semantically meaningful enough to persist?
5. How should lobby/session expiration be represented durably and enforced when cleanup has not run?
6. Which player-count constraints come only from `program.Definition.Players`, and which session/lobby facts, if any, deserve their own durable fields?

## Still Out Of Scope

- Creating WORK.
- Implementation code changes.
- Database migrations.
- Canonical ADR/domain/current-state documentation updates in this handoff.
- Detailed Create/Join/Leave/Start operation contracts.
- Full runtime-turn persistence schema.
- Final disconnect/reconnect contract.
- Completed-session archive ownership.
- Distributed deployment/routing design.
