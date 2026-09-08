# Session Disconnect, Reconnect, And Resynchronization Checkpoint

Process: Architecture Discussion

Status: RESOLVED - accepted 2026-09-07. No checkpoint is currently pending; this file is retained as the human-facing record of the resolved checkpoint until the next milestone (Game Language disconnect/reconnect semantics) produces its own checkpoint.

## What Was Approved

The disconnect/reconnect transport-and-platform boundary and the Session resynchronization architecture were reviewed and accepted.

- Three responsibilities stay distinct: the transport fact (Coordinator: connection disappeared/returned), the platform/runtime semantic event (Session Runtime boundary: this User is now considered disconnected/reconnected), and the gameplay consequence (Session Runtime + Game Language).
- A physical disconnect does not change logical participation - no deactivation of `session_participants`, no slot freed, no SessionActor deleted, no Session terminated, and no new authoritative connection-state fields (`is_connected`, `connection_id`, `websocket_id`).
- Coordinator may apply a short, in-memory transport grace period to absorb transient failures; reconnecting within grace cancels escalation with no semantic disconnect and no RuntimeTurn. The grace duration is not decided here.
- If grace expires, Coordinator escalates a semantic disconnect to Session Runtime; Coordinator never decides gameplay consequences. The Game Language disconnect contract/name is not approved - no name (including `PlayerDisconnected`) is frozen.
- Reconnect is not Join: it resolves `(SessionUUID, UserUUID) -> existing SessionActor` and reuses existing Participant identity - no new slot, no JoinCode, no lobby admission rerun.
- Transport reconnect does not imply gameplay reinstatement: if the game already reacted to the semantic disconnect, a transport-level reconnect does not itself undo that.
- No new durable connection-state/grace table for V1; transport-grace state stays ephemeral Coordinator state.
- Session Runtime exposes a resync read capability keyed by `(SessionUUID, UserUUID)` returning a player-facing projection (never the raw `engine.Snapshot`, never leaking internal engine identities), versioned by the existing RuntimeTurn `sequence`.
- Reconnect during transport grace is a pure resync read with no RuntimeTurn; reconnect after an already-escalated semantic disconnect processes gameplay/runtime semantics first (which may produce a RuntimeTurn) and then builds resync from the resulting state.
- Game-defined disconnect/reconnect policy (continue/wait/timer/forfeit/remove/pause/end) remains unresolved and is the next design topic - none of those are accepted syntax or policy.

## Where This Was Persisted

- `docs/decisions/architecture/ADR-0013-session-disconnect-reconnect-resync-boundary.md`
- `game/README.md` (new "Session Runtime Disconnect, Reconnect, and Resynchronization Boundary" section; the previously stale fully-deferred disconnect/reconnect paragraph in "Session Runtime Actor and Lifecycle Model" was updated to point at the newly accepted boundary and narrow what remains deferred to authored Game Language policy)
- `game/README.md` (the ADR-0003-derived timer-recovery paragraph in "Session Runtime Durable Boundary" was clarified to reference the ADR-0011 V1 tradeoff, so "reconstructible/recoverable" is not misread as requiring exact overdue wall-clock reconstruction)
- `docs/decisions/architecture/INDEX.md` (ADR-0013 row)

## Explicitly Not Authorized By This Checkpoint

No production code, WebSocket handlers, Coordinator code, timers, migrations, tests, authentication code, or Game Language changes were created or changed. No WORK was created. The pre-existing `UserDisconnected` placeholder named signal already present in `game/language/v1/program/signal.go` / `game/language/v1/engine/internal/compiler/compile_signals.go` was left untouched and is explicitly not endorsed as the accepted Game Language disconnect contract.

## Next Milestone (Not Designed Here)

Game Language disconnect/reconnect semantics: how Session Runtime represents disconnect/reconnect to the engine, whether they are standardized platform signals, how an authored game opts in/handles them, default behavior when a game defines no handler, interaction with timers/interactions, which policies are universal Session invariants versus authored behavior, and minimum safe default behavior. Process crash/interruption semantics and runaway/abuse protections stay deferred unless they turn out to be a direct consequence of that contract. See `AI_CONTEXT.md` for the full question list. A new checkpoint should be opened here once that design work produces a proposal for human review.
