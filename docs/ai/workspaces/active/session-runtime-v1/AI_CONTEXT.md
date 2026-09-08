Process: Architecture Discussion
Topic: Session Runtime lifecycle/runtime design
Current stage: Operational Lifecycle process-crash/recovery and durable inactivity expiration now closed at architecture level (process-agnostic recovery, RuntimeTurn crash/commit semantics, reconstruction from current checkpoint, `activity_expires_at` as the RUNNING inactivity source of truth, lazy materialization, Reaper vs. Archive Worker boundary); ready for the durable semantic presence across process loss milestone
Current execution surface: CONVERSATIONAL AI
Related durable artifacts: `ARCHITECTURE.md`, `game/README.md`, `identity/README.md`, `docs/ai/KNOWLEDGE_MAP.md`, `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md`, `docs/product/IDEAS.md`, `game/docs/decisions/GAME-ADR-0001-game-capability-persistence-transaction-boundary.md`, `game/docs/decisions/GAME-ADR-0002-session-runtime-durable-boundary.md`, `game/docs/decisions/GAME-ADR-0003-session-runtime-actor-and-lifecycle-foundations.md`, `docs/decisions/architecture/ADR-0005-cross-domain-public-entity-references.md`, `identity/docs/decisions/IDENTITY-ADR-0001-identity-user-public-identity-boundary.md`, `game/docs/decisions/GAME-ADR-0004-session-lobby-lifecycle-contract.md`, `game/docs/decisions/GAME-ADR-0005-session-public-and-internal-identity-boundary.md`, `game/docs/decisions/GAME-ADR-0006-game-language-root-player-roster-contract.md`, `game/docs/decisions/GAME-ADR-0007-session-runtime-turn-and-persistence-model.md`, `game/docs/decisions/GAME-ADR-0008-session-runtime-v1-timer-recovery-simplification.md`, `game/docs/decisions/GAME-ADR-0009-session-runtime-history-archival-and-hard-delete.md`, `game/docs/decisions/GAME-ADR-0010-session-disconnect-reconnect-resync-boundary.md`, `game/docs/decisions/GAME-ADR-0011-game-language-disconnect-reconnect-authored-semantics.md`, `game/docs/decisions/GAME-ADR-0012-game-language-keyed-timer-slots.md`, `game/docs/decisions/GAME-ADR-0013-session-runtime-process-agnostic-recovery.md`, `game/docs/decisions/GAME-ADR-0014-session-runtime-durable-inactivity-expiration.md`, `game/language/v1/program/README.md`, `game/language/v1/engine/README.md`, `game/language/v1/engine/LOGICAL_CONTRACT.md`, `docs/engineering/standards/cross-domain-reference-naming.md`
Blocked by: nothing at architecture level for process-crash/recovery/inactivity expiration; durable semantic presence across process loss (reconnect-after-process-loss resolving to bind-only vs. `UserReconnected`) is the next open topic
Next action: Conversational AI should design durable semantic presence across process loss (see Next Architecture Milestone below); do not enter implementation planning or create WORK yet
Last durable checkpoint: accepted (1) Session Runtime is process-agnostic - no `owner_process_id`/runtime-instance ownership/process-heartbeat ownership/ownership-only fencing generation/sticky ownership/durable `RECOVERING` phase; a process crash by itself is not a Session-domain event; (2) RuntimeTurn crash semantics - an uncommitted Turn rolls back entirely (no durable `PROCESSING` state, no Step-N resume), a committed Turn remains authoritative regardless of which process executed it; (3) recovery reconstructs from the current durable checkpoint (`sessions`, `session_runtime_state.current_turn_id`, its Snapshot, active Interactions/TimerObligations), not by replaying RuntimeTurn history; (4) every `RUNNING` Session gets a durable `activity_expires_at` inactivity deadline (configurable TTL, V1 illustrative ~10 minutes, not hard-coded into Game Language), renewed only by meaningful active-use operations, never by passive reads/polling; (5) `activity_expires_at` is the authoritative source of truth for expiration - not Reaper discovery - validated by every active-dependent operation under normal per-Session serialization, with lazy materialization (`phase=TERMINAL`, `terminal_reason=RUNTIME_INACTIVITY_EXPIRED`, `terminal_at=activity_expires_at`) and no revival/renewal/fabricated gameplay for a late-arriving operation; (6) a Session Reaper proactively materializes already-true expiration under the same serialization/revalidation rules and never itself decides expiration; (7) `terminal_at` always equals `activity_expires_at`, never Reaper/lazy-operation wall-clock time; (8) the Archive Worker remains completely separate and only consumes already-materialized `TERMINAL` Sessions; (9) rejected process-owned lease/heartbeat/fencing as a cleanup mechanism. Promoted to GAME-ADR-0013, GAME-ADR-0014, `game/README.md`, and `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` (new `sessions.activity_expires_at` column plus new documentation sections) as accepted-but-unimplemented. Also recorded the post-launch, non-authoritative "Session re-entry after complete client-state loss" idea in `docs/product/IDEAS.md` (registered-user "Current Sessions" resume, and guest same-JoinCode/same-display-name recovery explicitly not accepted as an identity/security mechanism). No WORK created; no production code/tests/migrations changed.
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

## Accepted Game Language Disconnect/Reconnect Authored Semantics

Status: HUMAN-APPROVED and canonically promoted; not yet implemented.

`UserDisconnected` and `UserReconnected` are standard `NamedSignalSource` platform/lifecycle signals (not new `SignalKind` variants), each exposing exactly one authored field `user: user` - Session-local runtime identity derived from `SessionActorID`, never `Identity.UserUUID`, connection/socket IDs, IP, timestamps, transport-grace information, or Coordinator internals. The pre-existing empty-schema `UserDisconnected` placeholder in `program/signal.go`/`compile_signals.go` is compatible scaffolding whose intended schema is now `user: user`; `UserReconnected` is newly accepted conceptually and does not exist in current code.

Session Runtime delivers both signals to the root workflow only - no implicit broadcast to nested child/task-group/ask-group instances, since the engine's `Step` contract addresses one instance path per call with no hidden multi-instance fan-out; nested workflows may later be coordinated explicitly by authored logic or a future language capability.

Handling either signal is optional. An unhandled signal is an ordinary rejected/unmatched signal (`ErrSignalRejected`) with no automatic gameplay consequence - Session Runtime must not infer remove/forfeit/pause/skip/end - and does not create a RuntimeTurn solely to record a no-op. "The game ignores disconnect" means only that disconnect itself does not mutate gameplay; it does not guarantee continued gameplay progress.

An interaction targeted at a SessionActor with no live transport connection is still created as a normal durable SessionInteraction and remains logically ACTIVE; Session Runtime must never suppress, auto-answer, skip, or redirect it, or deactivate the Participant, merely because the actor is offline (`logical interaction existence != successful live delivery`). Progress while an actor is offline is ordinary authored Game Language policy (e.g., an authored timer/timeout), not a Session Runtime inference; if no timeout applies and the player never reconnects, execution may legitimately remain waiting, and Session Runtime must not "intelligently" invent progress. A future max-runtime/inactivity/runaway protection may eventually terminate an abandoned Session but must never fabricate gameplay responses or silently perform authored actions - that topic remains deferred.

Canonical references: GAME-ADR-0011, `game/README.md` (Authored Game Language Disconnect/Reconnect Contract subsection), `game/language/v1/program/README.md`, `game/language/v1/engine/README.md`, `game/language/v1/engine/LOGICAL_CONTRACT.md`.

## Accepted Game Language Keyed Timer Slot Capability

Status: HUMAN-APPROVED and canonically promoted; not yet implemented.

The existing ordinary `TimerSlotDeclaration` holds at most one pending timer per statically named slot per workflow instance, which was found insufficient for a root-level policy needing multiple simultaneous independent timers (for example, a per-player disconnect timeout for P1 and P2 at once). This is not solved as a Session Runtime special case; Game Language instead gains a general `KeyedTimerSlot<Key>` capability - independently addressable pending timers identified by `(workflow instance/path, slot, key)`, at most one timer per exact tuple, with different keys fully independent. Scheduling into an already-occupied `(slot, key)` is an execution error with no implicit reset/replace/coalesce, mirroring the existing `TimerSlot` occupied-slot rule; cancellation affects only the selected key and is idempotent when none is pending; expiration exposes the authored key (conceptually `KeyedTimerExpired(slot)` carrying `key: KeyType`) without ever exposing internal timer-obligation UUIDs, wall-clock scheduling data, or database identifiers. This is a general primitive - also usable for cooldowns, team timers, per-object timers, keyed negotiations - explicitly not a `DisconnectTimer` special case; disconnect motivated but does not own the capability. Naming/API/Go type names are not frozen.

The accepted Session Runtime persistence model gains a nullable `session_timer_obligations.engine_key` discriminator (null for ordinary timers, populated for keyed timers) so a keyed timer's expiration can be reconstructed with the correct authored key on recovery; this is internal Session/engine routing metadata, never exposed to Coordinator/frontend merely because it is persisted. The long-term archive must eventually preserve whatever key metadata is necessary to replay archived keyed-timer history; the concrete archive JSON format remains deferred.

Canonical references: GAME-ADR-0012, `game/README.md` (Keyed Timer Slots subsection, and the Session Runtime Turn And Persistence Model section), `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` (Keyed Timer Discriminator section), `game/language/v1/program/README.md`, `game/language/v1/engine/README.md`, `game/language/v1/engine/LOGICAL_CONTRACT.md`.

## Accepted Process-Agnostic Recovery And Durable Inactivity Expiration

Status: HUMAN-APPROVED and canonically promoted; not yet implemented.

Session Runtime does not model durable ownership of a Session by a particular process/pod/runtime instance: no `owner_process_id`, no runtime instance ownership, no process heartbeat ownership, no fencing/takeover generation introduced solely for process ownership, no sticky Session ownership, no durable `RECOVERING` phase. A process crash by itself is not a Session-domain event and does not change `Session.phase`. The accepted RuntimeTurn transactional model (GAME-ADR-0007) is preserved for crash recovery: an uncommitted Turn rolls back entirely via ordinary database transaction atomicity (no durable `RuntimeTurn = PROCESSING`, no resuming "Step N"); a committed Turn remains fully authoritative regardless of which process executed it or what happened immediately after commit. A process resuming a `RUNNING` Session reconstructs current state from `sessions`, `session_runtime_state.current_turn_id`, the final Snapshot on that Turn, active `session_interactions`, and active `session_timer_obligations` - never by replaying RuntimeTurn `1..N` history, which remains useful only for history/debugging/archive/audit-replay tooling. Game Language timer recovery continues to use the GAME-ADR-0008 full-configured-delay tradeoff unchanged.

Every `RUNNING` Session gets a durable inactivity deadline, `sessions.activity_expires_at`, distinct from `lobby_expires_at` and not a process ownership lease. It is renewed (`= now + inactivity_ttl`, TTL configurable operational/product policy, V1 illustrative value ~10 minutes, never hard-coded into Game Language) only by operations that constitute meaningful evidence of continued active use (RuntimeTurn-producing gameplay, accepted interaction processing, timer expiration processing, meaningful lifecycle/runtime events, justified reconnect/resume activity - illustrative, not exhaustive); passive reads/polling must not renew it. `activity_expires_at` itself is the authoritative source of truth for expiration, not Reaper discovery, mirroring the already-accepted `lobby_expires_at` pattern (GAME-ADR-0004). Every active-dependent operation validates it under the normal per-Session serialization mechanism before processing and potentially renewing; an operation arriving after the deadline must not revive the Session merely because persisted `phase` still reads `RUNNING`, and may instead atomically materialize, in the same transaction, `phase = TERMINAL`, `terminal_reason = RUNTIME_INACTIVITY_EXPIRED`, `terminal_at = activity_expires_at`, then reject the attempted action - never renewing, reopening, fabricating gameplay, or creating a RuntimeTurn merely to represent expiration. A background Session Reaper proactively finds already-expired-but-unmaterialized Sessions and materializes them under the same serialization/revalidation rules, but does not itself decide expiration; if a legitimate operation already won the race and renewed the deadline, the Reaper rechecks and does nothing - the deadline, not Reaper scheduling latency, decides the outcome. `terminal_at` always equals `activity_expires_at`, never the Reaper's or a lazy operation's current wall-clock time (a multi-day platform outage does not move the semantic terminal instant). `terminal_reason = RUNTIME_INACTIVITY_EXPIRED` asserts only that the allowed inactivity period was exceeded - not a process crash, pod kill, host disconnect, or that every participant left - and is deliberately not named `PROCESS_CRASH`/`RUNTIME_ORPHANED`. The Archive Worker (GAME-ADR-0009) remains completely separate: it consumes only already-materialized `TERMINAL` Sessions per retention policy and must never inspect process ownership, detect crashes, determine RUNNING inactivity, or interpret `activity_expires_at`. Once `TERMINAL`, future gameplay timer/interactions are no longer executable as active gameplay; exact closure semantics for still-open Interactions/TimerObligations remain deferred, and materializing expiration must never fabricate engine responses or RuntimeTurns.

Canonical references: GAME-ADR-0013, GAME-ADR-0014, `game/README.md` (Session Runtime Process-Agnostic Recovery and Session Runtime Durable Inactivity Expiration sections), `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` (`sessions.activity_expires_at`, Process-Agnostic Recovery section, RUNNING Inactivity Deadline section).

## Deferred Design Topics

Authored Game Language disconnect/reconnect semantics, process-agnostic recovery/RuntimeTurn crash semantics, and durable RUNNING inactivity expiration are now CLOSED at architecture level (see the sections above) - these milestones are done, not deferred.

Other deferred topics: RUNNING-phase concurrent-input serialization strategy, bounded-loop protection while draining InternalSignals, post-commit Coordinator-delivery-failure handling, exhaustive `source_kind`/terminal-reason/interaction-kind enums, host transfer, max-runtime/runaway-session abuse policy (distinct from ordinary inactivity expiration), the final JSON archive schema and GCS implementation, mutable global display/profile ownership, Identity reconciliation/merge/alias semantics, Session Configuration/arbitrary external game-specific root params, the final idempotency JSON canonicalization/comparison algorithm, the concrete `KeyedTimerSlot<Key>` compiler/engine design and `engine_key` serialized representation, the exact renewal-triggering-operation enumeration and `inactivity_ttl` configuration surface, interaction/timer closure semantics at inactivity termination, durable semantic presence across process loss (now the next milestone - see below), and the post-launch client-state-loss re-entry idea recorded non-authoritatively in `docs/product/IDEAS.md` (not designed, not V1, not approved implementation).

## Next Architecture Milestone

Process crash / Session interruption semantics (process-agnostic recovery, RuntimeTurn crash semantics, and durable RUNNING inactivity expiration) are now CLOSED at architecture level. The next Operational Lifecycle topic is durable semantic presence across process loss:

Previously accepted (GAME-ADR-0010): physical socket/grace state is ephemeral Coordinator state; after transport grace, a disconnect may become semantic `UserDisconnected`; reconnection after semantic disconnect may produce `UserReconnected`.

Open question: if the process disappears after the semantic disconnect edge was already established, how does a later process know whether a reconnect should merely bind/resync with no gameplay signal, or transition semantic presence DISCONNECTED -> CONNECTED and emit `UserReconnected`?

Do not solve this now. Resume the Conversational AI from that question once this checkpoint's persistence is complete. Do not enter Feature Development, create WORK, or implement production code until this milestone is accepted and the repository workflow authorizes implementation.

## Current Implementation Facts And Drift To Carry Forward

- Identity has an accepted domain model but no implementation.
- `game/CURRENT_STATE.md` records Session Runtime as partial: session, session-state, session-player, and join-code schema exists; lifecycle scaffolding exists; `CreateRoom` and `JoinRoom` are stubs; no session execution flow was found implemented.
- Current Session Runtime scaffolding accepts externally supplied `engine.Program`, which conflicts with accepted architecture requiring Session Runtime to resolve/pin the playable immutable Game definition through the narrow Game Management read capability.
- Existing owner/player UUID fields are not aligned with the accepted `UserUUID` public boundary and internal `SessionActorID` runtime boundary.
- Join, Leave, Start, idempotency, and the standardized `players: list<user>` root roster contract are not implemented.
- None of the accepted RuntimeTurn/persistence-model tables (`session_requests`, `session_runtime_turns`, `session_runtime_steps`, `session_runtime_state`, `session_interactions`, `session_timer_obligations`, `session_history_archives`) exist in current migrations/schema, and `sessions.activity_expires_at` does not exist either; `game/docs/DATA_MODEL.md` still reflects only the pre-existing `sessions`/`session_players`/`session_states`/`join_codes` current-implementation shape.
- `game/language/v1/program/signal.go`'s `NamedSignalSource` doc comment and `game/language/v1/engine/internal/compiler/compile_signals.go`'s `namedLifecycleSignals` catalog still contain only a placeholder named signal `UserDisconnected` with an empty/unvalidated schema; `UserReconnected` does not exist anywhere in current code, and no `KeyedTimerSlot`/keyed-timer declaration, operation, or signal source exists anywhere in `program`/`engine`. GAME-ADR-0011 and GAME-ADR-0012 now accept the intended schema/capability (`UserDisconnected`/`UserReconnected` as `{user: user}`; a general keyed-timer-slot capability) but implementing them - extending the compiler's named-signal catalog, adding the keyed-timer declaration/operation/signal-source, and the `session_timer_obligations.engine_key` migration - remains unimplemented future work, not done by this checkpoint.
- `game/CURRENT_STATE.md` previously reported no known drift. This workspace records drift but does not update current-state documentation because current-state docs were excluded from this checkpoint.

## Explicitly Not Done

- No WORK was created.
- No production code, tests, migrations, authentication code, WebSocket handlers, Coordinator code, timers, or Session Runtime implementation were changed.
- No compiler/engine/program Go code was changed; the pre-existing empty-schema `UserDisconnected` placeholder in the compiler catalog was left untouched, `UserReconnected` was not added to it, and no keyed-timer declaration/operation/signal source was added anywhere.
- No current-state docs (`game/CURRENT_STATE.md`, `game/docs/DATA_MODEL.md`) were modified to pretend implementation exists.
- No GCS integration, archival worker, or final JSON archive schema was designed or implemented.
- No Reaper, recovery worker, timer/Coordinator implementation, or archival worker was implemented; no migration added `activity_expires_at` or any other column.
- No process ownership/heartbeat/fencing model was introduced anywhere.
- The post-launch "Session re-entry after complete client-state loss" idea was recorded in `docs/product/IDEAS.md` only as a non-authoritative idea; no UX/API/identity mechanism, guest-recovery credential design, or WORK was created for it, and guest identity semantics were not changed.
- No durable semantic presence across process loss (reconnect-after-process-loss bind-vs-`UserReconnected` question) was designed - that is the next milestone.
