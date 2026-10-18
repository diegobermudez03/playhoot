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
- [ ] Join code -> sessionUUID resolution -> owner resolve/claim -> route to pod (API side done and tested 2026-10-17: `GET /v1/join/{joinCode}` resolves the code (`api/play/join_code_routing.go`) before owner routing; the resolution itself is real since 2026-10-17: `play/session/usecases/resolvejoincode`, an active and unexpired code names its Session, matched case-insensitively; unit and repository tests. Pending: forwarding to an owner elsewhere, see A1)
- [ ] Join code is only a friendly identifier, never authN/authZ
- [x] Join code expiration uses platform time

### A4. Identity integration (§1, §2, §4)
- [ ] Account token -> account userUUID
- [ ] Guest token -> existing guest userUUID
- [ ] No identity -> guest created, returned userUUID + guest credentials for the FE to keep (decided by the human 2026-10-17: authorization always needs an existing token; a client without one gets `401` with code `token_required` and calls `POST /v1/guests {display_name, idempotency_key}` first, then joins with the guest token. Identity stores the guest's display name. API side done and tested (`api/identity`); Identity's `CreateGuest` is a placeholder, see Placeholders)
- [ ] CREATE_SESSION capability check (real account required, guest-only rejected)
- [x] Other domains receive only userUUID (no account/guest awareness)
- [x] Identity does not decide object-level permissions (creator check lives in Session)

### A5. Two clocks (§1)
- [x] Game/Session effective time: monotonic, starts at RUNNING, persisted in checkpoints, freezes during Runtime outage (2026-10-18: the controller's clock starts at RUNNING from the confirmed state's time, the checkpoint stores `executed_through_session_time_ms` and an adopted or recovered game resumes from it, so time spent without an owner does not count; `Date.now()` and `context.sessionTime` are this time; controller and sandbox tests)
- [ ] Platform time used for: transport grace, owner lease/heartbeat, join-code expiry, recovery window, inactivity limit, max lifetime
- [ ] A logical game pause does not freeze game time

### A6. One authoritative timeline (§1)
- [x] Single logical mutation lane per Session (2026-10-18: `play/runtime/workflows/gameloop` runs one lane per running game, the only code that mutates its state; started when the game starts or is adopted, ended with the controller or on shutdown; tests. Since 2026-10-18 (SESSION-ADR-0030): the lane is the controller's own main goroutine, alive from its creation, which runs Start, adoption, ticks and aborts; `gameloop` no longer exists)
- [x] Every accepted operation gets server-owned sessionTime + serverOrder; client never supplies authoritative timestamps (2026-10-18: the controller stamps each accepted transition with the effective Session time and an order counter; controller tests)
- [ ] Timeline carries: SAMPLE transitions, COMMAND, timer firing, update(), playerDisconnected, playerRejoined, playerLeft (2026-10-18: SAMPLE transitions and the default-inputs transition of a disconnect so far; the controller's main goroutine already takes a flush request (`requestFlush`) for commands and timers)
- [ ] executedThroughSessionTime advances on any materialized operation (not only Samples) (2026-10-18: advances with every committed boundary; only SAMPLE batches exist so far)
- [x] Sandbox receives elapsed effective time since the previous materialized operation (2026-10-18: `samples(state, transitions, context)` receives `context.sessionTime` and `context.elapsed`, and each transition its own `sessionTime`; sandbox tests)
- [x] Batching never changes semantic ordering (§7) (2026-10-18: one `samples()` call per boundary with every participant's transitions in arrival order, never grouped by player (decided by the human); commands will split the batch at their position; controller tests and the end-to-end run)

### A7. Message model and role gating (§26, §27)
- [ ] Platform messages vs game messages are distinct categories
- [x] Connection role (PLAYER/ADMIN) is authoritative for what it may send (each endpoint only accepts its own message catalog; any other type closes with 1008 before reaching the Runtime; tested for both, 2026-10-17)
- [x] ADMIN can never send SAMPLE/COMMAND (the admin catalog has no such message; any other type closes the connection with 1008 before reaching the Runtime)
- [ ] Contract validation happens before JavaScript is called (2026-10-18: done for SAMPLEs (undeclared input or invalid value closes the connection as malformed, 1007, before the timeline); commands pending)

---

## B. GameVersion and manifest contract (§1)

- [ ] Session pins exactly one GameVersion for its whole lifetime and for replay
- [x] Only versions in the executable state can create Sessions (the plan says PUBLISHED; this domain names its own state EXECUTABLE: immutable, scripts ready to run)
- [ ] Publishing a version rejects backend scripts that keep module or global state or mutate their inputs: the validator exists (`play/runtime/sandbox/scriptcheck.Validate`, AST-based, conservative, decided by the human 2026-10-18), nothing calls it until the publishing flow exists
- [ ] Published version artifacts are immutable and never physically deleted (backend JS, frontend package, manifest, assets)
- [ ] Artifacts addressed by locator + hash + size + metadata; content never silently changes
- [ ] Runtime manifest exists, versioned and built iteratively (decided by the human 2026-10-17: a `manifest JSONB NOT NULL` column on `play_game_versions`, `{"schema_version":1,...}`, read and validated by `play/session/internal/business/manifest`, loaded by `play/session/usecases/loadruntimemanifest` when a Session's controller is created and kept by it). Declared so far: player limits (`players.min`, `players.max`, 1 <= min <= max). Nothing writes manifests yet besides test data (see Placeholders: no flow creates game versions)
- [ ] Runtime manifest declares: SAMPLE input defs/types, defaults (when not inferable), COMMAND names + payload schemas, optional entrypoints present (update, playerRejoined), required input capabilities, frontend/backend artifact identity (2026-10-18: SAMPLE inputs declared and validated (`inputs` with boolean, number, vector2, string, enum; natural zeros implicit, explicit default required for enums or out-of-range zeros; `play/internal/business/inputs`, shared by `play/session` and `play/runtime`); the rest pending)
- [ ] Manifest drives: pre-JS rejection of malformed/undeclared input, discovery filtering by capability, update()/rejoin support detection (2026-10-18: done for SAMPLE inputs)
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
- [ ] Admin operations implemented: `SelectPerspective`, `StartSession` and `CancelSession` (`StartSession` is orchestrated by the Runtime since 2026-10-17, see G; `SelectPerspective` and `CancelSession` still answer "not implemented"), and `NextLobbyUpdate` (real since 2026-10-17)
- [ ] Admin receives lobby updates until the lobby closes (`lobby_update` done end to end 2026-10-17: the first is the lobby as it is, then every change. Decided by the human 2026-10-17: a start ends the lobby with `session_started`, never `lobby_closed`; `lobby_closed` is only for a lobby that closes without starting (host cancels, lobby expires), and the Runtime must then end every player's connection. The API sends it and stops the stream; nothing in the Runtime produces it yet, since cancellation and expiry are not built)
- [ ] Creator who also plays may not watch another participant: `SelectPerspective` answers `creator_is_playing` while the creator has a PLAYER connection (decided by the human 2026-10-17; the outcome is declared, the controller check is not implemented)
- [ ] Admin selects a player perspective; receives exactly projection(P), FULL, with a new baseline per selection
- [ ] No omniscient admin projection
- [x] Admin reconnect = just another admin WebSocket (no game-level admin rejoin)
- [ ] Admin connections do not receive participant SAMPLE acknowledgements

---

## F. Player lobby join (§4)

Player join (connection and lobby admission) completed 2026-10-17: implemented end to end and verified by unit, repository and WebSocket tests, an end-to-end run against a fresh database, and a manual test by the human. API side: `GET /v1/sessions/{sessionUUID}/player` and `GET /v1/join/{joinCode}` (`JOIN_SESSION_AS_PLAYER`, which guests may perform; join code resolved before owner routing; `playhoot.v1` upgrade), player wire messages, one lane per message type so a slow COMMAND never delays a SAMPLE, and `POST /v1/guests` for clients without an account (authorization failures carry a stable code; `token_required` sends such a client to create a guest first). Runtime side: `liveconnection.Manager.AttachPlayer` asks the durable Session `EligibleToJoinAsPlayer` (real: LOBBY before its expiry), then the Session's controller, created with the game version's runtime manifest (first iteration: player limits), admits into the lobby under its lock: up to the manifest's maximum, the same user's older connection replaced (closed 1000 `replaced`), every lobby change sent to admins and players. Decided by the human: admins and players are always separate connections, even for a creator who plays; leaving or disconnecting in the lobby takes the player out at once and they may join again; the player contract declares every operation now, implemented or not.

Left for future steps, on purpose:
- Identity's real behavior: `CreateGuest` returns the hardcoded test guest and `Authorize` only knows test tokens (see A4 and Placeholders; out of scope).
- The operations of a running game: `Sample`, `Command`, `Resync`, views and `Leave` in a running game; the controller has a lobby/running phase, but nothing moves it to running until Start (G) exists (see I, J, M, O and Placeholders).
- Start itself (G), which ends the lobby with `session_started` and uses the manifest's minimum (orchestrated since 2026-10-17, see G).
- Device/control capabilities checked against the manifest, once the manifest declares them (B).
- Rate limiting per message type on live connections and on guest creation (W); until then each lane's queue is unbounded.
- Ownership is still a single-instance placeholder, the controller serializes with a mutex instead of the single mutation lane, and controllers are never destroyed (A1, H).

- [x] Join by join code or Session UUID; guest display name when a new guest is created (the Session Runtime side is done: both routes, and the guest named through `POST /v1/guests` before joining; Identity's real guest creation is out of scope and tracked under Placeholders)
- [x] Identity resolution (the Session Runtime side is done: the routes require an existing token, a guest's or an account's, and Identity resolves it; Identity's real behavior is out of scope, see A4 and Placeholders)
- [x] Owner resolve/claim (the routing step; ownership itself is a placeholder, see A1)
- [x] Logical admission serialized through the SessionController (two users racing for the last slot cannot both be accepted): admission and the capacity check run under the controller's lock (a mutex until the single mutation lane exists, see H)
- [ ] Checks: Session still LOBBY, player count / GameVersion constraints, device/control capabilities vs manifest (LOBBY before its expiry and the manifest's maximum done; capabilities pending until the manifest declares them)
- [x] Same user's older PLAYER transport replaced if needed (the older connection ends `replaced`, closed 1000; the user keeps their place and nobody is told they left)
- [x] Accepted: ephemeral lobby member (not durable; lobby changes reach every admin and player as `lobby_update`)
- [x] Rejected: JOIN_REJECTED sent, socket closed (sent as `{"type":"rejected","reason":...}`, closed 1008: `session_unavailable`, `session_started`, `lobby_full`, `owner_changed`)
- [ ] Lobby pod loss: Session stays LOBBY, players can rejoin the lobby; no lobby checkpoint (holds by design today: the lobby lives only in the controller, nothing durable changes on joining, and a reconnect creates a new controller; not verified until ownership can move between instances, see A1 and Q)
- [ ] No late join of new participants once RUNNING (§30.6) (a RUNNING Session rejects new players as `session_started`; to tick together with invariant 6 once Start exists)

---

## G. Start Session (§5)

Progress note (2026-10-17): the Runtime side is implemented and tested; the durable side is implemented and tested against Postgres (see the update below); the Sandbox is implemented (see the sandbox update below). `liveconnection.Manager.startSession` (`step_start_session.go`) orchestrates; the controller only changes state under its lock (`BeginStart`, `CancelStart`, `CompleteStart`, `AbortStart` in `play/runtime/internal/controller/start.go`). Contracts: `sessionStarter` (`CheckStartEligibility`, `CommitStart`, `AbortStart`), `frontendSigner` (`FrontendAccess`) (types in `play/session/session.go`) and `gameSandbox` (`CreateContext`, `Start`, `Project`, `DiscardContext`; types in `play/runtime/sandbox/context.go`). The checkpoint is encoded by the Runtime (`play/runtime/internal/business/checkpoint`, versioned JSON) and stored by the Session as opaque bytes. Update (2026-10-17, decided by the human): checkpoints live in Postgres, latest only per Session (`play_session_checkpoints`; replay is a separate mechanism), so the checkpoint travels inside `CommitStart` and roster + checkpoint + RUNNING + join code release are one transaction (no separate `StoreCheckpoint`, no orphan checkpoints); game versions keep their backend script and frontend package locators in `play_game_versions` columns; the frontend package is one self-contained HTML file for v1 (chosen by the agent at the human's request; assets resolved separately by Playhoot); join codes are released on commit and abort; the object storage for local and dev stays open (leaning towards an emulator), selected by environment variables only. Decided by the human 2026-10-17: players and admins receive an explicit `session_started` (roster as `participants: [{participant_id, user_uuid}]`, plus the player's own `participant_id`, plus the game frontend as `frontend: {url, sha256, expires_at}`) through a new `Started` operation on both connections; the frontend package is delivered as a signed, expiring URL to private object storage (plan §1 "GameVersion is immutable and pinned", §29 "signed access"), issued by the durable Session (`FrontendAccess`, right after the eligibility check, before the Sandbox runs) and sent with the start, never as bytes over the WebSocket; the API sends lobby updates, then the start, then the views, in that order; a double script failure answers `start_failed` and ends every connection as `session_aborted` (closed 1000); an infrastructure failure returns the Session to its lobby and fails the call (1011). Choices made in implementation, to review: while an attempt runs, new players are rejected as `session_started`, a player who disconnects stays in the frozen roster (taken out of the lobby only if the attempt is cancelled), and `Leave` answers "not implemented"; the first projections are computed before the final transaction (projection is pure), so a `project()` script failure counts as a start failure instead of leaving a RUNNING Session without views; an infrastructure failure is not retried in a recreated context, the admin may start again; ParticipantIDs are `P1..Pn` in lobby join order; the seed of the script's randomness is drawn per attempt with `crypto/rand` and reused by the retry. Decided 2026-10-17 (human): every Session status change is recorded in `play_session_status_changes` (from, to, end cause, the user or the platform that caused it, when), in the same transaction, only through the `sessionlifecycle` repository (`CreateSession`, `ChangeSessionStatus`); implemented for creation, start and aborted start, with repository and integration tests. Decided 2026-10-17 (human, on the agent's recommendation): Start has no idempotency token; it is naturally idempotent (one LOBBY -> RUNNING transition, serialized by the controller and the commit's row lock) and a repeated `start_session` is evaluated against current state with a precise outcome: `already_started` (running), `start_in_progress` (another attempt running), `not_in_lobby` (ended or lobby expired); the durable check answers `already_started` for a RUNNING Session too. Sandbox update (2026-10-17, decided by the human): one long-lived worker process per Session context (re-exec of the binary, QuickJS on WebAssembly), not a shared sidecar; the old stateless executor and its `execute(previousState, event)` contract were replaced; script contract `start(participants, context)` -> `{ state }` and `project(state, participant, context)` -> view. Choices made in implementation, to review: an execution that outlives its 5 s budget kills the worker and counts as a script failure (plan §11: budget violations are script failures); a call abandoned by its caller is an infrastructure failure; `project()` has no randomness: `Math.random` throws while it runs (changed 2026-10-18 from rolling draws back); a script that fails to load is reported by `start()`. Sandbox constraints (2026-10-18, set by the human): (1) the Runtime mirror holds everything Restore needs: `controller.Game` now also keeps the game version and script, and no game state can live outside it (strict mode, globals and builtins deep-frozen after load, statically rejected at publication); (2) the script has no host capability, enforced by the patched runtime (`third_party/qjs`), the empty environment being defense in depth; (3) `project()` cannot change the authoritative context or consume its randomness (frozen state, `Math.random` throws, frozen globals); (4) the host has instance-wide limits on contexts and concurrent executions. Decided with the human: the JavaScript runtime stays hot per Session; persistent state is kept out of it by static validation at publication plus runtime freezing, not by a fresh runtime per call. Known gap: `CheckStartEligibility` never answers `game_unavailable` (no rule makes a pinned version unplayable yet). Known gap: a connection that receives the start later (an admin attached to a running game, and future rejoins) gets the URL issued at Start, which may have expired; re-issuing it per delivery is not built. Decided 2026-10-17 (human): the database commits before the controller runs, and the database is the truth. A `start_session` reaching a controller still in its lobby while the durable Session is already RUNNING (a lost commit answer, or a new controller after a restart) adopts that start: `LoadRunningSession` returns the committed roster and latest checkpoint, the Sandbox restores a context at it (`Restore`, no `start()` again), every participant is projected, and the controller runs with the committed roster (a lobby player outside it is ended); the admin is answered `already_started` (counted in `play_runtime_started_sessions_adopted_total`). A controller in `starting` still answers `start_in_progress`: its attempt is alive and always ends (completed, cancelled or aborted), so adopting concurrently would race it. No acknowledgement of `session_started` is awaited: while a connection lives the transport delivers it, and a client that lost its connection resyncs, which is the rejoin and transport-recovery work (§14, §17). The controller no longer checks the player count itself; the durable Session decides it against the manifest.

- [ ] START accepted only from the creator's ADMIN connection; no identity token in the live message (changed from the plan, which also accepts the creator's PLAYER connection, by the human on 2026-10-17: admin and player are always separate connections, and starting and cancelling are admin operations. `AdminConnection.StartSession` and the `start_session` message are wired end to end; the durable creator check waits for `CheckStartEligibility`)
- [x] Controller serializes START; lobby roster frozen for the attempt (`BeginStart` under the controller's lock; a second attempt answers `not_in_lobby`; joins are rejected during the attempt; controller tests)
- [x] Stable ParticipantIDs assigned (userUUID -> P1..Pn, in lobby join order; controller tests)
- [x] /play/session validates: LOBBY, not expired, caller is creator, participant count valid, pinned GameVersion available; returns artifact/manifest references (`sessionlifecycle.CheckStartEligibility`, returns the verified script itself; unit and integration tests; loading the script needs object storage configured)
- [x] Sandbox context created for the pinned GameVersion (`sandbox.Host.CreateContext`: a worker process per context with the script loaded; tests)
- [ ] start(participants, controlled context) executed; success returns initial full state + timers/events + initial PRNG state + context revision (`sandbox.Host.Start`: ParticipantIDs and seed in, state + exported PRNG state + revision 1 out; tests; timers/events are not in the contract yet, they come with K)
- [x] Initial recovery checkpoint written strongly/durably BEFORE RUNNING (§30.3) (written in the same transaction that marks RUNNING; integration test incl. rollback)
- [x] Final Start transaction: revalidate, persist roster, store checkpoint reference, mark RUNNING (`sessionlifecycle.CommitStart`: row lock, revalidation, roster, checkpoint itself, RUNNING, join code release; unit and integration tests)
- [x] Orphan checkpoint (written, but DB transition failed) is tolerated and cleanable; a RUNNING Session without recoverable initial state is impossible (no orphans can exist: the checkpoint is part of the start transaction; integration test)
- [x] Runtime commits live mirror; project() for each player; START + FULL initial projection sent (mirror kept in the controller, `session_started` with roster and signed frontend URL, then a FULL `player_view` per connected player from `sandbox.Host.Project`; run end to end on 2026-10-18 against Postgres, the GCS emulator and real sandbox workers: session created, player joined, admin started, player got `session_started` and its FULL view from the seed script, the signed frontend URL served the package with a matching SHA-256, and the database held RUNNING, roster, checkpoint revision 1, released join code and status history)
- [ ] start() script failure: retry once from the same pre-start state with fresh context (orchestration done and tested with mocks: same roster and seed, new context, flaky-start alert when the retry succeeds)
- [ ] Second failure: Session ABORTED / GAME_START_FAILURE, never RUNNING, clients informed, transports closed (Runtime side done and tested: `AbortStart` called, admin answered `start_failed`, every connection ended `session_aborted`; the durable `AbortStart` is not implemented)
- [ ] After Start: recovery, replay, and checkpoint scheduling begin; script sees only ParticipantIDs

---

## H. SessionController (§6)

Update (2026-10-18, decided by the human, SESSION-ADR-0030): the controller is the live Session. It moved to `play/runtime/workflows/liveconnection` as an unexported type, absorbing the Start orchestration and the game loop: it calls the durable Session, the Sandbox and replay itself, owns its Sandbox context (discarded when the Session ends here or the controller is retired), locks its own state, and has one main goroutine from creation that runs every operation touching the Sandbox or the database. The `liveconnection` ControllerResolver (formerly `Manager`) only finds or creates controllers, without asking the durable Session anything (a controller may exist for a rejected connection), and stops them on shutdown; the controller checks admission eligibility and reads its manifest on its first admission. A refactor without changing behavior: same tests ported, and the same end-to-end run.

Progress note (2026-10-17): a first, minimal controller exists (`play/runtime/internal/controller`): a per-instance registry finds or creates one controller per Session (refusing one under another ownership epoch), and the controller only holds its attached ADMIN connections. Admission is serialized with a mutex, not yet a single mutation lane. Controllers are never destroyed yet.

- [ ] Exactly one active controller per active Session on the owner pod
- [ ] Owns live state: connections, userUUID<->ParticipantID map, participant status, per-participant logical SAMPLE state, SAMPLE ingress/dedup buffers, COMMAND pending/processed tokens, timeline ordering, effective Session time, executedThroughSessionTime, pending timers, Sandbox context ID, confirmed state mirror, PRNG state, contextRevision, projection cache, per-viewer replication state, checkpoint orchestration, replay capture orchestration (2026-10-18: per-participant SAMPLE ingress (max accepted, consumed, current inputs), the timeline, effective Session time, the confirmed mirror and per-connection last view and acknowledgement are held; COMMAND tokens, timers and participant status pending)
- [ ] Three state levels distinct and consistent: Sandbox context / Runtime confirmed mirror / durable checkpoint
- [x] Controller created only when a Session first needs live behavior (created on the first admitted live connection)
- [ ] Controller destroyed on terminal states (with owner release and Sandbox context destruction)

---

## I. SAMPLE protocol (§7, §13)

- [x] SAMPLE is a full logical input snapshot; defaults applied from manifest (type-inferred zero or explicit default) (2026-10-18: normalized against the manifest's inputs, defaults applied, in one canonical encoding; inputs tests)
- [x] Per-participant monotonic client sequence; gaps allowed; Runtime never waits for a missing sequence (2026-10-18: controller tests)
- [x] maxAcceptedSampleSequence[P] and consumedThroughSampleSequence[P] tracked (2026-10-18: controller tests)
- [x] Sequence <= high-water mark is stale/duplicate and ignored (2026-10-18: controller tests)
- [x] Normalize then dedupe against current accepted input state (identical refreshes are not transitions) (2026-10-18: controller tests and the end-to-end run)
- [x] Every distinct accepted transition preserved until processed ("latest wins" is NOT the model; §30.11) (2026-10-18: controller tests)
- [x] Refreshes still advance max accepted sequence and are acknowledged once materialized (2026-10-18: an acknowledgement-only view `{}` when nothing executed; controller tests and the end-to-end run)
- [x] Acknowledgement (consumedThroughSampleSequence) can advance without a projection change; not sent to ADMIN connections (2026-10-18: admin connections receive no views yet, so never an acknowledgement)
- [x] SAMPLE transitions from all players merged into the global timeline (2026-10-18: controller tests)
- [x] Adjacent Samples batched into one samples(...) call only when no other operation belongs between them (2026-10-18: only SAMPLEs exist yet; boundary tests in `liveconnection`)
- [ ] Device switch: new transport starts above the highest accepted in-memory sequence
- [ ] After crash: checkpoint sequence is the baseline; higher client values with gaps accepted
- [x] Unknown/undeclared input, bad shape, and oversize SAMPLE rejected pre-JS (2026-10-18: undeclared input, wrong type or out-of-range value closes the connection as malformed (1007); a SAMPLE is bounded by the schema (at most 32 inputs, strings up to 256 bytes); API and inputs tests)

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

- [x] Normal live processing cadence (~100 ms, platform tuning value) (2026-10-18: `gameloop` ticks every 100 ms)
- [x] Boundary: drain pending timeline -> confirm state -> project -> compare with cache -> send changes/acks (2026-10-18: take the timeline, `samples()`, check the revision, `project()` each participant, commit the mirror, send each connection the patch against the view it last received with its acknowledgement, offer the capture to replay; boundary tests in `liveconnection` and the end-to-end run)
- [ ] Commands and timers can trigger earlier boundaries (2026-10-18: the lane runs a boundary on `RequestFlush` (tested); no command or timer uses it yet)
- [x] If update() is not declared, Runtime does not execute JS merely because time passes (2026-10-18: a boundary with no transition executes nothing; boundary tests in `liveconnection`)
- [ ] If declared, update() called frequently but with no exact-interval guarantee; receives current Session time + elapsed game time
- [ ] No unbounded backlog of missed updates
- [x] Consistent budget violations abort the Session as a script/runtime failure (2026-10-18: a batch over its budget is a script failure, retried once, then the Session is aborted (SCRIPT_FAILURE); boundary tests in `liveconnection`)
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

- [ ] Entrypoints: start, samples, command, update (optional), timer, playerDisconnected, playerRejoined (optional), playerLeft, project (2026-10-18: start, samples, project)
- [ ] Mutating entrypoints return full new global state + events + timer changes + terminal FINISHED signal + COMMAND response
- [x] project(globalState, participant, context) is pure and deterministic (no wall clock, no random, not even the controlled PRNG) (state deep-frozen, `Math.random` throws, globals frozen after load; `TestProjectCannotChangeTheContext`)
- [x] Controlled deterministic PRNG; state is part of Runtime state and checkpoints; no uncontrolled Math.random as source of truth (done for start: seeded mulberry32, state exported and checkpointed; restoring it into a recreated context comes with recovery) (2026-10-18: a restored context continues with the same randomness, revision and time (`TestRestoredContextContinuesTheSameGame`))
- [x] No access to DB, files, secrets, network, processes, or uncontrolled wall clock (corrected 2026-10-18: the earlier tick was wrong, a script could write into the mounted scratch directory and import `qjs:os`; now the patched qjs in `third_party/qjs` mounts nothing and gives fake clocks and randomness, the host helpers are removed, and the worker's empty environment is only defense in depth; `TestPatchedRuntimeHasNoHostCapabilities`, `TestStart/no_host_helper_is_reachable`)
- [x] A batch of ordered operations is atomic: no partial output escapes (state, events, timers, responses, terminal signal commit as one) (2026-10-18: nothing commits or reaches a client unless samples() and every projection succeed; events and timers will join the same commit; boundary tests in `liveconnection`)
- [ ] Terminal FINISHED mid-batch: later operations ignored
- [x] Context revision advances on every successful batch, separate from projection revision and SAMPLE sequence (2026-10-18: the worker and the mirror advance it per batch; a result that does not follow the mirror's revision is out of sync and retried in a restored context; sandbox, controller and boundary tests in `liveconnection`)
- [x] Executor/infrastructure failure: discard context, recreate from the last confirmed Runtime mirror, retry batch (2026-10-18: retried once in the boundary; if it still fails the transitions are put back for the next boundary, and after 50 boundaries in a row the Session is aborted (UNRECOVERABLE_INFRA_FAILURE); boundary tests in `liveconnection`)
- [x] Script failure arrives as a valid protocol response with SCRIPT_ERROR body; retry once from the same confirmed pre-batch state (2026-10-18: boundary tests in `liveconnection`)
- [x] Retry success: continue and raise monitoring/alert (flaky execution); retry failure: Session ABORTED / SCRIPT_FAILURE (2026-10-18: alert on success after a script failure; second failure aborts durably (`sessionlifecycle.AbortRunningSession`, RUNNING -> ABORTED, history by the platform) and ends every connection `session_aborted`; gameloop and session tests)
- [x] Sandbox-only death: recreate from the Runtime mirror without rolling back to checkpoint (2026-10-18: the infrastructure-failure path restores from the mirror, not the checkpoint; boundary tests in `liveconnection`)
- [ ] Runtime death: old sidecar context treated as orphaned; new owner recovers from durable state only
- [ ] Idle contexts evictable and recreatable; update() games keep the context hot
- [ ] Resource limits: CPU/execution budget, memory, deadline, state/output size, protection against non-cooperative code (see W)

---

## N. Projection, replication, resync (§12)

- [ ] Runtime keeps the current full projection per ParticipantID
- [x] Script returns full projections; Runtime computes patches/deltas as optimization (decision: patch representation) (2026-10-18, decided by the human: a patch holds only the fields whose value changed, nested objects patched the same way, lists sent whole; a view's shape is fixed, so a field that disappears makes the view be sent FULL; an unchanged view with a new acknowledgement is `{}`; `viewpatch` tests and the end-to-end run)
- [ ] Revision continuity belongs to each viewer connection, not to the participant projection (2026-10-18: each player connection keeps the view and acknowledgement it last received and gets patches against them; no revision numbers yet, so a client that missed a message cannot detect it until resync exists)
- [ ] FULL projection + fresh baseline on: new transport attach, admin selects a player, reconnect, recovery
- [ ] Resync is a platform operation (FE or Runtime requests); sends FULL projection + current consumed SAMPLE ack (PLAYER) + fresh baseline
- [ ] After pod recovery the cache is rebuilt via project()
- [ ] Immediate flush on Command completion, important events, Start, Finish
- [x] Raw global state never reaches the browser (§30.19) (2026-10-18: connections only receive projections; the global state only goes to the mirror, the checkpoint and the replay capture)

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
- [ ] Replay Recorder receives immutable consistent captures at safe boundaries (never reads mutable state) (2026-10-18: the lane offers `replay.Capture` (state, Session time, revision, participants) after each committed boundary, never blocking; the recorder is a placeholder that drops it)
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
- [ ] Guest creation rate limiting at the API edge (pending task, 2026-10-17): `POST /v1/guests` needs no token, so anyone can create Users with it; it must be limited per client (for example per IP) before production
- [ ] Per-message-type rate limiting at the API edge (pending task, requested by the human 2026-10-17). Every live connection reads with one loop that hands each message to its type's own lane (`api/internal/live/lanes.go`: one goroutine per type, so SAMPLEs only wait for SAMPLEs, COMMANDs for COMMANDs, resyncs for resyncs). Handing off never blocks, so a lane's queue is unbounded: until this limit exists, a client sending faster than its lane is handled grows that queue without bound. The limit must be enforced per type before enqueueing, and bound each lane's queue.
- [x] Sandbox: CPU/execution budget, memory, deadline, state/output size, non-cooperative code protection (5 s budget per call by killing the worker, 64 MiB memory, 1 MiB stack, 256 KiB state/view, soft 160 MiB Go heap per worker; per instance `SANDBOX_MAX_CONTEXTS` (default 32) and `SANDBOX_MAX_CONCURRENT_EXECUTIONS` (default CPUs), Start answers `sandbox_unavailable` without capacity; tests for each)
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
- [ ] Start duration end-to-end and start-failure counts (incl. orphan checkpoints) (declared and recorded 2026-10-17: `play_runtime_session_start_total{outcome}`, `play_runtime_session_start_duration_seconds{outcome}`, `play_runtime_start_script_failures_total`, `play_session_start_commit_total{outcome}`, `play_session_start_aborted_total`; orphan checkpoints cannot happen; emission pending)

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
- [ ] Execution latency per entrypoint (start, samples, command, update, timer, project, ...) (declared and recorded 2026-10-18 for the existing operations: `play_runtime_sandbox_execution_duration_seconds{operation,outcome}` for load, start, restore and project; the rest comes with their entrypoints)
- [ ] Script errors, infra errors, retry counts and retry outcomes (flaky-script alert), resource-limit violations (outcomes `script_failed`/`budget_exceeded`/`error` in the execution histogram, `play_runtime_start_script_failures_total`, the flaky-start alert, and `play_runtime_sandbox_workers_stopped_total{reason}`; retries of running-game batches come with them)
- [ ] Context creates/recreates/evictions; state size and output size (creations `play_runtime_sandbox_context_creation_duration_seconds`, stops by reason, output size `play_runtime_sandbox_output_bytes{operation}`; recreations and evictions come with recovery)
- [x] Sandbox capacity and resources (2026-10-18, asked by the human): `play_runtime_sandbox_contexts_active`/`_contexts_limit`, `play_runtime_sandbox_capacity_rejections_total` (each `ErrNoCapacity`), `play_runtime_sandbox_executions_in_flight`/`_executions_limit`, `play_runtime_sandbox_execution_slot_wait_seconds`, `play_runtime_sandbox_execution_slot_abandoned_total`, per-worker memory `play_runtime_sandbox_worker_memory_bytes` (reported by each worker after every call) and the instance total `play_runtime_sandbox_workers_memory_total_bytes`; Start's `sandbox_unavailable` outcome is in `play_runtime_session_start_total`. Recorded in the sandbox package; emission pending like every metric (`TestHostMetrics`)
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

- [ ] Guest creation (`identity/usecases/createguest`, out of scope, see A4): `CreateGuest` returns the hardcoded test guest (token `test-guest`) for every call and stores nothing. Real behavior: create the guest User with its display name, idempotent per key, and issue a real credential.
- [ ] Identity authorization (`identity/usecases/authorize`, out of scope): recognizes only hardcoded test tokens (`test-user-1`, `test-user-2`, `test-guest`), each mapped to a fixed UserUUID; any other token is unauthenticated, and the guest is forbidden from `CREATE_SESSION` and `JOIN_SESSION_AS_ADMIN` (allowed `JOIN_SESSION_AS_PLAYER`, like every other operation). Must never be deployed as-is. Real behavior: validate the JWT, resolve the stable UserUUID, decide per operation; `CREATE_SESSION` and `JOIN_SESSION_AS_ADMIN` must require an account and forbid guests.
- [ ] Game playability check (`game/usecases/checkplayability`, out of scope): answers `true` for any Game and user. Real behavior: the Game exists, has a published version, and is visible to the user.
- [x] Session creation stand-in (`unwiredSessionCreation` in `unwired.go`, in scope): always failed. Replaced by `sessionlifecycle.Manager.CreateSession` on 2026-10-15; `unwired.go` was deleted.
- [ ] No flow creates `play_games` / `play_game_versions` (in scope, see B): nothing registers Games or marks a version EXECUTABLE yet, so every Session creation answers `game_unavailable` until those rows exist (outside production, test-data migrations seed one playable and one unavailable Game; see `play/CURRENT_STATE.md`). Real behavior: the operation that validates a version's scripts, freezes it, and sets it EXECUTABLE as the Game's current version.
- [ ] Live Session ownership (`play/runtime/usecases/ownership`, in scope, see A1): `LocateOrClaimOwner` answers `owned_here` with epoch 1 for every live Session, without keeping a registry (since 2026-10-18 it claims only live Sessions: `checklivesession` answers LOBBY before its expiry or RUNNING, anything else is `session_unavailable`, so no controller is created for it). Real behavior: in-process registry with release; shared registry + lease/heartbeat before multi-instance (SESSION-ADR-0029).
- [x] Admin join eligibility (`play/session/usecases/checkadminjoin`, in scope, see E): replaced by the real check on 2026-10-17 (the Session exists, the user created it, and it is RUNNING or in a LOBBY whose expiry has not passed).
- [ ] Admin operations (`play/runtime/workflows/liveconnection`, in scope, see E): `SelectPerspective` and `CancelSession` return "not implemented", and `NextView` never produces a view (lobby updates are real since 2026-10-17; `StartSession` is orchestrated since 2026-10-17, see G). Real behavior: perspective selection with FULL projection, host cancellation.
- [x] Durable side of Start (`unwiredSessionStart` in `unwired.go`, in scope, see G): replaced on 2026-10-17 by `sessionlifecycle` (`CheckStartEligibility`, `CommitStart`, `AbortStart`) and `frontendaccess.FrontendAccess`; the stand-in was deleted. It used to: always fail so every `start_session` with a valid lobby fails the call and the API closes the admin connection with 1011. Real behavior: a `play/session` use case validating Start, signing a short-lived URL to the pinned version's frontend package in object storage (needs the frontend artifact locator on the game version and its identity in the manifest, see B), storing the checkpoint durably, committing roster + checkpoint reference + RUNNING in one transaction, and marking ABORTED / GAME_START_FAILURE.
- [ ] Object storage for deployed environments (`play/session/storage.Open`, in scope, see B and G): locally the `gcs` service of `docker-compose` (fake-gcs-server) is configured since 2026-10-18 by environment variables (signing under an emulator uses a throwaway key, since the emulator has no credentials and checks no signature; `TestGCSSignedURLServesTheObject`); without `GCS_PROJECT_ID` / `GCS_BUCKET` every storage call still fails. Real behavior: each deployed environment's bucket and a Cloud Run identity allowed to sign through IAM.
- [x] Sandbox execution contexts (`unwiredSandbox` in `unwired.go`, in scope, see G and M): replaced on 2026-10-17 by `sandbox.Host`; `unwired.go` was deleted.
- [ ] Sandbox contexts of running games (`play/runtime/sandbox`, in scope, see H and M): never discarded, since controllers are never destroyed; they live until the process exits. Real behavior: discarded when the Session ends or its controller is retired.
- [x] Player admission (`unwiredPlayerAdmission` in `unwired.go`, in scope, see F): replaced on 2026-10-17 by `liveconnection.Manager.AttachPlayer`; `unwired.go` was deleted.
- [ ] Player operations of a running game (`play/runtime/workflows/liveconnection`, in scope, see I, J, M): the controller has a phase (lobby, running; Start will move it to running) and lobby behavior only applies in the lobby: there `Sample` is dropped, `Resync` does nothing, `NextView` never produces a view and `Command` answers `session_not_running`. In a running game, `Leave` and `Command` answer "not implemented" (`Leave` also during a Start attempt), `Disconnect` only ends the connection (no transport grace yet), and new players are rejected as `session_started`; lobby updates are never sent once running.
- [x] Join code resolution (`unwiredJoinCodes` in `unwired.go`, in scope, see A3): replaced on 2026-10-17 by `play/session/usecases/resolvejoincode` (the Session of an active, unexpired join code; `not_found` otherwise); the stand-in was deleted.
- [ ] Controller lifetime (`play/runtime/workflows/liveconnection`, in scope, see H): controllers are created but never destroyed or released, so they accumulate for the process lifetime. Real behavior: destroyed on terminal states with owner release.
- [ ] Forwarding to a remote owner (`api/play/owner_routing.go`, in scope, see A1): a Session owned by another instance is answered `503`; unreachable while single-instance. Real behavior: proxy the connection to the owner (gateway), decided with the multi-instance topology.
- [ ] Replay recorder (`unwiredReplay` in `unwired.go`, in scope, see R): every capture a running game offers is dropped. Real behavior: the Replay Recorder building chunks (FULL_STATE, STATE_DELTA, events, command responses) and persisting them asynchronously, bounded and best effort.
- [ ] Running games never stop by themselves (`play/runtime/workflows/liveconnection`, in scope, see S and U): a controller keeps ticking until its Session is aborted, it is retired or the process stops, even with every player gone, so its Sandbox worker stays alive too. Real behavior: inactivity, maximum lifetime and finishing end the Session, its lane, controller and context.
- [ ] View delivery has no backpressure (`play/runtime/workflows/liveconnection`, in scope, see W): a slow connection's queued views grow without bound. Real behavior: coalesce into one FULL view when a connection falls behind.
- [ ] Metrics emission (`monitoring/metrics`, `record`): measurements are only written to the request log; nothing is sent to a metrics backend (see Y).

## Invariants verification (§30)

Each needs at least one test or other concrete evidence before ticking.

- [ ] 1. One Session, one live owner
- [x] 2. One authoritative timeline; per-player buffers are never authority (2026-10-18: controller and boundary tests in `liveconnection`)
- [ ] 3. RUNNING implies recoverable initial state
- [ ] 4. Lobby membership is ephemeral; durable participants only at Start
- [ ] 5. ParticipantID is the script's identity; userUUID stays platform-side
- [ ] 6. No late new players after Start
- [ ] 7. Transport recovery is not game rejoin
- [x] 8. Disconnect resets inputs immediately (2026-10-18: a disconnect enqueues the default inputs at once; controller tests)
- [ ] 9. Explicit leave is permanent
- [ ] 10. SAMPLE sequence vs COMMAND token identity rules
- [x] 11. Distinct SAMPLE transitions preserved; refreshes deduped (2026-10-18: controller tests)
- [ ] 12. COMMAND logical rejection still consumes its token
- [x] 13. Game time is not wall time (2026-10-18: scripts see only effective Session time)
- [ ] 14. update() is frequent, not exact
- [ ] 15. Timers belong to Runtime, checkpointed, game-time based
- [x] 16. Sandbox has no external side effects; ambiguous calls discarded and retried from the mirror (2026-10-18: boundary tests in `liveconnection`)
- [x] 17. Failed execution has no partial output (2026-10-18: boundary tests in `liveconnection`)
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
