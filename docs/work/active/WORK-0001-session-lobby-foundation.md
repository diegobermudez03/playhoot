# WORK-0001: Session Lobby Foundation

Status: READY
Created: 2026-09-08
Last status change: 2026-09-08

Related decisions:
- `game/docs/decisions/GAME-ADR-0001-game-capability-persistence-transaction-boundary.md`
- `game/docs/decisions/GAME-ADR-0002-session-runtime-durable-boundary.md`
- `game/docs/decisions/GAME-ADR-0003-session-runtime-actor-and-lifecycle-foundations.md`
- `game/docs/decisions/GAME-ADR-0004-session-lobby-lifecycle-contract.md`
- `game/docs/decisions/GAME-ADR-0005-session-public-and-internal-identity-boundary.md`
- `game/docs/decisions/GAME-ADR-0018-session-running-mutation-serialization.md` (reused design for the locking primitive; RUNNING itself is out of scope here)
- `docs/decisions/architecture/ADR-0005-cross-domain-public-entity-references.md`
- `identity/docs/decisions/IDENTITY-ADR-0001-identity-user-public-identity-boundary.md`

Canonical context:
- `game/README.md` (Session Runtime Lobby Lifecycle Contract, Session Runtime Actor and Lifecycle Model, Capability Persistence and Transaction Boundary sections)
- `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` (Lobby And Identity Tables)
- `game/docs/DATA_MODEL.md` and `game/CURRENT_STATE.md` (current implementation reality, superseded by this work)
- `docs/engineering/standards/cross-domain-reference-naming.md`, `repositories.md`, `error-handling.md`, `data-integrity.md`, `testing.md`, `domain-logic-placement.md`
- `docs/ai/workspaces/active/session-runtime-v1/PLAN.md` (Slice 1 of the approved initiative sequence)

## Outcome

A Session can be created, joined, left, and reconstructed from durable storage using the accepted Session/Actor/Participant/lobby identity model and per-Session DB-locking transactional rules. This replaces the current scaffolding, which does not compile and directly contradicts accepted architecture. It is the durable lobby foundation every later Session Runtime capability (Start/RuntimeTurn, interaction processing, timers, disconnect/reconnect, the live Coordinator) depends on.

No Game execution begins in this work. No WebSocket/runtime execution is implemented in this work. A completed WORK-0001 ends with a correct durable LOBBY, not a running game.

## Context

`session-runtime-v1`'s Architecture Discussion is CLOSED (human-approved); the initiative is in Implementation Planning with an approved 10-slice sequence (`PLAN.md`). This is Slice 1.

Actual codebase inspection (2026-09-08) confirms the drift already tracked in `PLAN.md`:

- `game/session/workflows/sessionlifecycle/internal/repo/step_create_room.go` declares `func (r *Repo) CreateRoom(ctx context.Context)` with **no function body** - the `game/session/...` tree does not currently compile.
- `game/session/workflows/sessionlifecycle/step_create_room.go`'s public `CreateRoom(ctx, program engine.Program, gameVersionUUID string, ownerUUID string)` accepts an externally-supplied, already-compiled `engine.Program` - GAME-ADR-0004 requires Session Runtime itself to resolve/pin/compile the definition instead.
- `game/session/workflows/sessionlifecycle/step_join_room.go`'s `JoinRoom(ctx, playerUUID string, sessionCode uint) error` is a no-op stub.
- The current schema (`game/session/internal/storage/tables.go`, migrated by `game/session/internal/storage/migrations/20260817000000_session_states.go` and `20260817000001_sessions.go`) models `sessions` (with raw `OwnerUUID string`), `sessionState`, `sessionPlayer` (raw `PlayerUUID string`), and `joinCode` - none of the accepted `session_actors`/`session_participants`/`session_requests` tables exist, and `sessions` has none of the accepted `phase`/`lobby_expires_at`/`host_actor_id` columns.
- `game/game/usecases/getgame.UseCase.GetPlayableGameWithCurrentVersion(ctx, gameUUID) (*game.Game, error)` **already exists, is tested, and is sufficient for Create**: it returns a `game.Game` with a decoded `program.Definition` and `VersionUUID` for the game's *current* playable version. It is **not** sufficient for Join/Leave or any later operation against an already-created Session, because it always resolves through `games.current_definition_id` - the current version, not the version a Session was pinned to at Create time. **A minimum new Game Management read capability is required**: load an immutable Game Definition by its own public Definition/Version UUID (not by Game UUID + "current"), so a Session can keep reading the exact definition it was pinned to even after the game's author publishes a newer version. See Game Management Dependency below.
- `game/language/v1/engine/engineservice.Compile(def program.Definition) (engine.Program, Diagnostics)` already exists and is sufficient to validate a definition at Create time.
- `utils.RunInDBTransaction[T](ctx, dbServicer, callback)` (in `utils/db_transactions.go`) already exists as the repository's one generic transaction helper (`sql.LevelRepeatableRead` isolation) and is not yet used anywhere - this work is expected to be its first real consumer.
- No Identity/Auth implementation exists anywhere (`identity/CURRENT_STATE.md`); per the approved boundary below, this work does not attempt to add one.

## Scope

### In Scope

- A `CreateSession` application/domain operation: public Game identity, authenticated/trusted host `UserUUID`, idempotency key -> a `LOBBY` Session pinned to a resolved/compiled Game Definition, with a host `SessionActor`, an active `JoinCode`, and `lobby_expires_at` set. Host is not automatically a Participant.
- A `JoinSession` application/domain operation: JoinCode, trusted `UserUUID`, display-name snapshot, idempotency key -> an active Participant, reusing an existing `SessionActor`/`Participant` where one already exists for that `(session, user_uuid)`.
- A `LeaveSession` application/domain operation: Session UUID, trusted `UserUUID`, idempotency key -> Participant deactivated, slot released, `SessionActor` retained, host authority retained if the leaving user is also host.
- Lazy `lobby_expires_at` materialization to `TERMINAL` performed by the above operations when they discover an expired deadline - no standalone sweeper/reaper worker.
- Migrations/schema for the Slice-1-relevant subset of the accepted persistence model: `sessions`, `session_actors`, `session_participants`, `join_codes`, `session_requests`.
- A per-Session database-locking serialization mechanism for LOBBY mutations (Join, Leave, lazy expiration), designed so the identical primitive is reusable for RUNNING serialization in Slice 2+ (GAME-ADR-0018) rather than a lobby-only abstraction that would need replacing.
- Idempotency for `CREATE`/`JOIN`/`LEAVE` via `session_requests`, keyed by the logical caller/operation namespace (see Idempotency Design).
- Consuming the existing Game Management `getgame` read capability at Create time to resolve the current playable Game Definition to pin; adding a **minimum new Game Management read capability** that loads an already-pinned immutable Game Definition by its own Definition/Version UUID, consumed by Join (and reusable by Slice 2's Start).
- Repository integration tests (disposable real Postgres DB) and service/use-case tests (mocked repository), per repository testing conventions, including concurrency tests proving the DB-locking mechanism and concurrent-Create idempotency.

### Out of Scope

Explicitly not part of this work (later slices in `PLAN.md`):

- Start, engine initialization, the `players: list<user>` root roster, `RuntimeTurn`/`RuntimeStep`/`session_runtime_state`.
- `session_interactions`, `session_timer_obligations`, keyed timers.
- The Live Session Coordinator, WebSockets, or any transport/connection code.
- Disconnect/reconnect, `session_actors.semantic_presence` beyond its Slice-1 default (see below), resync.
- Process-loss recovery mechanics beyond ordinary transaction atomicity already provided by Postgres.
- `sessions.activity_expires_at` / inactivity expiration / Session Reaper.
- `session_runtime_failures` / runtime failure diagnostics / terminal-cleanup atomicity for RUNNING causes.
- Long-term archival (`session_history_archives`).
- Identity/Auth implementation of any kind (see Approved Design below).
- Public HTTP/WebSocket transport DTOs or endpoints, unless an existing current API must be adapted to keep the repository compiling/tests coherent (none identified during inspection - `game/session/api.go` only exports a `Room` struct with no HTTP wiring anywhere in the repository).
- Any Game Management change beyond the one minimum read capability described in Game Management Dependency below - no publish/edit/lifecycle behavior, no redesign of existing Game Management ownership, no new persisted fields there.

## Approved Design

### Identity/Auth Boundary

Session application/domain APIs receive an already-trusted `UserUUID`, per GAME-ADR-0005. Credential-to-`UserUUID` resolution belongs outside Session Runtime and is integrated later when a real transport/auth boundary exists (a later slice). This work does not implement Identity/Auth and does not add a temporary fake-auth mechanism that would later need removal; test and internal callers supply `UserUUID` directly, exactly as the accepted public contract already expects.

### Persistence Model (Slice 1 subset of `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md`)

- **`sessions`**: `id`, `uuid`, `game_definition_uuid` (logical reference, no FK - the *pinned, immutable* Version/Definition UUID resolved exactly once at Create; see Pinned Game Definition below), `host_actor_id` (references `session_actors.id`; **nullable at storage level only** - see Host/SessionActor Creation Cycle below), `phase`, `lobby_expires_at`, `started_at` (nullable, always `NULL` after this work - no Session created here ever reaches `RUNNING`), `terminal_at` (nullable), `terminal_reason` (nullable), `created_at`, `updated_at`. `phase` needs only `LOBBY`/`TERMINAL` values for this work, but its representation should not hard-code an exhaustive enum that later slices (`RUNNING`) would have to migrate around.
- **`session_actors`**: `id`, `session_id`, `user_uuid` (logical reference, no FK), `semantic_presence`, `created_at`; unique `(session_id, user_uuid)`. A successful normal Join begins `semantic_presence = CONNECTED` (GAME-ADR-0015's accepted default) with no physical connection tracking of any kind.
- **`session_participants`**: `id`, `session_actor_id`, `display_name`, `active`, `joined_at`, `left_at` (nullable).
- **`join_codes`**: `id`, `session_id`, `code`, `created_at`, `revoked_at` (nullable) - authoritative Session persistence, not cache-only; one active code per lobby; revoked (not deleted) when the lobby is no longer admissible; historical/revoked codes retained.
- **`session_requests`**: `id`, `operation`, `idempotency_key`, `user_uuid`, `session_id` (nullable - a `CREATE` request may not yet have a resulting session at the moment of dedup lookup for a rejected/failed attempt, though a successful `CREATE` should populate it), `request_payload`, `outcome`, `response_payload`, `status` (distinguishes a completed logical outcome from an in-progress claim - see Concurrent Create Correctness and Completed-Outcome-vs-Transient-Failure below; exact value set is implementation freedom, e.g. `PENDING`/`COMPLETED`, or an equivalent that the DB-level uniqueness/claim mechanism can key off of), `created_at`. Unique `(user_uuid, operation, idempotency_key)` - **not** `(operation, idempotency_key)` alone, since two different users may independently produce the same opaque idempotency key with no collision; the same `(user_uuid, operation, idempotency_key)` identifies one logical operation. Only `CREATE`/`JOIN`/`LEAVE` operations are handled here; `START` support is explicitly deferred to Slice 2.

Cross-domain references (`game_definition_uuid`, `user_uuid`) remain logical only - no database foreign keys into another bounded context, no direct queries against Game Management or Identity tables from Session persistence code (GAME-ADR-0001, `cross-domain-reference-naming.md`).

### Pinned Game Definition Is Immutable For The Session

This is a correctness requirement, not an optimization. `CreateSession` resolves the requested public Game UUID to its **current** playable immutable Definition/Version UUID exactly once, and persists that resolved UUID as `sessions.game_definition_uuid`. From that moment on, every operation for that Session - Join in this work, Start/RuntimeTurn in later slices - reads and enforces *that pinned definition*, never the game's current version. A later publication of a new version (`games.current_definition_id` changing) must not alter lobby capacity, Start behavior, the Game Language definition, or any other semantics of an already-created Session.

Worked example: Create Session S while the Game's current version is V1 - S pins `game_definition_uuid = V1`. The author later publishes V2. Join against S must still enforce `V1.players.max`, never `V2.players.max`.

This is why a second Game Management read capability is required (see Game Management Dependency below): the existing `getgame.GetPlayableGameWithCurrentVersion` is Create-only - it is architecturally wrong to call it again after Create, because it always resolves through `games.current_definition_id`, which is exactly the value that must *not* influence an already-created Session.

### Database Locking / Serialization Design

- **Lock anchor**: the `sessions` row itself, selected `FOR UPDATE` by `id` inside the mutation's transaction.
- **Transaction boundary**: reuse `utils.RunInDBTransaction` (existing generic helper; `sql.LevelRepeatableRead`), the same mechanism already used elsewhere in the repository, rather than inventing a second transaction helper.
- **Responsible method**: a single narrow repository operation (naming left to implementation, e.g. `lockSessionForMutation(ctx, tx, sessionUUID) (sessionRow, error)`) that selects-for-update the `sessions` row and returns its current `phase`/`lobby_expires_at`. `Join`, `Leave`, and lazy-expiration materialization all call this same operation before mutating - they must not each invent their own locking query. `Create` does not use this operation, because it has no existing `sessions` row to contend on - its own concurrency correctness comes from the idempotency-identity claim described in Concurrent Create Correctness below, not from row locking.
- This primitive must be shaped so Slice 2+ can reuse it unchanged for RUNNING serialization (GAME-ADR-0018) - it must not become a lobby-only abstraction that later work has to replace.
- Rejected for V1 (per GAME-ADR-0004/GAME-ADR-0018, reaffirmed here): Redis or another distributed lock, process ownership, sticky routing, in-memory actor correctness.

### Idempotency Design

**Namespace.** Idempotency identity is `(user_uuid, operation, idempotency_key)`, enforced by a unique constraint on that triple - **not** `(operation, idempotency_key)` alone. An idempotency key is scoped to its caller: two different users may independently produce the same opaque key with no collision, since the key only has to be unique within one user's own logical command stream.

**Per-operation meaningful fields** (explicitly modeled structs, not a generic canonical-JSON diff):
- `CREATE`: `GameUUID`, plus the host `UserUUID` already implied by the idempotency identity itself.
- `JOIN`: `JoinCode`, `UserUUID`, display-name snapshot.
- `LEAVE`: `SessionUUID`, `UserUUID`.

Exact struct/field naming is implementation-local.

**Replay vs. conflict.** On a mutating call, look up an existing `session_requests` row by `(user_uuid, operation, idempotency_key)`:
- if found, `status = COMPLETED` (or equivalent), and the incoming request's meaningful fields match the stored ones - return the stored `outcome`/`response_payload` as an idempotent replay;
- if found, completed, but the meaningful fields differ - reject with a dedicated conflict sentinel error; do not silently apply the new request or silently return the old result;
- if found but not yet completed (see Completed Outcomes vs. Transient Failures below) - see that section for how this is resolved rather than treated as an immediate conflict or an immediate replay.

Do not build a generic JSON-canonicalization framework; model comparison narrowly per operation.

### Concurrent Create Correctness

`CreateSession` has no pre-existing `sessions` row to lock, so its concurrency correctness cannot come from the row-locking mechanism above - it must come from claiming the idempotency identity itself before any Session-creation effect happens. Required logical outcome for two concurrent `CreateSession` calls sharing the same `(user_uuid, CREATE, idempotency_key)`: **exactly one** Session-creation effect occurs, and both calls eventually observe/return the same logical result. It is not acceptable for both calls to create separate Sessions, nor for the idempotency uniqueness violation to be discovered only after both Session-creation effects have already happened.

Approach: within `CreateSession`'s transaction, attempt to insert the claiming `session_requests` row (`user_uuid`, `operation = CREATE`, `idempotency_key`, `request_payload`, `status = PENDING` or equivalent) *before* creating the Session/actor/join-code artifacts, relying on the `(user_uuid, operation, idempotency_key)` unique constraint to serialize concurrent claims at the database level:
- if the insert succeeds, this call owns the claim - proceed to create the Session/host actor/join code, then update the `session_requests` row to `status = COMPLETED` with the resulting `outcome`/`response_payload`, all in the same transaction;
- if the insert fails on the unique constraint, another call already claimed (or is claiming) this identity - re-read the existing row: if it is already `COMPLETED`, replay its stored outcome (after confirming the meaningful fields match, per Idempotency Design above); if it is not yet `COMPLETED`, see Completed Outcomes vs. Transient Failures below for how to handle a claim that is still in flight or was abandoned.

This is one acceptable implementation family (claim-then-create, unique-constraint-driven); another PostgreSQL-safe approach providing the same guarantee (exactly one effect, consistent replay) is equally acceptable - this section is not meant to over-specify SQL, but the acceptance criteria (see below) require a concurrency test that actually proves this, not one that merely happens to pass under low contention.

### Completed Outcomes vs. Transient Failures

A `session_requests` row represents a **completed deterministic logical outcome** only once its owning transaction commits with `status = COMPLETED`. It must never be possible for a transient infrastructure failure - Postgres becoming unavailable before commit, a transaction rolling back for any reason - to leave behind a permanently replayable "completed" outcome for an operation that never actually happened. Concretely:
- if the claiming transaction never commits (crash, rollback, connection loss before commit), no `session_requests` row survives with that identity at all - ordinary transaction atomicity already guarantees this, since the claim insert and the completion update happen in the same transaction as the Session-creation effect;
- a caller retrying after such a failure sees no existing row for that identity and proceeds as a fresh attempt, exactly as if the first attempt had never been made;
- the `status` field exists specifically so that *if* a future revision of this mechanism ever needs a claim to be visible before its owning transaction commits (it does not, under the single-transaction approach above), an in-flight/abandoned claim can be distinguished from a genuinely completed one rather than being treated as a cached failure response.

Do not turn a temporary infrastructure outage into a permanently cached Session response.

### Game Management Dependency

**No longer "no Game Management change" - a minimum new read capability is required.**

**At Create**, the existing `getgame.UseCase.GetPlayableGameWithCurrentVersion(ctx, gameUUID)` remains correct and sufficient: it resolves the game's *current* playable Definition/VersionUUID, which is exactly what "pin whatever is currently playable" means at the moment of creation. `CreateSession` calls it, then calls `engineservice.Compile(definition)` to validate the definition compiles, then persists the resolved `VersionUUID` as `sessions.game_definition_uuid`. Only that pinned version reference is persisted - the compiled `engine.Program` itself is never persisted or treated as a pinned artifact; a Session is pinned to an immutable *definition/version reference* (GAME-ADR-0001/GAME-ADR-0004), not to a compiled in-memory value. A definition that fails to compile must reject `CreateSession` with a clear error rather than creating a Session that can never Start. After a successful Create, "current version" is never consulted again for that Session.

**At Join** (and reusable by Slice 2's Start), a **new, minimum Game Management read capability** is required: load the immutable Game Definition by its own public Definition/Version UUID - conceptually `GetGameDefinition(ctx, gameDefinitionUUID)` or a repository-conventional equivalent name (exact naming is implementation freedom). Required semantics:
- input is the persisted logical `sessions.game_definition_uuid` (a Definition/Version UUID, not a Game UUID);
- returns the immutable definition needed for Session-side validation (at minimum, whatever the Definition exposes for `players.max`/`players.min`);
- does **not** resolve through `games.current_definition_id` - it must return the exact pinned version regardless of what is currently published as "current";
- Session persistence code never queries Game Management tables directly - it calls this capability;
- no cross-domain database foreign key is introduced;
- this is the minimum addition needed to make Join (and later Start) correct - it does not redesign Game Management's existing ownership, publish/lifecycle behavior, or storage.

Implementation may place the Game Management read immediately before or during the Join transaction, whichever keeps correctness clear and transaction duration reasonable - since the pinned `game_definition_uuid` is immutable, the definition itself cannot change underneath the Session regardless of exactly when it's read relative to the lock, so there is no race to worry about here the way there would be if "current version" were being re-resolved.

### Host/SessionActor Creation Cycle

The accepted model is intentionally cyclic at the logical level: `sessions.host_actor_id -> session_actors.id` and `session_actors.session_id -> sessions.id`. Use the approved simple transactional sequencing rather than deferred-FK complexity or preallocated IDs:

1. `BEGIN`
2. Insert the `sessions` row with `host_actor_id = NULL`.
3. Obtain the new Session's `id`.
4. Insert the host `session_actors` row referencing that Session (`session_id`, `user_uuid`, `semantic_presence = CONNECTED`).
5. Obtain the new actor's `id`.
6. `UPDATE sessions SET host_actor_id = <actor id> WHERE id = <session id>`.
7. Create the remaining Create artifacts (active `JoinCode`, completed `session_requests` outcome).
8. `COMMIT`.

`host_actor_id` is nullable **at storage level only**, purely to make step 2 possible before step 6 runs. The domain/commit invariant is stricter: a successfully committed, non-corrupt Session must have a valid `host_actor_id` by the time the transaction commits - no caller or subsequent normal operation should ever observe a successfully committed Session with a missing host actor. If the transaction fails at any step, nothing commits and no partially-created Session survives.

### Session/Actor/Participant Operation Behavior

- **CreateSession**: claim the idempotency identity first (see Concurrent Create Correctness above), resolve/compile/pin the current playable Game Definition (see Pinned Game Definition and Game Management Dependency above), then within the **same** transaction run the Host/SessionActor Creation Cycle above and create an active `JoinCode` with `lobby_expires_at = now + <lobby TTL>`. Do **not** create a `session_participants` row for the host - the host is never automatically a Participant. The successful logical result is atomic across all of: the `sessions` row, the host `SessionActor`, `sessions.host_actor_id` being set, the active `JoinCode`, `lobby_expires_at`, and the completed `session_requests` outcome - all in one transaction, all-or-nothing.
- **JoinSession**: resolve the `JoinCode` to its Session; obtain the Session's DB mutation lock (`FOR UPDATE`); validate `phase == LOBBY` and `now < lobby_expires_at` (otherwise lazily materialize `TERMINAL` in the same transaction and reject - see below); read `sessions.game_definition_uuid`; load that **pinned** immutable definition through the new Game Management capability (never "current version" - see Pinned Game Definition above) to obtain `players.max`; find-or-create the `SessionActor` for `(session, user_uuid)`; enforce `players.max` from the pinned definition; create or reactivate the `Participant` (`active = true`, snapshot `display_name`); commit. Repeated identical active Join is the same logical admission, not a new slot.
- **LeaveSession**: resolve `(session, user_uuid)` to `SessionActor`; lock the Session; if the Session is not `LOBBY` (for this work, the only other reachable phase is `TERMINAL`), reject with a dedicated sentinel rather than silently no-op-ing; otherwise deactivate the `Participant` (`active = false`, `left_at = now`), keep the `SessionActor` row, and never touch `host_actor_id` - a leaving host keeps host authority even though they stop participating.
- **Lazy lobby-expiration materialization**: any of the three operations above that finds `phase == LOBBY && now >= lobby_expires_at` while holding the Session lock may, in the same transaction, set `phase = TERMINAL`, `terminal_at = lobby_expires_at` (the semantic deadline, not the current operation's wall-clock time), a terminal reason denoting lobby expiration, and revoke the active `JoinCode` - then reject the originally attempted action. No sweeper/background worker is implemented by this work; operation-level correctness is the whole mechanism, per GAME-ADR-0004.

## Constraints and Invariants

- Session-local uniqueness: at most one active logical participation per `(session, user_uuid)` (GAME-ADR-0003).
- No database foreign keys and no direct table queries across bounded contexts (GAME-ADR-0001, `cross-domain-reference-naming.md`).
- `CreateSession` never accepts an externally-supplied `engine.Program` (GAME-ADR-0004).
- Host is never automatically an active Participant (GAME-ADR-0004).
- `lobby_expires_at` is authoritative; no operation may revive an already-expired lobby merely because persisted `phase` still reads `LOBBY` (GAME-ADR-0004).
- `sessions.game_definition_uuid` is resolved exactly once, at Create, and is immutable for the Session's lifetime; no later operation (Join in this work; Start/RuntimeTurn in later slices) may re-resolve "current version" - see Pinned Game Definition Is Immutable For The Session above.
- Idempotency identity is `(user_uuid, operation, idempotency_key)`, not `(operation, idempotency_key)` - different users never collide on the same opaque key.
- A committed Session must have a valid, non-null `host_actor_id`; `host_actor_id` is nullable only as a transient storage-level artifact of the Host/SessionActor Creation Cycle within one still-open transaction.
- Repository queries follow `repositories.md`: raw SQL for reads/updates/deletes; `Create()` for inserts.
- Errors follow `error-handling.md`: dedicated sentinel errors for intentional contracts (e.g. invalid/expired JoinCode, lobby full, conflicting idempotency key, non-`LOBBY` Leave), `%s` context by default rather than `%w` for incidental lower-level errors, logging at the appropriate boundary rather than every layer.
- Unexpected persisted-state inconsistency follows `data-integrity.md`: alert via `monitoring.Alert` and return a normal error; do not panic unless continuing would be genuinely unsafe.
- Tests follow `testing.md`: table-driven with underscore-separated case names, a shared disposable domain test database for repository tests (mirroring `game/game/internal/testdb`), mocked collaborator interfaces for service/use-case tests.

## Acceptance Criteria

**Create**
- Creates a `LOBBY` Session pinned to the correct resolved Game Definition/version.
- Creates the host `SessionActor`; the host is not created as an active Participant.
- Creates an active `JoinCode`.
- A repeated, semantically-equivalent Create using the same `(user_uuid, operation, idempotency_key)` returns the same logical result without creating a second Session.
- The same idempotency identity with a conflicting request is rejected.
- A Game Definition that fails to compile is rejected with a clear domain error, not a created-but-unusable Session.
- A successful Create commits with a valid, non-null `host_actor_id`, and the host `SessionActor` references that same Session; no partially-created Session (e.g. missing its host actor) survives a failed Create transaction.

**Join**
- A normal Join creates/reuses the `SessionActor` and activates a `Participant`.
- A repeated identical active Join is idempotent (no duplicate Participant/slot consumed).
- The same `User` never ends up with two `SessionActor` rows for the same Session.
- `players.max` is enforced correctly under concurrent Joins racing for the final slot.
- Lobby expiration is enforced even though no sweeper exists.
- A revoked/invalid `JoinCode` is rejected.
- The participant's display name is persisted as a Session-scoped snapshot.
- **Pinned definition**: Create a Session against Game version V1; publish/change the Game's current version to V2 with a different `players.max`; Join the existing Session; Join's `players.max` enforcement is still governed by pinned V1, not V2. (Test fixtures may simulate the "current version changed" condition directly in Game Management test data rather than literally invoking a publication workflow, if simpler.)
- **Cross-domain read correctness**: Join loads the Game Definition by the Session's pinned `game_definition_uuid`, not by re-resolving the Game's current version; a test should fail if the implementation accidentally substitutes a "current version" lookup for the pinned-definition lookup.

**Leave**
- An active Participant can leave the lobby; their slot becomes available to others.
- The `SessionActor` remains durable after Leave.
- Host authority is retained if the leaving user is also host.
- A retried Leave with the same idempotency identity behaves idempotently.

**Concurrency**
- Concurrent Joins competing for the final slot resolve correctly (exactly one succeeds when only one slot remains) via the DB-locking mechanism, not application-level check-then-act races.
- A Join racing a Leave around capacity resolves correctly under the same locking mechanism.
- An operation racing lobby-expiration materialization resolves without ever reviving an expired lobby.
- **Concurrent Create**: two equivalent concurrent `CreateSession` calls sharing the same `(user_uuid, CREATE, idempotency_key)` produce exactly one logical Session and both observe/replay the identical result - never two Sessions, and never a window where the duplicate is only discovered after both Session-creation effects already happened.

**Idempotency namespace and failure semantics**
- User A and User B independently using the same `operation`/`idempotency_key` value do not collide (different `user_uuid`, so different identity).
- The same `(user_uuid, operation, idempotency_key)` with a different, conflicting semantic payload (e.g. a different `JoinCode` or `GameUUID`) is rejected as a conflict, not silently replayed or silently applied.
- A transient infrastructure failure or transaction rollback before commit does not leave behind a completed, replayable logical outcome; a subsequent retry of the same identity executes as a fresh attempt rather than replaying a cached failure.

**Cross-domain / architecture guardrails**
- No test or production code path accepts an externally-supplied `engine.Program` into `CreateSession`.
- No test or production code path uses `Identity.UserUUID` as engine/runtime identity in this slice (there is no engine/runtime identity yet - this guards against prematurely introducing one).
- No repository code issues a direct SQL query against a Game Management or Identity table, and no database foreign key crosses into another bounded context's tables.
- No test or production code path calls `getgame.GetPlayableGameWithCurrentVersion` (or any "current version" resolution) for anything other than `CreateSession`.

## Implementation Freedom

- Exact package/file layout. Recommended: adopt the `usecases/<verb+noun>/` pattern already established by `game/game/usecases/getgame` (e.g. `game/session/usecases/createsession`, `.../joinsession`, `.../leavesession`) instead of extending the existing `workflows/sessionlifecycle` structure, for consistency with the rest of the repository - but this is a local structural choice, not a material constraint.
- The exact `lobby_expires_at` TTL value/source (a package-level constant is acceptable; no existing repository-wide configuration convention was found to reuse).
- The exact naming/shape of the new Game Management pinned-definition read capability (`GetGameDefinition` or equivalent) and its placement (new file in `getgame`, a new sibling usecase, etc.) - the required semantics are fixed (see Game Management Dependency above), the exact Go naming/package placement is not.
- Whether Join's pinned-definition read happens strictly before opening the Session transaction or during it - either is acceptable since the pinned definition cannot change underneath the Session either way.
- Exact Go sentinel/error type names, including the idempotency-conflict and concurrent-Create-claim-collision errors.
- Exact SQL column types/lengths/indexes beyond what's specified above, following `repositories.md`.
- Exact `session_requests.status` value set/type (e.g. a small string enum vs. a boolean "completed" flag) as long as it can distinguish a completed outcome from anything else.

## Verification

- `go build ./...` must succeed, including `game/session/...` (currently broken).
- `go test ./game/session/...` must pass: repository integration tests against a disposable real Postgres database (requiring `TEST_DATABASE_HOST`/`TEST_DATABASE_PORT`/`TEST_DATABASE_USERNAME`/`TEST_DATABASE_PASSWORD`/`TEST_DATABASE_NAME`/`TEST_DATABASE_SSL_MODE`, consistent with `game/game/internal/testdb`'s existing pattern) plus service/use-case tests using generated mocks (`mockgen`, consistent with `getgame`'s `//go:generate mockgen` convention).
- At least one concurrency-specific test proving the DB-locking mechanism actually serializes concurrent Joins/Leaves rather than relying on incidental timing.
- At least one concurrency-specific test proving concurrent `CreateSession` calls sharing the same `(user_uuid, CREATE, idempotency_key)` produce exactly one Session and a consistent replayed result - not merely a test that happens to pass under low contention.
- Exact test-invocation commands follow whatever convention is already used for `game/game`'s equivalent tests - do not invent a new one.

## Documentation Impact

### Accepted / Canonical Knowledge

- None. This work implements what GAME-ADR-0001 through GAME-ADR-0005 and `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` already accept; it does not change any accepted architecture/domain fact.

### Current-State Documentation After Implementation

- `game/CURRENT_STATE.md` - the Session Runtime capability-status row should be updated, once this work reaches DONE, from "PARTIAL... `CreateRoom` and `JoinRoom` are stubs" to reflect working Create/Join/Leave; the "Known Gaps"/"Known Drift" sections should be revised to remove now-resolved items.
- `game/docs/DATA_MODEL.md` - the Session Runtime Tables section should be updated, once DONE, to show the new `sessions`/`session_actors`/`session_participants`/`join_codes`/`session_requests` schema, replacing the old `sessions`/`session_players`/`session_states`/`join_codes` shape.

### Intentionally Unchanged

- `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` is accepted-design documentation, not current-state; it is not modified by this implementation.
- Game Management's existing ownership, publish/lifecycle behavior, and persisted schema are unchanged; this work adds exactly one minimum read capability (see Game Management Dependency above) and nothing else under `game/game/**`.

## Data / Migration Impact

The current `game/session` schema (`sessions`, `session_states`, `session_players`, `join_codes`, migrated by `20260817000000_session_states.go`/`20260817000001_sessions.go`) has never held real data: `CreateRoom`/`JoinRoom` are no-op stubs that never persist anything, and the `game/session/...` package tree does not even compile today. Discarding this data is safe - there is nothing to preserve.

Proposed approach: add **new** migration files (do not edit or delete the two existing historical migration files) that:
1. Drop the old `session_states`, `session_players`, `sessions`, and `join_codes` tables (in dependency order).
2. Create the new `sessions`, `session_actors`, `session_participants`, `join_codes`, and `session_requests` tables per the Approved Design above, each with a `Rollback` that reverses it, following the existing `gormigrate` convention (`game/session/internal/storage/migrations/migration.go`'s registration list) exactly as already used for the current two migrations.

Rationale for adding new migrations rather than editing the existing files in place: preserves `gormigrate`'s migration-tracking integrity (the `migrations` table records applied migration IDs) regardless of whether any environment has ever actually run the old migrations, which is a safer default than assuming no environment has. No repository-wide migration-safety rule beyond `gormigrate`'s own `Migrate`/`Rollback` mechanism was found during inspection, so this is a proposed convention for this work, not an existing mandate - the Codebase Agent may deviate if implementation-time inspection shows a stronger reason to, without that constituting a material scope change.

Dev-reset assumption: this project is pre-launch; no production deployment/data-preservation requirement is assumed to exist for this schema. If that assumption is wrong, this must return to DRAFT before implementation proceeds.

The constraint/index plan for the new tables must include at minimum:
- unique `(session_id, user_uuid)` on `session_actors`;
- unique Participant ownership per `session_actor_id` as already accepted (at most one `session_participants` row per `SessionActor`);
- unique `(user_uuid, operation, idempotency_key)` on `session_requests` (not `(operation, idempotency_key)`);
- `join_codes` constraints appropriate to the already-approved semantics (one active/non-revoked code per lobby).
- `sessions.host_actor_id` is nullable at the storage level only, per the Host/SessionActor Creation Cycle above.

## Blockers

- None material.

## Known Risks / Questions (resolvable during implementation, without reopening architecture)

- The exact `lobby_expires_at` TTL value and where it's defined (constant vs. a config surface) - implementation-level, not decided here.
- The exact naming/package placement of the new Game Management pinned-definition read capability - required semantics are fixed (see Game Management Dependency above); naming is not.
- Exact package layout (`usecases/<name>` vs. extending `workflows/sessionlifecycle`) - recommendation given above.
- Exact `session_requests.status` representation (string enum vs. boolean) - any type distinguishing completed from non-completed is acceptable.

## Completion Record

Not applicable - Status is READY (implementation has not started).
