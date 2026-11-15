# SESSION-ADR-0028: Real-Time Session Runtime Supersedes The Discrete-Turn Architecture

Status: ACCEPTED
Created: 2026-10-03
Last status change: 2026-10-03
Supersedes: ADR-0015, ADR-0016 (both archived), and, as an operative design, SESSION-ADR-0001 through SESSION-ADR-0027 (archived)
Superseded by: None

## Context

Session Runtime was designed, and partly implemented, for discrete games: games whose interactions are individual, fixed events (a card played, an answer submitted). That assumption shaped the whole runtime:

- every interaction was an authoritative, durable turn: its inputs were persisted, and the session state was read from the database for each one;
- the authored JavaScript backend script was loaded from storage and interpreted again for every interaction, a cost (on the order of 100 ms) acceptable only because card-style games produce few events per second;
- the JavaScript executor was a separately deployed service reached over gRPC;
- Session Runtime was the deterministic, authoritative record of every interaction.

Playhoot now also needs to support continuous games, where an interaction is not only a single event but can be a continuous stream such as movement. Event rates of many per second make a database read, a script load and a script interpretation per interaction unworkable. The authored-script approach adopted when the project moved to JavaScript is what makes this feasible, so JavaScript scripts are kept; the runtime around them cannot be.

## Decision

Session Runtime is rebuilt as a real-time runtime. The accepted direction is:

- Playhoot supports both discrete and continuous games (see PDR-0001).
- Each live session has its own real-time controller that handles that session's messages in memory and keeps an open connection with the session's script, so the script is not loaded from storage on every interaction.
- The JavaScript executor is no longer a separate service: no gRPC, no separately deployed executor. Script execution is part of Session Runtime's own process.
- Session Runtime's persistence is no longer the authoritative, deterministic record of every interaction; its role is reduced. The per-interaction durable turn log, per-interaction database reads and replay-based recovery of the previous design are retired.
- The authored JavaScript scripts are reused. The kinds of events a script can interact with change.

The following are deliberately not decided here and are each decided by a later record: the controller's concurrency and lifecycle model, what (if anything) is persisted and when, recovery after a process failure, the script/event protocol, the transport between clients and the controller, multi-instance behavior, and whether the frontend iframe contract and private object storage of game version content carry over.

## Rationale

The discrete architecture's costs are inherent to its core premise (durable, replayable, one-turn-per-interaction), not incidental, so they cannot be tuned away for continuous interaction. Keeping the premise while adding a continuous mode would force two architectures into one runtime. The project is not deployed, so there is no migration cost: the previous implementation is removed rather than adapted.

## Alternatives Considered

### Add a continuous mode beside the discrete turn architecture

Rejected: every continuous interaction would still need a path that avoids the per-interaction read, persist and script load, which is a second runtime in practice.

### Keep the executor as a separate service and optimize it

Rejected: a network hop per interaction adds latency that the open-connection-per-session model removes, and the isolation reason for the separate deployment is not decided to outweigh that.

## Consequences

- The implemented Session Runtime code is removed. The directory layout, migration mechanism, application wiring, API edge and the sandboxed script execution helpers are kept as structure for the new design. The removed code remains available in Git at tag `pre-realtime-pivot`.
- Open projects of the previous design are cancelled at their current state and archived with the rest of the previous documentation (`docs/archive/pre-realtime-pivot/`), which is history only.
- Domain-split decisions that do not depend on the discrete model (ADR-0014's separation of Game Management and Session Runtime) are unaffected.

## Canonical Knowledge Impact

- `session/README.md` - Session Runtime is described as being redesigned; the previous model is no longer described.
- `ARCHITECTURE.md` - Session Runtime no longer depends on a separately deployed JavaScript executor.
- `docs/archive/pre-realtime-pivot/README.md` - the previous documentation is frozen as history.

## Implementation Impact

Removal of the previous implementation (done together with this record). New design and work are defined by later records and WORK specifications.
