# Session Runtime - Flows

Status: CURRENT IMPLEMENTATION

## Create Session

`sessionlifecycle.Manager.CreateSession(userUUID, gameUUID, idempotencyKey)`, called by the play-lifecycle orchestration once Game Management said the Game is playable (see `docs/architecture/CROSS_DOMAIN_FLOWS.md`).

```mermaid
sequenceDiagram
    participant Caller as playlifecycle.Manager
    participant Manager as sessionlifecycle.Manager
    participant Repo as sessionlifecycle internal/repo
    participant DB as play_* tables

    Caller->>Manager: CreateSession(userUUID, gameUUID, key)
    Manager->>Manager: validate and normalize UUIDs, require key
    Note over Manager,DB: one transaction
    Manager->>Repo: ClaimCreateSessionRequest(user, key, game)
    Repo->>DB: INSERT play_create_session_requests ON CONFLICT DO NOTHING
    alt key already claimed
        Repo-->>Manager: existing request
        Manager-->>Caller: same Game: replay stored outcome / other Game: ErrIdempotencyConflict
    else fresh claim
        Manager->>Repo: FindCurrentGameVersion(game)
        alt no Game or no current version
            Manager->>Repo: CompleteCreateSessionRequest(game_unavailable)
            Manager-->>Caller: game_unavailable
        else current version not EXECUTABLE
            Manager-->>Caller: error + alert (broken data), transaction rolls back
        else EXECUTABLE version
            Manager->>Repo: CreateSession(version, user, LOBBY, lobbyExpiresAt)
            loop until a free code (max 10 candidates)
                Manager->>Repo: CreateJoinCode(code) / ReleaseExpiredJoinCode(code)
            end
            Manager->>Repo: CompleteCreateSessionRequest(created, session)
            Manager-->>Caller: created: sessionUUID, joinCode, expiresAt
        end
    end
```

Implemented behavior:

- Idempotent per `(userUUID, idempotencyKey)`. The claim, the Session, its join code and the recorded outcome commit together, so a committed request always has an outcome; two concurrent identical requests produce one Session and the same answer.
- A retry with the same key and Game replays the first answer, including `game_unavailable` even if the Game became playable since. The same key with another Game is `ErrIdempotencyConflict`.
- The Session pins the Game's current version, which must be `EXECUTABLE` (immutable, scripts ready to run). A current version in any other state is treated as broken data: alert, error, nothing persisted.
- The Session starts in `LOBBY` with a 30-minute lobby; its join code expires with the lobby. Join codes are 6 characters from an alphabet without look-alike characters; a code held by an active join code is skipped, one held by an expired join code is released and reused.
- No Runtime controller and no participant are created; the creator is not a player.
- Counts `play_session_create_total{outcome=created|game_unavailable|replayed}` once the transaction committed.

## Live Session Check

`checklivesession.Service.IsSessionLive(sessionUUID)`, called by the Runtime's ownership (`LocateOrClaimOwner`) before every live upgrade, so that only a live Session is ever claimed and gets a controller. It only reads `play_sessions`: the Session is live when it is RUNNING, or in LOBBY before `lobby_expires_at`. An unknown Session (or not a UUID), an ended one, or an overdue lobby is not live (the expiry is not recorded here); the API answers 404.

## Admin Join Eligibility

`checkadminjoin.Service.EligibleToJoinAsAdmin(sessionUUID, userUUID)`, called by the live Runtime before it attaches an admin connection (see `docs/architecture/CROSS_DOMAIN_FLOWS.md`, Admin Join). It only reads `play_sessions`.

| Session | Answer |
| --- | --- |
| No Session with that UUID (or not a UUID) | `session_not_found` |
| Created by another user | `not_creator` (whatever its state) |
| RUNNING | `eligible` |
| LOBBY with `lobby_expires_at` still in the future | `eligible` |
| LOBBY past `lobby_expires_at` | `session_not_joinable`; the expiry is not recorded here |
| FINISHED, CANCELLED, EXPIRED, ABORTED | `session_not_joinable` |

## Join Code Resolution

`resolvejoincode.Service.ResolveJoinCode(joinCode)`, called by the API before owner routing when a player joins by join code (`GET /join/{joinCode}`). It only reads `play_session_join_codes` joined with `play_sessions`. The code is matched in upper case, so codes typed in lower case resolve too.

| Join code | Answer |
| --- | --- |
| Unreleased row holding the code, `expires_at` still in the future | `found`, with that Session's UUID |
| Unreleased row past `expires_at` | `not_found`; the code is not released here |
| Only released rows, or no row | `not_found` |

The Session's state is not checked: a code expires with its lobby, and is released when the Session starts or its start is aborted, so a started Session's code resolves to `not_found`.

## Player Join Eligibility

`checkplayerjoin.Service.EligibleToJoinAsPlayer(sessionUUID)`, called by the live Runtime before it admits a player into the lobby. It only reads `play_sessions`; any user may join, the creator included.

| Session | Answer |
| --- | --- |
| No Session with that UUID (or not a UUID) | `session_not_found` |
| LOBBY with `lobby_expires_at` still in the future | `eligible` |
| LOBBY past `lobby_expires_at` | `session_not_joinable`; the expiry is not recorded here |
| RUNNING | `session_started` |
| FINISHED, CANCELLED, EXPIRED, ABORTED | `session_not_joinable` |

The lobby's capacity is not checked here: the live controller checks it against the manifest's maximum while admitting.

## Runtime Manifest Load

`loadruntimemanifest.Service.LoadRuntimeManifest(sessionUUID)`, called by a Session's live controller on its first admitted connection, which keeps the manifest for the Session's lifetime (it cannot change: the version is pinned and immutable). It reads `play_sessions` joined with `play_game_versions`. The manifest declares the player limits, the SAMPLE inputs and the COMMANDs, each command with payload fields declared like inputs (`play/internal/business/commands`). An unknown Session is `ErrSessionNotFound`; a pinned version that is missing or holds an invalid manifest raises an alert and is an error.

## Start Session (durable side)

Called by the live Runtime while it starts a Session's game (the Session's controller in `play/runtime/workflows/liveconnection`, `startSession`, on its main goroutine): first the eligibility check, then the frontend access, then, once the game's `start()` and first projections succeeded, either the commit or, after a second script failure, the abort.

### Start Eligibility

`sessionlifecycle.Manager.CheckStartEligibility(sessionUUID, userUUID, playerCount)`. It reads `play_sessions` joined with `play_game_versions`, then loads the version's backend script from object storage, verified against its recorded hash and size (and cached by hash).

| Session | Answer |
| --- | --- |
| No Session with that UUID (or not a UUID) | `session_not_found` |
| Created by another user | `not_creator` |
| RUNNING | `already_started` |
| FINISHED, CANCELLED, EXPIRED, ABORTED, or LOBBY past `lobby_expires_at` | `not_in_lobby`; the expiry is not recorded here |
| `playerCount` outside the manifest's `players.min..max` | `invalid_player_count` |
| Otherwise | `eligible`, with the version's UUID and script |

A pinned version that is not EXECUTABLE, has an invalid manifest or no backend script locator raises an alert and is an error (broken data). A script that cannot be loaded (missing, hash mismatch, storage unavailable) is an error. `game_unavailable` is declared but never answered: no business rule makes a pinned version unplayable yet.

### Frontend Access

`frontendaccess.Service.FrontendAccess(gameVersionUUID)` reads the version's frontend package locator from `play_game_versions` and signs a read URL to it in object storage, valid for 15 minutes, returned with the package's SHA-256 and its viewport: the version's aspect ratio and that ratio's logical resolution. An unknown or non-EXECUTABLE version is an error; an EXECUTABLE one without a frontend package, or without an aspect ratio Playhoot offers, raises an alert and is an error.

### Commit Start

`sessionlifecycle.Manager.CommitStart(sessionUUID, userUUID, commit)`, in one transaction (each participant is stored with the display name it had when the game started, a snapshot Play keeps; Identity stays the owner of names):

```mermaid
sequenceDiagram
    participant Runtime as Session controller (liveconnection)
    participant Manager as sessionlifecycle.Manager
    participant DB as play_* tables

    Runtime->>Manager: CommitStart(session, user, roster + checkpoint)
    Manager->>Manager: validate roster (users and ParticipantIDs unique) and checkpoint
    Note over Manager,DB: one transaction
    Manager->>DB: SELECT play_sessions ... FOR UPDATE
    alt no Session
        Manager-->>Runtime: ErrSessionNotFound
    else not the creator
        Manager-->>Runtime: error
    else not LOBBY, or lobby expired
        Manager-->>Runtime: not_in_lobby (nothing written)
    else
        Manager->>DB: INSERT play_session_participants (one row per participant)
        Manager->>DB: UPSERT play_session_checkpoints (latest only)
        Manager->>DB: UPDATE play_sessions SET status = RUNNING WHERE status = LOBBY
        Manager->>DB: INSERT play_session_status_changes (LOBBY -> RUNNING, by the creator)
        Manager->>DB: UPDATE play_session_join_codes SET released_at
        Manager-->>Runtime: committed
    end
```

Because the checkpoint is written in the same transaction, a RUNNING Session always has a recoverable initial state and no orphan checkpoint can exist. Any failure rolls everything back.

A start whose script fails twice ends the Session through End Session with `GAME_START_FAILURE` (LOBBY -> ABORTED).

## End Session

`sessionlifecycle.Manager.EndSession(sessionUUID, {cause, changedBy})` is the only way a live Session reaches a terminal status. Each cause leads to one status from given statuses (`play/session/internal/business/endcause`):

| Cause | Status | From |
| --- | --- | --- |
| `GAME_FINISHED` | FINISHED | RUNNING |
| `HOST_CANCELLED` | CANCELLED | LOBBY, RUNNING |
| `ALL_PLAYERS_LEFT` | CANCELLED | RUNNING |
| `PLATFORM_TERMINATED` | CANCELLED | LOBBY, RUNNING |
| `LOBBY_EXPIRED` | EXPIRED | LOBBY |
| `INACTIVITY_TIMEOUT` | EXPIRED | RUNNING |
| `MAX_LIFETIME` | EXPIRED | LOBBY, RUNNING |
| `GAME_START_FAILURE` | ABORTED | LOBBY |
| `SCRIPT_FAILURE`, `UNRECOVERABLE_INFRA_FAILURE`, `OWNER_LOST` | ABORTED | RUNNING |

In one transaction it locks the Session; in a status the cause applies to, it changes the status with the cause (by `changedBy`, the creator for `HOST_CANCELLED`, otherwise the platform) and releases its join codes, answering `ended` with the new status and cause. Otherwise it changes nothing and answers `not_ended` with the Session's current status and end cause, so a caller learns how a Session that already ended did. An unknown Session is `ErrSessionNotFound`, an unknown cause an error. Metric `play_session_ended_total{cause}`.

### Owner Heartbeat

`sessionlifecycle.Manager.RecordOwnerHeartbeat(sessionUUID)`, called by the controller running a game as soon as it runs on its instance and then every `sessionpkg.OwnerHeartbeatInterval` (30 s): in one transaction it locks the Session and, while it is RUNNING, renews its owner lease, setting `owner_lease_expires_at` to now + 10 min (`recorded`); otherwise it changes nothing and answers `session_not_running` with the Session's status and end cause, and the controller ends the game live with them. Commit Start grants the first lease in its own transaction. A Session whose lease ran out has no live owner, and nobody came back to recover it in that time. Metric `play_session_owner_heartbeats_total{outcome}`.

### Reconciliation

A background job outside the lifecycle workflow, in its own package with its own persistence (`play/session/batches/reconcilesessions`): `Service.Run`, started by `main` on every instance, runs `ReconcileSessions` every 5 minutes. It only reads Sessions past one of their deadlines, so every Session it reads must end; it never stores a cursor:

| Session, by deadline (one partial index each, holding only Sessions in that status) | Ends as, decided under the row lock by `internal/business/reconciliation.CauseFor` |
| --- | --- |
| LOBBY past `lobby_expires_at` | EXPIRED / `LOBBY_EXPIRED` |
| RUNNING past `expires_at` | finalized replay: FINISHED / `GAME_FINISHED`; otherwise EXPIRED / `MAX_LIFETIME` |
| RUNNING past `owner_lease_expires_at` | finalized replay: FINISHED / `GAME_FINISHED` (its owner died between storing the end and committing it); otherwise ABORTED / `OWNER_LOST` |

A pass fixes the time it began and reads the due Sessions 100 at a time, the longest overdue first (`FindDue`: one range scan per index, deduplicated). Each is judged again in its own transaction under `FOR UPDATE`, waiting at most 5 s (`lock_timeout`) for a transaction holding it, since all of them are short (heartbeat, checkpoint, End Session). A Session that changed since it was read (its lease renewed, its lobby started, ended elsewhere) is left as it is and is no longer due. A Session that must end is ended by the platform in that same transaction, by the job's own repository: status and end cause, a `play_session_status_changes` row (by the platform), join codes released, as End Session does. Each page ends what it reads, so the pass reads again until a page ends nothing; a Session it failed on (a lock it waited too long for, a database error) is tried once per pass, logged and left for the next one. Metrics `play_session_reconciled_total{cause}` (not counted in `play_session_ended_total`, which counts End Session's endings), `play_session_reconciliation_duration_seconds`, `play_session_reconciliation_failures_total`.

### Save Checkpoint

`sessionlifecycle.Manager.SaveCheckpoint(sessionUUID, checkpoint)`, called by a running game's controller every few seconds: in one transaction it locks the Session and, while it is RUNNING, makes the checkpoint its latest unless one of a higher context revision is stored (`saved`); otherwise it stores nothing (`session_not_running`). An unknown Session is `ErrSessionNotFound`.

### Load Session Limits

`play/session/usecases/loadsessionlimits` (`LoadSessionLimits`) returns `lobby_expires_at` and `expires_at` (set at creation: 30 minutes and 4 hours later). An unknown Session is `ErrSessionNotFound`.

## Session Status History

Every change of `play_sessions.status` appends a row to `play_session_status_changes` in the same transaction: the creation (`CREATED -> LOBBY`, by the creator), the start (`LOBBY -> RUNNING`, by the creator) and every end through End Session, with its cause and who caused it. Status changes only happen through `sessionlifecycle`'s repository, which writes both, and a change is only applied when the Session is still in the status it moves from.

### Load Running Session

`sessionlifecycle.Manager.LoadRunningSession(sessionUUID)` returns what running a started Session again needs: the pinned version's UUID and backend script (loaded from object storage, verified), the roster from `play_session_participants` in the order it was recorded (with each participant's display name), and the latest `play_session_checkpoints` row. An unknown Session is `ErrSessionNotFound`, one in another status `ErrSessionNotRunning`; a RUNNING Session without checkpoint, participants or script locator raises an alert and is an error. The live Runtime calls it when a start finds the Session already RUNNING, and recovery will reuse it.

## Replay Recording

Design: `play/docs/decisions/SESSION-ADR-0033-replay-recording-produces-immutable-chunks.md`.

1. When a controller starts, adopts or recovers a running game, it opens a recording (`replayrecording.Recorder.StartRecording`) and offers the starting state as its first point.
2. After every committed boundary, the controller offers a `replay.Capture` (state, Session time, revision, participants, COMMAND responses, logical errors, events) without blocking. A recording's goroutine takes it from its bounded inbox (a full inbox drops the tick) and adds it to its active chunk. The first point of a chunk is the full state. Every `REPLAY_STATE_SAMPLE_EVERY_N_TICKS`-th tick adds a JSON Patch from the last state it kept. Transients are always added, in timeline order, with their targets.
3. A tick at or past the chunk's end (a multiple of `REPLAY_CHUNK_DURATION` in Session time), or a chunk over 16 MiB, seals the chunk. The tick's transients stay in the old chunk and its state starts the next one. The sealed chunk is encoded (versioned JSON, zstd) and added to the instance's store queue. Recording goes on at once.
4. The queue's workers store each chunk: `play/session/usecases/storereplaychunk` writes the object `replays/{session}/{recording}/{index:06d}.rpl`, then registers it in `play_replay_chunks`, creating the Session's `play_replays` row with its pinned version on the first chunk. A failure is retried up to 5 times with a growing wait, then the chunk is dropped. A full queue (`REPLAY_MAX_PENDING_CHUNKS`) drops its oldest chunk. Drops are counted and alerted, and the game never waits.
5. A game its script finished: before End Session, the controller calls `Finish` with the final tick. The tick closes the last chunk as final (its transients and state included, even past the chunk's end), which is stored synchronously and finalizes `play_replays`. A failure fails the call, and the controller retries at the next tick, storing the same chunk. Only then does the Session become FINISHED.
6. When the controller ends (any other ending, or the instance shutting down), it stops its recording. What the recording holds is sealed and queued, best effort. On shutdown, the recorder waits up to 10 s for the queue to drain, after the controllers stop.

## Game Fault Reports

Added 2026-10-20 at the human's request, with the frontend's game isolation hardening (frontend ADR-0006): a player's client that stops the game frontend because it failed or misbehaved tells the backend, which keeps it for auditing.

1. The client sends `{"type":"game_fault","kind":...,"detail":{...}}` on its player connection (`api/play/player_join.go`), and usually `leave` right after (frontend ADR-0006: a game stopped for misbehaving makes its player leave). Both share one lane (`exit`: 2 a second, 20 a minute, 3 waiting; more are dropped), so the report is handled, and stored, before the leave. An unknown kind, or a detail that is not a JSON object within 2 KiB, closes the connection as malformed (`1007`). It is never answered.
2. The Runtime (`liveconnection`, `reportGameFault`, on the lane's goroutine, never the main one) drops it unless the connection is the current one of a participant of the running game, maps the kind to the durable one and hands it to `play/session/usecases/recordgamefault`.
3. `recordgamefault` checks the kind and the detail again and writes `play_game_faults` in one statement that requires the Session RUNNING and the reporter to be that participant; a second report of the same kind by the same participant in the Session adds to `occurrences`. A storing failure is only logged: the report is evidence, not an operation the client waits for.
4. Every report is counted in `play_runtime_game_faults_reported_total{kind,outcome}` (`recorded`, `not_recorded`, `dropped`, `failed`). Nothing else happens: the game, the Session and the other players are untouched.

## Limit Audit

Added 2026-10-21 at the human's request: every platform limit a live Session's user hits is also persisted, best effort, next to the game faults clients report.

1. Where a limit refuses something a user of a live Session sent, it is told who and which limit: the API's per-type lanes and per-connection rates (`api/internal/live`, through `LaneLimits.Refused` / `ConnectionLimits.Refused`, then the connection's `LimitHit`) and the Runtime's limits (`liveconnection`: SAMPLE and COMMAND limits per participant and per Session, the admin connection cap, and views collapsed into a FULL view for a slow connection).
2. `play/runtime/usecases/limitaudit.Auditor` counts each Session, user and limit in memory, never blocking the caller (at most 10,000 distinct ones between writes; more are dropped and counted). The resolver runs it: every 5 s, and once more when the instance stops, it hands the counts to `play/session/usecases/recordlimithits`.
3. `recordlimithits` adds them, in one transaction, to each Session and user's `SERVER_LIMIT` record in `play_game_faults`, summing each limit's count in `detail.limits`. A failed write is logged and the batch dropped. Counted in `play_runtime_limit_hits_audited_total{outcome}`.
4. Nothing else changes: each limit still answers as before (dropped, `rate_limited`, closed `1008`), and its metric still counts it. Limits outside a Session (HTTP, handshakes, connection caps) stay metrics only.
