# Game

Status: CANONICAL DOMAIN MODEL

## Responsibility

Game owns the authored lifecycle and runtime execution of Playhoot games.

## Owns

- Authored games and their definitions/versions.
- Game metadata and visibility/publication state.
- Live game sessions.
- Session runtime state and participation.
- Execution lifecycle of a game session.

## Does Not Own

- Transport/network connections.
- Identity/profile ownership.
- Public discovery/search experience.

## Internal Structure

### Business Capabilities

- Game Management: owns authored game lifecycle concerns.
- Session Runtime: owns execution/session lifecycle concerns.

### Supporting Subsystems

- Game Language: defines, compiles, and executes game behavior for Game; it is not a business domain.

## Capability Persistence and Transaction Boundary

Game Management and Session Runtime share one business domain but not one persistence or transaction boundary.

- Game Management owns its persisted state: authored games, definitions/versions, publication/visibility state, images, and related authored-game history.
- Session Runtime owns its persisted state: sessions, session state, participants, join codes, and other live-runtime persistence.
- Both may currently share the same physical PostgreSQL database. Physical co-location does not authorize either capability to mutate the other's tables, and does not require separate schemas/namespaces to simulate future separation.
- Their migrations and persistence implementations remain independently owned.
- No database transaction spans both Game Management-owned and Session Runtime-owned state, and a transaction handle must not be propagated from one capability into the other.

Session Runtime may require Game Management information — in particular, the game definition/version that defines runtime behavior — but depends on a narrow Game Management definition/read capability contract rather than on Game Management's repository as its architectural API. Concrete infrastructure may be reused underneath that contract, but persistence-implementation sharing must not erase capability ownership.

A session must be pinned to a concrete execution definition/version, and the definition observed by an existing session must remain semantically stable for that session's lifetime; version immutability for existing sessions is the preferred model. Changing a game's current definition affects only newly created sessions.

This boundary is Game-specific and preserves an inexpensive future path toward independently deploying Session Runtime; it is not a general rule that every pair of internal capabilities must have independent transaction boundaries. Rationale and alternatives are recorded in `game/docs/decisions/GAME-ADR-0001-game-capability-persistence-transaction-boundary.md`.

## Session Runtime Durable Boundary

Session Runtime owns all durable authoritative state that determines the meaning of a session and must be reconstructible after loss or restart of the process.

"Stateless" for Session Runtime means process-stateless/reconstructible. It does not mean the capability has no durable domain state.

`Live Session Coordinator` is the accepted conceptual responsibility boundary outside Session Runtime for ephemeral/runtime mechanisms:

- live connection bindings;
- delivery/fan-out to connected clients;
- detection of physical disconnects;
- physical timer/scheduling mechanisms.

The Coordinator does not own authoritative session/game truth or business consequences. This boundary does not require another deployed service; V1 may keep the Coordinator in the same Go process.

V1 does not introduce sticky-session correctness requirements, Redis, distributed session routing, distributed locks, or other multi-instance mechanisms. The initial single-process modular-monolith deployment may use local maps, channels, and Go timers for ephemeral mechanisms, but session correctness must not depend exclusively on those ephemeral objects.

Timer obligations that can affect game semantics are durable Session Runtime state. The Coordinator owns the physical timer and detects elapsed wall-clock time; Session Runtime owns the durable timer obligation and decides the semantic consequence, normally through its authoritative runtime/engine flow. A process crash may destroy physical timers but must not silently erase timer obligations or change game semantics. Pending or overdue obligations must be reconstructible/recoverable from durable state - recoverable means the obligation and its configured delay survive, not that the exact original overdue/elapsed wall-clock position is reconstructed. V1 persists only the obligation and its `delay_ms`, not an absolute deadline; on recovery, active obligations may be rescheduled using their full configured delay from the new scheduling moment, per the accepted `game/docs/decisions/GAME-ADR-0008-session-runtime-v1-timer-recovery-simplification.md` tradeoff.

Rationale and alternatives are recorded in `game/docs/decisions/GAME-ADR-0002-session-runtime-durable-boundary.md`.

## Session Runtime Actor and Lifecycle Model

Host and Participant are independent concepts. Creating a Session establishes a host but does not automatically make that host a gameplay participant. The same Session-owned actor may be both host and Participant, but those are separate relationships.

V1 does not introduce separate mutable `creator` and `host` concepts unless another accepted decision requires them. Host transfer is not currently designed.

Participant is session-scoped. Session Runtime does not own a cross-session Player/Profile master entity. A Participant belongs to one Session and contains Session-owned participation/lifecycle state plus a display-name snapshot.

V1 does not include a generic persisted `role` field merely to anticipate spectators, judges, controllers, or gameplay roles. Gameplay roles belong primarily to game definition/runtime state unless Session Runtime later has a concrete cross-game reason to own such a distinction.

Session-local uniqueness is the accepted invariant: an actor may occupy at most one active logical participation for the same Session. Game does not establish a platform-wide invariant that one User may belong to only one active Session.

Session Runtime conceptually owns a session-scoped identity such as `SessionActorID`. Host and Participant relationships refer to this local identity. Runtime/domain operations should primarily operate using Session-owned identity rather than propagating `UserUUID` throughout engine/runtime behavior.

A SessionActor may persist `user_uuid` as a cross-domain reference to `Identity.User`. This is a logical domain reference, not ownership of User/Profile, not permission to read Identity persistence, not dependency on Identity table layout, and not a cross-domain database foreign key.

Session Runtime owns its participation-time display-name snapshot associated with SessionActor/Participant representation. Mutable global display/profile ownership remains unresolved outside Game.

User-originated operations crossing the public/application boundary of Session Runtime identify the caller through authenticated `UserUUID`. The external client is not the trusted source of `UserUUID`; a trusted authentication/application layer resolves credentials into an authenticated `UserUUID`.

Session Runtime owns the translation from `(Session UUID, UserUUID)` to `SessionActorID`. `SessionActorID` is internal Session-owned identity, not part of Session Runtime's public/application-facing contract. Coordinator, Orchestrator, HTTP/WebSocket adapters, and clients do not own or maintain the mapping from `UserUUID` to `SessionActorID`.

Game Language / engine user identity represents the Session-local actor and must not receive `Identity.UserUUID`. When engine/runtime behavior targets a Session-local actor, Session Runtime resolves that internal actor back to `SessionActor.user_uuid` / `UserUUID` before crossing the Session Runtime boundary. Identity/Auth proves who the caller is; Session Runtime decides whether that User may act as host, active Participant, or another Session-owned relationship in the current lifecycle/runtime state.

Use the conceptual Session lifecycle:

```text
LOBBY -> RUNNING -> TERMINAL
```

`TERMINAL` is irreversible. Terminal cause/reason is modeled separately from lifecycle phase; the exhaustive terminal-reason enum is not yet accepted.

`LOBBY` has its own durable expiration concern, `lobby_expires_at`; Join/Start correctness must enforce this durable deadline even if no asynchronous cleanup/sweeper has materialized the Session as terminal yet. `RUNNING` has its own separate durable inactivity deadline, `activity_expires_at` (see Session Runtime Durable Inactivity Expiration below); neither deadline is a generic whole-session lifetime, and a later max-runtime/runaway-session abuse policy remains a different, still-deferred concern.

Physical connection presence belongs to the Live Session Coordinator and must not be persisted as authoritative state through fields such as `is_connected`, `connection_id`, or `websocket_id`. Session Runtime does durably persist a coarser, already-debounced platform fact on the SessionActor itself: `semantic_presence` (`CONNECTED | DISCONNECTED`), answering only "has this SessionActor crossed the Coordinator -> Session semantic connected/disconnected boundary?" This is distinct from `session_participants.active`, which answers "does this actor currently occupy/admit a participant position?" - a phase-dependent question (see Session Runtime Lobby Lifecycle Contract and Session Runtime Disconnect, Reconnect, and Resynchronization Boundary below). The two may legitimately diverge, for example a reconnecting actor that is semantically `CONNECTED` but locked out of a full lobby.

The disconnect/reconnect transport-and-platform boundary, the authored Game Language disconnect/reconnect contract, and the phase-dependent LOBBY-vs-RUNNING participation consequence are all now accepted; see Session Runtime Disconnect, Reconnect, and Resynchronization Boundary below. What remains authored (not a Session Runtime invariant) is how a game behaves while an actor is offline - waiting, timers, forfeiture - beyond the platform lifecycle signals themselves.

Rationale and alternatives are recorded in `game/docs/decisions/GAME-ADR-0003-session-runtime-actor-and-lifecycle-foundations.md`.

The public/internal identity boundary is refined by `game/docs/decisions/GAME-ADR-0005-session-public-and-internal-identity-boundary.md`.

## Session Runtime Lobby Lifecycle Contract

All mutations that can alter lobby membership or cross the lobby/start boundary for one Session must serialize against that Session. This includes Join, Leave, Start, and materialization of lobby expiration when performed by one of these operations or housekeeping. The accepted architecture is the per-Session serialization guarantee, not one specific SQL syntax. This rule applies to the LOBBY lifecycle only; high-frequency RUNNING runtime concurrency remains a later architecture topic.

CreateSession/CreateRoom receives conceptually a Game public UUID, authenticated host `UserUUID`, and opaque idempotency key. Session Runtime resolves the currently playable immutable Game definition/version through the narrow Game Management read capability, pins the Session to that immutable definition/version, and does not accept an externally-precompiled `engine.Program`.

Within one Session-owned transaction, Create creates a Session in `LOBBY`, a host SessionActor correlated through `user_uuid`, the Session host relationship pointing to that SessionActor, an active JoinCode, and `lobby_expires_at`. Creating the Session does not automatically make the host a gameplay Participant. `lobby_expires_at` is V1 Session Runtime policy unless another accepted product decision says otherwise. Public responses do not expose `SessionActorID`; QR codes and join URLs are projection/application concerns derived from JoinCode.

JoinCode is authoritative Session Runtime state, not cache-owned truth. V1 has one active JoinCode for a lobby unless a future accepted feature requires rotation/multiple codes. A JoinCode is valid only while its Session remains admissible for joining, is revoked when the Session exits `LOBBY`, and may retain historical records for traceability. A textual code value need not be permanently reserved forever after revocation.

Join receives conceptually a JoinCode, authenticated `UserUUID`, display-name snapshot supplied by the trusted application/integration layer, and opaque idempotency key. Session Runtime resolves the JoinCode to its Session, obtains the pinned Game definition needed for lobby constraints, enters the Session-serialized mutation, and revalidates JoinCode admissibility, `LOBBY` phase, and `lobby_expires_at`.

Join finds or creates the SessionActor for `(Session, UserUUID)`. If that actor is already an active Participant, repeated Join is the same logical admission rather than consuming another slot. Otherwise Join counts active Participants, enforces `players.max` from the pinned Game definition, activates Participant membership, and commits atomically. The Game definition remains the source of truth for authored `players.min/max`; Session does not copy those values as a second correctness source.

Leave is a `LOBBY` participation operation. Public/application caller identity is `UserUUID`; Session Runtime resolves `(Session, UserUUID)` to its internal SessionActor. Leaving deactivates logical Participant membership, releases a lobby slot, does not delete the SessionActor, does not remove host authority, and does not automatically cancel the Session. A host who is also participating may leave gameplay participation while remaining host. Rejoining the same admissible lobby reuses the same SessionActor.

A `LOBBY` semantic disconnect (see Session Runtime Disconnect, Reconnect, and Resynchronization Boundary below) has the same participation effect as Leave: the SessionActor's `semantic_presence` becomes `DISCONNECTED`, its active Participant (if any) is deactivated, and the released slot is immediately available to other joiners. This is intentionally stronger than the RUNNING disconnect rule below - a player still inside Coordinator transport grace never reaches Session Runtime as a disconnect and keeps their active lobby slot. A subsequent `LOBBY` reconnect transitions `semantic_presence` back to `CONNECTED` and reuses the same SessionActor/Participant record, but if the prior disconnect deactivated the Participant, re-admission is revalidated against current lobby constraints (Session still `LOBBY`, not expired, current active-participant count against `players.max`, and other applicable admission constraints) exactly as a fresh Join would be; admission is not guaranteed to succeed, and reconnect does not guarantee recovery of a previously released slot if another actor has since occupied it.

Join/Start correctness does not depend on a sweeper having already changed stored phase. If an operation discovers `phase == LOBBY && now >= lobby_expires_at`, it may atomically materialize `TERMINAL`, a terminal reason corresponding to lobby expiration, a terminal timestamp equal to the lobby deadline, and JoinCode revocation. A background sweeper is housekeeping/materialization support, not the source of business correctness. The exhaustive terminal-reason enum is not yet accepted.

Start receives conceptually a Session UUID, authenticated `UserUUID`, and opaque idempotency key. Session Runtime resolves `(Session UUID, UserUUID)` to `SessionActorID` and uses that internal identity to verify host authority. Start uses the Session's pinned immutable Game definition/version, not the latest current Game version; validates `LOBBY`, `lobby_expires_at`, host authority, active Participants, `players.min`, and defensively `players.max`; initializes engine/runtime state; processes the engine's required initial signal/first Commit; persists initial authoritative runtime state and durable consequences; transitions Session atomically to `RUNNING`; sets `started_at`; revokes JoinCode; and commits before any Coordinator/client-visible outputs are delivered.

If initialization or first authoritative execution fails, the transaction must not partially expose a `RUNNING` Session. No Coordinator/client-visible output is delivered before successful durable commit.

Mutating external Session commands in this lobby lifecycle accept an opaque idempotency key, including Create, Join, Leave, and Start. Session Runtime must guarantee that retrying the same logical command does not repeat its effect. Natural/domain idempotency is useful additional protection but is not a substitute for the explicit idempotency contract. The final idempotency storage algorithm is not yet accepted.

Start uses the accepted Game Language root roster contract `players: list<user>`, where each `user` is the Session-local runtime identity derived from `SessionActorID`; `Identity.UserUUID` is never exposed to authored Game Language.

The roster is built strictly from Participants active at the serialized Start moment. A SessionActor that is `DISCONNECTED` (and therefore inactive from a lobby disconnect) or `CONNECTED` but not currently re-admitted as an active Participant is not included. Whether a concurrent physical disconnect actually excludes a player from Start depends only on which side wins the existing per-Session serialization: if Coordinator grace has not yet expired when Start wins serialization, the Participant is still active and included; if the semantic disconnect was already processed first, it is excluded. This is not special-cased outside Session - the existing lobby serialization already decides it. An actor excluded from the Start-time roster does not join the running game merely by reconnecting afterward; Session Runtime must not deliver `UserReconnected` toward the Game Language runtime, or otherwise dynamically add, a SessionActor absent from that execution's `players` roster. Post-Start late admission is a distinct, separately deferred capability.

Rationale and alternatives are recorded in `game/docs/decisions/GAME-ADR-0004-session-lobby-lifecycle-contract.md`, `game/docs/decisions/GAME-ADR-0005-session-public-and-internal-identity-boundary.md`, `game/docs/decisions/GAME-ADR-0006-game-language-root-player-roster-contract.md`, and `game/docs/decisions/GAME-ADR-0015-session-actor-semantic-presence-and-lobby-membership.md`.

## Session Runtime Turn And Persistence Model

`RuntimeTurn` is the historical/transactional unit for RUNNING-phase execution. One RuntimeTurn begins from one external/runtime cause (an accepted interaction response, a timer expiration, or a future platform signal) and may execute 1..N internal `engine.Step` calls within the same transaction; only the final Snapshot after the whole Turn becomes an observable/authoritative Session state, and one committed Turn corresponds to exactly one Session runtime sequence. `RuntimeStep` is technical execution history only (engine debugging/audit) and does not own a Session-state sequence.

Session Runtime durably persists Interaction (a pending or resolved externally-visible question) and Timer Obligation (a durable logical timer) as entities referenced by the RuntimeTurns that open/create and close them (`opened_by_turn_id`/`created_by_turn_id`, `closed_by_turn_id`), and by the RuntimeTurns they in turn cause (`source_interaction_id`/`source_timer_obligation_id`). `session_runtime_state` is a thin pointer to the current authoritative `current_turn_id`; it does not duplicate the Snapshot payload.

Session Runtime does not persist an absolute due-at/deadline for Game Language timers in V1; it persists only the timer obligation and its configured delay. The Coordinator remains the time-aware layer and owns physical timers in memory. A durable Coordinator-owned `live_timer_schedules` table is rejected for V1. On recovery, active timer obligations may be rescheduled using their full configured delay from the new scheduling moment; preserving elapsed wall-clock time across a full process restart is not required for V1. This does not affect `lobby_expires_at`, which remains a separate, already-accepted Session lifecycle deadline.

The accepted lobby/lifecycle idempotency table for Create/Join/Leave/Start is `session_requests`, storing the request payload rather than a hash, so a retry with the same idempotency key can be checked for semantic equivalence against the originally stored request.

`session_timer_obligations` also accepts a nullable internal key-discriminator column (`engine_key` or an equivalent typed representation), null for an ordinary `TimerSlot` timer and populated for a keyed timer (see Keyed Timer Slots above), so a keyed timer's expiration can be reconstructed with the correct authored key.

The full accepted table/column/relationship diagram is recorded in `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md`. Rationale and alternatives are recorded in `game/docs/decisions/GAME-ADR-0007-session-runtime-turn-and-persistence-model.md`, `game/docs/decisions/GAME-ADR-0008-session-runtime-v1-timer-recovery-simplification.md`, and `game/docs/decisions/GAME-ADR-0012-game-language-keyed-timer-slots.md`.

## Session Runtime Process-Agnostic Recovery

Session Runtime does not model durable ownership of a Session by a particular process, pod, or runtime instance. It has no `owner_process_id`, no runtime instance ownership, no process heartbeat ownership, no fencing/takeover generation for the purpose of process ownership, no sticky Session ownership, and no durable `RECOVERING` process phase. Conceptually, one process calls the Session Runtime API against durable Session state, then disappears; another process later calls the same Session Runtime API against the same durable Session state, and Session Runtime does not need to know the caller changed. A process crash by itself is not a Session-domain event and does not change `Session.phase`.

The accepted RuntimeTurn transactional model (see Session Runtime Turn And Persistence Model above) is preserved unchanged for crash recovery. If a process dies before a RuntimeTurn's transaction commits, the entire Turn rolls back: `current_turn_id` remains at its previous value, no partially authoritative Turn exists, intermediate Steps are not resumed, and the external cause may later be retried under its own idempotency semantics. Session Runtime does not add a durable `RuntimeTurn = PROCESSING` state and does not resume "Step N" of a partially executed Turn. If the transaction committed but the process died before live delivery/response, the Turn did occur: `current_turn_id` points to it, its committed interaction/timer/state mutations remain authoritative, reconnect/resync returns the resulting state, and normal idempotency prevents replaying the same accepted external cause - which process happened to execute the Turn is irrelevant.

A process that needs to resume serving a `RUNNING` Session reconstructs it from current durable authoritative state - `sessions`, `session_runtime_state.current_turn_id`, the final Snapshot stored on that RuntimeTurn, active `session_interactions`, active `session_timer_obligations`, and other current durable Session-owned state - not by replaying RuntimeTurn `1..N`. Historical Turns/Steps remain useful for history, debugging, archive, and separately designed audit/replay tooling, but are not required bootstrapping state. Game Language timer recovery continues to use the GAME-ADR-0008 tradeoff unchanged: active durable timer obligations may be physically rescheduled after process loss using their full configured delay; no `due_at` is introduced.

Rationale and alternatives, including the rejected process-owned-lease/heartbeat/fencing direction, are recorded in `game/docs/decisions/GAME-ADR-0013-session-runtime-process-agnostic-recovery.md`.

## Session Runtime Durable Inactivity Expiration

Every `RUNNING` Session has a durable inactivity deadline, `sessions.activity_expires_at`, distinct from `lobby_expires_at`. It is not a process ownership lease; it answers until what instant the platform still considers the Session active absent meaningful activity, so abandoned Sessions and crashed-process Sessions eventually leave hot `RUNNING` state and become eligible for terminalization and archival.

Meaningful Session operations that demonstrate continued active use extend `activity_expires_at = now + inactivity_ttl`. V1 may use a configurable TTL around 10 minutes; the exact duration is configurable operational/product policy, not hard-coded Game Language semantics. Only operations that constitute meaningful evidence of active use may renew the deadline - for example successful RuntimeTurn-producing gameplay operations, accepted interaction processing, timer expiration processing, meaningful lifecycle/runtime events, or justified authenticated reconnection/resume activity (illustrative, not exhaustive) - arbitrary passive reads/polling must not renew it.

`activity_expires_at` is the source of truth: semantic expiration occurs because the deadline passed, not because a Reaper happened to discover the Session, mirroring the already-accepted `lobby_expires_at` -> lobby-expiration-terminal-reason pattern. Any Session Runtime operation that requires the Session still be active must, under the normal per-Session serialization mechanism, load/lock the Session, validate lifecycle, compare current time with `activity_expires_at`, and only if not expired may it process the operation and potentially renew the deadline; an operation arriving after the deadline must not revive the Session merely because persisted `phase` still says `RUNNING`. Such an operation may instead atomically materialize, in the same transaction, `phase = TERMINAL`, `terminal_reason = RUNTIME_INACTIVITY_EXPIRED`, and `terminal_at = activity_expires_at`, then reject the attempted action - without renewing the deadline, reopening the Session, fabricating gameplay, or creating an engine RuntimeTurn merely to represent expiration.

A background Session Reaper proactively finds Sessions where `phase = RUNNING` and `activity_expires_at <= now` but the persisted lifecycle representation has not yet been materialized, and - using the same Session serialization rules as normal mutations - re-validates and materializes `TERMINAL`/`RUNTIME_INACTIVITY_EXPIRED` if still expired; if a legitimate operation already won the serialization race and renewed the deadline, the Reaper rechecks and does nothing. The deadline, not Reaper scheduling latency, decides the outcome. `terminal_at` always equals `activity_expires_at`, never the Reaper's or a lazy operation's current wall-clock time - this matters for historical correctness, retention windows, archival timing, and analytics (for example, a multi-day platform outage does not move the semantic terminal instant forward). `terminal_reason = RUNTIME_INACTIVITY_EXPIRED` means only that the `RUNNING` Session exceeded its allowed inactivity period; it does not assert a process crashed, a pod was killed, the host disconnected, every participant left, or any other specific root cause.

The Archive Worker (see Session Runtime History Archival Direction below) remains completely separate from this mechanism: it consumes already-materialized `TERMINAL` Sessions under archival retention policy and must not inspect process ownership, detect crashes, determine `RUNNING` inactivity, or interpret `activity_expires_at` to mutate lifecycle. Once a Session is semantically `TERMINAL`, future gameplay timer/interactions are no longer executable as active gameplay; the exact persistence-state transitions/closure reasons for still-open `session_interactions`/`session_timer_obligations` at inactivity termination remain a later implementation/design detail, and materializing expiration must never fabricate engine responses or RuntimeTurns to close gameplay.

Rationale and alternatives, including the rejected process-ownership/heartbeat cleanup direction, are recorded in `game/docs/decisions/GAME-ADR-0014-session-runtime-durable-inactivity-expiration.md`.

## Session Runtime History Archival Direction

PostgreSQL remains the hot/runtime store. After a Session reaches an appropriate terminal/archiveable condition, heavy runtime history may eventually be serialized into a versioned JSON artifact written to long-term object storage such as Google Cloud Storage; the exact archive schema and storage implementation are deferred. Archive metadata (`session_history_archives`) tracks this per Session with a provider/key identifier rather than an expiring URL.

Heavy runtime/history tables (`session_runtime_state`, `session_runtime_turns`, `session_runtime_steps`, `session_interactions`, `session_timer_obligations`) may become an explicit hard-delete exception, but only after the long-term archive is successfully written and verified. `session_actors` and `session_participants` are excluded from this deletion policy and remain relationally stored indefinitely, since they support ongoing product queries such as which Sessions a User participated in.

Rationale and alternatives are recorded in `game/docs/decisions/GAME-ADR-0009-session-runtime-history-archival-and-hard-delete.md`.

## Session Runtime Disconnect, Reconnect, and Resynchronization Boundary

Three responsibilities remain distinct and must not collapse into one owner: the transport fact that a physical connection disappeared or returned (Coordinator); the platform/runtime semantic event that a Session User is now considered disconnected or reconnected (Session Runtime boundary); and the gameplay consequence of that event (Session Runtime + Game Language).

Loss of a physical connection is owned by the Live Session Coordinator and does not by itself deactivate `session_participants`, remove a Participant, free a gameplay slot, delete a SessionActor, or terminate the Session. Session Runtime does not add authoritative connection-state fields such as `is_connected`, `connection_id`, or `websocket_id`.

The Coordinator may apply a short, in-memory transport grace/debounce period when a connection disappears, to absorb transient failures (Wi-Fi loss, browser reconnection, socket replacement). A reconnect within this grace period rebinds the connection and cancels the pending escalation without Session Runtime/Game Language ever observing a semantic disconnect; the grace duration is not decided here and remains configurable/implementation-level. This grace period is not gameplay policy and is not the time a game gives a disconnected player before forfeiting/removing them.

If the grace period expires without reconnection, the Coordinator notifies Session Runtime that the User/SessionActor has semantically disconnected. Session Runtime serializes the resulting SessionActor `semantic_presence` transition (`CONNECTED -> DISCONNECTED`) per Session; a repeated disconnect report while already `DISCONNECTED` is idempotent and must not duplicate any Game Language disconnect effect. The Coordinator does not decide gameplay consequences such as removing, forfeiting, pausing, or terminating - and the consequence of the transition itself is phase-dependent:

- In `LOBBY`, semantic disconnect deactivates the active Participant and releases its slot (see Session Runtime Lobby Lifecycle Contract above); there is no root workflow yet, so no `UserDisconnected` signal exists to deliver.
- In `RUNNING`, semantic disconnect does not deactivate or remove the Participant from the runtime roster; Session Runtime translates the platform event into the accepted `UserDisconnected` Game Language signal (see Authored Game Language Disconnect/Reconnect Contract below), and authored Game Language - not Session Runtime - determines gameplay consequences.

Reconnect is not Join. Session Runtime resolves `(SessionUUID, UserUUID) -> existing SessionActor` and verifies the existing logical participation/session relationship; it must not create another SessionActor or Participant slot, consume another player slot, require a JoinCode, or rerun lobby Join admission. Transport reconnection does not imply gameplay reinstatement: if the game had already reacted to the semantic disconnect (removed, forfeited, disabled), a User may reconnect at the transport level while the current Game state still prevents them from resuming active gameplay. If the semantic disconnect had already been delivered to the game, a corresponding `UserReconnected` signal is delivered to Session Runtime/Game Language - but only for a SessionActor that was part of the current execution's `players` roster; an actor excluded from the roster at Start does not join the running game merely by reconnecting (see Session Runtime Lobby Lifecycle Contract above).

The accepted persistence model (`game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md`) does not gain a connection-presence table for disconnect/reconnect. Live connection bindings and transport-grace state remain ephemeral Coordinator state; a full process restart may lose an in-flight grace period, which is acceptable for V1. Existing durable state (SessionActor, Participant, current RuntimeTurn/Snapshot, active SessionInteractions, active TimerObligations) remains sufficient for Session Runtime's authoritative business/runtime state.

After the Coordinator authenticates/rebinds a connection, it may request a Session resynchronization view from Session Runtime using `(SessionUUID, UserUUID)`. Resync is a read/reconstruction capability and does not mutate the game; the exact transport endpoint/DTO is not frozen here. Session Runtime must never return the raw internal `engine.Snapshot` as this contract - it produces a player-facing Session projection for the resolved SessionActor (conceptually: Session lifecycle/phase, current player-visible game state, that User's currently active SessionInteractions, and other accepted player-visible state), without leaking internal engine identities, paths, slots, or SessionActorIDs. The resync representation carries the current RuntimeTurn `sequence` as its monotonic version, so Coordinator/client can reason about stale prior deliveries; no second, independent version counter is introduced.

Reconnect during transport grace creates no RuntimeTurn: the Coordinator rebinds, cancels the pending grace, and resends current resync state, with no semantic disconnect/reconnect and no engine execution. Reconnect after an already-escalated semantic disconnect processes gameplay/runtime semantics first (delivering `UserReconnected`, which may produce a RuntimeTurn if the root workflow handles it) and only then builds resync from the resulting current state - the client is synchronized to what the game currently believes, not blindly restored to its pre-disconnect state.

Different games may reasonably require different disconnect/reconnect behavior (continue, wait, start a timer, forfeit, remove, pause, end); the Game Language contract described below standardizes only the platform lifecycle signals themselves, not any particular game policy.

Rationale and alternatives are recorded in `game/docs/decisions/GAME-ADR-0010-session-disconnect-reconnect-resync-boundary.md`. The durable `session_actors.semantic_presence` field and the LOBBY-vs-RUNNING phase-dependent participation consequence above are recorded in `game/docs/decisions/GAME-ADR-0015-session-actor-semantic-presence-and-lobby-membership.md`.

### Authored Game Language Disconnect/Reconnect Contract

`UserDisconnected` and `UserReconnected` are standard `NamedSignalSource` platform/lifecycle signals - the same closed mechanism already used for `WorkflowStarted`/`SessionCancelled`/`ParentCancelled` - not new `SignalKind` variants. Each exposes exactly one authored field, `user: user` (the Session-local runtime identity derived from `SessionActorID`), and never `Identity.UserUUID`, connection/socket IDs, IP address, disconnect timestamp, transport-grace information, or other Coordinator internals. Session Runtime delivers both signals to the root workflow only; there is no implicit broadcast to nested child/task-group/ask-group instances, since the engine's `Step` contract addresses one instance path per call and does not currently support hidden multi-instance fan-out. Nested workflows may later be coordinated explicitly by authored root-level logic or a future language capability.

Handling either signal is optional. If the root workflow declares no matching transition, this is an ordinary rejected/unmatched signal (`engineservice.ErrSignalRejected`) with no automatic gameplay consequence - Session Runtime must not infer remove/forfeit/pause/skip/end, and a rejected delivery that produces no `Commit` does not create a `RuntimeTurn` merely to record a no-op. "The game ignores disconnect" means only that disconnect itself does not mutate gameplay; it does not mean the platform guarantees continued progress.

An interaction/question targeted at a SessionActor whose transport connection is currently absent is still created as a normal durable `SessionInteraction` - Session Runtime must not suppress, auto-answer, skip, or redirect it, or deactivate the Participant, merely because the actor is offline. The interaction remains logically ACTIVE independent of live delivery; a later reconnect can surface it through the existing resync capability. Whether/how gameplay proceeds while an actor is offline (an authored timer/timeout deciding skip/default/forfeit, or legitimately remaining unresolved if no timeout applies) is ordinary authored Game Language policy, not a Session Runtime inference; a future max-runtime/inactivity/runaway protection may eventually terminate an abandoned Session but must never fabricate gameplay responses.

Rationale and alternatives are recorded in `game/docs/decisions/GAME-ADR-0011-game-language-disconnect-reconnect-authored-semantics.md`.

### Keyed Timer Slots

The existing accepted `TimerSlotDeclaration` holds at most one pending timer per statically named slot per workflow instance, which makes an authored policy such as an independent disconnect timeout per player awkward with a single static slot. Game Language will support a general `KeyedTimerSlot<Key>` capability - distinct pending timers addressed by `(workflow instance/path, slot, key)`, so different keys (for example `disconnect_timeout[P1]` and `disconnect_timeout[P2]`) may have independent timers pending simultaneously, while at most one timer may be pending per exact tuple. Scheduling into an already-occupied `(slot, key)` is an execution error with no implicit reset/replace/coalesce, mirroring the existing ordinary `TimerSlot` rule; cancellation affects only the selected key and is idempotent; expiration exposes the authored key (conceptually `KeyedTimerExpired(slot)` carrying `key: KeyType`) without exposing internal timer-obligation UUIDs or wall-clock scheduling data. This is a general language primitive - not a disconnect-specific `DisconnectTimer` - also intended for player cooldowns, team timers, per-object timers, and similar keyed processes. Naming/API/Go types are not frozen; only the semantic capability is accepted. See Session Runtime Turn And Persistence Model below for the resulting `session_timer_obligations` persistence consequence.

Rationale and alternatives are recorded in `game/docs/decisions/GAME-ADR-0012-game-language-keyed-timer-slots.md`.

## Boundary Notes

Completed-session history archival direction is accepted (see above and GAME-ADR-0009); the concrete JSON archive schema and object-storage implementation remain deferred.
