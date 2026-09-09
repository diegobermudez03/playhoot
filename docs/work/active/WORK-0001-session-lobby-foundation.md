# WORK-0001: Session Lobby Foundation

Status: DRAFT
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
- `game/game/usecases/getgame.UseCase.GetPlayableGameWithCurrentVersion(ctx, gameUUID) (*game.Game, error)` **already exists, is tested, and is sufficient** for this work: it returns a `game.Game` with a decoded `program.Definition` and `VersionUUID`. No Game Management change is needed.
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
- Idempotency for `CREATE`/`JOIN`/`LEAVE` via `session_requests`.
- Consuming the existing Game Management `getgame` read capability to resolve/pin the playable Game Definition; no Game Management change.
- Repository integration tests (disposable real Postgres DB) and service/use-case tests (mocked repository), per repository testing conventions, including concurrency tests proving the DB-locking mechanism.

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
- Any change to Game Management (`game/game/**`) - the existing `getgame` capability already suffices.

## Approved Design

### Identity/Auth Boundary

Session application/domain APIs receive an already-trusted `UserUUID`, per GAME-ADR-0005. Credential-to-`UserUUID` resolution belongs outside Session Runtime and is integrated later when a real transport/auth boundary exists (a later slice). This work does not implement Identity/Auth and does not add a temporary fake-auth mechanism that would later need removal; test and internal callers supply `UserUUID` directly, exactly as the accepted public contract already expects.

### Persistence Model (Slice 1 subset of `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md`)

- **`sessions`**: `id`, `uuid`, `game_definition_uuid` (logical reference, no FK), `host_actor_id` (references `session_actors.id`), `phase`, `lobby_expires_at`, `started_at` (nullable, always `NULL` after this work - no Session created here ever reaches `RUNNING`), `terminal_at` (nullable), `terminal_reason` (nullable), `created_at`, `updated_at`. `phase` needs only `LOBBY`/`TERMINAL` values for this work, but its representation should not hard-code an exhaustive enum that later slices (`RUNNING`) would have to migrate around.
- **`session_actors`**: `id`, `session_id`, `user_uuid` (logical reference, no FK), `semantic_presence`, `created_at`; unique `(session_id, user_uuid)`. A successful normal Join begins `semantic_presence = CONNECTED` (GAME-ADR-0015's accepted default) with no physical connection tracking of any kind.
- **`session_participants`**: `id`, `session_actor_id`, `display_name`, `active`, `joined_at`, `left_at` (nullable).
- **`join_codes`**: `id`, `session_id`, `code`, `created_at`, `revoked_at` (nullable) - authoritative Session persistence, not cache-only; one active code per lobby; revoked (not deleted) when the lobby is no longer admissible; historical/revoked codes retained.
- **`session_requests`**: `id`, `operation`, `idempotency_key`, `user_uuid`, `session_id` (nullable - a `CREATE` request may not yet have a resulting session at the moment of dedup lookup for a rejected/failed attempt, though a successful `CREATE` should populate it), `request_payload`, `outcome`, `response_payload`, `created_at`. Only `CREATE`/`JOIN`/`LEAVE` operations are handled here; `START` support is explicitly deferred to Slice 2.

Cross-domain references (`game_definition_uuid`, `user_uuid`) remain logical only - no database foreign keys into another bounded context, no direct queries against Game Management or Identity tables from Session persistence code (GAME-ADR-0001, `cross-domain-reference-naming.md`).

### Database Locking / Serialization Design

- **Lock anchor**: the `sessions` row itself, selected `FOR UPDATE` by `id` inside the mutation's transaction.
- **Transaction boundary**: reuse `utils.RunInDBTransaction` (existing generic helper; `sql.LevelRepeatableRead`), the same mechanism already used elsewhere in the repository, rather than inventing a second transaction helper.
- **Responsible method**: a single narrow repository operation (naming left to implementation, e.g. `lockSessionForMutation(ctx, tx, sessionUUID) (sessionRow, error)`) that selects-for-update the `sessions` row and returns its current `phase`/`lobby_expires_at`. `Join`, `Leave`, and lazy-expiration materialization all call this same operation before mutating - they must not each invent their own locking query. `Create` does not need this operation (it inserts a new row; the session/actor/participant/join-code creation happens together in one `RunInDBTransaction` call for atomicity, but there is no existing row to contend on).
- This primitive must be shaped so Slice 2+ can reuse it unchanged for RUNNING serialization (GAME-ADR-0018) - it must not become a lobby-only abstraction that later work has to replace.
- Rejected for V1 (per GAME-ADR-0004/GAME-ADR-0018, reaffirmed here): Redis or another distributed lock, process ownership, sticky routing, in-memory actor correctness.

### Idempotency Design

- On a mutating call, look up an existing `session_requests` row by `(operation, idempotency_key)` (proposed unique constraint on that pair - a caller's idempotency key is expected to already be scoped uniquely per logical command).
- If found and the incoming request's semantically-relevant fields match the stored `request_payload` (compared via explicitly modeled per-operation fields - e.g. for `JOIN`: join code + user UUID + display name - not a generic canonical-JSON diff), return the stored `outcome`/`response_payload` as an idempotent replay.
- If found but the meaningful fields differ (same key, conflicting request), reject with a dedicated sentinel error - do not silently apply the new request or silently return the old result.
- Do not build a generic JSON-canonicalization framework; model comparison narrowly per operation.

### Game Management Dependency

Already satisfied by existing code - no Game Management change is authorized or required by this work. `CreateSession` calls `getgame.UseCase.GetPlayableGameWithCurrentVersion(ctx, gameUUID)` to obtain the playable `game.Game` (including `Definition program.Definition` and `VersionUUID`), then calls `engineservice.Compile(definition)` to validate it is a compilable definition before creating the Session. Only `game_definition_uuid` and the pinned version reference are persisted on `sessions` - the compiled `engine.Program` itself is not persisted or otherwise treated as a pinned artifact; a Session is pinned to an immutable *definition/version reference*, consistent with GAME-ADR-0001/GAME-ADR-0004, not to a compiled in-memory value. A definition that fails to compile must reject `CreateSession` with a clear error rather than creating a Session that can never Start.

### Session/Actor/Participant Operation Behavior

- **CreateSession**: resolve/pin the Game Definition (see above); within one transaction, create the host `SessionActor` (`semantic_presence = CONNECTED`), create the `sessions` row (`phase = LOBBY`, `host_actor_id` set to the new actor, `lobby_expires_at = now + <lobby TTL>`), create an active `JoinCode`; do **not** create a `session_participants` row for the host. Record the idempotency outcome.
- **JoinSession**: resolve the `JoinCode` to its Session; lock the Session; validate `phase == LOBBY` and `now < lobby_expires_at` (otherwise lazily materialize `TERMINAL` in the same transaction and reject - see below); find-or-create the `SessionActor` for `(session, user_uuid)`; enforce `players.max` from the pinned Game Definition (re-resolved via `getgame` at Join time is the recommended approach - see Implementation Freedom); create or reactivate the `Participant` (`active = true`, snapshot `display_name`); commit. Repeated identical active Join is the same logical admission, not a new slot.
- **LeaveSession**: resolve `(session, user_uuid)` to `SessionActor`; lock the Session; if the Session is not `LOBBY` (for this work, the only other reachable phase is `TERMINAL`), reject with a dedicated sentinel rather than silently no-op-ing; otherwise deactivate the `Participant` (`active = false`, `left_at = now`), keep the `SessionActor` row, and never touch `host_actor_id` - a leaving host keeps host authority even though they stop participating.
- **Lazy lobby-expiration materialization**: any of the three operations above that finds `phase == LOBBY && now >= lobby_expires_at` while holding the Session lock may, in the same transaction, set `phase = TERMINAL`, `terminal_at = lobby_expires_at` (the semantic deadline, not the current operation's wall-clock time), a terminal reason denoting lobby expiration, and revoke the active `JoinCode` - then reject the originally attempted action. No sweeper/background worker is implemented by this work; operation-level correctness is the whole mechanism, per GAME-ADR-0004.

## Constraints and Invariants

- Session-local uniqueness: at most one active logical participation per `(session, user_uuid)` (GAME-ADR-0003).
- No database foreign keys and no direct table queries across bounded contexts (GAME-ADR-0001, `cross-domain-reference-naming.md`).
- `CreateSession` never accepts an externally-supplied `engine.Program` (GAME-ADR-0004).
- Host is never automatically an active Participant (GAME-ADR-0004).
- `lobby_expires_at` is authoritative; no operation may revive an already-expired lobby merely because persisted `phase` still reads `LOBBY` (GAME-ADR-0004).
- Repository queries follow `repositories.md`: raw SQL for reads/updates/deletes; `Create()` for inserts.
- Errors follow `error-handling.md`: dedicated sentinel errors for intentional contracts (e.g. invalid/expired JoinCode, lobby full, conflicting idempotency key, non-`LOBBY` Leave), `%s` context by default rather than `%w` for incidental lower-level errors, logging at the appropriate boundary rather than every layer.
- Unexpected persisted-state inconsistency follows `data-integrity.md`: alert via `monitoring.Alert` and return a normal error; do not panic unless continuing would be genuinely unsafe.
- Tests follow `testing.md`: table-driven with underscore-separated case names, a shared disposable domain test database for repository tests (mirroring `game/game/internal/testdb`), mocked collaborator interfaces for service/use-case tests.

## Acceptance Criteria

**Create**
- Creates a `LOBBY` Session pinned to the correct resolved Game Definition/version.
- Creates the host `SessionActor`; the host is not created as an active Participant.
- Creates an active `JoinCode`.
- A repeated, semantically-equivalent Create using the same idempotency key returns the same logical result without creating a second Session.
- The same idempotency key with a conflicting request is rejected.
- A Game Definition that fails to compile is rejected with a clear domain error, not a created-but-unusable Session.

**Join**
- A normal Join creates/reuses the `SessionActor` and activates a `Participant`.
- A repeated identical active Join is idempotent (no duplicate Participant/slot consumed).
- The same `User` never ends up with two `SessionActor` rows for the same Session.
- `players.max` is enforced correctly under concurrent Joins racing for the final slot.
- Lobby expiration is enforced even though no sweeper exists.
- A revoked/invalid `JoinCode` is rejected.
- The participant's display name is persisted as a Session-scoped snapshot.

**Leave**
- An active Participant can leave the lobby; their slot becomes available to others.
- The `SessionActor` remains durable after Leave.
- Host authority is retained if the leaving user is also host.
- A retried Leave with the same idempotency key behaves idempotently.

**Concurrency**
- Concurrent Joins competing for the final slot resolve correctly (exactly one succeeds when only one slot remains) via the DB-locking mechanism, not application-level check-then-act races.
- A Join racing a Leave around capacity resolves correctly under the same locking mechanism.
- An operation racing lobby-expiration materialization resolves without ever reviving an expired lobby.

**Cross-domain / architecture guardrails**
- No test or production code path accepts an externally-supplied `engine.Program` into `CreateSession`.
- No test or production code path uses `Identity.UserUUID` as engine/runtime identity in this slice (there is no engine/runtime identity yet - this guards against prematurely introducing one).
- No repository code issues a direct SQL query against a Game Management or Identity table, and no database foreign key crosses into another bounded context's tables.

## Implementation Freedom

- Exact package/file layout. Recommended: adopt the `usecases/<verb+noun>/` pattern already established by `game/game/usecases/getgame` (e.g. `game/session/usecases/createsession`, `.../joinsession`, `.../leavesession`) instead of extending the existing `workflows/sessionlifecycle` structure, for consistency with the rest of the repository - but this is a local structural choice, not a material constraint.
- The exact `lobby_expires_at` TTL value/source (a package-level constant is acceptable; no existing repository-wide configuration convention was found to reuse).
- Whether `players.max`/`players.min` is re-resolved via `getgame` on every Join or snapshotted on the Session row at Create - re-resolving is recommended for simplicity and to avoid a second source of truth, but either is acceptable if documented.
- Exact Go sentinel error names/types.
- Exact SQL column types/lengths/indexes beyond what's specified above, following `repositories.md`.
- Exact transaction sequencing within `CreateSession` (e.g., whether the host `SessionActor` insert and the `sessions` insert happen as two statements in one transaction, or via some other ordering) - any approach preserving one-transaction atomicity is acceptable.

## Verification

- `go build ./...` must succeed, including `game/session/...` (currently broken).
- `go test ./game/session/...` must pass: repository integration tests against a disposable real Postgres database (requiring `TEST_DATABASE_HOST`/`TEST_DATABASE_PORT`/`TEST_DATABASE_USERNAME`/`TEST_DATABASE_PASSWORD`/`TEST_DATABASE_NAME`/`TEST_DATABASE_SSL_MODE`, consistent with `game/game/internal/testdb`'s existing pattern) plus service/use-case tests using generated mocks (`mockgen`, consistent with `getgame`'s `//go:generate mockgen` convention).
- At least one concurrency-specific test proving the DB-locking mechanism actually serializes concurrent Joins/Leaves rather than relying on incidental timing.
- Exact test-invocation commands follow whatever convention is already used for `game/game`'s equivalent tests - do not invent a new one.

## Documentation Impact

### Accepted / Canonical Knowledge

- None. This work implements what GAME-ADR-0001 through GAME-ADR-0005 and `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` already accept; it does not change any accepted architecture/domain fact.

### Current-State Documentation After Implementation

- `game/CURRENT_STATE.md` - the Session Runtime capability-status row should be updated, once this work reaches DONE, from "PARTIAL... `CreateRoom` and `JoinRoom` are stubs" to reflect working Create/Join/Leave; the "Known Gaps"/"Known Drift" sections should be revised to remove now-resolved items.
- `game/docs/DATA_MODEL.md` - the Session Runtime Tables section should be updated, once DONE, to show the new `sessions`/`session_actors`/`session_participants`/`join_codes`/`session_requests` schema, replacing the old `sessions`/`session_players`/`session_states`/`join_codes` shape.

### Intentionally Unchanged

- `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` is accepted-design documentation, not current-state; it is not modified by this implementation.
- No file under `game/game/**` changes - the existing `getgame` capability is sufficient as-is.

## Data / Migration Impact

The current `game/session` schema (`sessions`, `session_states`, `session_players`, `join_codes`, migrated by `20260817000000_session_states.go`/`20260817000001_sessions.go`) has never held real data: `CreateRoom`/`JoinRoom` are no-op stubs that never persist anything, and the `game/session/...` package tree does not even compile today. Discarding this data is safe - there is nothing to preserve.

Proposed approach: add **new** migration files (do not edit or delete the two existing historical migration files) that:
1. Drop the old `session_states`, `session_players`, `sessions`, and `join_codes` tables (in dependency order).
2. Create the new `sessions`, `session_actors`, `session_participants`, `join_codes`, and `session_requests` tables per the Approved Design above, each with a `Rollback` that reverses it, following the existing `gormigrate` convention (`game/session/internal/storage/migrations/migration.go`'s registration list) exactly as already used for the current two migrations.

Rationale for adding new migrations rather than editing the existing files in place: preserves `gormigrate`'s migration-tracking integrity (the `migrations` table records applied migration IDs) regardless of whether any environment has ever actually run the old migrations, which is a safer default than assuming no environment has. No repository-wide migration-safety rule beyond `gormigrate`'s own `Migrate`/`Rollback` mechanism was found during inspection, so this is a proposed convention for this work, not an existing mandate - the Codebase Agent may deviate if implementation-time inspection shows a stronger reason to, without that constituting a material scope change.

Dev-reset assumption: this project is pre-launch; no production deployment/data-preservation requirement is assumed to exist for this schema. If that assumption is wrong, this must return to DRAFT before implementation proceeds.

## Blockers

- None material.

## Known Risks / Questions (resolvable during implementation, without reopening architecture)

- The exact `lobby_expires_at` TTL value and where it's defined (constant vs. a config surface) - implementation-level, not decided here.
- Whether to re-resolve `players.max` via `getgame` on every Join versus snapshotting it on the Session row at Create - recommendation given above; either is acceptable.
- The exact per-operation idempotency-conflict comparison shape (which fields of a `JOIN`/`LEAVE`/`CREATE` request must match for a replay to be considered "the same request") - needs a concrete, narrow decision during implementation, not a generic framework.
- Exact package layout (`usecases/<name>` vs. extending `workflows/sessionlifecycle`) - recommendation given above.
- Exact sequencing of the host-`SessionActor`-then-`sessions`-row insert within `CreateSession`'s single transaction - any atomicity-preserving approach is acceptable.

## Completion Record

Not applicable - Status is DRAFT.
