Process: Architecture Discussion
Topic: Session Runtime lifecycle/runtime design
Current stage: Operational Lifecycle transport/platform boundary accepted (disconnect, reconnect, resync); ready for Game Language disconnect/reconnect semantics design
Current execution surface: CONVERSATIONAL AI
Related durable artifacts: `ARCHITECTURE.md`, `game/README.md`, `identity/README.md`, `docs/ai/KNOWLEDGE_MAP.md`, `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md`, `game/docs/decisions/GAME-ADR-0001-game-capability-persistence-transaction-boundary.md`, `game/docs/decisions/GAME-ADR-0002-session-runtime-durable-boundary.md`, `game/docs/decisions/GAME-ADR-0003-session-runtime-actor-and-lifecycle-foundations.md`, `docs/decisions/architecture/ADR-0005-cross-domain-public-entity-references.md`, `identity/docs/decisions/IDENTITY-ADR-0001-identity-user-public-identity-boundary.md`, `game/docs/decisions/GAME-ADR-0004-session-lobby-lifecycle-contract.md`, `game/docs/decisions/GAME-ADR-0005-session-public-and-internal-identity-boundary.md`, `game/docs/decisions/GAME-ADR-0006-game-language-root-player-roster-contract.md`, `game/docs/decisions/GAME-ADR-0007-session-runtime-turn-and-persistence-model.md`, `game/docs/decisions/GAME-ADR-0008-session-runtime-v1-timer-recovery-simplification.md`, `game/docs/decisions/GAME-ADR-0009-session-runtime-history-archival-and-hard-delete.md`, `game/docs/decisions/GAME-ADR-0010-session-disconnect-reconnect-resync-boundary.md`, `game/language/v1/program/README.md`, `game/language/v1/engine/README.md`, `game/language/v1/engine/LOGICAL_CONTRACT.md`, `docs/engineering/standards/cross-domain-reference-naming.md`
Blocked by: Game Language disconnect/reconnect semantics decisions
Next action: Conversational AI should design Game Language disconnect/reconnect semantics (see Next Architecture Milestone questions below); do not enter implementation planning or create WORK yet
Last durable checkpoint: accepted the disconnect/reconnect transport-and-platform boundary and the resynchronization architecture (AX-BH: physical disconnect does not change logical participation, Coordinator transport grace, semantic-disconnect escalation, reconnect reuses SessionActor/Participant, transport reconnect does not imply gameplay reinstatement, no new durable connection-state table, resync capability with a player-facing projection versioned by RuntimeTurn sequence, and the transport-grace-vs-semantic-disconnect RuntimeTurn rules); promoted to GAME-ADR-0010 and `game/README.md`; no WORK created
Last updated: 2026-09-07

# Resume Context

This workspace preserves the active initiative `session-runtime-v1`.

Goal: design and then incrementally implement the complete Session Runtime lifecycle. This is prioritized because playable multiplayer sessions are a core initial product capability.

The first architecture checkpoints are accepted and promoted to canonical owners. The initiative is still in Architecture Discussion. No Feature Development process or WORK has been opened.

This workspace records process continuity only. Accepted architecture/domain facts were promoted to canonical owners and ADRs where required. This file is not canonical implementation authority.

## Accepted Foundation Decisions

Status: HUMAN-APPROVED and canonically promoted.

Session Runtime owns all durable authoritative state that determines the meaning of a session and must be reconstructible after loss or restart of the process. `Stateless` means process-stateless/reconstructible, not that the domain itself has no state.

`Live Session Coordinator` is the conceptual responsibility boundary outside Session Runtime. The Coordinator owns ephemeral/runtime mechanisms such as live connection bindings, delivery/fan-out to connected clients, detection of physical disconnects, and physical timer/scheduling mechanisms. It does not own authoritative session/game truth or business consequences. This is a responsibility boundary, not a requirement to create another deployed service. V1 may keep it in the same Go process.

V1 does not introduce sticky-session correctness requirements, Redis, distributed session routing, distributed locks, or other multi-instance mechanisms. The initial single-process modular-monolith deployment may use local maps, channels, and Go timers for ephemeral mechanisms. Correctness of a session must not depend exclusively on those ephemeral objects.

Timer obligations that can affect game semantics are durable Session Runtime state. Coordinator owns the physical timer and knowledge that wall-clock time elapsed; Session Runtime owns the durable timer obligation and decides the semantic consequence. When a timer expires, Coordinator calls Session Runtime with the corresponding expiration signal/event. Pending or overdue obligations must be reconstructible/recoverable from durable state - this means the obligation and its configured delay survive, not that the exact original overdue/elapsed wall-clock position is reconstructed. GAME-ADR-0008 later accepted the concrete V1 tradeoff: no durable absolute deadline, full-configured-delay rescheduling on recovery.

Identity owns stable `User` identity and public `UserUUID`. A guest is already a `User`; normal guest-to-registered conversion preserves the same `UserUUID`. Session Runtime owns local `SessionActorID`. A SessionActor persistently correlates to `Identity.User` through `user_uuid`.

Public/app Session Runtime boundaries use an authenticated `UserUUID`; clients are not trusted sources of `UserUUID`. A trusted auth/application layer resolves credentials to `UserUUID`. Identity/Auth proves who the caller is; Session Runtime decides what the caller may do inside a Session.

`SessionActorID` is internal-only and is not a public/app contract. Coordinator, Orchestrator, adapters, and clients do not own the mapping. Host and Participant are independent relationships to Session-owned actor identity.

Canonical references: GAME-ADR-0002, GAME-ADR-0003, ADR-0005, IDENTITY-ADR-0001, GAME-ADR-0005, `ARCHITECTURE.md`, `identity/README.md`, `game/README.md`, and `docs/engineering/standards/cross-domain-reference-naming.md`.

## Accepted Lobby Lifecycle Contract

Status: HUMAN-APPROVED and canonically promoted.

Session-scoped lobby operations that materialize Session meaning serialize per Session. Join, Leave, Start, and expiration materialization are mutually serialized while the Session is in LOBBY. A database row lock is the natural V1 mechanism but not the canonical requirement. This accepted serialization statement applies to LOBBY only, not RUNNING.

Create Session/Room takes Game public UUID, authenticated host UserUUID, and opaque idempotency key. The caller does not provide an external `engine.Program`. Session Runtime resolves the current playable immutable Game definition through the narrow Game Management read capability, pins it, and may compile/validate outside the Session transaction. In the Session transaction it creates the LOBBY Session, host SessionActor correlated to `user_uuid`, host relationship, active JoinCode, and `lobby_expires_at`. The host is not automatically a Participant. QR codes and URLs are projections over JoinCode state.

JoinCode is authoritative Session Runtime state, not cache state. One active JoinCode exists per join-admissible lobby. It is revoked when the Session leaves LOBBY. Historical codes may be retained and textual code values are not forever reserved.

Join takes JoinCode, authenticated UserUUID, display-name snapshot from a trusted app/integration boundary, and opaque idempotency key. It resolves the JoinCode, obtains the pinned Game definition for constraints, enters serialized mutation, revalidates LOBBY and expiration, finds or creates the SessionActor for `(Session, UserUUID)`, and creates or reactivates Participant state. Repeated active Join has the same admission effect. Active Participants must not exceed `players.max` from the pinned Game definition.

Leave is a LOBBY participation operation. The public caller is `UserUUID`, resolved to SessionActor. It deactivates Participant and releases the slot, but does not delete SessionActor, remove host, or cancel the Session. A participating host may leave gameplay participation but remains host. Rejoin to an admissible lobby reuses the same SessionActor.

`lobby_expires_at` is authoritative. Join and Start correctness do not depend on a sweeper. If phase is LOBBY and now is at or after the deadline, an operation may atomically materialize TERMINAL with a lobby-expiration terminal reason, terminal timestamp equal to the deadline, and JoinCode revocation.

Start takes Session UUID, authenticated UserUUID, and opaque idempotency key. It resolves `(SessionUUID, UserUUID) -> SessionActorID`, verifies host authority, uses the pinned immutable definition/version, validates LOBBY, expiration, host, active Participants, `players.min`, and defensively `players.max`, initializes engine state, processes the engine-required initial signal/first Commit per engine contract, persists authoritative runtime state and durable consequences, atomically transitions to RUNNING, sets `started_at`, revokes JoinCode, and commits before external outputs. Failure must not produce partial RUNNING. Coordinator/client outputs occur after durable commit.

External mutating Session commands Create, Join, Leave, and Start accept an opaque idempotency key. Retries of the same logical command must not repeat effects. The final idempotency schema/algorithm remains undesigned.

Canonical references: GAME-ADR-0004, GAME-ADR-0005, and `game/README.md`.

## Accepted Game Language Root Roster Contract

Status: HUMAN-APPROVED and canonically promoted; not yet implemented as a complete validated contract.

Session Runtime supplies this standardized root input at Start:

```text
players: list<user>
```

Session Runtime builds `players` from active Participants at Start. Each `user` corresponds to Session-local runtime identity derived from SessionActorID. Identity.UserUUID is never exposed to authored Game Language or engine semantics.

Authored games should use the standard `players` root parameter instead of inventing custom root parameter names for the participant roster.

For V1, arbitrary external game-specific root parameters are deferred until Session Configuration is designed.

Canonical references: GAME-ADR-0006, `game/language/v1/program/README.md`, `game/language/v1/engine/README.md`, and `game/language/v1/engine/LOGICAL_CONTRACT.md`.

## Accepted Runtime Turn, Persistence Model, Timer Recovery, And Archival Direction

Status: HUMAN-APPROVED and canonically promoted.

`RuntimeTurn` is the historical/transactional unit for RUNNING-phase execution: one external/runtime cause, 1..N internal `engine.Step` calls in one transaction, one resulting Session-state `sequence`, and one authoritative final Snapshot. `RuntimeStep` is technical-only execution history beneath a Turn and does not own a Session-state sequence.

The accepted persistence model adds `session_requests` (idempotency, replacing the earlier proposed `session_command_receipts` name, storing `request_payload` rather than a `request_hash`), `session_runtime_turns`, `session_runtime_steps`, `session_runtime_state` (a thin pointer to `current_turn_id`, no duplicated Snapshot), `session_interactions`, and `session_timer_obligations`. Interaction and Timer durability/recovery is modeled through Turn-level references (`opened_by_turn_id`/`created_by_turn_id`, `closed_by_turn_id`, `source_interaction_id`/`source_timer_obligation_id`), not Step-level or sequence-range bookkeeping.

V1 does not persist an absolute due-at/deadline for Game Language timers and does not include a durable Coordinator-owned `live_timer_schedules` table. Session Runtime persists only the timer obligation and its configured delay; on recovery, active obligations may be rescheduled using their full configured delay from the new scheduling moment, and preserving elapsed wall-clock time across a full process restart is not required for V1. `lobby_expires_at` is unaffected and remains a separate, already-accepted Session lifecycle deadline.

Long-term runtime-history archival direction is accepted: PostgreSQL remains the hot/runtime store; a versioned JSON artifact may eventually be written to long-term object storage (for example GCS); `session_history_archives` tracks this per Session with a provider/key identifier (not an expiring URL). Heavy runtime/history tables (`session_runtime_state`, `session_runtime_turns`, `session_runtime_steps`, `session_interactions`, `session_timer_obligations`) may become an explicit hard-delete exception, but only after the archive is successfully written and verified (`READY` status, checksum). `session_actors` and `session_participants` are excluded from this deletion policy and remain relationally stored indefinitely for product queries. The concrete JSON archive schema and GCS implementation remain DEFERRED.

Canonical references: GAME-ADR-0007, GAME-ADR-0008, GAME-ADR-0009, `game/README.md`, and `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` (full accepted table/column/relationship diagram).

## Accepted Disconnect, Reconnect, And Resynchronization Boundary

Status: HUMAN-APPROVED and canonically promoted.

Three responsibilities stay distinct and must not collapse into one owner: the transport fact that a physical connection disappeared/returned (Coordinator); the platform/runtime semantic event that a Session User is now considered disconnected/reconnected (Session Runtime boundary); and the gameplay consequence of that event (Session Runtime + Game Language).

A physical disconnect does not deactivate `session_participants`, remove a Participant, free a slot, delete a SessionActor, or terminate the Session; Session Runtime does not add authoritative connection-state fields (`is_connected`, `connection_id`, `websocket_id`). The Coordinator may apply a short, in-memory transport grace/debounce period (duration not decided here, implementation-level/configurable) to absorb transient failures; a reconnect within grace rebinds the connection and cancels escalation with no semantic disconnect ever observed and no RuntimeTurn created. If grace expires without reconnection, the Coordinator notifies Session Runtime of a semantic disconnect; the Coordinator never decides gameplay consequences (remove/forfeit/pause/terminate/continue) - those belong to Session Runtime/Game Language. The exact Game Language disconnect contract/name is not approved; no name (for example `PlayerDisconnected`) is frozen as syntax/API.

Reconnect is not Join: Session Runtime resolves `(SessionUUID, UserUUID) -> existing SessionActor`, reusing existing SessionActor/Participant identity without creating a new slot, requiring a JoinCode, or rerunning lobby admission. Transport reconnect does not imply gameplay reinstatement - if the game already reacted to the semantic disconnect (removed/forfeited/disabled), a transport-level reconnect does not itself undo that; a corresponding reconnect semantic event may need to reach Session Runtime/Game Language once that contract exists.

No new durable connection-state/grace table is introduced for V1; live connection bindings and transport-grace state remain ephemeral Coordinator state, and existing durable state (SessionActor, Participant, current RuntimeTurn/Snapshot, active Interactions/TimerObligations) remains sufficient.

Session Runtime exposes a resync read capability keyed by `(SessionUUID, UserUUID)` (exact transport/DTO not frozen) that returns a player-facing Session projection - never the raw `engine.Snapshot`, and never leaking SessionActorIDs/engine paths/slots - carrying the current RuntimeTurn `sequence` as its monotonic version (no second version counter). Reconnect during transport grace produces no RuntimeTurn and is a pure resync read; reconnect after an already-escalated semantic disconnect processes gameplay/runtime semantics first (through the future Game Language contract, which may produce a RuntimeTurn) and only then builds resync from the resulting current state.

Game-defined disconnect/reconnect policy (continue/wait/timer/forfeit/remove/pause/end - examples only) remains unresolved and is the next design topic; no signal name, syntax, required handler, default policy, timeout, or automatic removal semantics is invented by this checkpoint.

Canonical references: GAME-ADR-0010, `game/README.md` (Session Runtime Disconnect, Reconnect, and Resynchronization Boundary section).

## Deferred Design Topics

Authored Game Language disconnect/reconnect policy remains DEFERRED - see Next Architecture Milestone. This is narrower than before: the transport-and-platform boundary itself (grace, semantic escalation, reconnect identity, resync) is now accepted; what remains open is how authored games observe and react to these platform events.

Current human preference/intuition, not an accepted design: some reconnect/inactivity semantics may need to be expressible by the game itself because different games can require different behavior.

Other deferred topics: RUNNING-phase concurrent-input serialization strategy, bounded-loop protection while draining InternalSignals, post-commit Coordinator-delivery-failure handling, exhaustive `source_kind`/terminal-reason/interaction-kind enums, host transfer, max-runtime/runaway-session policy, the final JSON archive schema and GCS implementation, mutable global display/profile ownership, Identity reconciliation/merge/alias semantics, Session Configuration/arbitrary external game-specific root params, the final idempotency JSON canonicalization/comparison algorithm, process crash/session-interruption semantics, and runaway/abuse protections (deferred unless they turn out to be a direct consequence of the Game Language disconnect contract).

## Next Architecture Milestone

Design Game Language disconnect/reconnect semantics before implementation planning:

1. How does Session Runtime represent disconnect/reconnect to the engine?
2. Are they standardized platform signals?
3. How does an authored game opt in/handle them?
4. What happens if a game defines no disconnect handler?
5. Can disconnect behavior create normal Game Language timers/interactions?
6. How does a reconnect interact with a timer previously started because of disconnect?
7. Which policies are universal Session invariants versus authored game behavior?
8. What minimum safe/default behavior exists if a game ignores these events?

Do not yet design process crash/interruption behavior or runaway/abuse limits unless needed as a direct consequence of the Game Language disconnect contract. Do not enter Feature Development, create WORK, or implement production code until this milestone is accepted and the repository workflow authorizes implementation.

## Current Implementation Facts And Drift To Carry Forward

- Identity has an accepted domain model but no implementation.
- `game/CURRENT_STATE.md` records Session Runtime as partial: session, session-state, session-player, and join-code schema exists; lifecycle scaffolding exists; `CreateRoom` and `JoinRoom` are stubs; no session execution flow was found implemented.
- Current Session Runtime scaffolding accepts externally supplied `engine.Program`, which conflicts with accepted architecture requiring Session Runtime to resolve/pin the playable immutable Game definition through the narrow Game Management read capability.
- Existing owner/player UUID fields are not aligned with the accepted `UserUUID` public boundary and internal `SessionActorID` runtime boundary.
- Join, Leave, Start, idempotency, and the standardized `players: list<user>` root roster contract are not implemented.
- None of the accepted RuntimeTurn/persistence-model tables (`session_requests`, `session_runtime_turns`, `session_runtime_steps`, `session_runtime_state`, `session_interactions`, `session_timer_obligations`, `session_history_archives`) exist in current migrations/schema; `game/docs/DATA_MODEL.md` still reflects only the pre-existing `sessions`/`session_players`/`session_states`/`join_codes` current-implementation shape.
- `game/language/v1/program/signal.go`'s `NamedSignalSource` doc comment and `game/language/v1/engine/internal/compiler/compile_signals.go`'s `namedLifecycleSignals` catalog already contain a placeholder named signal `UserDisconnected` with an empty/unvalidated schema, predating this checkpoint. This is existing implementation scaffolding, not an accepted Game Language disconnect contract - GAME-ADR-0010 explicitly declines to freeze any name (including `PlayerDisconnected` or `UserDisconnected`) before that contract is designed. The next milestone (Game Language disconnect/reconnect semantics) must explicitly reconcile whether this placeholder is kept, renamed, or replaced.
- `game/CURRENT_STATE.md` previously reported no known drift. This workspace records drift but does not update current-state documentation because current-state docs were excluded from this checkpoint.

## Explicitly Not Done

- No WORK was created.
- No production code, tests, migrations, authentication code, WebSocket handlers, Coordinator code, timers, or Session Runtime implementation were changed.
- No current-state docs (`game/CURRENT_STATE.md`, `game/docs/DATA_MODEL.md`) were modified to pretend implementation exists.
- No Game Language disconnect/reconnect contract (signal names, syntax, handlers, default policy) was designed; the pre-existing `UserDisconnected` placeholder in the compiler catalog was not changed.
- No final reconnect/Operational Lifecycle contract beyond the accepted transport/platform boundary was designed.
- No GCS integration, archival worker, or final JSON archive schema was designed or implemented.
