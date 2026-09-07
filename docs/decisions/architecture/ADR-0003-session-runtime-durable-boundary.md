# ADR-0003: Session Runtime Durable Boundary, Live Coordinator, and V1 Scaling

Status: ACCEPTED
Created: 2026-09-06
Last status change: 2026-09-06
Supersedes: None
Superseded by: None

## Context

Session Runtime is an accepted internal capability of the Game bounded context, and ADR-0002 already gives it independent persistence and transaction ownership from Game Management. The next design pressure is what Session Runtime must own durably versus what can remain process-local while Playhoot starts as a modular monolith.

Live multiplayer sessions need connection delivery, fan-out, disconnect detection, timer scheduling, recovery behavior, and eventually a path toward more than one process or instance. Adding distributed machinery now would overfit a future deployment problem, but treating local maps, channels, or timers as the only source of session truth would make crash recovery and later scaling unsafe.

## Decision

Session Runtime owns all durable authoritative state that determines the meaning of a session and must be reconstructible after loss or restart of the process.

"Stateless" means process-stateless/reconstructible. It does not mean the domain has no state.

Introduce the conceptual responsibility boundary `Live Session Coordinator` outside Session Runtime. The Coordinator owns ephemeral/runtime mechanisms such as:

- live connection bindings;
- delivery/fan-out to connected clients;
- detection of physical disconnects;
- physical timer/scheduling mechanisms.

The Coordinator does not own authoritative session/game truth or business consequences.

This is a responsibility boundary, not a requirement to create another deployed service. V1 may keep it in the same Go process.

V1 does not introduce sticky-session correctness requirements, Redis, distributed session routing, distributed locks, or other multi-instance mechanisms. The initial single-process modular-monolith deployment may use local maps, channels, and Go timers for ephemeral mechanisms.

Correctness of a session must not depend exclusively on those ephemeral objects, so the architecture remains evolvable toward multiple processes or instances later.

Timer obligations that can affect game semantics are durable Session Runtime state. The Coordinator owns the physical timer and knowledge that wall-clock time has elapsed. Session Runtime owns the durable timer obligation and decides the semantic consequence.

When a timer expires, the Coordinator calls Session Runtime with the corresponding expiration signal/event. Session Runtime decides what happens, normally through its authoritative runtime/engine flow. A process crash may destroy physical timers but must not silently erase timer obligations or change game semantics. Pending or overdue obligations must be reconstructible/recoverable from durable state.

## Rationale

This preserves a small V1 implementation shape while keeping the domain model honest. Local maps, channels, and Go timers are useful process mechanics in the modular monolith, but they are not acceptable as the sole source of facts that change session meaning. Making Session Runtime reconstructible from durable state gives crash recovery and future scaling a place to stand without paying for distributed coordination before it is needed.

Separating the Coordinator from Session Runtime keeps transport and physical runtime concerns out of authoritative session truth. It also prevents an accidental architecture where WebSocket handlers or timer goroutines become the business owner of game consequences.

## Alternatives Considered

### WebSocket/application edge owns live session state directly

Rejected. It is simple initially, but it mixes transport concerns with authoritative session truth and makes recovery, reconnect, and future multi-process routing harder to reason about.

### Add Redis, distributed routing, or distributed locks in V1

Rejected. There is no current deployment or scaling requirement that justifies distributed-system complexity. V1 should remain a modular monolith while preserving durable correctness boundaries.

### Make sticky session routing a correctness invariant

Rejected. It would make process placement part of the semantic contract and make later recovery or scaling more fragile. Process-local mechanisms are allowed as optimizations/convenience, not as the only source of truth.

### Treat timers as purely process-local

Rejected for semantic timers. A timer that affects game meaning cannot disappear because the process restarted. Physical timer machinery may be ephemeral, but the obligation to react to time must be recoverable.

## Consequences

- Session Runtime design and implementation must distinguish durable authoritative state from ephemeral process mechanisms.
- A logical Coordinator boundary may exist inside the same Go process for V1.
- V1 implementation work may use local maps, channels, and Go timers for ephemeral coordination.
- V1 implementation work must not rely on Redis, distributed locks, sticky routing, or multi-instance session routing for correctness.
- Timer recovery and overdue timer handling must be considered when timer obligations affect game semantics.
- Disconnect/reconnect semantics remain a deferred design topic; physical connection presence is not the same as logical participation.

## Canonical Knowledge Impact

- `game/README.md` - adds the accepted Session Runtime durable-state boundary, Live Session Coordinator responsibility boundary, V1 scaling stance, and durable timer-obligation rule.

## Implementation Impact

Downstream implementation must model reconstructible durable Session Runtime state and keep ephemeral coordination outside authoritative session truth. No implementation, migration, distributed infrastructure, or WORK is authorized by this record.
