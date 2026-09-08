# GAME-ADR-0013: Session Runtime Process-Agnostic Recovery And RuntimeTurn Crash Semantics

Status: ACCEPTED
Created: 2026-09-07
Last status change: 2026-09-07
Supersedes: None
Superseded by: None

## Context

GAME-ADR-0002/GAME-ADR-0003 already accept that Session Runtime owns durable authoritative state that must be reconstructible after loss or restart of the process serving it (`Stateless` means process-stateless/reconstructible, not that the domain itself has no state). GAME-ADR-0007 accepts `RuntimeTurn` as the atomic historical/transactional unit for RUNNING-phase execution, and GAME-ADR-0008 accepts that Game Language timer recovery uses full-configured-delay rescheduling rather than a preserved absolute deadline.

What remained open is what happens, concretely, when the OS process executing Session Runtime dies while a Session is `RUNNING`, and how a different process later resumes serving that same Session. Without an accepted answer, an implementation could drift toward modeling process/instance ownership (a durable owner, a heartbeat, a fencing generation) to decide "who is allowed to keep handling this Session," which would contradict the already-accepted process-stateless architecture. This record closes that gap for the "process disappears and something else needs to keep going" case, distinct from Coordinator-level transport disconnect/reconnect (GAME-ADR-0010), which concerns a client connection, not the server process executing Session Runtime.

## Decision

### Session Runtime is process-agnostic

Session Runtime does not model durable ownership of a Session by a particular process, pod, or runtime instance. It does not introduce `owner_process_id`, runtime instance ownership, process heartbeat ownership, a fencing/takeover generation whose sole purpose is process ownership, sticky Session ownership, or a durable `RECOVERING` process phase.

Conceptually: Process A calls the Session Runtime API against durable Session state, then disappears; Process B later calls the same Session Runtime API against the same durable Session state. Session Runtime does not need to know, and does not record, that the caller is a different process. A process crash by itself is not a Session-domain event and does not change `Session.phase`.

### RuntimeTurn crash semantics

The accepted RuntimeTurn transactional model (GAME-ADR-0007) is preserved unchanged for crash recovery:

- **Transaction did not commit.** If the process dies before a RuntimeTurn's transaction commits, the entire Turn rolls back. `current_turn_id` remains at its previous value; no partially authoritative Turn exists; intermediate Steps are not resumed later; the external cause (interaction response, timer expiration, or future platform signal) may later be retried according to its own normal idempotency semantics. Session Runtime does not add a durable `RuntimeTurn = PROCESSING` state, and it does not resume "Step N" of a partially executed Turn - this would require durable per-Step resumability that GAME-ADR-0007 explicitly rejected as Session-state-significant.
- **Transaction committed.** If the transaction committed but the process died before live delivery/response, the Turn did occur: `current_turn_id` points to the committed Turn, the interaction/timer/state mutations that committed with it remain authoritative, reconnect/resync (GAME-ADR-0010) returns the resulting state, and normal idempotency prevents replaying the same accepted external cause. Which process happened to execute the Turn is irrelevant to any of this.

### Recovery reconstructs from the current durable checkpoint, not event replay

A process that needs to resume serving a `RUNNING` Session reconstructs it from current durable authoritative state: `sessions`, `session_runtime_state.current_turn_id`, the final Snapshot stored on that RuntimeTurn, active `session_interactions`, active `session_timer_obligations`, and other current durable Session-owned state. Session Runtime does not require replaying RuntimeTurn `1..N` to reconstruct the current Session - GAME-ADR-0007 already stores the authoritative final Snapshot per Turn precisely so recovery does not need to replay history.

Historical Turns/Steps remain useful for history, debugging, archive, and audit/replay tooling where separately designed (see GAME-ADR-0009), but they are not required bootstrapping state for resuming a `RUNNING` Session.

For Game Language timers, GAME-ADR-0008's accepted tradeoff continues to apply unchanged: active durable timer obligations may be physically rescheduled after process loss using their full configured delay; no `due_at` is introduced by this record.

## Rationale

Session Runtime already commits to being process-stateless and reconstructible from durable state (GAME-ADR-0002/GAME-ADR-0003); introducing an ownership/lease/heartbeat mechanism to decide which process may act would directly contradict that commitment and would add a correctness mechanism (fencing, takeover, liveness detection) that per-Session serialization (GAME-ADR-0004) does not need in order to remain correct. Serialization already guarantees that only one mutation proceeds at a time for a given Session; it does not need to know or care which process holds that serialization at any moment.

Relying on ordinary database transactional atomicity for in-flight RuntimeTurns is simpler and strictly sufficient: a transaction that never committed leaves no trace to reconcile, and a transaction that committed is by definition authoritative regardless of what happened to the process immediately afterward. Adding a durable "in-progress" marker would only recreate, at the application layer, a guarantee the database transaction already provides.

Reconstructing from the current checkpoint (Session + `current_turn_id` + its Snapshot + active Interactions/Timers) rather than replaying history keeps recovery cost bounded and independent of how long a Session has been running or how many Turns it has accumulated, consistent with why GAME-ADR-0007 persists a final Snapshot per Turn in the first place.

## Alternatives Considered

### Durable process-owned Session lease with heartbeat and fencing/takeover generation

Rejected. This would make Session Runtime track process liveness and ownership - a concern explicitly assigned away from Session Runtime by GAME-ADR-0002/GAME-ADR-0003 - without a concrete correctness driver, since per-Session serialization already prevents concurrent conflicting mutation regardless of which process executes it.

### Durable `RECOVERING` phase between `RUNNING` and normal operation

Rejected. A process crash by itself is not a Session-domain event. There is no meaningfully different action to take "while recovering" beyond what the next normal operation already does (load current durable state and proceed); inventing a phase for it would imply Session Runtime tracks process health, contradicting process-agnostic design.

### Event-sourced replay of RuntimeTurn `1..N` to rebuild current state on every recovery

Rejected. GAME-ADR-0007 already persists the authoritative final Snapshot per Turn specifically so current state is available in one read. Requiring full replay would duplicate that correctness mechanism, grow recovery cost with Session history length, and give historical Turns a bootstrapping responsibility beyond their accepted role (history/debugging/archive/audit).

### Durable `RuntimeTurn = PROCESSING` marker with Step-level resume

Rejected. Contradicts the accepted atomic RuntimeTurn transactional model (GAME-ADR-0007): a Turn's internal Steps are execution mechanics within one transaction, not independently durable/resumable state. Resuming "Step N" would require deciding which partial engine effects are safe to continue from, which GAME-ADR-0007 already rejected as a source of Session-state truth.

## Consequences

- Future Session Runtime implementation must not add process/instance ownership fields, ownership heartbeats, or fencing/takeover generations for the purpose of deciding which process may serve a Session.
- Recovery-path implementation must reconstruct current state from `sessions`, `session_runtime_state.current_turn_id`, that Turn's Snapshot, active `session_interactions`, and active `session_timer_obligations`, not from replaying RuntimeTurn history.
- An uncommitted RuntimeTurn at the moment of process death is fully handled by ordinary database transaction rollback; Session Runtime does not need application-level cleanup for it.
- Any process may serve any `RUNNING` Session's next operation without prior process affinity or handoff protocol.
- This record does not change `lobby_expires_at` or introduce the RUNNING-phase inactivity deadline; that is a separate concern (see GAME-ADR-0014).

## Canonical Knowledge Impact

- `game/README.md` - adds a Process-Agnostic Recovery section under Session Runtime lifecycle, referencing this ADR and confirming no process-ownership model exists.

## Implementation Impact

Future implementation must ensure recovery paths read current durable checkpoint state rather than replaying history, and must not introduce process-ownership/heartbeat/fencing mechanisms. No recovery worker, migration, production code, or WORK is authorized by this record.
