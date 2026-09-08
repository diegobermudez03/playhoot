# Game Language Disconnect/Reconnect Semantics Checkpoint

Process: Architecture Discussion

Status: RESOLVED - accepted 2026-09-07. No checkpoint is currently pending; this file is retained as the human-facing record of the resolved checkpoint until the next milestone (process crash / Session interruption semantics) produces its own checkpoint.

## What Was Approved

Authored Game Language disconnect/reconnect semantics - the milestone GAME-ADR-0010 left open - were reviewed and accepted, along with a general Game Language capability the disconnect use case exposed a need for.

- `UserDisconnected` and `UserReconnected` are standard `NamedSignalSource` platform/lifecycle signals (not new `SignalKind` variants), each exposing exactly one authored field `user: user` - Session-local runtime identity derived from SessionActorID, never `Identity.UserUUID`, connection/socket IDs, IP, timestamps, transport-grace information, or Coordinator internals. The pre-existing empty-schema `UserDisconnected` placeholder is treated as compatible scaffolding whose intended schema is now `user: user`; `UserReconnected` is newly accepted conceptually.
- Session Runtime delivers both signals to the root workflow only - no implicit broadcast to nested child/task-group/ask-group instances, since the engine's `Step` contract addresses one instance path per call with no hidden multi-instance fan-out.
- Handling either signal is optional. An unhandled signal has no automatic gameplay consequence (no inferred remove/forfeit/pause/skip/end) and does not create a RuntimeTurn solely to record a no-op. "The game ignores disconnect" means only that disconnect itself does not mutate gameplay - it does not guarantee continued progress.
- An interaction targeted at an offline SessionActor is still created as a normal durable SessionInteraction, remaining logically ACTIVE; Session Runtime must never suppress, auto-answer, skip, redirect it, or deactivate the Participant merely because the actor is offline. Logical interaction existence is independent of successful live delivery; reconnect/resync can recover a still-active interaction.
- Progress while an actor is offline is ordinary authored Game Language policy (e.g., an authored timer/timeout), not a Session Runtime inference; if no timeout applies and the player never reconnects, execution may legitimately remain waiting. A future max-runtime/inactivity/runaway protection may eventually terminate an abandoned Session but must never fabricate gameplay responses.
- The existing single-pending-timer `TimerSlotDeclaration` was found insufficient for expressing independent per-player offline-timeout policy (e.g., separate disconnect timers for P1 and P2). Game Language will support a general `KeyedTimerSlot<Key>` capability - independently addressable pending timers per `(workflow instance/path, slot, key)`, at most one timer per exact tuple, schedule-into-occupied-tuple is an atomic execution error with no implicit reset/replace, cancellation affects only the selected key, and expiration exposes the authored key. This is a general primitive (also for cooldowns, team timers, per-object timers), explicitly not a `DisconnectTimer` special case. Naming/API/Go types are not frozen.
- The accepted Session Runtime persistence model gains a nullable `session_timer_obligations.engine_key` discriminator (null for ordinary timers, populated for keyed timers) as internal Session/engine routing metadata, never exposed to Coordinator/frontend merely because it is persisted.

## Where This Was Persisted

- `game/docs/decisions/GAME-ADR-0011-game-language-disconnect-reconnect-authored-semantics.md`
- `game/docs/decisions/GAME-ADR-0012-game-language-keyed-timer-slots.md`
- `game/docs/decisions/INDEX.md` (GAME-ADR-0011 and GAME-ADR-0012 rows; next Game ADR is now `GAME-ADR-0013`)
- `game/README.md` (Session Runtime Disconnect, Reconnect, and Resynchronization Boundary section gains "Authored Game Language Disconnect/Reconnect Contract" and "Keyed Timer Slots" subsections; the Session Runtime Turn And Persistence Model section notes the `engine_key` persistence consequence; the previously deferred framing in Session Runtime Actor and Lifecycle Model is updated to reflect this milestone as closed)
- `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` (`session_timer_obligations.engine_key` added to the diagram, with a new "Keyed Timer Discriminator" section; archive-metadata and "Not Yet Decided" sections updated)
- `game/language/v1/program/README.md` and `game/language/v1/engine/README.md` (accepted-but-not-yet-implemented notes for both the disconnect/reconnect signal contract and the keyed-timer capability, explicit that current code does not implement either)
- `game/language/v1/engine/LOGICAL_CONTRACT.md` (same accepted-but-unimplemented notes, concise form)

## Explicitly Not Authorized By This Checkpoint

No production code, compiler/engine/program Go changes, migrations, tests, Coordinator behavior, WebSocket handlers, or WORK were created or changed. The pre-existing `UserDisconnected` placeholder in `game/language/v1/program/signal.go` / `game/language/v1/engine/internal/compiler/compile_signals.go` (empty schema) was left untouched, as was the absence of `UserReconnected` and of any keyed-timer declarations/operations/signal source anywhere in the current implementation. This drift (accepted design vs. current code) is intentionally left visible rather than papered over in source comments.

## Next Milestone (Not Designed Here)

Process crash / Session interruption semantics: what happens to RUNNING Sessions when the process dies and restarts; what durable state is sufficient to reconstruct a Session; how Coordinator rediscovers/rebuilds active timer obligations; how live clients reconnect to a restarted process; whether Session needs an explicit `INTERRUPTED`/`RECOVERING` lifecycle state; what happens if a RuntimeTurn transaction was in-flight when the process died; how to distinguish recoverable process loss from a permanently invalid/corrupted Session; which failures terminalize a Session versus simply require reconstruction; and how the already-accepted "restart full timer delay" tradeoff (GAME-ADR-0008) fits recovery. General abuse/runaway-session limits stay a separate, not-yet-scheduled concern unless they turn out to be a direct consequence of this topic. See `AI_CONTEXT.md` for the full question list. A new checkpoint should be opened here once that design work produces a proposal for human review.
