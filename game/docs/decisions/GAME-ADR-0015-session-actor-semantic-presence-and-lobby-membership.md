# GAME-ADR-0015: Session Actor Semantic Presence and Phase-Dependent Participation Consequences

Status: ACCEPTED
Created: 2026-09-07
Last status change: 2026-09-07
Supersedes: None
Superseded by: None

## Context

GAME-ADR-0010 accepted the transport/platform/gameplay three-way boundary for disconnect and reconnect, and explicitly rejected persisting physical connection state (`is_connected`, `connection_id`, `websocket_id`) on Session Runtime entities. GAME-ADR-0011 accepted `UserDisconnected`/`UserReconnected` as authored Game Language signals delivered after the Coordinator's transport grace expires. Neither record decided two things this checkpoint closes: whether Session Runtime durably tracks *that* a semantic disconnect/reconnect boundary was crossed (as opposed to only reacting to the transient notification), and what a semantic disconnect concretely does to lobby participation versus running-game membership.

Without an accepted answer, two failure modes were possible: collapsing "has this actor crossed the Session-level connected/disconnected boundary" into the same concept as "does this actor currently occupy a participant/roster position" (which are legitimately different questions - an actor can be semantically connected but locked out of a full lobby), or inventing ad hoc, undocumented behavior differences between LOBBY and RUNNING the first time an implementer had to decide what a semantic disconnect does.

## Decision

### `session_actors.semantic_presence` is durable, `CONNECTED | DISCONNECTED`

Session Runtime persists a durable field on `session_actors`, conceptually named `semantic_presence` (not `is_connected`, to avoid implying physical socket presence), with values `CONNECTED | DISCONNECTED`. It answers only "has this Session actor crossed the Coordinator -> Session semantic connected/disconnected boundary?" - a platform/runtime-level fact distinct from `session_participants.active`, which answers "does this actor currently occupy/admit a participant position?" These values may legitimately diverge (for example: an actor reconnects successfully, but the lobby is already full, so it is semantically `CONNECTED` while its Participant remains inactive).

This does not reopen GAME-ADR-0010's rejection of physical connection-state persistence. Socket IDs, connection IDs, live WebSocket presence, Coordinator bindings, grace timers, and process IDs remain ephemeral Coordinator-owned state and are not persisted anywhere in Session Runtime. `semantic_presence` is a coarser, already-debounced platform fact - it changes only after the Coordinator's transport grace has already been applied and has already expired (see GAME-ADR-0010) - not a raw connection flag.

### Semantic disconnect is a serialized, idempotent SessionActor transition

After Coordinator grace expires, the Coordinator reports the semantic disconnect using trusted Session identity. Session Runtime serializes the `CONNECTED -> DISCONNECTED` SessionActor transition per Session (consistent with the existing accepted per-Session serialization for LOBBY, GAME-ADR-0004; RUNNING per-Session serialization is unchanged by this record). Repeated disconnect reports while already `DISCONNECTED` are no-op/idempotent with respect to `semantic_presence` and must not duplicate any Game Language disconnect effect (consistent with GAME-ADR-0011's no-RuntimeTurn-for-a-rejected-signal rule).

The consequence of the transition depends on Session phase.

### LOBBY: semantic disconnect deactivates participation

In `LOBBY`, after semantic disconnect: the SessionActor becomes `DISCONNECTED`; if it currently has an active `SessionParticipant`, that participation becomes inactive and its slot is released; the disconnected user is no longer counted toward current lobby participant capacity; no Game Language `UserDisconnected` is delivered, because the game runtime has not started and there is no root workflow instance yet. This is intentionally stronger than RUNNING behavior (below): a player still inside Coordinator transport grace is still semantically `CONNECTED` and keeps their active lobby slot, since grace absorption already happens before Session Runtime ever observes the disconnect.

### LOBBY reconnect re-validates current admission; slot recovery is not guaranteed

Reconnect reuses the same SessionActor and the same existing `SessionParticipant` record where present - it is not a new Join identity. On successful semantic reconnect, `DISCONNECTED -> CONNECTED`. If the prior lobby disconnect made the Participant inactive, participation must be re-admitted against *current* lobby constraints under the normal Session serialization, validating at least: Session still in `LOBBY`; lobby not semantically expired; current participant capacity against the pinned Game Definition `players.max` (GAME-ADR-0004); and other applicable accepted admission constraints. If admission succeeds, `Participant.active = true`. If it fails, the SessionActor may remain semantically `CONNECTED` while the Participant remains inactive and no slot is reserved - reconnect does not guarantee recovery of a previously released lobby slot (for example: P1 disconnects and releases the last slot; P2 joins and occupies it; P1 reconnects but the lobby is full and cannot reactivate).

### Start uses active Participants only, decided by existing Session serialization

Start (GAME-ADR-0004) already serializes the Session. At that authoritative, serialized moment: lobby expiration is evaluated; current active Participants are collected; accepted `players.min`/`players.max` constraints are validated; and the Game Language root `players: list<user>` (GAME-ADR-0006) is built from those active Participants only. A SessionActor that is `DISCONNECTED` (and therefore inactive from a lobby disconnect) or `CONNECTED` but not currently re-admitted as an active Participant is not included. Whether a concurrent physical disconnect actually excludes a player from Start depends only on whether Coordinator grace or Start wins the existing per-Session serialization (GAME-ADR-0004) - if grace has not yet expired when Start wins serialization, the Participant is still active and is included; if the semantic disconnect was already processed first, it is excluded. This is not special-cased outside Session; the existing serialization already decides it.

### Reconnect after Start-time exclusion does not join the running game

If an actor was not an active Participant when Start finalized the roster, reconnecting afterward does not add that actor to the running game. Session Runtime must not deliver `UserReconnected` toward the Game Language runtime for a SessionActor that was never included in that execution's `players` roster, and must not dynamically add the actor to the game. Post-Start late admission is a distinct, separately deferred capability and is not designed by this record.

### RUNNING: semantic disconnect preserves runtime membership

Once a Participant was included in the Game runtime roster at Start, a semantic disconnect during `RUNNING` does not deactivate or remove that Participant from the Session runtime roster. Instead: `semantic_presence` becomes `DISCONNECTED`; the `SessionParticipant` remains logically part of the running Session; Session Runtime may emit the already-accepted root `UserDisconnected(user)` signal (GAME-ADR-0011); and authored Game Language determines gameplay consequences. Session Runtime must not free a runtime roster slot, silently remove the player, auto-forfeit, or auto-skip, or otherwise mutate authored gameplay state merely because of the semantic disconnect. Reconnect of such a runtime member may produce the already-accepted `UserReconnected(user)` transition (GAME-ADR-0011) before resync (GAME-ADR-0010).

### Concept boundary

Three concepts must not collapse into one: `SessionActor.semantic_presence` (semantic connectivity to the Session, durable, phase-independent); `SessionParticipant.active` during `LOBBY` (currently admitted participant / currently occupying a Start-eligible lobby slot); and the Game runtime roster after Start (the immutable initial set of Session actors selected from active Participants at Start, unless a future, separately approved Game Language feature introduces dynamic membership).

## Rationale

Naming the field `semantic_presence` rather than `is_connected` keeps the already-accepted GAME-ADR-0010 boundary intact: the field records a platform-level fact about crossing the Session-level connected/disconnected boundary, not a physical socket/transport fact, which remains entirely Coordinator-owned and ephemeral. Separating it from `SessionParticipant.active` is necessary because the two questions are independently answerable in at least one concrete, non-hypothetical case (reconnect into a full lobby); collapsing them would either wrongly reactivate a slot that current capacity cannot support, or wrongly discard the platform-level fact that the actor is, in fact, connected again.

Making LOBBY disconnect release participation (stronger than RUNNING) reflects that a lobby slot is a scarce, currently-contested resource other prospective joiners are waiting on, while a RUNNING roster position was already committed at Start and has no equivalent "other players are waiting to take it" pressure - GAME-ADR-0011 already established that RUNNING disconnect delegates consequence entirely to authored Game Language rather than an automatic platform removal, and this record keeps that RUNNING behavior unchanged.

Not guaranteeing lobby slot recovery on reconnect avoids inventing a reservation/priority mechanism (e.g., holding a phantom slot for a disconnected user against other joiners) that was never accepted and that would silently reduce effective lobby capacity for everyone else. Excluding Start-time-excluded actors from later joining the running game preserves the already-accepted immutable initial roster contract (GAME-ADR-0006) and avoids introducing dynamic runtime membership as an unreviewed side effect of this checkpoint.

Relying on the existing per-Session serialization (GAME-ADR-0004) to resolve the grace-vs-Start race avoids introducing a second, bespoke concurrency mechanism solely for this interaction; the same lock that already prevents other lobby race conditions naturally decides which side committed first.

## Alternatives Considered

### Name the field `is_connected`

Rejected. It reads as a physical connection flag, which GAME-ADR-0010 already rejected persisting, and would confuse future readers about what the field actually represents.

### Collapse `semantic_presence` and `Participant.active` into one field

Rejected. They are independently observable in the reconnect-into-a-full-lobby case; collapsing them would force an incorrect answer to one of the two questions.

### Treat RUNNING disconnect the same as LOBBY disconnect (free the roster slot)

Rejected. It would silently override GAME-ADR-0011's decision that RUNNING disconnect consequences belong to authored Game Language, not an automatic platform removal.

### Guarantee lobby slot recovery on reconnect

Rejected. It would require reserving capacity against other prospective joiners for an indefinite/undecided period, which was never accepted and effectively reduces usable lobby capacity.

### Allow reconnect after Start-time exclusion to join the running game

Rejected. It would introduce dynamic runtime membership - a materially separate, unreviewed capability - as an unintended side effect of reconnect handling, contradicting the accepted immutable initial roster contract (GAME-ADR-0006).

### Special-case the grace-versus-Start race outside normal Session serialization

Rejected. The existing per-Session serialization (GAME-ADR-0004) already resolves ordering between concurrent Session-mutating operations; a bespoke second mechanism would duplicate that guarantee for no additional correctness benefit.

## Consequences

- `session_actors` gains a durable `semantic_presence` column (`CONNECTED | DISCONNECTED`), distinct from and never collapsed with `session_participants.active`.
- No physical connection/grace/process state is added to Session Runtime; GAME-ADR-0010's rejection of `is_connected`/`connection_id`/`websocket_id` persistence is unchanged.
- Future LOBBY disconnect implementation must deactivate the Participant and release its slot; future LOBBY reconnect implementation must re-validate current admission constraints rather than unconditionally reactivating the Participant.
- Future Start implementation must select the roster strictly from Participants active at the serialized Start moment; no special-case branch is needed for a concurrent grace-vs-Start race beyond the existing per-Session serialization.
- Future reconnect-delivery implementation must not emit `UserReconnected` toward the engine for a SessionActor absent from the current execution's `players` roster.
- Future RUNNING disconnect implementation must leave the runtime roster/Participant untouched and only optionally emit `UserDisconnected` to authored Game Language.
- Semantic presence recovery after total process loss (whether a restarted Coordinator gives durably-`CONNECTED` actors a fresh recovery grace, and when absence of a new binding becomes a new semantic disconnect) remains open and is the next architecture milestone; it is not decided here.

## Canonical Knowledge Impact

- `game/README.md` - updates the Session Runtime lifecycle section's connection-state paragraph to introduce durable `semantic_presence` alongside the unchanged rejection of physical connection-state persistence; updates the Lobby Lifecycle Contract section with LOBBY disconnect/reconnect admission semantics and the Start active-Participants-only roster rule; updates the Disconnect/Reconnect Boundary section with the LOBBY-vs-RUNNING phase-dependent consequence and the Start-exclusion-permanence rule.
- `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` - adds `session_actors.semantic_presence` to the accepted ER diagrams and a new section explaining it, and clarifies `session_participants.active`'s concrete lobby/admission meaning.

## Implementation Impact

Future implementation must add the `semantic_presence` column/migration, the serialized SessionActor transition path, LOBBY disconnect/reconnect admission logic, the Start roster query restricted to active Participants, the RUNNING disconnect/reconnect delivery path respecting immutable Start-time exclusion, and Coordinator wiring for the grace-expiration report. No production code, migration, Coordinator logic, Game Language change, tests, or WORK is authorized by this record.
