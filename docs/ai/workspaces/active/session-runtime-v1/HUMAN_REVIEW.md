# Process Crash / Session Interruption Semantics Checkpoint

Process: Architecture Discussion

Status: RESOLVED - accepted 2026-09-07. No checkpoint is currently pending; this file is retained as the human-facing record of the resolved checkpoint until the next milestone (durable semantic presence across process loss) produces its own checkpoint.

## What Was Approved

Process crash / Session interruption semantics - the milestone GAME-ADR-0010/GAME-ADR-0011/GAME-ADR-0012 left open - were reviewed and accepted as two related decisions.

### Session Runtime is process-agnostic (GAME-ADR-0013)

- No durable process/instance ownership: no `owner_process_id`, no runtime instance ownership, no process heartbeat ownership, no fencing/takeover generation for process ownership, no sticky Session ownership, no durable `RECOVERING` phase. A process crash by itself is not a Session-domain event and does not change `Session.phase`.
- RuntimeTurn crash semantics preserve the accepted transactional model (GAME-ADR-0007): if the transaction did not commit, the entire Turn rolls back (previous `current_turn_id` remains current, no partial Turn, no resumed Steps, external cause retried per its own idempotency); if the transaction committed but the process died before delivery, the Turn did occur and remains authoritative regardless of which process executed it.
- Recovery reconstructs a `RUNNING` Session from the current durable checkpoint - `sessions`, `session_runtime_state.current_turn_id`, the final Snapshot on that Turn, active `session_interactions`, active `session_timer_obligations` - not by replaying RuntimeTurn `1..N` history. Historical Turns/Steps remain useful for history/debugging/archive/audit-replay tooling only. Game Language timer recovery keeps the GAME-ADR-0008 full-configured-delay tradeoff unchanged.
- Rejected: a durable process-owned Session lease, `owner_instance_id`, periodic per-process heartbeat, or takeover generations/fencing solely to determine process ownership.

### Durable RUNNING inactivity expiration (GAME-ADR-0014)

- Every `RUNNING` Session gets a durable `activity_expires_at` inactivity deadline, distinct from `lobby_expires_at` and not a process ownership lease.
- Renewal (`= now + inactivity_ttl`, TTL configurable operational/product policy, V1 illustrative ~10 minutes, never hard-coded into Game Language) is limited to operations demonstrating meaningful active use (RuntimeTurn-producing gameplay, accepted interaction processing, timer expiration processing, meaningful lifecycle/runtime events, justified reconnect/resume - illustrative, not exhaustive); passive reads/polling must not renew it.
- `activity_expires_at` is the authoritative source of truth for expiration, not Reaper discovery, mirroring the already-accepted `lobby_expires_at` pattern. Every active-dependent operation validates it under normal per-Session serialization before processing; a late-arriving operation must not revive a Session merely because persisted `phase` still reads `RUNNING`.
- Lazy materialization: a discovering operation may atomically materialize `phase = TERMINAL`, `terminal_reason = RUNTIME_INACTIVITY_EXPIRED`, `terminal_at = activity_expires_at` in the same transaction, then reject the attempted action - never renewing, reopening, fabricating gameplay, or creating a RuntimeTurn merely to represent expiration.
- A Session Reaper proactively materializes already-expired-but-unmaterialized Sessions under the same serialization/revalidation rules; it does not itself decide expiration, and rechecks/no-ops if a legitimate operation already renewed the deadline first. The deadline, not Reaper scheduling latency, decides the outcome.
- `terminal_at` always equals `activity_expires_at`, never Reaper/lazy-operation wall-clock time - preserving the correct historical/retention/archival instant even across a multi-day outage.
- `terminal_reason = RUNTIME_INACTIVITY_EXPIRED` means only that the allowed inactivity period was exceeded; it does not assert a process crash, pod kill, host disconnect, or that every participant left, and is deliberately not named `PROCESS_CRASH`/`RUNTIME_ORPHANED`.
- The Archive Worker (GAME-ADR-0009) remains completely separate: it consumes only already-materialized `TERMINAL` Sessions per retention policy and must never inspect process ownership, detect crashes, determine RUNNING inactivity, or interpret `activity_expires_at`.
- Interaction/timer closure semantics for still-open `session_interactions`/`session_timer_obligations` at inactivity termination remain a later implementation/design detail; materializing expiration must never fabricate engine responses or RuntimeTurns.

## Where This Was Persisted

- `game/docs/decisions/GAME-ADR-0013-session-runtime-process-agnostic-recovery.md`
- `game/docs/decisions/GAME-ADR-0014-session-runtime-durable-inactivity-expiration.md`
- `game/docs/decisions/INDEX.md` (GAME-ADR-0013 and GAME-ADR-0014 rows added; next Game ADR is now `GAME-ADR-0015`)
- `game/README.md` (new "Session Runtime Process-Agnostic Recovery" and "Session Runtime Durable Inactivity Expiration" sections; the earlier LOBBY-only expiration paragraph updated to also describe `activity_expires_at` and distinguish both from a still-deferred max-runtime/abuse policy)
- `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` (`sessions.activity_expires_at` added to all three ER diagrams; new "Process-Agnostic Recovery" and "RUNNING Inactivity Deadline: `activity_expires_at`" sections; "Not Yet Decided" section extended)
- `docs/ai/workspaces/active/session-runtime-v1/AI_CONTEXT.md` (new accepted-decision section, updated deferred topics, updated next-milestone question, updated drift/explicitly-not-done notes)
- `docs/product/IDEAS.md` (new non-authoritative "Session Re-Entry After Complete Client-State Loss" post-launch idea)

## Explicitly Not Authorized By This Checkpoint

No production code, migrations, Reaper/recovery-worker implementation, Coordinator behavior, WebSocket handlers, tests, or WORK were created or changed. No process-ownership/heartbeat/fencing model was introduced anywhere. The post-launch client-state-loss re-entry idea was recorded only as a non-authoritative product idea in `docs/product/IDEAS.md` - no UX/API/identity mechanism was designed or approved, and guest identity semantics were not changed ("same username" recovery is explicitly not accepted as an identity/security mechanism).

## Next Milestone (Not Designed Here)

Durable semantic presence across process loss: previously accepted (GAME-ADR-0010), physical socket/grace state is ephemeral Coordinator state, and after transport grace a disconnect may become semantic `UserDisconnected` with reconnection producing `UserReconnected`. Open question: if the process disappears after the semantic disconnect edge was already established, how does a later process know whether a reconnect should merely bind/resync with no gameplay signal, or transition semantic presence DISCONNECTED -> CONNECTED and emit `UserReconnected`? See `AI_CONTEXT.md` for the full framing. A new checkpoint should be opened here once that design work produces a proposal for human review.
