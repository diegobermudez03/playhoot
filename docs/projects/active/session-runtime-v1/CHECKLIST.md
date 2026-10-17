# Session Runtime v1 - Completion Checklist

Status: ACTIVE TRACKING (non-authoritative for design; the architecture plan owns behavior)

Source of behavior: `plans/session_runtime/Playhoot_Session_Runtime_Architecture_Plan_with_TOC.docx` ("plan"). Section numbers below (`§N`) refer to that plan. Section Y (Observability) is not in the plan; it is a mandatory addition of this project.

## How To Use

- This is an end-state list, not a task order. The human directs the implementation order.
- Tick `[x]` only when the behavior is implemented AND verified (test or other concrete evidence). Code that merely exists is not done.
- Optionally append an owner/evidence note after an item, e.g. `- WORK-0123`.
- If the human's instructions skip an item, or an instruction conflicts with a plan requirement, raise it before ticking anything.
- Items marked `(decision)` need a human design decision before they can be implemented; the plan leaves the choice open (§31).

---

## A. Cross-cutting foundations

### A1. Owner registry and single-owner invariant (§1 topology, §30.1)

Decided 2026-10-15 in SESSION-ADR-0029: ownership in `play/runtime`, routing at the API edge, one atomic locate-or-claim, epoch as fencing token, placement policy inside the ownership implementation.

Progress note (2026-10-15): the API side is implemented and tested (`api/play/owner_routing.go`, before every live upgrade); `LocateOrClaimOwner` itself is a placeholder (see Placeholders). Release and the registry are not implemented. The Runtime already fences by epoch when attaching (2026-10-17): a request under an older epoch than the Session's controller is rejected (`owner_changed`); a request under a newer one retires the stale controller (its connections end with `owner_changed`, closed as 1012 so clients reconnect) and creates a new one. Recovering the new controller's state is pending (Q).

- [ ] Owner abstraction exists: Resolve(sessionID), Claim(sessionID), Release(sessionID) (Resolve and Claim exist as one atomic `LocateOrClaimOwner`, today a placeholder; Release does not exist yet)
- [ ] Exactly one Runtime pod owns an active Session; two pods can never mutate the same Session concurrently (decision: lease/epoch/fencing mechanism)
- [x] Browser never learns a pod address; routing is resolved before the request reaches Session logic (`OwnerPlacement.Address` never reaches a client; routing runs before the upgrade)
- [ ] Owner released on Session termination and on controller destruction
- [ ] Owner loss is detectable (needed by recovery and reconciliation)
- [ ] Before more than one instance may own Sessions (separate gateway, or more than one instance of the single binary): shared registry, owner-renewed lease + heartbeat, self-fencing, epoch on owner-only durable writes, gateway-to-Runtime trusted identity, addressable-instance deployment. Instructions: `play/docs/decisions/SESSION-ADR-0029-live-session-ownership-and-owner-routing.md`

### A2. API <-> Runtime duplex boundary (§1)

Progress note (2026-10-17): the boundary, decided with the human, is a set of operations per kind of connection, returned by the Runtime as function fields over its own types (`runtime.AdminConnection`: `SelectPerspective`, `CancelSession`, `NextView`, `Ended`, `Disconnect`; contract in `play/runtime/runtime.go`). This replaces the plan's generic attach/detach/inbound and send/close/replace vocabulary: attach is `AttachAdmin`, detach is `Disconnect`, inbound messages are the typed operations, outbound messages are `NextView`, and closing is `Ended`. The Runtime never sees the socket; each API endpoint owns its wire messages and drives its connection's operations (`api/play/admin_join.go`), with the shared, domain-free loop orchestration in `api/internal/live`. Implemented for ADMIN connections and verified by tests and a manual test; player connections will get their own operations and types. Graceful shutdown of live connections is implemented on the API side (`api/internal/live`: every connection runs under the controller, which on shutdown closes it with 1001 and calls its Disconnect, and waits within the shutdown window); the Runtime stopping its controllers is still open, and so is delivering answers that are in flight when shutdown starts (they are cut today).
- [x] /api owns the WebSocket; Runtime owns the logical connection; Runtime does not know WebSocket details
- [ ] API reports: transport attached, transport detached, inbound message (done for ADMIN as `AttachAdmin`, `Disconnect` and the typed admin operations; PLAYER done on the API side as `AttachPlayer`, `Disconnect` and the typed player operations, 2026-10-17, but the Runtime does not implement `AttachPlayer` yet: see F)
- [ ] Runtime emits: send outbound message, close transport, replace transport (done for ADMIN as `NextView` and `Ended`; replacing a transport belongs to device switch, O5)
- [x] Identity + owner routing happen before upgrade; logical admission happens after upgrade
- [x] Rejected admission sends a rejection and closes the socket
- [ ] Late messages from an obsolete transport are ignored via Runtime-internal connection generation (§19)
- [ ] Graceful shutdown covers live connections: on SIGTERM, WebSocket transports are closed and the Runtime stops its controllers cleanly within the shutdown window. Done: HTTP request draining (`main.go`) and closing live connections (`Server.Shutdown` → `api/internal/live`, after `http.Server.Shutdown`, which does not track hijacked connections). Pending: the Runtime stopping its controllers, and delivering in-flight answers before closing.

### A3. Join code and routing (§1, §2)
- [ ] Join code -> sessionUUID resolution -> owner resolve/claim -> route to pod (API side done and tested 2026-10-17: `GET /v1/join/{joinCode}` resolves the code (`api/play/join_code_routing.go`) before owner routing; the resolution itself, `ResolveJoinCode` in `play/session`, is not implemented: see Placeholders)
- [ ] Join code is only a friendly identifier, never authN/authZ
- [x] Join code expiration uses platform time

### A4. Identity integration (§1, §2, §4)
- [ ] Account token -> account userUUID
- [ ] Guest token -> existing guest userUUID
- [ ] No identity -> guest created, returned userUUID + guest credentials for the FE to keep (pending, out of scope for the API step of player join: a player without a token is refused `401` today; the comment on the player routes in `api/play/handler.go` marks it. Open: how the credentials reach the FE, since a browser cannot read headers of a WebSocket handshake: a first live message, or a REST call before connecting. The guest display name is part of this input)
- [ ] CREATE_SESSION capability check (real account required, guest-only rejected)
- [x] Other domains receive only userUUID (no account/guest awareness)
- [x] Identity does not decide object-level permissions (creator check lives in Session)

### A5. Two clocks (§1)
- [ ] Game/Session effective time: monotonic, starts at RUNNING, persisted in checkpoints, freezes during Runtime outage
- [ ] Platform time used for: transport grace, owner lease/heartbeat, join-code expiry, recovery window, inactivity limit, max lifetime
- [ ] A logical game pause does not freeze game time

### A6. One authoritative timeline (§1)
- [ ] Single logical mutation lane per Session
- [ ] Every accepted operation gets server-owned sessionTime + serverOrder; client never supplies authoritative timestamps
- [ ] Timeline carries: SAMPLE transitions, COMMAND, timer firing, update(), playerDisconnected, playerRejoined, playerLeft
- [ ] executedThroughSessionTime advances on any materialized operation (not only Samples)
- [ ] Sandbox receives elapsed effective time since the previous materialized operation
- [ ] Batching never changes semantic ordering (§7)

### A7. Message model and role gating (§26, §27)
- [ ] Platform messages vs game messages are distinct categories
- [x] Connection role (PLAYER/ADMIN) is authoritative for what it may send (each endpoint only accepts its own message catalog; any other type closes with 1008 before reaching the Runtime; tested for both, 2026-10-17)
- [x] ADMIN can never send SAMPLE/COMMAND (the admin catalog has no such message; any other type closes the connection with 1008 before reaching the Runtime)
- [ ] Contract validation happens before JavaScript is called

---

## B. GameVersion and manifest contract (§1)

- [ ] Session pins exactly one GameVersion for its whole lifetime and for replay
- [x] Only versions in the executable state can create Sessions (the plan says PUBLISHED; this domain names its own state EXECUTABLE: immutable, scripts ready to run)
- [ ] Published version artifacts are immutable and never physically deleted (backend JS, frontend package, manifest, assets)
- [ ] Artifacts addressed by locator + hash + size + metadata; content never silently changes
- [ ] Runtime manifest declares: SAMPLE input defs/types, defaults (when not inferable), COMMAND names + payload schemas, optional entrypoints present (update, playerRejoined), required input capabilities, frontend/backend artifact identity
- [ ] Manifest drives: pre-JS rejection of malformed/undeclared input, discovery filtering by capability, update()/rejoin support detection
- [ ] Logical deletion/supersession of a game affects only new Sessions, not old Sessions/replays (§30.28)

---

## C. Durable Session lifecycle (`/play/session`) (§1)

Progress note (2026-10-15): the parts Session creation needs are done and verified; lifecycle transitions, end causes, roster and checkpoints come with later operations.

- [ ] Session persistence: lifecycle states (LOBBY, RUNNING, FINISHED, CANCELLED, EXPIRED, ABORTED) with enforced transitions
- [ ] endCause recorded on terminal transitions (HOST_CANCELLED, ALL_PLAYERS_LEFT, INACTIVITY_TIMEOUT, MAX_LIFETIME, GAME_START_FAILURE, SCRIPT_FAILURE, RUNTIME_RECOVERY_TIMEOUT, UNRECOVERABLE_INFRA_FAILURE, ...)
- [x] Pinned GameVersion reference stored
- [x] creatorUUID stored
- [x] Join code + expiry stored
- [ ] Durable participant roster (userUUID <-> ParticipantID) written only at Start
- [ ] Recovery checkpoint reference/metadata stored
- [ ] Runtime never writes Session tables directly; all durable operations go through the /play/session boundary
- [ ] Migrations for all of the above
- [x] `DATA_MODEL.md` created and matching the implementation (created as `play/docs/DATA_MODEL.md`; keep it current as the schema grows)

---

## D. Create Session endpoint (§2)

Completed 2026-10-15: implemented end to end and verified (unit, repository and integration tests, manual local test of `POST /sessions`). Flows: `play/docs/FLOWS.md`, `docs/architecture/CROSS_DOMAIN_FLOWS.md`. Known behavior kept as is: a retry after the Game stopped being playable answers `game_unavailable` (see PROJECT.md decisions).

- [x] REST endpoint receives: account auth, game UUID, frontend idempotency token
- [x] Identity resolves userUUID and verifies CREATE_SESSION (the Session Runtime side is done: the endpoint calls Identity and honors its decision; Identity's real behavior is out of scope and tracked under Placeholders)
- [x] Orchestrator composes the operation
- [x] /game validates the Game is currently playable/visible for this user (the orchestration calls Game Management and honors its answer; Game's real check is out of scope and tracked under Placeholders)
- [x] /play/session resolves the executable GameVersion and requires it to be EXECUTABLE
- [x] GameVersion pinned
- [x] Session created in LOBBY
- [x] Join code created with platform-time expiration
- [x] creatorUUID stored
- [x] Response: sessionUUID + joinCode + useful lobby metadata
- [x] Idempotent by frontend token (replay returns the same outcome; conflict semantics per `docs/engineering/standards/idempotency.md`)
- [x] No Runtime controller created at this point
- [x] Creator is not automatically a player

---

## E. Admin join (§3, §18)

Admin join (connection and admission) completed 2026-10-17: implemented end to end and verified by unit, repository and WebSocket tests and by a manual test from Postman. API side: WebSocket auth with header or Kubernetes-style token subprotocol, `JOIN_SESSION_AS_ADMIN`, owner routing, `playhoot.v1` upgrade with origin check, admin wire messages, live connection controller with graceful shutdown, handshake and connection metrics. Runtime side: `liveconnection.Manager.AttachAdmin` asks the durable Session `EligibleToJoinAsAdmin` (real check), then finds or creates the Session's controller (epoch-fenced) and registers the ADMIN connection.

Left for future steps, on purpose:
- The admin operations themselves: `SelectPerspective` and `CancelSession` are wired end to end but answer "not implemented" (the API closes the connection with 1011), and `NextView` never produces a view.
- Ownership is still a placeholder (every Session owned here under epoch 1), and forwarding to another owner does not exist (see A1 and Placeholders).
- The controller only holds ADMIN connections, serializes with a mutex instead of the single mutation lane, and is never destroyed (see H).

- [x] Authenticated account required (the Session Runtime side is done: the route requires `JOIN_SESSION_AS_ADMIN`, which Identity must answer forbidden for guests; Identity's real behavior is out of scope and tracked under Placeholders)
- [x] Owner pod resolved before upgrade (the routing step is done; ownership itself is a placeholder tracked under Placeholders)
- [x] Runtime asks /play/session to validate: Session exists + connectable state + userUUID == creatorUUID (`EligibleToJoinAsAdmin`: RUNNING, or LOBBY before its expiry; an overdue LOBBY counts as not joinable even before its expiry is recorded)
- [x] Accepted: ADMIN logical connection registered; no gameplay participant, no extra durable membership row
- [x] Rejected: rejection sent, socket closed
- [x] Multiple simultaneous ADMIN connections for the same creator
- [ ] Admin operations implemented: `SelectPerspective`, `StartSession` and `CancelSession` (today they answer "not implemented"), and `NextLobbyUpdate` (never produces an update today)
- [ ] Admin receives lobby updates until the lobby closes (`lobby_update`, then `lobby_closed` when the Session starts; API side done and tested 2026-10-17)
- [ ] Creator who also plays may not watch another participant: `SelectPerspective` answers `creator_is_playing` while the creator has a PLAYER connection (decided by the human 2026-10-17; the outcome is declared, the controller check is not implemented)
- [ ] Admin selects a player perspective; receives exactly projection(P), FULL, with a new baseline per selection
- [ ] No omniscient admin projection
- [x] Admin reconnect = just another admin WebSocket (no game-level admin rejoin)
- [ ] Admin connections do not receive participant SAMPLE acknowledgements

---

## F. Player lobby join (§4)

Progress note (2026-10-17): the API layer is implemented and tested. `GET /v1/sessions/{sessionUUID}/player` and `GET /v1/join/{joinCode}` authorize `JOIN_SESSION_AS_PLAYER` (Identity's placeholder allows guests), resolve the join code, route to the owner, upgrade, and serve the `runtime.PlayerConnection` the Runtime returns (`api/play/player_join.go`). The contract, decided with the human, declares every player operation now so the controller work knows what it must implement: `Leave`, `Sample`, `Command`, `Resync`, `NextLobbyUpdate`, `NextView` (own view with `ConsumedThroughSampleSequence`), `Ended`, `Disconnect` (`play/runtime/runtime.go`). Admins and players are always separate connections, even for a creator who plays. Not implemented: the Runtime's `AttachPlayer` and the durable `ResolveJoinCode` (both stand-ins in `unwired.go` that fail, see Placeholders), and guest creation (see A4).

- [ ] Join by join code or Session UUID; guest display name when a new guest is created
- [ ] Identity resolution (see A4)
- [ ] Owner resolve/claim
- [ ] Logical admission serialized through the SessionController (two users racing for the last slot cannot both be accepted)
- [ ] Checks: Session still LOBBY, player count / GameVersion constraints, device/control capabilities vs manifest
- [ ] Same user's older PLAYER transport replaced if needed
- [ ] Accepted: ephemeral lobby member (not durable)
- [ ] Rejected: JOIN_REJECTED sent, socket closed
- [ ] Lobby pod loss: Session stays LOBBY, players can rejoin the lobby; no lobby checkpoint
- [ ] No late join of new participants once RUNNING (§30.6)

---

## G. Start Session (§5)

- [ ] START accepted only from the creator's ADMIN connection; no identity token in the live message (changed from the plan, which also accepts the creator's PLAYER connection, by the human on 2026-10-17: admin and player are always separate connections, and starting and cancelling are admin operations. `AdminConnection.StartSession` and the `start_session` message are declared and wired; the Runtime answers "not implemented")
- [ ] Controller serializes START; lobby roster frozen for the attempt
- [ ] Stable ParticipantIDs assigned (userUUID -> P1..Pn)
- [ ] /play/session validates: LOBBY, not expired, caller is creator, participant count valid, pinned GameVersion available; returns artifact/manifest references
- [ ] Sandbox context created for the pinned GameVersion
- [ ] start(participants, controlled context) executed; success returns initial full state + timers/events + initial PRNG state + context revision
- [ ] Initial recovery checkpoint written strongly/durably BEFORE RUNNING (§30.3)
- [ ] Final Start transaction: revalidate, persist roster, store checkpoint reference, mark RUNNING
- [ ] Orphan checkpoint (written, but DB transition failed) is tolerated and cleanable; a RUNNING Session without recoverable initial state is impossible
- [ ] Runtime commits live mirror; project() for each player; START + FULL initial projection sent
- [ ] start() script failure: retry once from the same pre-start state with fresh context
- [ ] Second failure: Session ABORTED / GAME_START_FAILURE, never RUNNING, clients informed, transports closed
- [ ] After Start: recovery, replay, and checkpoint scheduling begin; script sees only ParticipantIDs

---

## H. SessionController (§6)

Progress note (2026-10-17): a first, minimal controller exists (`play/runtime/internal/controller`): a per-instance registry finds or creates one controller per Session (refusing one under another ownership epoch), and the controller only holds its attached ADMIN connections. Admission is serialized with a mutex, not yet a single mutation lane. Controllers are never destroyed yet.

- [ ] Exactly one active controller per active Session on the owner pod
- [ ] Owns live state: connections, userUUID<->ParticipantID map, participant status, per-participant logical SAMPLE state, SAMPLE ingress/dedup buffers, COMMAND pending/processed tokens, timeline ordering, effective Session time, executedThroughSessionTime, pending timers, Sandbox context ID, confirmed state mirror, PRNG state, contextRevision, projection cache, per-viewer replication state, checkpoint orchestration, replay capture orchestration
- [ ] Three state levels distinct and consistent: Sandbox context / Runtime confirmed mirror / durable checkpoint
- [x] Controller created only when a Session first needs live behavior (created on the first admitted live connection)
- [ ] Controller destroyed on terminal states (with owner release and Sandbox context destruction)

---

## I. SAMPLE protocol (§7, §13)

- [ ] SAMPLE is a full logical input snapshot; defaults applied from manifest (type-inferred zero or explicit default)
- [ ] Per-participant monotonic client sequence; gaps allowed; Runtime never waits for a missing sequence
- [ ] maxAcceptedSampleSequence[P] and consumedThroughSampleSequence[P] tracked
- [ ] Sequence <= high-water mark is stale/duplicate and ignored
- [ ] Normalize then dedupe against current accepted input state (identical refreshes are not transitions)
- [ ] Every distinct accepted transition preserved until processed ("latest wins" is NOT the model; §30.11)
- [ ] Refreshes still advance max accepted sequence and are acknowledged once materialized
- [ ] Acknowledgement (consumedThroughSampleSequence) can advance without a projection change; not sent to ADMIN connections
- [ ] SAMPLE transitions from all players merged into the global timeline
- [ ] Adjacent Samples batched into one samples(...) call only when no other operation belongs between them
- [ ] Device switch: new transport starts above the highest accepted in-memory sequence
- [ ] After crash: checkpoint sequence is the baseline; higher client values with gaps accepted
- [ ] Unknown/undeclared input, bad shape, and oversize SAMPLE rejected pre-JS

---

## J. COMMAND protocol (§8)

- [ ] COMMAND(token, name, payload), token scoped to Session + ParticipantID; separate namespace from SAMPLE sequence
- [ ] Pre-JS validation: declared name, payload schema, size limits, message valid for this connection role
- [ ] Token pending -> not enqueued twice
- [ ] Token processed (including logical game rejection) -> later retry returns ALREADY_PROCESSED-equivalent
- [ ] Original response is not persisted for idempotency
- [ ] Processed tokens retained for the Session lifetime and included in checkpoints; pending tokens memory-only
- [ ] Command can trigger an early execution boundary, processing older pending operations first
- [ ] command(...) executed via Sandbox; full successful result committed; correlated response sent
- [ ] Immediate projection flush when observable state changed
- [ ] Response offered to Replay (best effort) with its target participant
- [ ] Logical rejection (NOT_YOUR_TURN, etc.) is a normal result; Session continues

---

## K. Periodic processing and update() (§9)

- [ ] Normal live processing cadence (~100 ms, platform tuning value)
- [ ] Boundary: drain pending timeline -> confirm state -> project -> compare with cache -> send changes/acks
- [ ] Commands and timers can trigger earlier boundaries
- [ ] If update() is not declared, Runtime does not execute JS merely because time passes
- [ ] If declared, update() called frequently but with no exact-interval guarantee; receives current Session time + elapsed game time
- [ ] No unbounded backlog of missed updates
- [ ] Consistent budget violations abort the Session as a script/runtime failure
- [ ] update() and timers do not count as human activity (see U)

---

## L. Timers and logical events (§10)

- [ ] scheduleTimer(id, dueAt|delay, data) and cancelTimer(id) available to the script
- [ ] Runtime owns the schedule; timers use effective game time
- [ ] Due timer enters the global timeline and calls timer(...); may trigger an early boundary
- [ ] Pending timers included in checkpoints; recovery preserves remaining game-time distance
- [ ] Logical events emitted by mutating functions; delivered live only after the batch commits
- [ ] Events offered to Replay as best effort; never durable authoritative effects

---

## M. Sandbox execution (§11, §1 entrypoints, §1 determinism)

- [ ] Entrypoints: start, samples, command, update (optional), timer, playerDisconnected, playerRejoined (optional), playerLeft, project
- [ ] Mutating entrypoints return full new global state + events + timer changes + terminal FINISHED signal + COMMAND response
- [ ] project(globalState, participant, context) is pure and deterministic (no wall clock, no random, not even the controlled PRNG)
- [ ] Controlled deterministic PRNG; state is part of Runtime state and checkpoints; no uncontrolled Math.random as source of truth
- [ ] No access to DB, files, secrets, network, processes, or uncontrolled wall clock
- [ ] A batch of ordered operations is atomic: no partial output escapes (state, events, timers, responses, terminal signal commit as one)
- [ ] Terminal FINISHED mid-batch: later operations ignored
- [ ] Context revision advances on every successful batch, separate from projection revision and SAMPLE sequence
- [ ] Executor/infrastructure failure: discard context, recreate from the last confirmed Runtime mirror, retry batch
- [ ] Script failure arrives as a valid protocol response with SCRIPT_ERROR body; retry once from the same confirmed pre-batch state
- [ ] Retry success: continue and raise monitoring/alert (flaky execution); retry failure: Session ABORTED / SCRIPT_FAILURE
- [ ] Sandbox-only death: recreate from the Runtime mirror without rolling back to checkpoint
- [ ] Runtime death: old sidecar context treated as orphaned; new owner recovers from durable state only
- [ ] Idle contexts evictable and recreatable; update() games keep the context hot
- [ ] Resource limits: CPU/execution budget, memory, deadline, state/output size, protection against non-cooperative code (see W)

---

## N. Projection, replication, resync (§12)

- [ ] Runtime keeps the current full projection per ParticipantID
- [ ] Script returns full projections; Runtime computes patches/deltas as optimization (decision: patch representation)
- [ ] Revision continuity belongs to each viewer connection, not to the participant projection
- [ ] FULL projection + fresh baseline on: new transport attach, admin selects a player, reconnect, recovery
- [ ] Resync is a platform operation (FE or Runtime requests); sends FULL projection + current consumed SAMPLE ack (PLAYER) + fresh baseline
- [ ] After pod recovery the cache is rebuilt via project()
- [ ] Immediate flush on Command completion, important events, Start, Finish
- [ ] Raw global state never reaches the browser (§30.19)

---

## O. Connection resilience and participant lifecycle (§14-§19)

### O1. Transport loss and reattach (§14)
- [ ] Transport detach reported immediately by /api
- [ ] Participant stays CONNECTED for the platform-time grace (~3 s)
- [ ] Logical SAMPLE state reset to defaults immediately on detach
- [ ] No playerDisconnected() during grace
- [ ] Reattach within grace: FULL projection + fresh baseline; FE sends full SAMPLE snapshot; no playerDisconnected/playerRejoined calls

### O2. Logical disconnect (§15)
- [ ] Grace expiry inserts playerDisconnected(P) into the timeline
- [ ] Participant status becomes DISCONNECTED
- [ ] Game without playerRejoined(): no route back
- [ ] All remaining participants disconnected and no rejoin capability: Session proceeds toward termination per platform rules
- [ ] With rejoin support, controller stays active, bounded by inactivity and max lifetime

### O3. Explicit leave (§16)
- [ ] LEAVE resets inputs, inserts playerLeft(P) immediately (no grace), status LEFT, transport closed
- [ ] LEFT is permanent (no later rejoin)
- [ ] All gameplay participants gone/left: Session cancelled (ALL_PLAYERS_LEFT)

### O4. Rejoin (§17)
- [ ] Owner resolved; userUUID -> existing ParticipantID
- [ ] Reject: LEFT, never a participant, game without playerRejoined
- [ ] playerRejoined(P) inserted into the timeline; script decides (state, game time, deadlines)
- [ ] Rejected -> REJOIN_REJECTED + socket closed
- [ ] Accepted -> CONNECTED, PLAYER connection attached, inputs at defaults, FULL projection + fresh baseline, FE sends snapshot
- [ ] Transport dying right after accepted rejoin starts a normal grace flow (rejoin not rolled back)

### O5. Device switch (§19)
- [ ] At most one active PLAYER transport per participant
- [ ] New transport replaces old; old closed; no playerDisconnected/playerRejoined
- [ ] Inputs reset to defaults; new transport gets FULL projection + highest accepted SAMPLE sequence baseline

---

## P. Recovery checkpoint (§20)

- [ ] Checkpoint contains: global state, executedThroughSessionTime, contextRevision, PRNG state, pending timers, processed COMMAND tokens per participant, consumedThroughSampleSequence per participant, participant status (CONNECTED/DISCONNECTED/LEFT), other controller state needed for exact recovery
- [ ] Checkpoint excludes: current SAMPLE values, pending transitions, pending COMMANDs, projection cache, projection revisions, WebSocket state
- [ ] State and metadata describe the same materialized execution boundary (never state through SAMPLE 100 with consumed sequence 103)
- [ ] Strong initial checkpoint (see G)
- [ ] Periodic checkpoints: best effort, asynchronous, taken from an immutable snapshot at a safe boundary, never blocking gameplay on the external write
- [ ] Checkpoint failure does not stop the Session; later checkpoints can succeed
- [ ] Storage technology chosen and implemented (decision)

---

## Q. Recovery after Runtime pod loss (§21)

- [ ] Lazy: no eager recreation of crashed Sessions
- [ ] First reconnect: owner registry sees no valid owner -> claims new pod -> request routed
- [ ] New pod creates the controller, loads Session + pinned GameVersion + latest checkpoint
- [ ] Restores state, timers, PRNG, sequences, participant statuses
- [ ] All logical inputs default
- [ ] Fresh Sandbox context from recovered state; projections rebuilt via project()
- [ ] Requesting connection attached by role; FULL projection + fresh baseline sent
- [ ] Game time resumes from the checkpoint value (outage not added)
- [ ] Recovery window (platform time, ~30 s): previously-CONNECTED participants may return without being converted to disconnects
- [ ] At window end, playerDisconnected() inserted for previously-connected participants who never returned
- [ ] Participants DISCONNECTED in the checkpoint return only via real playerRejoined flow; LEFT remain unable to rejoin
- [ ] Rollback to the last checkpoint is handled; clients reconcile on the full projection

---

## R. Replay recording - producer side (§22)

- [ ] Replay data contract defined: sessionUUID, gameVersionID, replay format version, participant metadata (ParticipantIDs), full base state, gameTime per state point, sampled state deltas, logical events, COMMAND responses with target participant, timeline order for equal gameTime (decision: encoding/compression/storage technology)
- [ ] No recovery-only data in replay (sequences, tokens, timers, PRNG, connection state, lease)
- [ ] Capture cadence independent from execution cadence and chunk duration (three separate knobs)
- [ ] Replay Recorder receives immutable consistent captures at safe boundaries (never reads mutable state)
- [ ] Chunk = FULL_STATE first, then STATE_DELTA, EVENT, COMMAND_RESPONSE records
- [ ] Sealed chunks persisted asynchronously; the next chunk starts capturing immediately
- [ ] Bounded memory queue; retry on failure; when exhausted, degrade/drop best-effort history without stopping gameplay
- [ ] Transient records keep visibility/target (broadcast, participant-targeted, COMMAND response); a P1-private response is never replayable under P2
- [ ] Final state: durable final Replay point persisted BEFORE the durable FINISHED transition and cleanup; final chunk may be shorter
- [ ] Raw global state stored backend-only

---

## S. Session termination (§23)

- [ ] FINISHED: Sandbox success with FINISHED -> commit -> force durable final Replay state / game result -> durable FINISHED -> final live state/events to connected users -> close transports -> destroy Sandbox context + controller -> release owner
- [ ] CANCELLED (host), EXPIRED (lobby/inactivity), ABORTED (unrecoverable) complete without calling JavaScript
- [ ] Clients receive a platform terminal message with cause
- [ ] FINISHED Sessions are historical only; no live reactivation (§30.26)

---

## T. Reconciliation / lifecycle jobs (§24)

- [ ] Expired LOBBY Sessions marked EXPIRED
- [ ] RUNNING without a valid live owner: not aborted immediately; after the recovery/owner-loss window, marked ABORTED with the right cause
- [ ] "Valid owner" defined by the owner registry/lease (see A1)
- [ ] No transparent suspension of healthy controllers; a running Session stays live while meaningfully active

---

## U. Inactivity and lifetime (§25)

- [ ] Maximum Session lifetime enforced
- [ ] Meaningful-inactivity timeout enforced
- [ ] Counts as activity: distinct SAMPLE changes after dedup, COMMANDs
- [ ] Does NOT count: identical refreshes, ping/pong, acks/resync metadata, update() calls, timer firings
- [ ] An update() game cannot consume compute indefinitely without human interaction

---

## V. Errors (§27)

- [ ] Protocol/contract errors rejected pre-JS (malformed message, undeclared command, bad payload, bad SAMPLE shape, over-limit, wrong role)
- [ ] Logical game results returned as normal data; Session continues
- [ ] Script/runtime failure handled per M (retry once, then abort)
- [ ] Infrastructure failure handled per M (recreate + retry)
- [ ] No partial state/events/timers/responses ever escape a failed batch

---

## W. Rate limits and resource controls (§28)

- [ ] SAMPLE: per-participant ingress rate, max distinct transitions per window, bounded pending buffer, payload size, identical-refresh dedup
- [ ] COMMAND: per-participant token bucket, max pending, bounded queue, payload size
- [ ] WebSocket: messages/s, bytes/s, max message size, outbound backpressure protection
- [ ] Per-message-type rate limiting at the API edge (pending task, requested by the human 2026-10-17). Every live connection reads with one loop that hands each message to its type's own lane (`api/internal/live/lanes.go`: one goroutine per type, so SAMPLEs only wait for SAMPLEs, COMMANDs for COMMANDs, resyncs for resyncs). Handing off never blocks, so a lane's queue is unbounded: until this limit exists, a client sending faster than its lane is handled grows that queue without bound. The limit must be enforced per type before enqueueing, and bound each lane's queue.
- [ ] Sandbox: CPU/execution budget, memory, deadline, state/output size, non-cooperative code protection
- [ ] Session: max lifetime, inactivity timeout, owner/recovery deadlines, bounded Replay and checkpoint queues
- [ ] Scripts cannot request higher tick rates or unbounded buffers
- [ ] All numeric values chosen and configurable (decision)

---

## X. Security (§29)

- [ ] Backend JS has no external-world access
- [ ] Frontend package never receives backend script, raw state, or credentials
- [ ] Assets reached only through Playhoot-controlled handles / signed access
- [ ] All mutations pass through one ordered lane per Session
- [ ] Contract validated before executing authored JavaScript
- [ ] Script exceptions treated as implementation failures, not game rejections
- [ ] Published artifacts immutable and retained

---

## Y. Observability: monitoring and metrics (MANDATORY, not in the plan)

Ownership rule (`docs/engineering/standards/metrics.md`): business-outcome metrics below (sessions created, joins, starts, recoveries, ...) are declared and recorded by the Session domain, the owner of that logic, never by an endpoint. The API edge only measures its own transport: HTTP per route (request count by status, duration) centrally in `restObservability` and `wsObservability`, and the live connections it runs in `api/internal/live`.

Foundation (existing: `logging` package, `monitoring` package with alerts only, `docs/engineering/standards/logging.md`):
- [x] Metrics entry-point API exists: `monitoring/metrics` (Counter, Gauge, Histogram; domain-declared closed-set labels; every call written to the request log). Emission is not implemented yet.
- [ ] Metrics emission to the real backend implemented in `monitoring/metrics` (decision: provider/format; GCP Cloud Run target)
- [x] Metrics applied centrally for REST and WebSocket in `api` Server, per the Centralized Observability rule (`api_http_requests_total{route,status}` and `api_http_request_duration_seconds{route}` in `restObservability`; `api_websocket_handshakes_total{route,status}` and `api_websocket_handshake_duration_seconds{route}` in `wsObservability`)
- [ ] Per-message logging on live connections follows the logging standard (each inbound message is its own entry point; trace correlated per connection)
- [ ] Alerts use the monitoring abstraction; internal inconsistencies follow `data-integrity.md`
- [ ] Metric cardinality is controlled (no per-session or per-user labels on high-volume metrics)

Session lifecycle:
- [ ] Sessions by state and endCause (created, started, finished, cancelled, expired, aborted)
- [ ] Active controllers and active Sessions per pod
- [ ] Create-session / join / start / rejoin outcomes (accepted vs rejected, by reason) and latency
- [ ] Start duration end-to-end and start-failure counts (incl. orphan checkpoints)

Connections and protocol:
- [ ] Open WebSockets by role (PLAYER/ADMIN); attach/detach rates; transport-grace expirations (API side so far, without role: `api_live_connections_active`, `api_live_connections_closed_total{close_cause}`, `api_live_connection_duration_seconds` in `api/internal/live`; per-role and admission counts belong to the Runtime)
- [ ] Logical disconnects, leaves, rejoins (accepted/rejected), device switches
- [ ] Protocol/contract rejections by reason; rate-limit hits by limit type
- [ ] Inbound/outbound message and byte rates; outbound backpressure/slow-consumer events (network message counts belong in `api/internal/live`; per-Session aggregates such as messages per Session belong to the Runtime)

Execution timeline:
- [ ] Processing boundary duration, batch size, queue depth (pending timeline operations)
- [ ] Boundary lag vs target cadence (~100 ms); update() call interval and skipped/dropped updates
- [ ] SAMPLE: accepted vs deduped vs stale; pending-transition buffer depth
- [ ] COMMAND: rate, latency (receive -> response), ALREADY_PROCESSED count, logical rejections vs structural rejections
- [ ] Timers: scheduled, fired, firing lateness

Sandbox:
- [ ] Execution latency per entrypoint (start, samples, command, update, timer, project, ...)
- [ ] Script errors, infra errors, retry counts and retry outcomes (flaky-script alert), resource-limit violations
- [ ] Context creates/recreates/evictions; state size and output size
- [ ] Sandbox sidecar health/restarts

Projection and replication:
- [ ] Projection computation latency; projection/patch size; full-vs-patch send ratio
- [ ] Resync requests and reasons

Recovery and ownership:
- [ ] Checkpoint success/failure, duration, size, age of latest successful checkpoint per active Session (alert on stale)
- [ ] Recoveries performed, recovery duration, recovery-window expirations
- [ ] Owner claim/release/lease-loss events; any detection of dual ownership (critical alert)
- [ ] Reconciliation jobs: Sessions expired/aborted per run, job failures

Replay:
- [ ] Capture rate, chunk size/duration, persistence latency, retry count
- [ ] Queue depth and dropped chunks/records (alert on drops)
- [ ] Final-state persistence success/failure (critical alert)

Alerts and dashboards:
- [ ] Alerts defined for: dual ownership, sandbox retry-after-script-error, repeated session aborts, checkpoint staleness, replay drops, final replay failure, boundary lag, sandbox sidecar unhealthy, queue saturation
- [ ] Dashboards for: lifecycle funnel, live load (controllers/connections/boundary lag), sandbox health, recovery/checkpoint health, replay pipeline
- [ ] Per-Session log correlation: one trace per connection, Session/Participant identifiers present in logs (without sensitive data)

---

## Z. Documentation synchronization

- [ ] Accepted architecture promoted from the plan `.docx` to durable owners (ADR(s), `play/README.md`, `ARCHITECTURE.md` as needed) (decision: form/timing)
- [ ] `play/CURRENT_STATE.md` reflects the implemented state
- [ ] `play/docs/DATA_MODEL.md` and `play/docs/FLOWS.md` created and accurate
- [ ] `docs/architecture/CROSS_DOMAIN_FLOWS.md` and `SYSTEM_MAP.md` updated for implemented flows
- [ ] `docs/ai/KNOWLEDGE_MAP.md` row for Session Runtime updated (and `session/` vs `play/` path drift resolved)
- [ ] Observability decisions captured in the right owner (standard/ADR), not only in code

---

## Placeholders left behind (code that exists and is called, but does nothing real)

Everything here is wired into the running system, so a request flows through it, but it returns hardcoded or failing answers. It must never be mistaken for implemented behavior. Items marked "out of scope" are not part of this project (which focuses on `play/`) and stay open after it ends; the owning domain's `CURRENT_STATE.md` also records them. Tick an item only when the real behavior replaces the placeholder.

- [ ] Identity authorization (`identity/usecases/authorize`, out of scope): recognizes only hardcoded test tokens (`test-user-1`, `test-user-2`, `test-guest`), each mapped to a fixed UserUUID; any other token is unauthenticated, and the guest is forbidden from `CREATE_SESSION` and `JOIN_SESSION_AS_ADMIN` (allowed `JOIN_SESSION_AS_PLAYER`, like every other operation). Must never be deployed as-is. Real behavior: validate the JWT, resolve the stable UserUUID, decide per operation; `CREATE_SESSION` and `JOIN_SESSION_AS_ADMIN` must require an account and forbid guests.
- [ ] Game playability check (`game/usecases/checkplayability`, out of scope): answers `true` for any Game and user. Real behavior: the Game exists, has a published version, and is visible to the user.
- [x] Session creation stand-in (`unwiredSessionCreation` in `unwired.go`, in scope): always failed. Replaced by `sessionlifecycle.Manager.CreateSession` on 2026-10-15; `unwired.go` was deleted.
- [ ] No flow creates `play_games` / `play_game_versions` (in scope, see B): nothing registers Games or marks a version EXECUTABLE yet, so every Session creation answers `game_unavailable` until those rows exist (outside production, test-data migrations seed one playable and one unavailable Game; see `play/CURRENT_STATE.md`). Real behavior: the operation that validates a version's scripts, freezes it, and sets it EXECUTABLE as the Game's current version.
- [ ] Live Session ownership (`play/runtime/usecases/ownership`, in scope, see A1): `LocateOrClaimOwner` answers `owned_here` with epoch 1 for any Session UUID, without looking the Session up or keeping a registry. Real behavior: claim only Sessions in LOBBY/RUNNING, in-process registry with release; shared registry + lease/heartbeat before multi-instance (SESSION-ADR-0029).
- [x] Admin join eligibility (`play/session/usecases/checkadminjoin`, in scope, see E): replaced by the real check on 2026-10-17 (the Session exists, the user created it, and it is RUNNING or in a LOBBY whose expiry has not passed).
- [ ] Admin operations (`play/runtime/internal/controller`, in scope, see E): `SelectPerspective`, `StartSession` and `CancelSession` return "not implemented", and `NextView` and `NextLobbyUpdate` never produce anything. Real behavior: perspective selection with FULL projection, Start (G), host cancellation, lobby updates.
- [ ] Player admission (`unwiredPlayerAdmission` in `unwired.go`, in scope, see F): `AttachPlayer` always fails, so every player connection is closed with 1011 right after the upgrade. Real behavior: `liveconnection.Manager.AttachPlayer`, lobby admission serialized by the controller, returning a `runtime.PlayerConnection`; then delete the stand-in.
- [ ] Join code resolution (`unwiredJoinCodes` in `unwired.go`, in scope, see A3): `ResolveJoinCode` always fails, so joining by join code answers `500`. Real behavior: `play/session` finds the Session of an active, unexpired join code (`not_found` otherwise); then delete the stand-in.
- [ ] Controller lifetime (`play/runtime/internal/controller`, in scope, see H): controllers are created but never destroyed or released, so they accumulate for the process lifetime. Real behavior: destroyed on terminal states with owner release.
- [ ] Forwarding to a remote owner (`api/play/owner_routing.go`, in scope, see A1): a Session owned by another instance is answered `503`; unreachable while single-instance. Real behavior: proxy the connection to the owner (gateway), decided with the multi-instance topology.
- [ ] Metrics emission (`monitoring/metrics`, `record`): measurements are only written to the request log; nothing is sent to a metrics backend (see Y).

## Invariants verification (§30)

Each needs at least one test or other concrete evidence before ticking.

- [ ] 1. One Session, one live owner
- [ ] 2. One authoritative timeline; per-player buffers are never authority
- [ ] 3. RUNNING implies recoverable initial state
- [ ] 4. Lobby membership is ephemeral; durable participants only at Start
- [ ] 5. ParticipantID is the script's identity; userUUID stays platform-side
- [ ] 6. No late new players after Start
- [ ] 7. Transport recovery is not game rejoin
- [ ] 8. Disconnect resets inputs immediately
- [ ] 9. Explicit leave is permanent
- [ ] 10. SAMPLE sequence vs COMMAND token identity rules
- [ ] 11. Distinct SAMPLE transitions preserved; refreshes deduped
- [ ] 12. COMMAND logical rejection still consumes its token
- [ ] 13. Game time is not wall time
- [ ] 14. update() is frequent, not exact
- [ ] 15. Timers belong to Runtime, checkpointed, game-time based
- [ ] 16. Sandbox has no external side effects; ambiguous calls discarded and retried from the mirror
- [ ] 17. Failed execution has no partial output
- [ ] 18. project() is pure and deterministic
- [ ] 19. Projection privacy is backend-only
- [ ] 20. Projection revision belongs to the viewer stream
- [ ] 21. Full resync always available
- [ ] 22. Recovery may roll back
- [ ] 23. Recovery and Replay are separate
- [ ] 24. Replay never blocks gameplay indefinitely
- [ ] 25. FINISHED has a durable ending
- [ ] 26. Finished Sessions are historical, not live
- [ ] 27. Platform termination does not depend on script cleanup
- [ ] 28. Published GameVersion stays available
