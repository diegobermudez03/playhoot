# GAME-ADR-0016: Session Semantic Presence Recovery After Total Coordinator State Loss

Status: ACCEPTED
Created: 2026-09-07
Last status change: 2026-09-07
Supersedes: None
Superseded by: None

## Context

GAME-ADR-0015 accepted durable `session_actors.semantic_presence` (`CONNECTED | DISCONNECTED`) and the phase-dependent LOBBY/RUNNING participation consequences of a semantic disconnect/reconnect, but explicitly left open what happens when the Coordinator process itself is lost entirely: a `RUNNING` Session may durably record actors as `CONNECTED` while a newly started Coordinator process has no live socket bindings for any of them, because all ephemeral connection/grace state (GAME-ADR-0010) was lost with the previous process. GAME-ADR-0013 already accepts that Session Runtime is process-agnostic and that a process crash by itself is not a Session-domain event; this record answers how that principle composes with semantic presence and Coordinator-owned transport grace specifically, without reopening either.

Without an accepted answer, an implementer would face an unreviewed choice between two bad outcomes: treating "no live binding after restart" as an immediate semantic disconnect for every actor (which would emit spurious `UserDisconnected` for every still-actually-connected player on every ordinary process restart/deployment), or treating durably `CONNECTED` as ground truth forever (which would never let the platform notice a player who is truly gone). A second, independent risk is that the durable `semantic_presence` edge and its corresponding Game Language lifecycle signal could become separated by a crash - the presence edge persisting while the authored `UserDisconnected`/`UserReconnected` delivery is lost, or vice versa - silently corrupting the already-accepted invariant that Game Language state and Session Runtime state stay consistent.

## Decision

### Total process loss does not mutate semantic presence

Loss of all in-memory Coordinator/process state does not itself change `session_actors.semantic_presence`. Durable values before process loss (for example P1 = `CONNECTED`, P2 = `CONNECTED`, P3 = `DISCONNECTED`) remain unchanged merely because the process crashed, sockets disappeared, or Coordinator maps/timers/graces were lost. Session Runtime remains process-agnostic (GAME-ADR-0013); this record does not introduce process ownership, process IDs, process heartbeat state, a `RECOVERING`/`UNKNOWN` presence value, or durable socket state.

### Durable `CONNECTED` means an unprocessed disconnect edge, not a live socket

`semantic_presence = CONNECTED` does not mean "a physical socket definitely exists right now." It means "Session Runtime has not yet processed a semantic `CONNECTED -> DISCONNECTED` edge for this actor." Physical connectivity remains entirely Coordinator-owned ephemeral state (GAME-ADR-0010). This distinction is what makes recovery after process restart possible without conflating the two concepts GAME-ADR-0015 already separated.

### Fresh recovery grace for durably `CONNECTED` actors with no binding

When Coordinator reconstructs/reconciles a Session that is still semantically active and finds a SessionActor with `semantic_presence = CONNECTED` but no current physical binding (because ephemeral state was lost), Coordinator starts a fresh transport/recovery grace for that actor - conceptually the same transport-grace mechanism already accepted for ordinary transient disconnects (GAME-ADR-0010), not a distinct "process-crash disconnect" event or API.

### Rebind during recovery grace produces no lifecycle signal

If the actor establishes a new authenticated connection before the recovery grace expires: Coordinator binds the new connection and cancels the recovery grace; the SessionActor remains semantically `CONNECTED`; no `UserDisconnected` and no `UserReconnected` is emitted; Coordinator obtains/sends normal Session resync state. From the Game Language perspective, no semantic disconnect occurred. This prevents ordinary process restarts/deployments from producing artificial `UserDisconnected -> UserReconnected` pairs for every still-connected player.

### Recovery grace expiry uses the ordinary semantic disconnect path

If the fresh recovery grace expires without a binding, Coordinator invokes the normal semantic disconnect path exactly as for an ordinary transport-grace expiry - no special process-loss API is needed. Session Runtime then applies the already-accepted phase-specific rules (GAME-ADR-0015): in `LOBBY`, `CONNECTED -> DISCONNECTED`, the active Participant becomes inactive, the lobby slot is released, and no Game Language disconnect signal exists because the runtime has not started; in `RUNNING`, for an actor belonging to the Start-time runtime roster, `CONNECTED -> DISCONNECTED` while runtime membership remains intact, and Session Runtime processes the already-accepted root `UserDisconnected(user)` according to Game Language semantics (GAME-ADR-0011).

### Durably `DISCONNECTED` actors need no recovery grace

If an actor was already `semantic_presence = DISCONNECTED` before total process loss, the new Coordinator does not start a recovery grace for that actor - there is no semantic uncertainty to absorb. A new authenticated connection instead uses the already-accepted normal semantic reconnect path (GAME-ADR-0015): in `LOBBY`, `DISCONNECTED -> CONNECTED`, then current lobby admission constraints are re-evaluated if participation had been deactivated, with no guaranteed slot recovery; in `RUNNING`, if the actor belongs to the immutable Start-time runtime roster, `DISCONNECTED -> CONNECTED` and Session Runtime processes `UserReconnected(user)` before producing resync; if the actor was excluded from the Start-time roster, reconnect must not invent runtime membership or deliver `UserReconnected` into that Game execution (unchanged from GAME-ADR-0015).

### V1 restarts the full grace duration

V1 does not persist a physical-disconnect-start timestamp or remaining transport-grace duration. After total Coordinator/process loss, the new Coordinator may therefore start the full configured grace duration again for durably `CONNECTED` actors with no binding, rather than reconstructing elapsed grace time. No `physical_disconnected_at`, durable grace deadline, or remaining-grace persistence is added. This mirrors the same simplicity tradeoff already accepted for Game Language timer obligations (GAME-ADR-0008's full-configured-delay rescheduling), while remaining a distinct mechanism - one is Coordinator transport grace, the other is a durable Game Language timer obligation.

### Authoritative lifecycle deadlines take precedence over recovery grace

Recovery grace must never revive a Session whose authoritative lifecycle deadline already passed. Normal Session lifecycle truth is validated before reconstructing live behavior. If `now >= lobby_expires_at`, the lobby is already semantically expired (GAME-ADR-0004) and a fresh Coordinator recovery grace must not preserve/revive it. If `now >= activity_expires_at`, the Session is already semantically expired under the accepted inactivity-expiration model (GAME-ADR-0014) and must not receive a fresh opportunity to continue merely because Coordinator restarted; expiration may be lazily/proactively materialized per the already-accepted rules. Authoritative Session lifecycle deadlines take precedence over ephemeral Coordinator recovery grace.

### Recovery mechanism reconstruction is not activity

Merely reconstructing infrastructure mechanisms after a process restart does not renew `activity_expires_at` (GAME-ADR-0014). Scanning `RUNNING` Sessions, loading their current Snapshot, discovering active `session_timer_obligations`, rescheduling physical timers, discovering durably `CONNECTED` actors, starting recovery graces, and rebuilding Coordinator maps must not, by themselves, extend Session lifetime - otherwise infrastructure restarts could keep abandoned Sessions alive indefinitely. A later real operation, such as an authenticated reconnect, may separately qualify as meaningful activity under the already-accepted renewal policy.

### No process-specific Session APIs

Session Runtime does not gain APIs equivalent to `RecoverFromProcessCrash`, `TransferSessionOwnership`, `ClaimSession`, `ProcessDied`, or `CoordinatorRestarted`. It continues to expose only domain/runtime operations (semantic disconnect, semantic reconnect, resync, and existing lobby/runtime operations) independent of which process invokes them; Coordinator reconstruction is entirely an upper-layer concern outside Session Runtime's API surface.

### Atomic semantic-presence edge and Game Language processing during RUNNING

This is the critical correctness rule for `RUNNING` Sessions. A durable semantic-presence transition and the corresponding Game Language lifecycle processing must not be allowed to become separated by a process crash. The bad state to avoid: Session persists `CONNECTED -> DISCONNECTED`; the process crashes; `UserDisconnected(user)` never reaches authoritative Game execution; after restart, Session sees `DISCONNECTED` and therefore never attempts the lost lifecycle edge again.

For a RUNNING runtime member, disconnect and reconnect are therefore each processed as one Session transaction, extending the already-accepted RuntimeTurn transactional model (GAME-ADR-0007) to cover the presence mutation itself:

- **Disconnect**: validate/serialize the Session; transition `semantic_presence: CONNECTED -> DISCONNECTED`; construct the root `UserDisconnected(user)` signal; execute normal runtime processing; if handled, persist the resulting RuntimeTurn/final Snapshot/consequences; if valid but unhandled, preserve the already-accepted no-op gameplay semantics (GAME-ADR-0011) and create no no-op RuntimeTurn; commit the semantic-presence transition and any gameplay consequences atomically, in the same transaction.
- **Reconnect**: transition `semantic_presence: DISCONNECTED -> CONNECTED`; process the root `UserReconnected(user)` signal; persist any resulting RuntimeTurn/consequences; commit atomically; only after durable commit does Coordinator resync/deliver the resulting state.

If the process crashes before the transaction commits, the semantic-presence edge did not occur authoritatively - for example, an attempted `CONNECTED -> DISCONNECTED` whose transaction rolled back leaves durable state at `CONNECTED`, and Coordinator may later report the semantic disconnect again. If the transaction committed, durable semantic presence and any corresponding Game Language effect are already authoritative together; no lifecycle edge is lost. This composes with, and does not modify, the already-accepted RuntimeTurn all-or-nothing crash semantics (GAME-ADR-0007, GAME-ADR-0013).

### Unhandled lifecycle signals still commit the presence edge alone

This preserves GAME-ADR-0011's previously accepted behavior. If a RUNNING actor transitions `CONNECTED -> DISCONNECTED` (or the reverse) but the authored game does not handle the corresponding signal: semantic presence still commits as the new value; no automatic gameplay consequence occurs; no RuntimeTurn is created merely to represent an identical Snapshot. The atomicity requirement means "the semantic edge and the definitive result of attempting its Game Language lifecycle processing commit together" - it does not require fabricating a RuntimeTurn for an unhandled signal.

## Rationale

Making total process loss a no-op for `semantic_presence` is the direct consequence of Session Runtime already being process-agnostic (GAME-ADR-0013): if a process crash is not itself a Session-domain event, it cannot be allowed to silently mutate Session-domain state either. Reinterpreting durable `CONNECTED` as "no disconnect edge processed yet" rather than "a live socket exists" is what makes this coherent - it was always a slightly coarser fact than physical presence (GAME-ADR-0015 already accepted the two could diverge), and process-loss recovery simply exercises that same divergence at a larger scale.

Reusing the ordinary transport-grace mechanism for recovery, rather than inventing a distinct process-crash-disconnect concept, avoids doubling the number of disconnect/reconnect code paths Session Runtime and Coordinator must maintain, and keeps GAME-ADR-0010's accepted three-way responsibility boundary (transport fact / platform event / gameplay consequence) intact. Restarting the full grace duration rather than reconstructing elapsed time is consistent with the project's existing preference (GAME-ADR-0008) for full-delay rescheduling over persisting fine-grained elapsed-time bookkeeping purely for precision V1 does not need; the cost is a slightly longer worst-case detection window after a crash, which is an acceptable, explicitly named tradeoff.

Giving lifecycle deadlines precedence over recovery grace prevents an operational restart from accidentally reviving a Session that had already legitimately expired - `lobby_expires_at` and `activity_expires_at` are already each other's accepted source of truth (GAME-ADR-0004, GAME-ADR-0014), and recovery grace is a strictly lower-priority, ephemeral concern layered on top of already-durable lifecycle truth. Excluding mechanism reconstruction from activity renewal is necessary because otherwise the inactivity-expiration model (GAME-ADR-0014) could be defeated simply by restarting the Coordinator process periodically, which would make abandoned-Session cleanup unreliable exactly when process instability is most likely.

The atomic-commit requirement for RUNNING disconnect/reconnect is the most safety-critical piece of this record: without it, a crash at exactly the wrong moment could durably desynchronize Session Runtime's presence bookkeeping from what the Game Language execution actually believes happened, which is a correctness bug no amount of process-agnostic design elsewhere would catch. Folding the presence mutation into the same transactional boundary already used for RuntimeTurns (GAME-ADR-0007) costs nothing new - it is the same mechanism, applied to one more kind of durable mutation - and preserves the existing guarantee that "if it committed, it happened; if it didn't commit, it didn't."

## Alternatives Considered

### Immediately treat "no live binding after restart" as semantic disconnect for every durably `CONNECTED` actor

Rejected. Every ordinary process restart or deployment would emit spurious `UserDisconnected` (and later `UserReconnected`) for every player who was, in fact, still connected and simply reconnected moments later, needlessly triggering authored gameplay consequences for a routine operational event.

### Treat durable `CONNECTED` as permanent ground truth with no recovery mechanism

Rejected. It would mean the platform could never notice a player who genuinely disappeared during a Coordinator outage, leaving Sessions durably believing actors are connected indefinitely and defeating the purpose of semantic presence entirely.

### Invent a distinct `ProcessCrashDisconnected` Session-domain event/API

Rejected. It would duplicate the already-accepted transport-grace/semantic-disconnect mechanism (GAME-ADR-0010) for no behavioral difference the game or Session Runtime actually needs to observe, and would reintroduce process-awareness into an API surface GAME-ADR-0013 deliberately kept process-agnostic.

### Persist elapsed/remaining physical-disconnect grace time to give exact grace-duration precision across restarts

Rejected for V1. It would add a new durable field/mechanism purely for precision no accepted requirement demands, mirroring the already-rejected direction in GAME-ADR-0008 for timer obligations; restarting the full grace duration is an acceptable, explicitly named V1 tradeoff.

### Let recovery grace revive a Session past its `lobby_expires_at`/`activity_expires_at` deadline

Rejected. It would let an operational restart override already-accepted, deadline-driven lifecycle truth (GAME-ADR-0004, GAME-ADR-0014), effectively giving Coordinator restarts the power to grant a Session extra lifetime it was never entitled to.

### Treat recovery-mechanism reconstruction (Snapshot loads, timer rescheduling, grace starts) as activity that renews `activity_expires_at`

Rejected. It would let periodic Coordinator restarts keep an otherwise-abandoned Session alive indefinitely, directly undermining GAME-ADR-0014's inactivity-expiration guarantee.

### Persist the semantic-presence transition and the Game Language signal delivery as two separate operations/transactions

Rejected. A crash between the two could durably commit the presence edge while losing the corresponding authored lifecycle effect (or vice versa), silently desynchronizing Session Runtime state from Game Language state with no way to detect or repair the gap; a single atomic transaction, consistent with the existing RuntimeTurn model, closes this gap entirely.

## Consequences

- Future Coordinator recovery-reconciliation implementation must, for each `RUNNING` Session it reconstructs, distinguish durably `CONNECTED` actors with no live binding (start a fresh full-duration recovery grace) from durably `DISCONNECTED` actors (no grace; normal reconnect path on a later connection).
- Future recovery-grace implementation must reuse the existing transport-grace/semantic-disconnect code path rather than a new process-crash-specific one, and must not persist elapsed/remaining grace duration.
- Future implementation of any operation that discovers/reconstructs Session state after restart (Snapshot loads, timer obligation discovery, recovery-grace starts) must validate `lobby_expires_at`/`activity_expires_at` before treating the Session as live, and must not treat that discovery itself as activity that renews `activity_expires_at`.
- Future RUNNING disconnect/reconnect implementation must persist the `semantic_presence` transition and any resulting Game Language RuntimeTurn/consequences within one Session transaction; an unhandled signal still commits the presence transition alone, with no RuntimeTurn.
- Session Runtime's API surface must remain free of process-specific operations; Coordinator-side recovery/reconciliation logic is not part of Session Runtime's domain API.
- Runtime/internal failure semantics (which engine/runtime failures are ordinary rejections versus Session-failing, when a Session becomes `TERMINAL` from an internal execution failure, and related terminal-reason/observability questions) remain the next, separately deferred architecture topic and are not decided here.

## Canonical Knowledge Impact

- `game/README.md` - Session Runtime Process-Agnostic Recovery section gains the recovery-grace/semantic-presence-atomicity behavior; Session Runtime Disconnect, Reconnect, and Resynchronization Boundary section gains the fresh-recovery-grace/no-spurious-signal behavior after total process loss and the RUNNING atomic-commit rule; Session Runtime Durable Inactivity Expiration section gains the explicit "mechanism reconstruction is not activity" clarification.
- `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` - Process-Agnostic Recovery section gains a clarifying note that the RUNNING disconnect/reconnect presence transition commits within the same transactional boundary as its RuntimeTurn consequence; no schema/ER diagram change, since `session_actors.semantic_presence` already exists and no new durable field is introduced.

## Implementation Impact

Future implementation must build the Coordinator recovery-reconciliation logic (distinguishing durably `CONNECTED`-no-binding from durably `DISCONNECTED` actors on Session reconstruction), the fresh full-duration recovery-grace mechanism reusing the existing transport-grace path, the lifecycle-deadline precedence check before granting recovery grace, and the single-transaction RUNNING disconnect/reconnect processing path covering both the `semantic_presence` mutation and Game Language signal delivery. No production code, migration, Coordinator implementation, timer implementation, compiler/engine/program Game Language change, tests, or WORK is authorized by this record.
