# Session Runtime Turn Architecture Checkpoint

Process: Architecture Discussion

Status: RESOLVED - accepted 2026-09-07. No checkpoint is currently pending; this file is retained as the human-facing record of the resolved checkpoint until the next milestone (Operational Lifecycle) produces its own checkpoint.

## What Was Approved

The RuntimeTurn architecture, the full Session Runtime persistence model, the V1 Game Language timer recovery simplification, and the runtime-history archival/hard-delete direction were reviewed and accepted.

- `RuntimeTurn` is the historical/transactional unit: one external cause, 1..N internal engine Steps in one transaction, one Session-state sequence, one final authoritative Snapshot. `RuntimeStep` is technical-only history and does not carry a Session-state sequence.
- The accepted schema adds `session_requests` (idempotency; not `session_command_receipts`; no `request_hash`), `session_runtime_turns`, `session_runtime_steps`, `session_runtime_state` (a thin current-position pointer, no duplicated Snapshot), `session_interactions`, and `session_timer_obligations`, alongside the previously accepted `sessions`, `session_actors`, `session_participants`, and `join_codes`.
- Interaction and Timer durability/recovery is modeled through Turn-level references, not Step-level or sequence-range bookkeeping (see the worked examples in `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md`).
- V1 does not persist an absolute timer deadline and does not include a durable `live_timer_schedules` table; Coordinator remains the time-aware layer; recovery reschedules using the full configured delay. `lobby_expires_at` is unaffected.
- Long-term archival direction is accepted: PostgreSQL stays hot; a versioned JSON artifact may eventually go to long-term object storage (for example GCS), tracked by `session_history_archives` (provider/key, not an expiring URL). Heavy runtime/history tables may be hard-deleted only after verified (`READY`, checksummed) archival. `session_actors`/`session_participants` are excluded from that deletion policy and remain relationally stored indefinitely. The concrete JSON schema and GCS implementation are deferred.

## Where This Was Persisted

- `docs/decisions/architecture/ADR-0010-session-runtime-turn-and-persistence-model.md`
- `docs/decisions/architecture/ADR-0011-session-runtime-v1-timer-recovery-simplification.md`
- `docs/decisions/architecture/ADR-0012-session-runtime-history-archival-and-hard-delete.md`
- `game/README.md` (Session Runtime Turn And Persistence Model, Session Runtime History Archival Direction sections)
- `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` (full accepted table/column/relationship diagram; `game/docs/DATA_MODEL.md` remains current-implementation only and was not changed)
- `docs/ai/KNOWLEDGE_MAP.md` (Specialized Documentation entry for the new accepted-design document)

## Explicitly Not Authorized By This Checkpoint

No migrations, production code, repositories, tests, workers, GCS integration, or Coordinator code were created or changed. No WORK was created.

## Next Milestone (Not Designed Here)

Operational Lifecycle: disconnect, reconnect, temporary connection loss versus logical Participant membership, what Session Runtime knows versus Coordinator, whether/how Game Language can define disconnect/reconnect/inactivity behavior, process crash/session interruption semantics, and eventual runaway/abuse protections. See `AI_CONTEXT.md` for the full question list. A new checkpoint should be opened here once that design work produces a proposal for human review.
