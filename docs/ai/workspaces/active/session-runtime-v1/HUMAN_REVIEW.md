# Session Runtime V1 — Architecture Closure And Implementation Plan Review

Process: Initiative Implementation Planning (Architecture Discussion CLOSED)

Status: awaiting your review of `PLAN.md`'s slice decomposition before Slice 1 is materialized into Feature Development.

## What Was Approved

Two things in this checkpoint:

1. **The final Operational Lifecycle architecture concern - post-commit client delivery - accepted as GAME-ADR-0020.**
2. **Your explicit approval that Session Runtime V1 Architecture Discussion is COMPLETE**, closing Architecture Discussion and moving this initiative into Implementation Planning.

### GAME-ADR-0020: Post-Commit Client Delivery Semantics

- **Durable commit is the correctness boundary.** Once a Session Runtime transaction commits, that state is authoritative. A later failure to deliver to Coordinator/WebSocket/clients never rolls back the RuntimeTurn, Snapshot, a closed interaction, a timer obligation, or terminalization.
- **No generic delivery outbox in V1.** No `session_delivery_outbox`, per-client ACK table, or delivery-attempt tracking.
- **Resync recovers current truth, not history.** If delivery fails or the process dies, a reconnecting client gets the current state through the already-accepted resync capability - not a replay of every missed message. Example: Turn 42 opens interaction Q17; the process dies before Q17 reaches the client; after reconnect, Q17 is simply still an active interaction in the resync projection.
- **Presentation effects (animation/sound) may be lost** with no guaranteed replay - they don't affect gameplay correctness.
- **This does not cover future irreversible external effects.** If Game Language later gains a payment/external-API/notification output, that needs its own separately designed delivery guarantee - this decision explicitly does not extend "best effort" to that case.

### Architecture Discussion Closure

You approved: *"Session Runtime V1 Architecture Discussion: COMPLETE."* No known unresolved issue blocks moving into implementation planning across domain ownership, the Session/Coordinator boundary, lifecycle, lobby admission, Start, RuntimeTurn, concurrency/DB locking, persistence, interactions, timers, disconnect/reconnect, semantic presence, resync, recovery, inactivity expiration, archival direction, the failure taxonomy, terminal cleanup, the RuntimeTurn execution bound, and post-commit delivery. Remaining specifics (exact TTLs, grace durations, rate limits, SQL types, HTTP statuses, WebSocket DTO shapes, archive JSON layout) are implementation-planning details, not architecture blockers.

### Drift Repaired

Two sentences in `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` still said interaction/timer closure at inactivity termination was "a later implementation/design detail" - stale, since GAME-ADR-0019 already resolved this generally (no `ACTIVE` obligations survive any terminalization). Both were updated to state the now-accepted rule. The other two drift areas you flagged (RUNNING serialization scope, disconnect/participation phase distinction) were checked and found already synchronized from the prior checkpoint - no further changes were needed there.

## Implementation Plan Review

The initiative now has a persisted decomposition in `PLAN.md` (temporary, non-canonical - it doesn't replace the ADRs/README as the source of truth, it only sequences the work).

### Proposed Slices (in order)

1. **Session Lobby Foundation** - Create/Join/Leave under the accepted persistence model, replacing the current broken/misaligned scaffolding.
2. **Start & First RuntimeTurn** - engine wiring: `NewSnapshot`, draining internal signals up to the 20-Step bound, persisting Turn 1, RUNNING transition.
3. **RUNNING Interaction Response Processing** - answering an open interaction, first real exercise of RUNNING serialization.
4. **Timer Obligations & Expiration Processing** - ordinary timers, first real Coordinator-owned physical-timer piece.
5. **RuntimeTurn Execution Bound & Runtime Failure Diagnostics** - Step-limit enforcement, `session_runtime_failures`, terminal cleanup atomicity (tightly coupled with slices 2-4 in practice).
6. **Disconnect, Reconnect, Semantic Presence & Resync** - needs a Game Language prerequisite first (giving `UserDisconnected`/`UserReconnected` their accepted schema in the compiler).
7. **Durable Inactivity Expiration & Reaper.**
8. **Keyed Timer Slots** - Game Language capability, only needed once a game wants simultaneous independent timers.
9. **Live Session Coordinator & Post-Commit Delivery** - the actual WebSocket/transport edge; mostly infrastructure wiring once domain logic (1-6) is proven, but could be pulled forward earlier for a thin demo if useful.
10. **Long-Term Runtime History Archival** - lowest risk, purely additive, last.

### Sequencing Rationale (highlights)

- Slice 1 first because the *existing* scaffolding doesn't even compile and actively contradicts the accepted architecture (it takes an externally-compiled `engine.Program`, uses raw owner/player UUID fields instead of the accepted identity boundary, and has none of the accepted tables) - this is a correctness gap to fix, not new ground to break.
- Slice 2 comes right after because RuntimeTurn is the highest-risk, most novel mechanism in the whole design - proving it early avoids building more surface area on an unproven foundation.
- Slice 6 has a genuine Game Language dependency: the compiler needs `UserReconnected` added and `UserDisconnected` given its real schema before Session Runtime can deliver either signal - that's Game Language work with its own lead time, so it's sequenced after the core loop rather than blocking it.
- Slice 9 (Coordinator/WebSocket) is pushed later because it's mostly wiring, not a domain-correctness risk - but it's flagged as the thing actually needed before a human can play a real game end-to-end, so it shouldn't be indefinitely deferred.

### Material Choices Worth Your Attention

- **Identity has no implementation yet**, but Session Runtime's public contract requires an authenticated `UserUUID`. Slice 1 will need to decide how early development obtains a `UserUUID` before real authentication exists - most likely a temporary trusted-caller assumption for internal testing. This is flagged as a question for Slice 1's Feature Development pass, not decided here.
- **Slice 5 is called out separately from Slices 2-4 but is tightly coupled to them** - the Step-limit counting itself is really part of "any RuntimeTurn execution" (so naturally lands in Slice 2), while the fuller diagnostic-persistence/terminal-cleanup surface is what's distinctly its own slice. Feel free to push back if you'd rather see this folded directly into Slice 2.
- **Slice 9 could be pulled forward** for a thin end-to-end demo (create a room, start it, answer one question, see it over a real WebSocket) if that's more valuable early than deferring all transport work - this is a sequencing preference, not an architecture question, and easy to revisit.

### Recommended First Slice

**Slice 1 - Session Lobby Foundation.** It's the most urgent correctness gap (current code doesn't build), everything else depends on it, and it proves out the per-Session DB-locking mechanism at the lowest-risk point before any engine execution is involved.

### Out Of Scope (confirmed, not pulled into this plan)

- Post-launch "Session re-entry after complete client-state loss" (still just a `docs/product/IDEAS.md` idea).
- Post-Start dynamic player admission.
- Full archival JSON/GCS design beyond what Slice 10 minimally needs.
- Future external irreversible Game Language side effects (payment, etc.) - explicitly excluded from GAME-ADR-0020 and would need its own decision later.

## Next Human Action

Review the slice ordering and the two flagged choices above (Identity/`UserUUID` for Slice 1, and whether Slice 5/9 sequencing matches your preference). Once you're comfortable with the plan, say so and the next step is materializing Slice 1 into a DRAFT Feature Development WORK specification - nothing has been implemented yet, and no WORK exists.
