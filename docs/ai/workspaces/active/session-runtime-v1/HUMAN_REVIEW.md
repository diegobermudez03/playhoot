# Session Semantic Presence Recovery After Total Coordinator State Loss Checkpoint

Process: Architecture Discussion

Status: RESOLVED - accepted 2026-09-07. No checkpoint is currently pending; this file is retained as the human-facing record of the resolved checkpoint until the next milestone (runtime/internal failure semantics) produces its own checkpoint.

## What Was Approved

Semantic presence recovery after total Coordinator/process loss - the previously explicit open question left by GAME-ADR-0015 - was reviewed and accepted as GAME-ADR-0016.

### Process loss does not mutate semantic presence

- Total loss of ephemeral Coordinator/process state does not itself change `session_actors.semantic_presence`. Durable `CONNECTED`/`DISCONNECTED` values survive a crash, lost sockets, and lost Coordinator maps/timers/graces unchanged. Session Runtime remains process-agnostic (GAME-ADR-0013): no process ownership, process IDs, heartbeat state, `RECOVERING`/`UNKNOWN` presence value, or durable socket state was introduced.
- Durable `CONNECTED` means only "no semantic disconnect edge processed yet," never "a live socket exists right now." Physical connectivity remains entirely Coordinator-owned ephemeral state.

### Fresh recovery grace, reusing the ordinary mechanism

- When Coordinator reconstructs a still-semantically-active Session and finds a durably `CONNECTED` actor with no current physical binding, it starts a fresh transport/recovery grace for that actor - the same mechanism already accepted for ordinary transient disconnects (GAME-ADR-0010), not a distinct process-crash-disconnect event or API.
- Rebinding within that fresh grace preserves `CONNECTED`, emits no `UserDisconnected`/`UserReconnected`, and proceeds to ordinary resync - this is what keeps an ordinary process restart/deployment from producing artificial disconnect/reconnect pairs for every still-connected player.
- Grace expiry uses the ordinary semantic-disconnect path; the already-accepted phase-dependent LOBBY/RUNNING rules (GAME-ADR-0015) apply unchanged, with no special process-crash Session event.
- A durably `DISCONNECTED` actor gets no recovery grace (no semantic uncertainty to absorb); a later connection follows the ordinary semantic-reconnect path.

### V1 precision tradeoff

- V1 does not persist a physical-disconnect-start timestamp or remaining grace duration. A restarted Coordinator may restart the full configured grace duration for a durably `CONNECTED` actor with no binding - the same simplicity tradeoff already accepted for Game Language timer obligations (GAME-ADR-0008), as a distinct mechanism. No `physical_disconnected_at`, durable grace deadline, or remaining-grace persistence was added.

### Lifecycle deadlines and activity renewal stay authoritative

- Recovery grace must never revive a Session whose `lobby_expires_at`/`activity_expires_at` deadline already passed; authoritative lifecycle deadlines take precedence over ephemeral recovery grace.
- Merely reconstructing infrastructure mechanisms after restart (Snapshot loads, timer-obligation discovery, discovering durably `CONNECTED` actors, starting recovery graces, rebuilding Coordinator maps) does not by itself renew `activity_expires_at` - only a later real operation (for example an authenticated reconnect) can qualify as meaningful activity under the already-accepted renewal policy (GAME-ADR-0014).

### No process-specific Session APIs

- Session Runtime gains no APIs such as `RecoverFromProcessCrash`, `TransferSessionOwnership`, `ClaimSession`, `ProcessDied`, or `CoordinatorRestarted`. It continues to expose only ordinary domain/runtime operations regardless of which process invokes them; Coordinator reconstruction remains an upper-layer concern.

### Atomic semantic-presence/Game-Language commit for RUNNING (critical correctness rule)

- For a RUNNING runtime member, the durable `semantic_presence` transition and its corresponding Game Language lifecycle processing (`UserDisconnected`/`UserReconnected`) commit as one Session transaction - extending the already-accepted RuntimeTurn transactional model (GAME-ADR-0007) to the presence mutation itself.
- If the process crashes before that transaction commits, the presence edge did not occur authoritatively; durable state remains at its prior value, and Coordinator may later report the disconnect again.
- If the transaction commits, presence and any Game Language effect are already authoritative together - a crash can never durably record the presence edge while silently losing the corresponding authored lifecycle effect, or the reverse.
- An unhandled signal still commits the presence transition alone, with no RuntimeTurn created merely to represent an identical Snapshot (GAME-ADR-0011 unchanged).

## Where This Was Persisted

- `game/docs/decisions/GAME-ADR-0016-session-semantic-presence-recovery-after-total-coordinator-state-loss.md`
- `game/docs/decisions/INDEX.md` (GAME-ADR-0016 row added; next Game ADR is now `GAME-ADR-0017`)
- `game/README.md` (Session Runtime Process-Agnostic Recovery section gains the recovery-grace/atomic-commit behavior; Session Runtime Durable Inactivity Expiration section gains the "mechanism reconstruction is not activity" clarification; Session Runtime Disconnect, Reconnect, and Resynchronization Boundary section gains the fresh-recovery-grace-after-total-process-loss behavior and the RUNNING atomic-commit rule)
- `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` (clarifying prose added to the Process-Agnostic Recovery section and the rationale-links list; no schema/ER diagram change, since `session_actors.semantic_presence` already exists and no new durable field/table was introduced)
- `docs/ai/workspaces/active/session-runtime-v1/AI_CONTEXT.md` (new accepted-decision section, updated deferred topics, updated next-milestone questions, updated drift/explicitly-not-done notes)

## Explicitly Not Authorized By This Checkpoint

No production code, migrations, Coordinator recovery-grace implementation, timer implementation, compiler/engine/program Game Language changes, tests, or WORK were created or changed. No process ownership/heartbeat/fencing model was introduced. No new persistence schema was introduced - `session_actors.semantic_presence` already existed from GAME-ADR-0015, and no recovery/grace/process field was added anywhere. GAME-ADR-0010, GAME-ADR-0011, GAME-ADR-0013, GAME-ADR-0014, and GAME-ADR-0015's historical rationale were not rewritten; this checkpoint closes the open question they each left, as an additive/refining new record.

## Next Milestone (Not Designed Here)

Runtime/internal failure semantics. Process crash/recovery and semantic presence recovery are now considered CLOSED enough at architecture level. Conversational AI should next determine: (1) which engine/runtime failures are expected operation rejections versus Session failures; (2) which failures roll back only the current RuntimeTurn while the Session remains RUNNING; (3) which invariant/corruption failures make continued execution unsafe; (4) when a Session should become TERMINAL because of an internal execution failure; (5) what terminal reasons/categories are needed; (6) what clients/Coordinator should observe after such a failure; (7) how retryable infrastructure/database failures differ from deterministic Game/engine failures; (8) how max-step/runaway/execution-budget failures behave; (9) which errors are visible to authored games versus platform/operator telemetry; (10) how archival preserves failed-session history. A new checkpoint should be opened here once that design work produces a proposal for human review.
