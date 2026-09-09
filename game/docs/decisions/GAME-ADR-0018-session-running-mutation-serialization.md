# GAME-ADR-0018: Session RUNNING Mutation Serialization

Status: ACCEPTED
Created: 2026-09-08
Last status change: 2026-09-08
Supersedes: None
Superseded by: None

## Context

GAME-ADR-0004 accepts that LOBBY-phase mutations (Join, Leave, Start, lobby-expiration materialization) serialize per Session, but explicitly scopes that guarantee to LOBBY only and defers RUNNING-phase concurrent-input serialization as a separate topic. GAME-ADR-0007 accepts `RuntimeTurn` as the atomic historical/transactional unit for RUNNING-phase execution, and GAME-ADR-0013 accepts that an uncommitted RuntimeTurn transaction rolls back entirely on process death - but neither record says what happens when two independent causes capable of producing a RuntimeTurn for the *same* Session arrive concurrently (for example, a player answers an open interaction at nearly the same instant a timer for that same interaction expires).

Without an accepted answer, an implementation could plausibly let two RuntimeTurns execute concurrently against the same Session, each reading the same starting Snapshot and each attempting to commit a divergent "next" state - a correctness hazard, not merely a performance one, since Game Language execution assumes it is transforming one authoritative prior Snapshot into one authoritative next Snapshot. GAME-ADR-0002/GAME-ADR-0013 already reject process ownership, sticky routing, and distributed-locking infrastructure as V1 mechanisms for unrelated concerns (recovery, actor lifecycle); this record settles whether RUNNING-phase correctness needs any of that machinery, or whether the same simple mechanism already accepted for LOBBY extends naturally.

## Decision

### RUNNING mutations serialize per Session

The LOBBY serialization guarantee (GAME-ADR-0004) is extended to RUNNING. For one Session, every mutation capable of changing authoritative RUNNING-phase runtime state executes serially. This includes, without being an exhaustive enumeration: interaction responses, timer expirations, `UserDisconnected`/`UserReconnected` semantic-presence processing (GAME-ADR-0016), and any future platform/runtime signal capable of creating a RuntimeTurn or otherwise mutating authoritative Session runtime state. There must never be two RuntimeTurns concurrently executing against the same authoritative Session state. This is the same kind of guarantee GAME-ADR-0004 already accepts for LOBBY, now stated to cover the Session's entire authoritative-mutation lifetime rather than one phase.

### V1 mechanism: database-backed pessimistic locking

Session Runtime imposes this per-Session serialization through a database-backed pessimistic locking strategy inside its own transaction boundary, exactly as GAME-ADR-0004 already assumes for LOBBY ("a database row lock is the natural V1 mechanism"). Conceptually: if process A begins `AnswerInteraction` and process B begins `TimerExpired` targeting the same Session S, both contend on the same Session serialization boundary; one proceeds first; only after it commits or rolls back may the other continue against the resulting authoritative state. The exact SQL statement/table row used as the lock anchor is an implementation-planning detail, not frozen here - the accepted architectural commitment is that correctness uses DB locking, not a specific lock syntax.

This record explicitly rejects, for V1 RUNNING serialization: process ownership of a Session, sticky routing of a Session to one process/pod, an in-memory actor owning correctness for a Session, Redis or another distributed lock service, or Coordinator ownership of correctness. Coordinator remains the ephemeral delivery/connection-binding layer (GAME-ADR-0002); it must not become a second, competing source of Session mutation ordering.

### Reload current state after obtaining serialization

A runtime mutation must not read a Snapshot, then wait to obtain the Session serialization boundary, then execute against that now-possibly-stale Snapshot. Once serialization is obtained, execution must (re)load the current committed `session_runtime_state.current_turn_id` and its corresponding authoritative Snapshot before executing. Each RuntimeTurn therefore starts from the latest committed state visible at the moment serialization was actually acquired, not from whatever state happened to be visible before the caller began waiting for the lock.

### Authoritative ordering is defined by serialization, not arrival

The authoritative order of two concurrent causes is whichever one actually wins Session serialization and commits first - nothing else. Worked example: current authoritative state is Turn 40; an interaction response Q5 and a timer expiration T1 are both concurrently attempting to mutate the same Session. If Q5 wins serialization, it processes from Turn 40 and commits Turn 41; T1 then obtains serialization, processes from the now-current Turn 41, and commits Turn 42. Session Runtime must never attempt to reconstruct a different authoritative ordering from wall-clock receive timestamps, Coordinator arrival timestamps, process identity, or socket/transport timing - the persisted Turn sequence produced by serialization is the only authoritative execution order, and no other signal is consulted to second-guess it.

### Reads are not RUNNING mutations

Read-only operations such as resync (GAME-ADR-0010) do not themselves create RuntimeTurns and are not required to participate in the same per-Session serialization boundary as mutations merely to be modeled consistently. A read should observe a coherent committed state, but this record does not freeze the exact read-isolation/locking implementation beyond what repository-conventional transaction-isolation standards already require; that remains an implementation-planning concern, not an architecture decision this record needs to settle.

## Rationale

Extending, rather than re-deriving, the LOBBY serialization mechanism keeps the whole Session lifecycle under one conceptually uniform correctness strategy: a Session is a single-writer-at-a-time domain object for its entire life, whether the mutation is a lobby membership change or a RUNNING RuntimeTurn. Introducing a second, RUNNING-specific concurrency mechanism (an actor, sticky routing, a distributed lock) would mean the codebase carries two different answers to "how do we serialize a Session" depending on phase, for no correctness benefit - V1's actual concurrency volume per Session (one small group of players) does not need anything more sophisticated than the mechanism already accepted for LOBBY.

Reloading current state after acquiring serialization - rather than trusting a Snapshot read before the lock was obtained - is the only way to make the "one committed Turn = one authoritative next state" invariant (GAME-ADR-0007) actually hold under concurrency; executing against a Snapshot that might already be stale by the time the lock is granted would silently reintroduce a lost-update hazard through the back door, even though the lock itself prevented two *simultaneous* writers.

Defining authoritative ordering purely by serialization outcome - not by any timestamp - follows directly from Session Runtime's already-accepted process-agnostic stance (GAME-ADR-0013): wall-clock and arrival timestamps are inherently untrustworthy across processes/network hops (clock skew, queueing delay, retries), while "which write actually committed first under the database's own lock" is the one fact the system can be certain of. Treating persisted Turn sequence as authoritative also composes cleanly with the already-accepted crash-rollback semantics: whichever cause's transaction commits is, by definition, the one that happened.

Not requiring reads to participate in RUNNING mutation serialization avoids manufacturing artificial contention for resync, which GAME-ADR-0010 already defines as a pure projection/reconstruction capability with no gameplay side effect; forcing every resync through the same write-serialization boundary as gameplay mutations would needlessly throttle read-heavy reconnect/resync traffic for no correctness gain, provided the read observes a genuinely committed, coherent state.

## Alternatives Considered

### Actor-per-Session ownership (in-memory actor holds correctness)

Rejected. This would reintroduce exactly the process-ownership model GAME-ADR-0013 already rejected for recovery purposes (an owning process, implicit sticky affinity, a crash-recovery/handoff story for "who owns the actor now"), just scoped to RUNNING serialization instead of general recovery. It solves nothing GAME-ADR-0013 didn't already solve without it, at the cost of reintroducing the problem GAME-ADR-0013 was written specifically to avoid.

### Sticky routing (always route a given Session's traffic to the same process/pod)

Rejected. Sticky routing does not eliminate the need for a correctness mechanism (a same-process race between two goroutines handling the same Session is still possible); it only reduces cross-process contention while introducing deployment-topology coupling (load balancer/routing configuration, uneven load distribution, added failure modes on rebalance/redeploy) that V1's actual scale does not need.

### Redis or another distributed lock service

Rejected as unnecessary V1 infrastructure. GAME-ADR-0002 already commits V1 to a single-process modular-monolith deployment with no sticky-session correctness requirement; introducing a distributed lock service purely to serialize a Session whose durable state already lives in the same PostgreSQL instance duplicates a serialization primitive PostgreSQL already provides via row locking, for additional operational cost (another service to run, monitor, and reason about failure modes for) and no correctness benefit at this scale.

### Optimistic concurrency (compare-and-swap on a version column, retry on conflict)

Rejected for V1, though not implausible as a later alternative. Optimistic concurrency would require every RuntimeTurn-producing operation to handle a conflict-and-retry loop, re-running potentially expensive engine execution multiple times under contention; pessimistic DB locking, by contrast, makes the second contender simply wait, executing engine logic exactly once per eventually-committed Turn - simpler reasoning for V1's expected low per-Session contention (small player groups), at the cost of one contender briefly blocking rather than retrying.

### Coordinator decides ordering (arrival timestamps at the Coordinator layer)

Rejected. This would make Coordinator - an ephemeral, per-process, non-durable layer (GAME-ADR-0002) - the source of authoritative ordering, directly contradicting the accepted boundary that Coordinator owns delivery/connection mechanisms, not authoritative session/game truth or business consequences. It would also make ordering depend on which process happened to receive which signal first, reintroducing exactly the untrustworthy-timestamp problem this record's ordering rule avoids.

## Consequences

- Every RUNNING-phase authoritative mutation must acquire the same kind of per-Session serialization boundary already used for LOBBY before executing, and must not commit a RuntimeTurn without having done so.
- Implementation planning must choose the concrete DB locking mechanism (for example, `SELECT ... FOR UPDATE` against a `sessions`/`session_runtime_state` row, or an equivalent) but that choice does not reopen the architectural commitment made here.
- A RuntimeTurn's execution must reload `session_runtime_state.current_turn_id` and its Snapshot after acquiring serialization, never trusting a pre-lock read.
- No implementation may introduce process/pod ownership, sticky routing, an in-memory actor, or a distributed lock as a *correctness-bearing* mechanism for RUNNING serialization without a new decision record superseding this one.
- Resync and other read-only operations remain unconstrained by this specific mutation-serialization rule, subject to ordinary repository-conventional read-isolation practice.
- Authoritative-ordering questions ("which cause happened first") must always be answered by inspecting persisted Turn sequence, never by timestamp/process/socket metadata, including in future debugging/observability tooling.

## Canonical Knowledge Impact

- `game/README.md` - Session Runtime Lobby Lifecycle Contract's serialization statement is extended by a new section stating RUNNING mutations also serialize per Session using the same DB-locking mechanism, replacing the prior "high-frequency RUNNING runtime concurrency remains a later architecture topic" deferral.
- `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` - adds a short note that RUNNING-phase RuntimeTurn creation participates in the same per-Session serialization boundary as LOBBY mutations, and that a RuntimeTurn always executes against state reloaded after serialization is acquired.

## Implementation Impact

None. No DB locking mechanism, migration, transaction code, or production behavior is implemented by this record. The exact SQL lock anchor/statement is deferred to implementation planning.
