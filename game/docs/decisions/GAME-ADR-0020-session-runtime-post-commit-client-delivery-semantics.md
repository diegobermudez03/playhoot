# GAME-ADR-0020: Session Runtime Post-Commit Client Delivery Semantics

Status: ACCEPTED
Created: 2026-09-08
Last status change: 2026-09-08
Supersedes: None
Superseded by: None

## Context

Every prior Session Runtime accepted decision establishes what must be durable and when it must commit - RuntimeTurn atomicity (GAME-ADR-0007), process-agnostic crash-rollback semantics (GAME-ADR-0013), per-Session serialization for both LOBBY and RUNNING (GAME-ADR-0004, GAME-ADR-0018), the RuntimeTurn execution bound and terminal-cleanup invariant (GAME-ADR-0019), and the four-class runtime-failure taxonomy with atomic fatal materialization (GAME-ADR-0017). None of these records say what happens *after* a transaction commits successfully: specifically, what Session Runtime owes the Live Session Coordinator/WebSocket/client delivery path, and what happens if that delivery fails or the process dies immediately after commit.

Without an accepted answer, an implementation could plausibly assume the opposite of what every prior decision already implies: that a delivery failure requires "undoing" the committed Turn, or that V1 needs a generic durable outbox recording every emitted live message so it can be replayed later. Both directions contradict the domain model this initiative has already built. GAME-ADR-0002 already establishes Coordinator as the ephemeral delivery/connection-binding boundary, explicitly separate from authoritative session/game truth; GAME-ADR-0010 already gives reconnecting clients a resync capability that reconstructs current truth rather than replaying history; GAME-ADR-0013 already treats a process crash as a non-Session-domain event once a transaction has committed. This record makes the connection between those already-accepted facts and the specific question of post-commit delivery reliability explicit, so an implementation does not have to reverse-engineer it from adjacent decisions, and so a future Game Language capability with genuine external-delivery requirements is not silently assumed to be covered by the same "best effort" answer.

## Decision

### Durable commit is the authoritative correctness boundary

Once a Session Runtime transaction commits successfully, the resulting Session state is authoritative, full stop. A later failure to deliver Coordinator/WebSocket/client outputs must never roll back the RuntimeTurn, undo the Snapshot, reopen a closed interaction, undo a timer obligation, undo terminalization, or otherwise reinterpret the committed Session state in any way. Conceptually: `runtime execution -> durable Session commit -> best-effort live delivery`. The commit is the correctness boundary; everything after it is delivery, not truth.

### No generic durable client-delivery outbox in V1

V1 does not introduce a generic durable outbox solely for WebSocket messages, Coordinator fan-out, client delivery, per-client acknowledgements, replay of every emitted live message, or replay of every intermediate presentation transition. No accepted future schema such as `session_delivery_outbox`, `delivery_attempts`, persistent connection delivery offsets, or per-client ACK tables is introduced by this record, unless a future, separately-approved requirement introduces them.

### Semantic state must already be durable before delivery

Anything required for authoritative gameplay correctness or client recovery must already be represented durably before live delivery is even attempted - current RuntimeTurn/Snapshot, Session lifecycle state, SessionInteractions, SessionTimerObligations, semantic presence, terminal state, and fatal runtime diagnostic state where applicable are all already covered by prior accepted decisions. Live delivery communicates durable truth; it does not create that truth, and nothing about delivery reliability changes what must already be true in the database before delivery is attempted.

### Resync is recovery from missed delivery, not event replay

If live delivery fails, or the process dies after commit, a reconnecting client uses the already-accepted Session resync capability (GAME-ADR-0010) to reconstruct the *current* player-facing truth. V1 does not promise to reproduce every live message that was missed; resync recovers what still matters now, not a transcript of everything that happened. Worked example: Turn 42 commits and opens interaction Q17; the process dies before Q17 is sent over WebSocket; after reconnect, Q17 is still an `ACTIVE` durable `SessionInteraction` and appears in the player's current resync projection - no generic delivery outbox is required for this to be correct.

Client recovery in V1 is "recover current truth," not "replay every event/message since sequence N." The already-accepted RuntimeTurn `sequence` may still identify the version a resync projection represents (GAME-ADR-0010); this record does not introduce a guaranteed client event log/replay protocol.

### Timer scheduling failure after commit does not roll back the Turn

Physical timer scheduling remains Coordinator-owned and ephemeral (GAME-ADR-0002, GAME-ADR-0008). If a Turn commits a durable, active `SessionTimerObligation` but the process dies before the in-memory physical timer is successfully established, the durable obligation remains authoritative; Coordinator recovery/reconciliation may rediscover and physically (re)schedule it according to the already-accepted timer-recovery semantics (GAME-ADR-0008, GAME-ADR-0013). The Turn must never be rolled back because physical scheduling failed after commit - that would let an ephemeral, best-effort mechanism reach back and invalidate an already-authoritative transaction.

### Pure presentation effects may be lost

Purely ephemeral/presentation-oriented effects whose loss does not alter authoritative gameplay - conceptually including animation, sound, confetti, and other presentation-only effects (`EmitEffectOutput` and similar) - may be lost if delivery fails, the socket disappears, the Coordinator crashes, or the process dies immediately after commit. V1 does not guarantee replay of these effects after resync.

### Future external irreversible side effects are excluded from this rule

This best-effort delivery rule covers Coordinator/client/presentation delivery only. It must not silently become the reliability contract for a future Game Language output capable of causing an external irreversible side effect - for example payment, an external API mutation, a durable notification requiring delivery guarantees, a cross-domain command, or another irreversible external effect. If Game Language later gains such a capability, that capability must explicitly design its own delivery/idempotency/retry semantics as a separate decision; this record's "no client outbox in V1" conclusion must never be read as "all future external side effects are best effort."

## Rationale

Treating durable commit as the correctness boundary is not a new choice this record invents - it is the necessary conclusion of every prior Session Runtime decision that already treats a committed RuntimeTurn as authoritative regardless of what happens to the process afterward (GAME-ADR-0007, GAME-ADR-0013). Making that conclusion explicit for the delivery case specifically closes a gap none of those records addressed directly: without this record, an implementation could plausibly (and wrongly) treat "the client never got the message" as a reason to reconsider whether the Turn should have committed at all, reintroducing exactly the coupling between transient delivery failure and authoritative correctness the whole runtime-failure taxonomy (GAME-ADR-0017) was designed to avoid for infrastructure failure generally.

Rejecting a generic durable delivery outbox for V1 follows directly from the same anti-overengineering discipline already applied elsewhere in this initiative (GAME-ADR-0002's rejection of Redis/distributed mechanisms, GAME-ADR-0018's rejection of a distributed lock service): a durable outbox solves a problem V1 does not have, because the actual correctness-relevant state (interactions, timers, terminal state) is *already* durable through the accepted persistence model, and GAME-ADR-0010's resync capability already gives a reconnecting client a path back to current truth without needing a message log. Building a delivery-replay pipeline on top of that would duplicate a recovery path the domain model already provides, at real ongoing complexity cost (per-client offsets, ACK tracking, outbox compaction) for a benefit - replaying already-superseded intermediate messages - that does not serve correctness, since only *current* truth is ever authoritative.

Distinguishing "recover current truth" from "replay every missed message" matters because they are different products with very different cost profiles: reconstructing current truth is what GAME-ADR-0010 already built and is proportionate to what a reconnecting player actually needs (what does the game look like right now, what can I currently do), while message replay would require Session Runtime to durably retain and version every emitted live/presentation event indefinitely - a fundamentally different, heavier feature nobody has asked for and no accepted product requirement currently justifies.

Not rolling back a Turn because physical timer scheduling failed follows the same logic as GAME-ADR-0008's original timer-recovery tradeoff: physical scheduling is explicitly Coordinator-owned ephemeral infrastructure, and its failure is a Coordinator/infrastructure concern with its own already-accepted recovery path (rediscover and reschedule), not a reason to question whether the underlying gameplay decision (the Turn that created the obligation) was valid.

Allowing pure presentation effects to be lost, explicitly and by name, prevents an implementation from over-engineering guaranteed delivery for effects whose entire purpose is transient audiovisual feedback - guaranteeing their replay would add real complexity (the same outbox/ACK machinery rejected above) to preserve something with no gameplay consequence if missed.

Explicitly excluding future external irreversible side effects from this rule is necessary because "no outbox in V1" is a statement about the *current* set of Game Language outputs (all Coordinator/client/presentation-facing), not a general policy about acceptable reliability for anything Game Language might ever produce. A future payment or external-API output would have a fundamentally different risk profile (an unrecoverable real-world consequence, not a redrawable pixel), and conflating the two would either dangerously under-engineer a future critical path by assuming this record already covers it, or wrongly justify over-engineering V1 client delivery today by anticipating a requirement that does not yet exist. Keeping them as clearly separate concerns lets each be judged on its own actual requirements when it actually arises.

## Alternatives Considered

### Roll back or "un-terminalize" a Session/Turn when live delivery fails

Rejected. This would make authoritative Session state depend on an inherently unreliable, ephemeral mechanism (network delivery, a live socket, a Coordinator process staying alive), directly contradicting every already-accepted separation between durable authoritative truth and best-effort delivery (GAME-ADR-0002, GAME-ADR-0013). It would also make correctness nondeterministic from the same committed input depending on transient network conditions.

### Generic durable delivery outbox with per-client acknowledgement tracking

Rejected for V1. This would duplicate the recovery path the resync capability (GAME-ADR-0010) already provides, at the ongoing cost of outbox growth/compaction, per-connection offset tracking, and ACK-timeout handling, for a benefit (guaranteed replay of every intermediate message) no accepted product requirement currently demands. Nothing here forecloses adding one later if a concrete requirement emerges (for example, a game genuinely needing a full move-by-move audit trail delivered live) - but that would be a new, separately justified decision.

### Guaranteed event-stream replay to clients (replay every event since sequence N)

Rejected. This conflates "recover current truth" with "replay history," which are different products. Guaranteeing full event replay would require Session Runtime to durably retain and version every emitted live/presentation event indefinitely, well beyond what current resync already provides, and beyond what any accepted product requirement currently justifies.

### Apply the same best-effort delivery rule uniformly to all future Game Language outputs, including external side effects

Rejected. Coordinator/client/presentation delivery and an irreversible external side effect (payment, external API mutation) have fundamentally different risk profiles - a lost animation has no lasting consequence, while a lost/duplicated payment does. Applying one uniform "best effort, no outbox" rule to both would either dangerously under-engineer a future critical external-effect path or become an excuse to avoid designing its actual delivery/idempotency/retry semantics when that capability is proposed.

## Consequences

- Session Runtime implementation must never implement compensating logic that reverses a committed RuntimeTurn, Snapshot, terminalization, or obligation closure because Coordinator/client delivery failed.
- No `session_delivery_outbox`, `delivery_attempts`, persistent connection delivery offset, or per-client ACK table may be added to the accepted persistence model without a new, separately justified decision record.
- Resync (GAME-ADR-0010) remains the sole accepted client-recovery mechanism for missed delivery; it must continue to reconstruct current truth from already-durable state, not from any delivery log.
- Coordinator's physical-timer recovery/reconciliation (GAME-ADR-0008, GAME-ADR-0013) remains the accepted mechanism for recovering from a timer that failed to be physically scheduled after a Turn committed.
- Presentation-only outputs (animation/sound/effect-shaped `Output` variants) may be designed and implemented without any delivery-guarantee mechanism.
- Any future Game Language capability with an externally irreversible side effect must undergo its own explicit architecture decision about delivery/idempotency/retry semantics; it may not inherit this record's "best effort" conclusion by default.

## Canonical Knowledge Impact

- `game/README.md` - adds a Session Runtime Post-Commit Client Delivery Semantics section referencing this ADR, and clarifies Session Runtime's `Live Session Coordinator` responsibility boundary to state the best-effort delivery rule explicitly.
- `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` - adds a short section stating that no durable client-delivery outbox is part of the accepted persistence model, and cross-references resync (GAME-ADR-0010) as the accepted recovery path.

## Implementation Impact

None. No outbox schema, migration, Coordinator delivery code, or retry/ACK mechanism is authorized or implemented by this record.
