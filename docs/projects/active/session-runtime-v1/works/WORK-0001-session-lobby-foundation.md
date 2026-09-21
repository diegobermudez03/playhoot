# WORK-0001: Session Lobby Foundation

Status: DONE
Created: 2026-09-08
Last status change: 2026-09-18

Related decisions:
- `game/docs/decisions/GAME-ADR-0001-game-capability-persistence-transaction-boundary.md`
- `game/docs/decisions/GAME-ADR-0002-session-runtime-durable-boundary.md`
- `game/docs/decisions/GAME-ADR-0003-session-runtime-actor-and-lifecycle-foundations.md`
- `game/docs/decisions/GAME-ADR-0004-session-lobby-lifecycle-contract.md`
- `game/docs/decisions/GAME-ADR-0005-session-public-and-internal-identity-boundary.md`
- `game/docs/decisions/GAME-ADR-0018-session-running-mutation-serialization.md` (reused design for the locking primitive; RUNNING itself is out of scope here)
- `game/docs/decisions/GAME-ADR-0021-session-lobby-command-idempotency-token-semantics.md` (Join idempotency token-replay-vs-new-command semantics)
- `game/docs/decisions/GAME-ADR-0022-session-lobby-business-declines-as-workflow-outcomes.md` (business declines represented as workflow outcomes, not errors)
- `docs/decisions/architecture/ADR-0005-cross-domain-public-entity-references.md`
- `identity/docs/decisions/IDENTITY-ADR-0001-identity-user-public-identity-boundary.md`

Canonical context:
- `game/README.md` (Session Runtime Lobby Lifecycle Contract, Session Runtime Actor and Lifecycle Model, Capability Persistence and Transaction Boundary sections)
- `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` (Lobby And Identity Tables)
- `game/docs/DATA_MODEL.md` and `game/CURRENT_STATE.md` (current implementation reality; the implementation boundary this WORK approves is now materially ahead of what these describe - see "2026-09-16 Design Revision" below)
- `docs/engineering/standards/cross-domain-reference-naming.md`, `repositories.md`, `error-handling.md`, `data-integrity.md`, `testing.md`, `domain-logic-placement.md`, `function-signatures.md`, `idempotency.md`
- `docs/projects/active/session-runtime-v1/PROJECT.md` (Slice 1 of the approved initiative sequence)

## 2026-09-16 Design Revision (Material, HUMAN-APPROVED)

The first implementation pass (2026-09-08, standard-compliance-migrated 2026-09-11) exposed a further service/repository/workflow-organization mismatch beyond what the 2026-09-09/09-11 migration already fixed: three sibling packages (`createsession`/`joinsession`/`leavesession`), each with its own `Input`-style params struct (e.g. `leaveParams`) and its own `repo.leaveSession(...)`-style method that performs the entire business operation (lock, expiration decision, idempotency decision, persistence) inside the repository layer, and a shared `sessionlock.MaterializeExpirationIfDue` that decides lifecycle-expiration policy from inside a locking primitive. This handoff clarifies and persists the reusable engineering standards this implementation must actually follow (`docs/engineering/standards/function-signatures.md` (new), `docs/engineering/standards/idempotency.md` (new), and refined sections of `domain-logic-placement.md`/`repositories.md`), clarifies Session Join idempotency token semantics (`game/docs/decisions/GAME-ADR-0021-session-lobby-command-idempotency-token-semantics.md`, refining GAME-ADR-0004's Join wording), and revises this WORK's approved design accordingly. This WORK was returned to DRAFT for this revision and is now re-approved and READY again; the human explicitly approved the material decisions in this handoff. **No Go source, tests, or migrations were changed by this revision** - only this specification. The next implementation pass must bring the existing Create/Join/Leave implementation into compliance with the revised Approved Design below before Slice 2 begins.

Material changes from the previously-approved design (see revised subsections below for full detail):

1. **One `SessionLifecycle` workflow controller** (`Manager`) exposing `Create`/`Join`/`Leave` (and later `Start`) as its methods, replacing the three independent sibling packages - see Workflow Package Structure below.
2. **Explicit method parameters**, not ceremonial `Input`/`Params` structs, per `function-signatures.md` - `leaveParams`-style bundling structs are removed from the target design.
3. **Explicit transaction ownership**: the Manager decides transaction scope via a transactor abstraction; no repository method independently opens/commits its own transaction; a business rejection can still commit an intended durable outcome (e.g. lazy expiration materialization, idempotency-request completion) - see Transaction Ownership below.
4. **Repository methods are renamed to persistence-oriented names** and stop deciding business/lifecycle policy - `repo.leaveSession(...)`-style methods that decide admission/expiration/idempotency-replay meaning move that decision into the Manager/workflow layer; the repository exposes narrow persistence-oriented operations instead - see Repository Responsibility below.
5. **Expiration policy moves out of the shared locking primitive.** `sessionlock` may continue to expose the Session's current locked-row facts (phase, `lobby_expires_at`), but the decision "phase is LOBBY and now is at or after the deadline, therefore materialize TERMINAL" is Manager/workflow policy, not something the shared lock package decides on the workflow's behalf - see Expiration Ownership below.
6. **Admission (capacity/phase/expiration) is explicit workflow policy**, not something inferred by a repository method that also happens to do the persistence.
7. **Join idempotency is now token-aware** per GAME-ADR-0021: a differently-tokened Join while already an active Participant is a new command evaluated against current state, rejected as `AlreadyJoined`-equivalent - not silently treated as success merely because the desired state already holds. The previous wording ("repeated active Join is idempotent") under-specified this; the corrected acceptance criteria are below.

## 2026-09-17 Design Revision (Transaction/Outcome/Mechanism Refinement, HUMAN-APPROVED)

The 2026-09-17 implementation pass (see the Implementation Status Note immediately below) faithfully implemented the 2026-09-16 revised design, but a second implementation review the same day found that design itself still carried transitional ceremony and one modeling mismatch worth correcting before this WORK closes. This handoff is documentation-only: **no Go source, tests, or migrations were changed by this revision.** This WORK was returned to DRAFT for this revision and is now re-approved and READY again; the human explicitly approved the material decisions in this handoff. The next implementation pass must bring the existing Create/Join/Leave implementation into compliance with this further-revised Approved Design before Slice 2 begins.

Material changes from the 2026-09-16 revision (see revised subsections below for full detail):

1. **The Manager does not depend on an injected `transactor` object.** Transaction execution is persistence/infrastructure mechanics, not a business/application collaborator; the Manager calls a transaction helper (conceptually `RunInTransaction`, wrapping the existing generic `utils.RunInDBTransaction[T]`) directly rather than storing a `transactor` interface/field whose only purpose is to indirect into it. The workflow still decides transaction *scope* (which operations belong to one atomic unit) - only the ceremonial injected-dependency shape is removed. See Transaction Ownership below and `docs/engineering/standards/repositories.md -> Transaction Ownership`.
2. **The transaction callback returns its result directly; there is no `bizErr` and no outer-scoped result-variable mutation.** The prior design's `var result T; var bizErr error; txErr := m.tx.RunInTransaction(ctx, func(ctx, tx) error { ...; bizErr = ...; return nil })` pattern is replaced by a callback with the generic shape `func(ctx, tx) (T, error)` that returns its result explicitly, per `docs/engineering/standards/function-signatures.md -> Callback-Based Helpers Return Values, Not Mutate Closures`.
3. **Expected business declines are represented as result-value outcomes, not as returned `error`s** (GAME-ADR-0022, `docs/engineering/standards/error-handling.md -> Expected Business Outcome vs. Error`). `JOINED`/`LOBBY_EXPIRED`/`LOBBY_FULL`/`ALREADY_JOINED` become values of a `JoinResult.Outcome`-shaped field (exact Go naming/shape is implementation freedom), returned alongside a `nil` error once the operation has been correctly evaluated. This removes the need for the `bizErr` pattern in (2) above: a deterministic decline that must durably commit (together with a lazy-expiration materialization or an idempotency completion) is decided and persisted inside the transaction callback, then returned as a normal `(result, nil)` value carrying the declined outcome - the transaction commits through the ordinary success path. Create's and Leave's equivalent already-accepted declines (for example, Leave outside `LOBBY`) are remapped the same way. Protocol/contract errors (missing/empty idempotency token, conflicting same-token payload, infrastructure/invariant failures) remain `error`s - this is a representation change for already-accepted business declines only, not new domain behavior.
4. **Idempotency mechanism packages are called directly by workflow steps; the narrow repository forwarding methods that only called them are removed.** `internal/repo`'s `ClaimSessionRequest`/`CompleteSessionRequest` (which only forwarded to `internal/idempotency`'s `Claim`/`Complete`) and `LockSessionByID`/`LockSessionByUUID` (which only forwarded to `internal/sessionlock`'s `LockByID`/`LockByUUID`) are removed from the target design; each step calls `idempotency.Claim`/`idempotency.Complete` and `sessionlock.LockByID`/`LockByUUID` directly, the same way it would call a domain type's method. See Repository Responsibility and Database Locking / Serialization Design below, and `docs/engineering/standards/repositories.md -> Sharing Rule`.
5. **`idempotency.Claim`'s contract drops the redundant `claimed bool`.** It returns `(requestID, existingRequest, error)` with the invariant `requestID != 0 && existingRequest == nil` (fresh claim) or `requestID == 0 && existingRequest != nil` (identity already existed). `idempotency.Row` is renamed `idempotency.Request` for the shape returned to workflow code (a private GORM model may still be named as a row internally). See Idempotency Design below and `docs/engineering/standards/idempotency.md -> Claim Mechanism Contract`.
6. **The PostgreSQL claim mechanic changes from provoke-then-recover to a non-error conflict path.** Instead of inserting, catching the resulting unique-constraint violation, and then querying for the existing row through the same transaction, the claim insert uses `INSERT ... ON CONFLICT (user_uuid, operation, idempotency_key) DO NOTHING RETURNING id` (or an equivalent PostgreSQL-safe mechanism): a returned id means a fresh claim, no returned row means read the existing request. Concurrency/integration coverage for this is required. See Idempotency Design below.
7. **Per-operation idempotency payload/interpretation moves out of the shared workflow-level `idempotency.go` and into each owning step's own file.** `createRequestPayload`/`interpretExistingCreateClaim` move into `step_create.go`; `joinRequestPayload`/`interpretExistingJoinClaim` into `step_join.go`; `leaveRequestPayload`/`interpretExistingLeaveClaim` into `step_leave.go`. Only a genuinely mechanical, non-interpretive helper (e.g. `marshalPayload`) may remain shared. See `docs/engineering/standards/idempotency.md -> Organizational Guidance`.
8. **Workflow-facing repository return types/variables are named for the entity/fact they represent, not for the storage row.** `SessionRow`/`ActorRow`/`ParticipantRow`/`RequestRow`-suffixed types and `row`-named local variables in workflow-facing contracts are renamed toward `session`/`actor`/`participant`/`request`-style naming; a private persistence struct internal to the repository implementation may still be named as a row. See `docs/engineering/standards/repositories.md -> Naming Exposed To Workflow Code`.
9. **Repository methods no longer take a `now` parameter used only to populate an audit timestamp.** `CreateActor`, `CreateSessionWithHost`, and `CreateJoinCode` currently accept `now` solely to stamp `created_at`/`updated_at`; that parameter is removed, and the repository/ORM/DB populates those columns itself. Semantic lifecycle timestamps that already have their own explicit, meaningfully-named parameter are unaffected and are NOT removed: `lobbyExpiresAt` (already explicit), `terminalAt` (already explicit, `SetSessionTerminal` also drops its separate audit-only `now`), and `RevokeActiveJoinCode`'s revocation timestamp remain required explicit inputs. `CreateParticipant`'s and `ActivateParticipant`'s `now` parameter is kept but should be understood/named as the semantic `joined_at` value it actually sets, and `DeactivateParticipant`'s as the semantic `left_at` value - both remain explicit workflow-supplied inputs, not audit metadata. See `docs/engineering/standards/repositories.md -> Timestamp Ownership`.

Everything else from the 2026-09-16 revision stands unchanged: one `Manager` workflow controller with explicit-parameter methods; the `sessions`-row `FOR UPDATE` lock reused unchanged for Slice 2's RUNNING serialization; persistence-oriented (not business-command-named) repository method names; Manager-owned expiration/admission policy; token-aware Join idempotency per GAME-ADR-0021; the pinned-Game-Definition and Host/SessionActor Creation Cycle designs.

## Implementation Status Note (Current, 2026-09-18)

The 2026-09-17 Design Revision above (transactor removal, outcome-vs-error representation, direct mechanism usage, simplified Claim contract, step-local idempotency interpretation, entity-not-row naming, audit-timestamp removal) **has now been implemented** (2026-09-18 pass). `game/session/workflows/sessionlifecycle/manager.go` no longer holds a `transactor` field; `Manager` implements `utils.DBServicer` itself (`GetDB()`) and each step (`step_create.go`/`step_join.go`/`step_leave.go`) calls `utils.RunInDBTransaction(ctx, m, callback)` directly, with the callback returning `(Result, error)` directly (no outer-scoped `bizErr`/result variable). Each step calls `sessionlock.LockByID`/`LockByUUID` and `idempotency.Claim`/`Complete` directly; the `internal/repo` forwarding methods (`LockSessionByID`/`LockSessionByUUID`/`ClaimSessionRequest`/`CompleteSessionRequest`) and `internal/repo/request.go` were deleted. `JoinResult`/`LeaveResult` gained `Outcome` fields (`JoinOutcome`/`LeaveOutcome`); `JOINED`/`LOBBY_EXPIRED`/`LOBBY_FULL`/`ALREADY_JOINED` and Leave's `LEFT`/`NOT_IN_LOBBY`/`ACTOR_NOT_FOUND` are returned as outcome values alongside a `nil` error, never as a Go `error`; the corresponding sentinel errors were removed from `game/session/session.go`. `idempotency.Row`/`sessionlock.Row` were renamed `idempotency.Request`/`sessionlock.Session`; `idempotency.Claim` dropped `claimed bool`. Per-operation idempotency payload/interpretation code moved next to each owning step; the shared workflow-level `idempotency.go` bucket file was deleted (replaced by a small shared `payload.go` holding only the mechanical `marshalPayload` helper). `internal/repo`'s `ActorRow`/`ParticipantRow` were renamed `Actor`/`Participant`; `CreateActor`/`CreateSessionWithHost`/`CreateJoinCode` dropped their audit-only `now` parameter; `CreateParticipant`/`ActivateParticipant`/`DeactivateParticipant`/`RevokeActiveJoinCode` renamed `now` to the semantic value it sets. See this pass's IMPLEMENTATION REPORT for full detail, including two Local Implementation Decisions worth human awareness: (1) the idempotency claim's PostgreSQL mechanic uses a single `INSERT ... ON CONFLICT DO UPDATE ... RETURNING` upsert (distinguishing fresh-vs-existing via the `xmax` system column) rather than this WORK's illustrative `ON CONFLICT DO NOTHING RETURNING id` + fallback `SELECT`, because the fallback-`SELECT` approach can fail to observe a concurrently-committed row for the remainder of a REPEATABLE READ transaction (`utils.RunInDBTransaction`'s isolation level) - this stays within this WORK's "an equivalent PostgreSQL-safe mechanism providing the same guarantee is acceptable" allowance, but is flagged since it touches concurrency-sensitive mechanics and has not been verified against a real Postgres in any sandbox used so far; (2) `CreateJoinCode`'s collision-retry loop had the same provoke-then-recover defect this WORK identified for the idempotency claim (a real unique-violation error aborts the whole Create transaction, breaking the retry loop) and was fixed the same way. Repository-level and Manager-level (mocked-collaborator, real-Postgres integration, and concurrency) tests all exist and compile; the mocked-collaborator tests pass in this sandbox. **The real-Postgres integration/concurrency tests remain unexecuted** in this sandbox (no reachable Docker/Postgres engine - `TEST_DATABASE_*` env vars unset, tests self-report `SKIP`), the same known limitation recorded by every prior pass - this specifically means the `xmax`-based claim mechanic and the `CreateJoinCode` fix above have not been empirically verified against real Postgres concurrency, only reasoned through. Independent review per `docs/ai/protocols/IMPLEMENTATION_REVIEW.md` has still not been performed. See `game/CURRENT_STATE.md` and `game/docs/FLOWS.md` for the current actual-code-reality description (synchronized by this pass).

**Superseded implementation-status text (accurate to the 2026-09-17 first pass, before this same-day revision was implemented) preserved below for history:**

**Superseded by the 2026-09-17 Design Revision immediately above** (transactor-as-dependency removal, outcome-vs-error representation, direct idempotency/sessionlock mechanism usage, simplified Claim contract, step-local idempotency interpretation, entity-not-row naming, audit-timestamp removal). This note is preserved as a factual pointer to what the prior (also 2026-09-17) implementation pass actually did, before this same-day revision - it remains accurate evidence of the 2026-09-16 design's implementation, not of the current target design.

This note is a factual pointer to current reality. **Superseded by the 2026-09-17 implementation pass**: the Create/Join/Leave implementation has been migrated to the 2026-09-16 revised Approved Design below. `game/session/workflows/sessionlifecycle/` is now one package exposing one `Manager` (`manager.go`) with `Create`/`Join`/`Leave` steps split by file (`step_create.go`/`step_join.go`/`step_leave.go`), explicit-parameter methods (`types.go` holds the semantic parameter/result types), a `transactor` abstraction the Manager uses to own transaction scope (`manager.go`), Manager-owned expiration policy (`expiration.go`) and idempotency-replay policy (`idempotency.go`), and a narrow `internal/repo/` persistence layer (`repo.go`/`session.go`/`actor.go`/`participant.go`/`join_code.go`/`request.go`) whose methods are named for the persistence operation they perform, not the business command. The previously-implemented three sibling packages (`createsession`/`joinsession`/`leavesession`) were removed. `game/session/internal/sessionlock` was narrowed to locking-fact-reporting only (`LockByID`/`LockByUUID`); its prior `MaterializeExpirationIfDue` (which decided expiration policy from inside the lock primitive) was removed and its decision now lives in the Manager's `expiration.go`. Join is now token-aware per GAME-ADR-0021: a differently-tokened Join while already an active Participant is rejected `ErrAlreadyJoined` (new sentinel), and a deterministic post-claim rejection (`AlreadyJoined`, lobby-full, Leave's actor-not-found) commits its idempotency completion (with the rejection as its outcome) in the same transaction rather than being rolled back with the claim, per the Transaction Ownership standard - this also fixes a genuine pre-revision correctness bug where a rejected operation's callback error caused `utils.RunInDBTransaction` to roll back an already-performed lazy-expiration materialization in the same transaction (see Local Implementation Decisions in the latest IMPLEMENTATION REPORT). `game/game/usecases/getgamedefinition/` (the pinned-definition Game Management capability) is unchanged - it already existed from the prior pass and already matched the revised design. Repository-level (`internal/repo`) and Manager-level (mocked-collaborator, real-Postgres integration, and concurrency) tests all exist and compile; the mocked-collaborator tests pass in this sandbox. The real-Postgres integration/concurrency tests remain **unexecuted** in this sandbox (no reachable Docker/Postgres engine - `TEST_DATABASE_*` env vars unset, tests self-report `SKIP`), the same known limitation recorded by every prior pass. Independent review per `docs/ai/protocols/IMPLEMENTATION_REVIEW.md` has still not been performed. See `game/CURRENT_STATE.md` and `game/docs/FLOWS.md` for the current actual-code-reality description (synchronized by this pass).

**Standard-compliance migration gate (2026-09-11 scope): CLOSED** - see "Standard-Compliance Migration Record" below; that migration remains valid and is not undone by this revision. **2026-09-16 design-revision migration gate: CLOSED by the 2026-09-17 implementation pass** (Manager/workflow restructuring, explicit parameters, transaction ownership, repository naming, expiration/admission ownership, token-aware Join idempotency - all implemented; see the Implementation Status Note above). **2026-09-17 transaction/outcome/mechanism refinement gate: CLOSED by the 2026-09-18 implementation pass** (transactor removal, generic callback return, outcome-vs-error representation, direct `sessionlock`/`idempotency` usage, simplified Claim contract, non-error Postgres claim mechanic, step-local idempotency interpretation, entity-not-row naming, audit-timestamp removal - all implemented; see the 2026-09-18 Implementation Status Note above). **Independent implementation review: CLOSED, verdict APPROVED** (no unresolved REQUIRED_FIX/DECISION_REQUIRED findings). **Real-Postgres integration/concurrency verification gate: CLOSED by the 2026-09-18 real-Postgres verification pass** - see "Verification Performed So Far (2026-09-18 real-Postgres verification pass, post-independent-review)" above; every `game/session/...` integration/concurrency test now actually passes against a real PostgreSQL instance. **All closure gates are now satisfied; this WORK is DONE** - see Completion Record below.

The "Context" section immediately below describes the codebase as it was found before this WORK's own implementation pass (2026-09-08) and is preserved as historical evidence for why the work was scoped this way - it is not current state.

## Standard-Compliance Migration Record (2026-09-11)

A human-approved engineering-standard clarification (`docs/engineering/standards/domain-logic-placement.md`/`repositories.md`, 2026-09-09) required a targeted migration of this implementation before Slice 2 could continue, recorded in `docs/projects/active/session-runtime-v1/internal/AI_CONTEXT.md` -> "Mandatory Standard-Compliance Migration". That migration is now complete:

- `game/session/usecases/{createsession,joinsession,leavesession}/` moved, unchanged in behavior, to `game/session/workflows/sessionlifecycle/{createsession,joinsession,leavesession}/` - a discoverable Session-lifecycle workflow package, per `domain-logic-placement.md`'s Workflow vs. Use Case guidance. Each operation keeps its own focused `service.go`/`repo.go`/tests; no combined lifecycle service/struct was introduced.
- The horizontal `game/session/internal/actors` package (shared `Find`/`Create`/`FindParticipant`/`CreateParticipant`/`ReactivateParticipant`/`RefreshActiveDisplayName`/`DeactivateParticipant`/`CountActive` consumed directly by both Join and Leave) was removed. Its behavior was migrated, unchanged in semantics and SQL, into behavior-local unexported functions inside each consumer: `joinsession/repo.go` now owns its own `findActor`/`createActor`/`findParticipant`/`createParticipant`/`reactivateParticipant`/`refreshActiveDisplayName`/`countActiveParticipants`; `leavesession/repo.go` owns only the read-side subset it actually needs (`findActor`/`findParticipant`/`deactivateParticipant` - Leave never creates/admits). The small duplication between Join's and Leave's `findActor`/`findParticipant` is intentional per `repositories.md`'s Sharing Rule - both query the same tables today, but remain separate behavior-local consumers whose needs may diverge independently, not a shared horizontal entity API.
- `createsession` does not touch `session_actors`/`session_participants` beyond the host actor it creates directly in its own transaction (see its `repo.go`), so it never depended on `internal/actors` and required no equivalent change beyond the package move.
- Legitimate shared cross-cutting mechanisms were left exactly as they were: `game/session/internal/sessionlock`, `game/session/internal/idempotency`, and `game/session/internal/pgerrs` remain shared internal packages, since they implement genuine protocols/mechanics (locking, idempotency, DB error classification) rather than a horizontal entity API.
- No behavior changed: same SQL, same transaction boundaries, same public `Input`/`Result`/error types, same acceptance criteria. Only package location and where the small persistence helpers live changed.
- No GAME-ADR-0001 boundary changed: Game Management/Session Runtime transaction independence, and Game Management reads completing before the Session mutation transaction opens, are unaffected by this purely internal reorganization.

## Outcome

A Session can be created, joined, left, and reconstructed from durable storage using the accepted Session/Actor/Participant/lobby identity model and per-Session DB-locking transactional rules. This replaces the current scaffolding, which does not compile and directly contradicts accepted architecture. It is the durable lobby foundation every later Session Runtime capability (Start/RuntimeTurn, interaction processing, timers, disconnect/reconnect, the live Coordinator) depends on.

No Game execution begins in this work. No WebSocket/runtime execution is implemented in this work. A completed WORK-0001 ends with a correct durable LOBBY, not a running game.

## Context

`session-runtime-v1`'s Architecture Discussion is CLOSED (human-approved); the initiative is in Implementation Planning with an approved 10-slice sequence (`PLAN.md`). This is Slice 1.

Actual codebase inspection (2026-09-08, before this WORK's own implementation pass) confirmed the drift already tracked in `PLAN.md`. **Historical evidence, superseded by this WORK's own implementation - see the Implementation Status Note above for current reality:**

- `game/session/workflows/sessionlifecycle/internal/repo/step_create_room.go` declared `func (r *Repo) CreateRoom(ctx context.Context)` with **no function body** - the `game/session/...` tree did not compile at that time. This package tree no longer exists.
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

### Workflow Package Structure (2026-09-16 revision)

Create/Join/Leave (and later Start) are steps of one Session lifecycle workflow and are organized as one workflow package exposing one discoverable controller, per `domain-logic-placement.md -> Preferred Workflow Package Shape`, superseding the previously-approved three-sibling-package layout:

```text
game/session/workflows/sessionlifecycle/
    manager.go
    step_create.go
    step_join.go
    step_leave.go
    expiration.go
    idempotency.go
    internal/
        repo/
            repo.go
            session.go
            actor.go
            participant.go
            join_code.go
            request.go
```

Exact filenames are implementation freedom; the invariants are: one `sessionlifecycle` package, one `Manager` type exposing `Create`/`Join`/`Leave` (and later `Start`) as its methods, implementation split by step into separate files, and a narrow `internal/repo` persistence layer the Manager calls into - not three independently-discoverable sibling packages, and not a giant `Manager` accreting unrelated non-lifecycle capabilities (see `domain-logic-placement.md -> Workflow Controller vs Domain-Wide God Service`). `step_start.go` is added when Slice 2 begins; it is not part of this WORK's scope.

`Manager` may depend on: its own `internal/repo` persistence repository/repositories; the Game Management pinned-definition read capability; a clock; a JoinCode generator; and other already-approved collaborators. It must not depend on a generically-named `GeneralPurposeAPI`/`CommonRepository`/`BaseRepository`. Per the 2026-09-17 Design Revision above, `Manager` does **not** depend on a separate injected `transactor` object whose only purpose is to reach the existing generic transaction helper (`utils.RunInDBTransaction`) - transaction execution is persistence/infrastructure mechanics, called directly, not a modeled business/application dependency (`docs/engineering/standards/repositories.md -> Transaction Ownership`).

### Function Contract Convention (2026-09-16 revision)

`Manager` methods use explicit parameters per `function-signatures.md`, not a ceremonial `Input`/`Params` bundling struct:

```go
func (m *Manager) Join(
    ctx context.Context,
    joinCode JoinCode,
    userUUID UserUUID,
    displayName DisplayName,
    idempotencyKey IdempotencyKey,
) (JoinResult, error)
```

This supersedes the previously-implemented `leaveParams`-style structs (and any equivalent Join/Create params struct), which exist only to bundle method arguments and are removed from the target design. A struct remains appropriate only when it represents a genuine, cohesive concept (e.g. a `CreatedSession` result), not merely to reduce a parameter count - see `function-signatures.md`.

### Persistence Model (Slice 1 subset of `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md`)

- **`sessions`**: `id`, `uuid`, `game_definition_uuid` (logical reference, no FK - the *pinned, immutable* Version/Definition UUID resolved exactly once at Create; see Pinned Game Definition below), `host_actor_id` (references `session_actors.id`; **nullable at storage level only** - see Host/SessionActor Creation Cycle below), `phase`, `lobby_expires_at`, `started_at` (nullable, always `NULL` after this work - no Session created here ever reaches `RUNNING`), `terminal_at` (nullable), `terminal_reason` (nullable), `created_at`, `updated_at`. `phase` needs only `LOBBY`/`TERMINAL` values for this work, but its representation should not hard-code an exhaustive enum that later slices (`RUNNING`) would have to migrate around.
- **`session_actors`**: `id`, `session_id`, `user_uuid` (logical reference, no FK), `semantic_presence`, `created_at`; unique `(session_id, user_uuid)`. A successful normal Join begins `semantic_presence = CONNECTED` (GAME-ADR-0015's accepted default) with no physical connection tracking of any kind.
- **`session_participants`**: `id`, `session_actor_id`, `display_name`, `active`, `joined_at`, `left_at` (nullable).
- **`join_codes`**: `id`, `session_id`, `code`, `created_at`, `revoked_at` (nullable) - authoritative Session persistence, not cache-only; one active code per lobby; revoked (not deleted) when the lobby is no longer admissible; historical/revoked codes retained.
- **`session_requests`**: `id`, `operation`, `idempotency_key`, `user_uuid`, `session_id` (nullable - a `CREATE` request may not yet have a resulting session at the moment of dedup lookup for a rejected/failed attempt, though a successful `CREATE` should populate it), `request_payload`, `outcome`, `response_payload`, `status` (distinguishes a completed logical outcome from an in-progress claim - see Concurrent Create Correctness and Completed-Outcome-vs-Transient-Failure below; exact value set is implementation freedom, e.g. `PENDING`/`COMPLETED`, or an equivalent that the DB-level uniqueness/claim mechanism can key off of), `created_at`. Unique `(user_uuid, operation, idempotency_key)` - **not** `(operation, idempotency_key)` alone, since two different users may independently produce the same opaque idempotency key with no collision; the same `(user_uuid, operation, idempotency_key)` identifies one logical operation. Only `CREATE`/`JOIN`/`LEAVE` operations are handled here; `START` support is explicitly deferred to Slice 2.

`game_definition_uuid` is a logical cross-capability reference to Game Management, within the same Game bounded context (GAME-ADR-0001) - not a cross-domain reference. `user_uuid` is a logical cross-domain reference to Identity, a different bounded context (`cross-domain-reference-naming.md`). Both remain logical references only - no database foreign keys crossing either boundary, and no direct queries against Game Management or Identity tables from Session persistence code.

### Pinned Game Definition Is Immutable For The Session

This is a correctness requirement, not an optimization. `CreateSession` resolves the requested public Game UUID to its **current** playable immutable Definition/Version UUID exactly once, and persists that resolved UUID as `sessions.game_definition_uuid`. From that moment on, every operation for that Session - Join in this work, Start/RuntimeTurn in later slices - reads and enforces *that pinned definition*, never the game's current version. A later publication of a new version (`games.current_definition_id` changing) must not alter lobby capacity, Start behavior, the Game Language definition, or any other semantics of an already-created Session.

Worked example: Create Session S while the Game's current version is V1 - S pins `game_definition_uuid = V1`. The author later publishes V2. Join against S must still enforce `V1.players.max`, never `V2.players.max`.

This is why a second Game Management read capability is required (see Game Management Dependency below): the existing `getgame.GetPlayableGameWithCurrentVersion` is Create-only - it is architecturally wrong to call it again after Create, because it always resolves through `games.current_definition_id`, which is exactly the value that must *not* influence an already-created Session.

### Database Locking / Serialization Design

- **Lock anchor**: the `sessions` row itself, selected `FOR UPDATE` by `id` inside the mutation's transaction.
- **Transaction boundary**: the Manager decides transaction scope; BEGIN/COMMIT/ROLLBACK is performed by calling the existing generic transaction helper directly - see Transaction Ownership below. Reuse `utils.RunInDBTransaction` (existing generic helper; `sql.LevelRepeatableRead`), the same mechanism already used elsewhere in the repository, rather than inventing a second transaction helper or wrapping it in an injected `transactor` dependency.
- **Responsible mechanism**: the shared `sessionlock` package's locking function (e.g. `sessionlock.LockByID(ctx, tx, sessionID)`/`LockByUUID`) selects-for-update the `sessions` row and returns its current `phase`/`lobby_expires_at` as facts. `Join`, `Leave`, and lazy-expiration materialization all call this same shared mechanism before mutating - they must not each invent their own locking query. Per the 2026-09-17 Design Revision above, the Manager's steps call `sessionlock.LockByID`/`LockByUUID` **directly**; `internal/repo` does not carry a `LockSessionByID`/`LockSessionByUUID` method whose entire body only forwards to it - such a forwarding method adds indirection without adding meaning (`docs/engineering/standards/repositories.md -> Sharing Rule`). The workflow-facing return value is named/treated as the locked Session's facts (e.g. `session`/`lockedSession`), not as a generic `row` (`repositories.md -> Naming Exposed To Workflow Code`). `Create` does not use this mechanism, because it has no existing `sessions` row to contend on - its own concurrency correctness comes from the idempotency-identity claim described in Concurrent Create Correctness below, not from row locking.
- **This primitive owns locking only, not the expiration decision.** `sessionlock.LockByID`/`LockByUUID` return the locked row's current facts; the package must not itself decide "phase is LOBBY and now is at or after `lobby_expires_at`, therefore materialize TERMINAL" - that decision belongs to the Manager, per Expiration Ownership below. This narrows the previously-implemented `sessionlock.MaterializeExpirationIfDue`, which decided lifecycle-expiration policy from inside the shared locking primitive.
- This primitive must be shaped so Slice 2+ can reuse it unchanged for RUNNING serialization (GAME-ADR-0018) - it must not become a lobby-only abstraction that later work has to replace.
- Rejected for V1 (per GAME-ADR-0004/GAME-ADR-0018, reaffirmed here): Redis or another distributed lock, process ownership, sticky routing, in-memory actor correctness.

### Transaction Ownership (2026-09-16 revision; transactor/outcome shape refined 2026-09-17)

Per `repositories.md -> Transaction Ownership`: the Manager decides which operations constitute one logical atomic transaction (e.g. for Join: lock, lazy-expiration materialization if due, idempotency claim/replay, admission decision, Actor/Participant persistence, idempotency completion). Transaction execution (BEGIN/COMMIT/ROLLBACK) is performed by calling the existing generic transaction helper (`utils.RunInDBTransaction[T](ctx, dbServicer, callback)`, or a thin persistence-package function that hides its `DBServicer` mechanics) **directly** - the Manager does not call `db.Begin()`/`tx.Commit()`/`tx.Rollback()` itself, and, per the 2026-09-17 Design Revision above, does **not** hold a separate injected `transactor` interface/field whose only purpose is to indirect into that already-generic helper. Repository methods consumed inside that transaction use the caller-supplied transaction-scoped handle and never independently open/commit their own transaction.

**Callback contract (2026-09-17 refinement).** The callback returns its result explicitly, per `function-signatures.md -> Callback-Based Helpers Return Values, Not Mutate Closures`:

```go
result, err := RunInTransaction(
    ctx,
    repo,
    func(ctx context.Context, tx *gorm.DB) (JoinResult, error) {
        ...
        return result, nil
    },
)
```

Do not declare an outer-scoped `result`/`bizErr`-style variable and mutate it from inside the callback when the callback's own return value expresses the same flow. The commit rule stays simple: `(result, nil)` from the callback commits; `(zeroResult, err)` rolls back; if COMMIT itself then fails, the transaction helper returns that commit error. There is no separate business-error channel and no `CommitWithError`-style method.

**A deterministic business decline is a value, not a rollback trigger** (GAME-ADR-0022, `error-handling.md -> Expected Business Outcome vs. Error`). `AlreadyJoined`, capacity exceeded, an already-expired lobby, and non-`LOBBY` Leave are expected outcomes of a *correctly evaluated* operation - not execution failures. A step that discovers one of these declines, after already performing an intended durable outcome that must survive in the same transaction (lazy lobby-expiration materialization, an idempotency-request completion recording the decline itself), persists that durable outcome and then returns it as a normal successful callback result, e.g. `JoinResult{Outcome: LobbyExpired}, nil` - letting the transaction commit through the ordinary success path. A non-nil callback `error` is reserved for cases the operation could not be correctly executed/evaluated at all (infrastructure failure, a violated invariant) - see `error-handling.md`.

### Repository Responsibility (2026-09-16 revision; direct-mechanism usage 2026-09-17)

Per `repositories.md -> Repository Naming`: repository methods are named for the persistence/data operation they perform, not the business command (`JoinSession`/`LeaveSession`-style names are removed from the target design). The Manager/workflow layer decides business/lifecycle policy (admission, expiration, idempotency-replay meaning); `internal/repo` reports facts (existing Actor/Participant rows) and performs the Session/Actor/Participant/JoinCode mutations the Manager requests (e.g., conceptually: `SetSessionTerminal`, `CreateSessionWithHost`, `CreateJoinCode`, `RevokeActiveJoinCode`, `FindActor`, `CreateActor`, `FindParticipant`, `CountActiveParticipants`, `CreateParticipant`, `ActivateParticipant`, `DeactivateParticipant`). Exact naming is implementation freedom; the invariant is the naming axis (business action vs. persistence operation), not this specific vocabulary. This supersedes the previously-implemented `repo.leaveSession(...)`/`repo.joinSession(...)`-style single methods that performed the entire business decision and persistence together inside the repository.

**Direct mechanism usage (2026-09-17 Design Revision).** `internal/repo` does **not** carry `LockSessionByID`/`LockSessionByUUID` or `ClaimSessionRequest`/`CompleteSessionRequest` methods whose entire body only forwards to the shared `sessionlock`/`idempotency` packages - each Manager step calls `sessionlock.LockByID`/`LockByUUID` and `idempotency.Claim`/`idempotency.Complete` directly, the same way it would call a domain type's method, per `repositories.md -> Sharing Rule`. A repository method remains justified when it does persistence work beyond forwarding (e.g. `CreateSessionWithHost`'s multi-statement encapsulation below) - the removed methods did none.

This does not require CRUD-minimal repository methods - a persistence-oriented method may still encapsulate several statements (e.g. `CreateSessionWithHost` inserting the Session, host Actor, and connecting them) per `repositories.md -> Multi-Table Persistence Encapsulation`.

### Expiration Ownership (2026-09-16 revision)

The Manager decides lazy lobby-expiration materialization: after obtaining the locked row's facts (`phase`, `lobby_expires_at`) from `LockSession`, the Manager itself evaluates `phase == LOBBY && now >= lobby_expires_at` and, if true, requests the repository perform the resulting mutations (set terminal state, revoke the active JoinCode) within the same transaction, then rejects the originally attempted action. The repository/lock primitive must not make this decision on the Manager's behalf (see Database Locking above).

### Admission Ownership (2026-09-16 revision)

The Manager decides Join admission: current lifecycle phase validity, expiration, `players.max` capacity from the pinned Game definition, and whether an already-admitted Participant means the current command should replay, succeed as a no-op-equivalent, or be rejected as `AlreadyJoined` (see Idempotency Design below and GAME-ADR-0021). The repository only supplies the facts (current active-Participant count, existing Actor/Participant rows) and performs the requested persistence (create/reactivate Participant). The repository must not itself decide whether admission is currently allowed.

### Join Outcomes Are Result Values (2026-09-17 revision, GAME-ADR-0022)

A correctly evaluated Join produces one of four outcomes, all represented as a value on `JoinResult` (conceptually `JoinResult{Outcome: ...}`; exact Go naming/shape is implementation freedom) alongside a `nil` error - none of these four are modeled as a returned Go `error`:

- `JOINED` - admission succeeded.
- `LOBBY_EXPIRED` - the lobby was discovered already expired and materialized `TERMINAL` in the same transaction.
- `LOBBY_FULL` - admission would exceed the pinned Definition's `players.max`.
- `ALREADY_JOINED` - a differently-tokened Join while already an active Participant, evaluated as a new command against current state (GAME-ADR-0021).

A same-token retry replays the exact previously completed outcome above as a value (`idempotency.md -> Completed Outcomes vs. Transient Failures`); a same-token conflicting-payload retry remains an idempotency-conflict `error`; a missing/empty idempotency token remains an `error` (invalid command/protocol contract). Create's and Leave's own already-accepted deterministic declines (e.g. Leave outside `LOBBY`) are represented the same way, using each operation's own result type - this is a representation change only, not new Create/Leave/Join domain behavior. See `docs/engineering/standards/error-handling.md -> Expected Business Outcome vs. Error` and GAME-ADR-0022 for the general rule this applies.

### Idempotency Design

**Namespace.** Idempotency identity is `(user_uuid, operation, idempotency_key)`, enforced by a unique constraint on that triple - **not** `(operation, idempotency_key)` alone. An idempotency key is scoped to its caller: two different users may independently produce the same opaque key with no collision, since the key only has to be unique within one user's own logical command stream.

**Per-operation meaningful fields** (explicitly modeled structs, not a generic canonical-JSON diff):
- `CREATE`: `GameUUID`, plus the host `UserUUID` already implied by the idempotency identity itself.
- `JOIN`: `JoinCode`, `UserUUID`, display-name snapshot.
- `LEAVE`: `SessionUUID`, `UserUUID`.

Exact struct/field naming is implementation-local.

**Replay vs. conflict vs. new command** (per `docs/engineering/standards/idempotency.md`). On a mutating call, look up an existing `session_requests` row by `(user_uuid, operation, idempotency_key)`:
- if found, `status = COMPLETED` (or equivalent), and the incoming request's meaningful fields match the stored ones - return the stored `outcome`/`response_payload` as an idempotent replay;
- if found, completed, but the meaningful fields differ - reject with a dedicated conflict sentinel error; do not silently apply the new request or silently return the old result;
- if found but not yet completed (see Completed Outcomes vs. Transient Failures below) - see that section for how this is resolved rather than treated as an immediate conflict or an immediate replay;
- **if not found (a token not previously used by this user for this operation) - this is always a new logical command, evaluated against current business state, never inferred to be a retry merely because the resulting state already holds.** For `JOIN` specifically (GAME-ADR-0021): a new token while the caller is already an active Participant is rejected with the `AlreadyJoined`-equivalent business error - it is not silently treated as success, and it is not treated as a replay of whichever token originally admitted the Participant.

Do not build a generic JSON-canonicalization framework; model comparison narrowly per operation. The shared `idempotency` package stores the claim/request, payload, and outcome; it must not decide what a replay/conflict/new-command means - that decision belongs to the Manager (`domain-logic-placement.md -> Responsibility Categories`, `idempotency.md -> Idempotency Policy Ownership`).

**Business outcomes are replayed as values, not recreated as errors (2026-09-17 revision, GAME-ADR-0022).** A completed outcome that was itself a deterministic business decline (`LOBBY_EXPIRED`, `LOBBY_FULL`, `ALREADY_JOINED`, and Leave's equivalent declines) is replayed by returning that decline as a normal result value (e.g. `JoinResult{Outcome: AlreadyJoined}, nil`), not by reconstructing a Go `error`. Only genuinely invalid idempotency usage - a missing/empty token, a same-token conflicting request, corrupt/unreadable persisted idempotency state, or a persistence failure - remains an `error` (`idempotency.md -> Completed Outcomes vs. Transient Failures`).

### Claim Mechanism And Step-Local Interpretation (2026-09-17 revision)

**Direct usage, no repository forwarding.** Each Manager step calls the shared `idempotency` package's claim/complete functions directly (see Repository Responsibility above); `internal/repo` does not carry a forwarding `ClaimSessionRequest`/`CompleteSessionRequest` method.

**Simplified Claim contract.** `idempotency.Claim` returns `(requestID, existingRequest, error)`, dropping the redundant `claimed bool`: `requestID != 0 && existingRequest == nil` means the caller acquired a fresh claim; `requestID == 0 && existingRequest != nil` means the identity already existed. The shape returned to workflow code is named `idempotency.Request`, not `idempotency.Row` (`repositories.md -> Naming Exposed To Workflow Code`); a private GORM model internal to the `idempotency` package's own implementation may still be named as a row.

**PostgreSQL conflict mechanic.** The claim insert uses a non-error conflict path - conceptually `INSERT INTO session_requests (...) VALUES (...) ON CONFLICT (user_uuid, operation, idempotency_key) DO NOTHING RETURNING id` - rather than deliberately inserting, catching the resulting unique-constraint violation, and then querying for the existing row through the same (now error-carrying) transaction. If an id is returned, the claim is fresh; if no row is returned, the existing request is read via an ordinary `SELECT`. An equivalent PostgreSQL-safe mechanism providing the same guarantee (exactly one fresh claim among concurrent attempts, the others observing the existing identity, with the transaction remaining usable afterward either way) is acceptable. Concurrency/integration test coverage proving this - not merely a test that happens to pass under low contention - is required (see Verification/Acceptance Criteria below).

**Step-local interpretation.** Per-operation idempotency payload structs (`createRequestPayload`/`joinRequestPayload`/`leaveRequestPayload`) and their interpretation functions (`interpretExistingCreateClaim`/`interpretExistingJoinClaim`/`interpretExistingLeaveClaim`) live next to the step that owns that command (`step_create.go`/`step_join.go`/`step_leave.go` respectively), not centralized in one shared workflow-level `idempotency.go` bucket file - the claim/complete mechanism is shared; the command-specific interpretation is not (`idempotency.md -> Organizational Guidance`). A small, genuinely mechanical helper with no command-specific interpretation of its own (e.g. `marshalPayload`) may remain shared.

### Concurrent Create Correctness

`CreateSession` has no pre-existing `sessions` row to lock, so its concurrency correctness cannot come from the row-locking mechanism above - it must come from claiming the idempotency identity itself before any Session-creation effect happens. Required logical outcome for two concurrent `CreateSession` calls sharing the same `(user_uuid, CREATE, idempotency_key)`: **exactly one** Session-creation effect occurs, and both calls eventually observe/return the same logical result. It is not acceptable for both calls to create separate Sessions, nor for the idempotency uniqueness violation to be discovered only after both Session-creation effects have already happened.

Approach: within `CreateSession`'s transaction, attempt to claim the `session_requests` identity (`user_uuid`, `operation = CREATE`, `idempotency_key`, `request_payload`, `status = PENDING` or equivalent) *before* creating the Session/actor/join-code artifacts, via `idempotency.Claim`'s non-error Postgres conflict path (see Claim Mechanism And Step-Local Interpretation above) relying on the `(user_uuid, operation, idempotency_key)` unique constraint to serialize concurrent claims at the database level:
- if the claim is fresh (an id was returned), this call owns the claim - proceed to create the Session/host actor/join code, then update the `session_requests` row to `status = COMPLETED` with the resulting `outcome`/`response_payload`, all in the same transaction;
- if the claim finds an existing request (no id returned), another call already claimed (or is claiming) this identity - inspect the existing request: if it is already `COMPLETED`, replay its stored outcome (after confirming the meaningful fields match, per Idempotency Design above); if it is not yet `COMPLETED`, see Completed Outcomes vs. Transient Failures below for how to handle a claim that is still in flight or was abandoned.

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
- no database foreign key crossing into Game Management's persistence is introduced (`game_definition_uuid` is a cross-capability reference within the same Game bounded context, not a cross-domain one - GAME-ADR-0001);
- this is the minimum addition needed to make Join (and later Start) correct - it does not redesign Game Management's existing ownership, publish/lifecycle behavior, or storage.

Per GAME-ADR-0001, Game Management capability calls required for a Session mutation must complete **before** opening the Session write/locking transaction; Session Runtime must not hold its mutation transaction/row lock while invoking Game Management. Join therefore performs this Game Management read before opening its mutation transaction, not during it. (The pinned `game_definition_uuid` is immutable, so the definition itself cannot change underneath the Session regardless of exactly when it's read relative to the lock - that immutability is why sequencing the read first is always safe, not a justification for reading it while the lock is held.)

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

### Timestamp Ownership (2026-09-17 revision)

Per `docs/engineering/standards/repositories.md -> Timestamp Ownership`: repository methods stop accepting a `now` parameter whose only use is stamping an audit timestamp (`created_at`/`updated_at`). This affects the Host/SessionActor Creation Cycle's own persistence methods and JoinCode creation specifically: the method that inserts the `sessions` row (step 2 above) and the method that inserts the host `session_actors` row (step 4 above) no longer take a `now` argument for `created_at`/`updated_at` - the repository/ORM/DB populates those columns itself. The same applies to JoinCode creation's `created_at`. `lobby_expires_at` (already an explicit, separately-named parameter) is unaffected - it is semantic lifecycle state, not audit metadata, and remains required Manager-supplied input. Likewise, the method that materializes lobby expiration keeps `terminalAt` (the semantic deadline) as an explicit parameter but drops any separate audit-only `now`; `revoked_at`/`joined_at`/`left_at` remain explicit, meaningfully-named semantic inputs the Manager supplies (they represent the logical revocation/join/leave event, not bookkeeping).

### Manager Operation Behavior

Each bullet below is `Manager` orchestration (explicit parameters, per Function Contract Convention above): it decides business/lifecycle policy and transaction scope, and calls the narrow persistence-oriented repository methods described in Repository Responsibility above to read facts and perform mutations. None of this orchestration lives inside a single repository method.

- **Create**: claim the idempotency identity first via `idempotency.Claim` (see Concurrent Create Correctness above), resolve/compile/pin the current playable Game Definition (see Pinned Game Definition and Game Management Dependency above), then within the **same** transaction run the Host/SessionActor Creation Cycle above and create an active `JoinCode` with `lobby_expires_at = now + <lobby TTL>`. Do **not** create a `session_participants` row for the host - the host is never automatically a Participant. The successful logical result is atomic across all of: the `sessions` row, the host `SessionActor`, `sessions.host_actor_id` being set, the active `JoinCode`, `lobby_expires_at`, and the completed `session_requests` outcome - all in one transaction, all-or-nothing.
- **Join**: resolve the `JoinCode` to its Session; obtain the Session's DB mutation lock (`FOR UPDATE`) via `sessionlock.LockByID` directly; evaluate lazy lobby-expiration itself (Expiration Ownership above) - if due, materialize `TERMINAL` in the same transaction and return the `LOBBY_EXPIRED` outcome (a value, not an error - GAME-ADR-0022); claim/inspect the idempotency request via `idempotency.Claim` directly (Idempotency Design above) - same token + same fields replays the stored outcome as a value; same token + different fields is an idempotency-conflict `error`; a **new token evaluates current state**, and if the caller is already an active Participant, returns the `ALREADY_JOINED` outcome (GAME-ADR-0021/GAME-ADR-0022) rather than replaying or silently succeeding; otherwise read `sessions.game_definition_uuid`, load the **pinned** immutable definition through the new Game Management capability (never "current version" - see Pinned Game Definition above) to obtain `players.max`, find-or-create the `SessionActor` for `(session, user_uuid)`, enforce `players.max` from the pinned definition (returning `LOBBY_FULL` as a value if exceeded), create or reactivate the `Participant` (`active = true`, snapshot `display_name`), complete the idempotency outcome, and commit with the `JOINED` outcome.
- **Leave**: resolve `(session, user_uuid)` to `SessionActor`; lock the Session via `sessionlock.LockByID`/`LockByUUID` directly; evaluate lazy lobby-expiration itself; if the Session is not `LOBBY` (for this work, the only other reachable phase is `TERMINAL`), return the corresponding decline outcome as a value rather than an error, and rather than silently no-op-ing; otherwise deactivate the `Participant` (`active = false`, `left_at`), keep the `SessionActor` row, and never touch `host_actor_id` - a leaving host keeps host authority even though they stop participating.
- **Lazy lobby-expiration materialization**: the Manager, for any of the three operations above, that finds `phase == LOBBY && now >= lobby_expires_at` (from the facts `sessionlock.LockByID`/`LockByUUID` returned - see Expiration Ownership above) may, in the same transaction, request the repository set `phase = TERMINAL`, `terminal_at = lobby_expires_at` (the semantic deadline, not the current operation's wall-clock time), a terminal reason denoting lobby expiration, and revoke the active `JoinCode` - then return the originally attempted action's decline as a normal outcome value. No sweeper/background worker is implemented by this work; operation-level correctness is the whole mechanism, per GAME-ADR-0004. This materialization must commit together with that decline through the ordinary transaction-callback success path (Transaction Ownership above) - it must never be discarded by a rollback.

## Constraints and Invariants

- Session-local uniqueness: at most one active logical participation per `(session, user_uuid)` (GAME-ADR-0003).
- No database foreign keys and no direct table queries across persistence-ownership boundaries: Game Management vs. Session Runtime, a cross-capability boundary within the same Game bounded context (GAME-ADR-0001), and Session Runtime vs. Identity, a cross-domain boundary (`cross-domain-reference-naming.md`).
- Game Management reads required by a Session mutation (Join's pinned-definition lookup; later Start's) complete before Session Runtime opens its write/locking transaction - Session Runtime never holds its mutation transaction/row lock while calling into Game Management (GAME-ADR-0001).
- `CreateSession` never accepts an externally-supplied `engine.Program` (GAME-ADR-0004).
- Host is never automatically an active Participant (GAME-ADR-0004).
- `lobby_expires_at` is authoritative; no operation may revive an already-expired lobby merely because persisted `phase` still reads `LOBBY` (GAME-ADR-0004).
- `sessions.game_definition_uuid` is resolved exactly once, at Create, and is immutable for the Session's lifetime; no later operation (Join in this work; Start/RuntimeTurn in later slices) may re-resolve "current version" - see Pinned Game Definition Is Immutable For The Session above.
- Idempotency identity is `(user_uuid, operation, idempotency_key)`, not `(operation, idempotency_key)` - different users never collide on the same opaque key.
- A Join with a different idempotency token than any previously used is always a new logical command evaluated against current state; it is never inferred to be a retry merely because the resulting "already joined" state already holds (GAME-ADR-0021, `idempotency.md`).
- A committed Session must have a valid, non-null `host_actor_id`; `host_actor_id` is nullable only as a transient storage-level artifact of the Host/SessionActor Creation Cycle within one still-open transaction.
- Create/Join/Leave reject a missing/empty idempotency token as invalid input (`idempotency.md -> Required Token`).
- The Manager, not the repository, decides transaction scope, admission policy, and expiration policy (Transaction Ownership, Admission Ownership, Expiration Ownership above). No repository method independently opens/commits its own transaction.
- Repository methods are named for the persistence operation they perform, not the business command (`repositories.md -> Repository Naming`); no `JoinSession`/`LeaveSession`-style repository method decides business policy internally.
- Manager methods use explicit parameters, not ceremonial `Input`/`Params` structs (`function-signatures.md`).
- Repository queries follow `repositories.md`: raw SQL for reads/updates/deletes; `Create()` for inserts.
- The Manager does not depend on an injected `transactor` object; it calls the existing generic transaction helper directly (2026-09-17 revision, `repositories.md -> Transaction Ownership`).
- The transaction callback returns its result via the generic helper's own `(result, error)` return value; no outer-scoped result/error variable is mutated from inside the callback, and no separate business-error channel (`bizErr`, `CommitWithError`) exists alongside it (`function-signatures.md`, `repositories.md -> Transaction Ownership`).
- A deterministic Join/Create/Leave business decline (`LOBBY_EXPIRED`, `LOBBY_FULL`, `ALREADY_JOINED`, and Leave's equivalent declines) is a result-value outcome returned alongside a `nil` error, not a returned Go `error` (GAME-ADR-0022, `error-handling.md -> Expected Business Outcome vs. Error`); only an invalid command/protocol contract or an execution/infrastructure/invariant failure is an `error`.
- SessionLifecycle steps call the shared `sessionlock`/`idempotency` mechanism packages directly; no repository method exists solely to forward to them (`repositories.md -> Sharing Rule`).
- `idempotency.Claim` returns `(requestID, existingRequest, error)`, with no separate `claimed bool`; its PostgreSQL conflict path uses a non-error mechanism (e.g. `ON CONFLICT ... DO NOTHING RETURNING id`), not a deliberately-provoked unique-violation recovered through the same transaction (`idempotency.md -> Claim Mechanism Contract`).
- Per-operation idempotency payload/interpretation code lives next to the step that owns that command, not in one shared workflow-level bucket file (`idempotency.md -> Organizational Guidance`).
- Repository methods do not take a `now` parameter used only to populate an audit timestamp (`created_at`/`updated_at`); semantic timestamps (`joined_at`, `left_at`, `terminal_at`, `revoked_at`, `lobby_expires_at`) remain explicit, meaningfully-named parameters (`repositories.md -> Timestamp Ownership`).
- Workflow-facing repository return types/variables are named for the entity/fact they represent (e.g. `session`, `actor`, `participant`, `request`), not with a `Row` suffix; a private persistence struct internal to a repository implementation may still be named as a row (`repositories.md -> Naming Exposed To Workflow Code`).
- Errors follow `error-handling.md`: dedicated sentinel errors for intentional contracts (e.g. invalid/expired JoinCode, lobby full, conflicting idempotency key, non-`LOBBY` Leave), `%s` context by default rather than `%w` for incidental lower-level errors, logging at the appropriate boundary rather than every layer.
- Unexpected persisted-state inconsistency follows `data-integrity.md`: alert via `monitoring.Alert` and return a normal error; do not panic unless continuing would be genuinely unsafe.
- Tests follow `testing.md`: table-driven with underscore-separated case names, a shared disposable domain test database for repository tests (mirroring `game/game/internal/testdb`), mocked collaborator interfaces for service/use-case tests.

## Acceptance Criteria

**Method contracts (2026-09-16 addition)**
- Lifecycle application methods (`Manager.Create`/`.Join`/`.Leave`) use explicit parameters and do not use a ceremonial `Input`/`Params` struct solely to bundle required parameters (`function-signatures.md`).

**Transaction ownership (2026-09-16 addition)**
- The Manager, not the repository, defines the transaction containing each logical operation; repository calls do not independently commit.
- A failed logical operation (business rejection) never leaves a partial repository mutation from the same attempted operation surviving outside an intended durable outcome (lazy expiration materialization, idempotency completion).
- A deterministic business rejection (e.g. `AlreadyJoined`, capacity exceeded) can still commit a required lazy-expiration materialization or idempotency-completion outcome in the same transaction before returning the business error.

**Missing token (2026-09-16 addition)**
- Create/Join/Leave reject a missing/empty idempotency token as invalid input.

**Business outcomes as values, not errors (2026-09-17 addition; GAME-ADR-0022)**
- A successful Join returns the `JOINED` outcome with a `nil` error.
- An expired lobby returns the `LOBBY_EXPIRED` outcome with a `nil` error, and the lobby-expiration state (`TERMINAL`, JoinCode revocation) is committed in the same transaction.
- A full lobby returns the `LOBBY_FULL` outcome with a `nil` error.
- A new idempotency token while the caller is already an active Participant returns the `ALREADY_JOINED` outcome with a `nil` error.
- A same-token retry replays the exact prior outcome/result value, whether the original outcome was success or a decline.
- A same-token retry with a conflicting semantic payload is rejected as an idempotency-conflict `error`, not as a replayed outcome.
- Create's and Leave's own already-accepted deterministic declines are represented the same way (result-value outcome, `nil` error), using each operation's own result type.
- No test or production code path represents an already-accepted Join/Create/Leave business decline as a returned Go `error`.

**Transaction functional shape (2026-09-17 addition)**
- The transaction callback contract returns its result via the generic helper's own return value; no test or production code declares an outer-scoped result/error variable mutated from inside the callback, and no separate `bizErr`/`CommitWithError`-style channel exists.
- No repository method independently commits a transaction opened by the Manager; a business decline that must commit a durable outcome (lazy-expiration materialization, idempotency completion) does so through the callback's own successful return path, never through a separate commit call outside the Manager's transaction boundary.

**Direct mechanism usage and claim contract (2026-09-17 addition)**
- No repository method exists whose entire body only forwards to `sessionlock.LockByID`/`LockByUUID` or `idempotency.Claim`/`idempotency.Complete`; Manager steps call these directly.
- `idempotency.Claim`'s contract distinguishes a fresh claim from an existing request without a separate `claimed bool` (mutually exclusive `requestID`/`existingRequest`).
- The claim mechanism's PostgreSQL implementation uses a non-error conflict path; a concurrency/integration test proves two concurrent same-identity claims resolve to exactly one fresh claim, with the other observing the existing identity and the transaction remaining usable afterward - not merely a test that happens to pass under low contention.

**Row vs. entity naming and audit timestamps (2026-09-17 addition)**
- Workflow-facing repository return types/variables use entity/fact naming (`session`, `actor`, `participant`, `request`), not a `Row` suffix, in the Manager-visible contract.
- No repository method accepts a `now` parameter used only to populate `created_at`/`updated_at`; semantic timestamps (`joined_at`, `left_at`, `terminal_at`, `revoked_at`, `lobby_expires_at`) remain explicit workflow/clock-supplied inputs and are persisted exactly as supplied.

**Admission and expiration ownership (2026-09-16 addition)**
- Capacity (`players.max`) and lifecycle admission (phase/expiration validity) are decided in Manager code, not inside a repository method.
- The shared session-lock primitive returns locked-row facts only; it does not itself decide lobby-expiration policy.

**Repository naming/boundary (2026-09-16 addition)**
- No repository method named after a business command (e.g. `JoinSession`/`LeaveSession`) internally decides admission/expiration/idempotency-replay meaning; repository methods represent persistence/data operations.

**Create**
- Creates a `LOBBY` Session pinned to the correct resolved Game Definition/version.
- Creates the host `SessionActor`; the host is not created as an active Participant.
- Creates an active `JoinCode`.
- A repeated, semantically-equivalent Create using the same `(user_uuid, operation, idempotency_key)` returns the same logical result without creating a second Session.
- The same idempotency identity with a conflicting request is rejected.
- A Game Definition that fails to compile is rejected with a clear domain error, not a created-but-unusable Session.
- A successful Create commits with a valid, non-null `host_actor_id`, and the host `SessionActor` references that same Session; no partially-created Session (e.g. missing its host actor) survives a failed Create transaction.
- JoinCode creation is Manager-orchestrated (an explicit step after the host/actor persistence step) but part of the same atomic transaction as the rest of Create - not hidden inside the repository operation that creates the Session/host relationship.

**Join**
- A normal Join creates/reuses the `SessionActor` and activates a `Participant`.
- A repeated Join using the same idempotency token and the same semantic request (JoinCode, UserUUID, display name) replays the original outcome (no duplicate Participant/slot consumed).
- The same token with a different semantic request is rejected as an idempotency conflict.
- A Join using a **different** idempotency token while the caller is already an active Participant returns the `AlreadyJoined`-equivalent outcome (a result value, not an error) - it is a new command evaluated against current state, not silently replayed or silently treated as success (GAME-ADR-0021, GAME-ADR-0022).
- User A and User B may each independently reuse the same opaque idempotency-key value for their own Join without colliding (different `user_uuid`, different identity).
- The same `User` never ends up with two `SessionActor` rows for the same Session.
- `players.max` is enforced correctly under concurrent Joins racing for the final slot.
- Lobby expiration is enforced even though no sweeper exists.
- A revoked/invalid `JoinCode` is rejected.
- The participant's display name is persisted as a Session-scoped snapshot.
- **Pinned definition**: Create a Session against Game version V1; publish/change the Game's current version to V2 with a different `players.max`; Join the existing Session; Join's `players.max` enforcement is still governed by pinned V1, not V2. (Test fixtures may simulate the "current version changed" condition directly in Game Management test data rather than literally invoking a publication workflow, if simpler.)
- **Cross-capability read correctness**: Join loads the Game Definition by the Session's pinned `game_definition_uuid`, not by re-resolving the Game's current version; a test should fail if the implementation accidentally substitutes a "current version" lookup for the pinned-definition lookup.

**Leave**
- An active Participant can leave the lobby; their slot becomes available to others.
- The `SessionActor` remains durable after Leave.
- Host authority is retained if the leaving user is also host.
- A retried Leave with the same idempotency identity behaves idempotently.

**Concurrency**
- Concurrent Joins competing for the final slot resolve correctly (exactly one succeeds when only one slot remains) via the DB-locking mechanism, not application-level check-then-act races.
- A Join racing a Leave around capacity resolves correctly under the same locking mechanism.
- An operation racing lobby-expiration materialization resolves without ever reviving an expired lobby.
- **Concurrent Create**: two equivalent concurrent `Manager.Create` calls sharing the same `(user_uuid, CREATE, idempotency_key)` produce exactly one logical Session and both observe/replay the identical result - never two Sessions, and never a window where the duplicate is only discovered after both Session-creation effects already happened.

**Idempotency namespace and failure semantics**
- User A and User B independently using the same `operation`/`idempotency_key` value do not collide (different `user_uuid`, so different identity).
- The same `(user_uuid, operation, idempotency_key)` with a different, conflicting semantic payload (e.g. a different `JoinCode` or `GameUUID`) is rejected as a conflict, not silently replayed or silently applied.
- A transient infrastructure failure or transaction rollback before commit does not leave behind a completed, replayable logical outcome; a subsequent retry of the same identity executes as a fresh attempt rather than replaying a cached failure.

**Capability/domain-boundary and architecture guardrails**
- No test or production code path accepts an externally-supplied `engine.Program` into `Manager.Create`.
- No test or production code path uses `Identity.UserUUID` as engine/runtime identity in this slice (there is no engine/runtime identity yet - this guards against prematurely introducing one).
- No repository code issues a direct SQL query against a Game Management or Identity table, and no database foreign key crosses into Game Management's persistence (cross-capability, same Game bounded context) or Identity's persistence (cross-domain, a different bounded context).
- No Session mutation opens its write/locking transaction before its required Game Management read(s) complete, and Session Runtime never holds that transaction/row lock while calling into Game Management (GAME-ADR-0001).
- No test or production code path calls `getgame.GetPlayableGameWithCurrentVersion` (or any "current version" resolution) for anything other than `CreateSession`.

## Implementation Freedom

- Exact filenames below the workflow root, beyond the invariants stated in Workflow Package Structure above. **Historical**: this originally recommended the `usecases/<verb+noun>/` pattern (e.g. `game/session/usecases/createsession`), then (2026-09-11) three sibling packages under `workflows/sessionlifecycle/{createsession,joinsession,leavesession}/`. **Superseded 2026-09-16**: the current-implementation three-sibling-package layout is itself now superseded by the one-package-one-`Manager` structure in Workflow Package Structure above - this is required target structure for the next implementation pass, not merely a local option.
- The exact `lobby_expires_at` TTL value/source (a package-level constant is acceptable; no existing repository-wide configuration convention was found to reuse).
- The exact naming/shape of the new Game Management pinned-definition read capability (`GetGameDefinition` or equivalent) and its placement (new file in `getgame`, a new sibling usecase, etc.) - the required semantics are fixed (see Game Management Dependency above), the exact Go naming/package placement is not.
- Exact Go sentinel/error type names for genuine errors (idempotency conflict, missing token, infrastructure/invariant failures), and exact Go naming/shape of the outcome values a result carries (e.g. how `JoinResult`'s `Outcome` field is typed) - the required outcome set (`JOINED`/`LOBBY_EXPIRED`/`LOBBY_FULL`/`ALREADY_JOINED` for Join, and Create's/Leave's equivalents) is fixed by GAME-ADR-0021/GAME-ADR-0022; the Go representation is not.
- Exact SQL column types/lengths/indexes beyond what's specified above, following `repositories.md`.
- Exact `session_requests.status` value set/type (e.g. a small string enum vs. a boolean "completed" flag) as long as it can distinguish a completed outcome from anything else.

## Verification

- `go build ./...` must succeed, including `game/session/...` (was broken before this WORK's implementation; succeeds as of 2026-09-11).
- `go test ./game/session/...` must pass: repository integration tests against a disposable real Postgres database (requiring `TEST_DATABASE_HOST`/`TEST_DATABASE_PORT`/`TEST_DATABASE_USERNAME`/`TEST_DATABASE_PASSWORD`/`TEST_DATABASE_NAME`/`TEST_DATABASE_SSL_MODE`, consistent with `game/game/internal/testdb`'s existing pattern) plus service/use-case tests using generated mocks (`mockgen`, consistent with `getgame`'s `//go:generate mockgen` convention).
- At least one concurrency-specific test proving the DB-locking mechanism actually serializes concurrent Joins/Leaves rather than relying on incidental timing.
- At least one concurrency-specific test proving concurrent `CreateSession` calls sharing the same `(user_uuid, CREATE, idempotency_key)` produce exactly one Session and a consistent replayed result - not merely a test that happens to pass under low contention.
- Exact test-invocation commands follow whatever convention is already used for `game/game`'s equivalent tests - do not invent a new one.

### Verification Performed So Far (2026-09-11 reconciliation)

- `go build ./...` - succeeds repository-wide.
- `go vet ./...` - clean.
- `go test ./game/session/... ./game/game/...` - all service-level (mocked-collaborator) tests pass; every repository-integration and concurrency test (`TestRepoCreateSession*`, `TestRepoJoinSession*`, `TestRepoLeaveSession*`, `TestJoinSession_Concurrent*`, and the equivalent `getgame`/`getgamedefinition` repo tests) self-reports `SKIP` because `TEST_DATABASE_*` is unset - this sandbox has no reachable Postgres (Docker Desktop's engine is not running here). These tests exist and compile but have not actually been executed end-to-end against a real database in any environment used so far. Running them (`docker compose up -d postgres` with `DATABASE_USERNAME`/`DATABASE_PASSWORD`/`DATABASE_NAME`/`DATABASE_PORT` set, then the matching `TEST_DATABASE_*` vars, then `go test ./game/...`) remains required before independent review can treat the concurrency/integration acceptance criteria as verified.

### Verification Performed So Far (2026-09-17 design-revision implementation pass)

**Predates the same-day 2026-09-17 Design Revision above** (transactor removal, outcome-vs-error representation, direct mechanism usage, simplified Claim contract, step-local idempotency interpretation, entity-not-row naming, audit-timestamp removal); this verification evidence is accurate to the 2026-09-16 design's implementation, not to the current target design. It remains valid evidence that the codebase compiled/tested cleanly at that point.

- `go build ./...` - succeeds repository-wide.
- `go vet ./...` - clean repository-wide.
- `gofmt -l` - clean for all new/changed files.
- `go test ./game/... -count=1` - all mocked-collaborator tests pass, including the new `sessionlifecycle` package's `TestManagerCreate`/`TestManagerJoin`/`TestManagerLeave` (covering missing-token rejection, game-not-found/non-compiling-definition rejection, idempotency replay/conflict/in-flight, `AlreadyJoined`/lobby-full rejection with committed idempotency-completion outcome, and lazy-expiration materialization-then-rejection). Every repository-integration and concurrency test (`internal/repo`'s and `sessionlifecycle`'s `*_Integration*`/`*_Concurrent*` tests) self-reports `SKIP` - same known sandbox limitation as every prior pass (`docker ps` confirms Docker Desktop's engine is still not reachable here; `TEST_DATABASE_*` unset). These tests exist, compile, and were reviewed for correctness, but have not been executed end-to-end against a real database in this sandbox.

### Verification Performed So Far (2026-09-18 transaction/outcome/mechanism-refinement implementation pass)

Predates independent review. Implements the 2026-09-17 Design Revision above against the code the 2026-09-17 pass produced.

- `go build ./...` - succeeds repository-wide.
- `go vet ./...` - clean repository-wide.
- `gofmt -l` - clean for all files changed by this pass (three pre-existing, untouched-by-this-pass files elsewhere in `game/session` were already gofmt-dirty before this pass and are out of this pass's scope).
- `go test ./... -count=1` - clean repository-wide (no failures anywhere, not just `game/session`). All mocked-collaborator tests pass, including `sessionlifecycle`'s `TestManagerCreate`/`TestManagerJoin`/`TestManagerLeave` (now scoped to each operation's pre-transaction validation only - see Local Implementation Decisions in the IMPLEMENTATION REPORT for why) and two new tests added directly against the shared `game/session/internal/idempotency` and `game/session/internal/sessionlock` packages (`TestClaimAndComplete`, `TestClaim_ConcurrentSameIdentityResolvesToExactlyOneFreshClaim`, `TestLockByIDAndByUUID`). Every repository-integration and concurrency test (`internal/repo`'s, `internal/idempotency`'s, `internal/sessionlock`'s, and `sessionlifecycle`'s `*_Integration*`/`*_Concurrent*` tests) self-reports `SKIP` - `docker ps`/`docker version` confirm Docker Desktop's engine is still not reachable in this sandbox (`TEST_DATABASE_*` unset), the same known limitation recorded by every prior pass. These tests exist, compile, and were reviewed for correctness, but have not been executed end-to-end against a real database in this sandbox - this is a materially relevant limitation for this pass specifically because it changed the idempotency claim's PostgreSQL conflict mechanic (see Local Implementation Decisions in the IMPLEMENTATION REPORT), which real-Postgres execution would verify most directly.

### Verification Performed So Far (2026-09-18 real-Postgres verification pass, post-independent-review)

Independent implementation review per `docs/ai/protocols/IMPLEMENTATION_REVIEW.md` completed with verdict **APPROVED** (no unresolved REQUIRED_FIX/DECISION_REQUIRED findings). This pass closes the one remaining gate: real-Postgres execution of the repository-integration/concurrency tests every prior pass could only compile-check. This sandbox has a reachable Docker/Postgres engine (`playhoot-postgres-1`, `postgres:17-alpine`, already running via the repository's `docker-compose.yaml`), so `TEST_DATABASE_HOST=localhost`/`TEST_DATABASE_PORT=5432`/`TEST_DATABASE_USERNAME=playhoot`/`TEST_DATABASE_PASSWORD=DBPLAYHOOT`/`TEST_DATABASE_NAME=playhoot`/`TEST_DATABASE_SSL_MODE=disable` (matching the container's actual credentials) were exported and the full suite was actually executed end-to-end, not merely compiled:

- `go build ./...` - succeeds repository-wide.
- `go vet ./...` - clean repository-wide.
- `go test ./game/session/... -count=1 -v` - **all tests pass, none skipped**: every repository-integration test (`internal/repo`'s `TestRepoFindAndCreateActor`/`TestRepoResolveSessionForJoinCode`/`TestRepoCreateAndRevokeJoinCode`/`TestRepoParticipantLifecycle`/`TestRepoCreateSessionWithHost`/`TestRepoSetSessionTerminal`), every shared-mechanism test (`internal/idempotency`'s `TestClaimAndComplete`/`TestClaim_ConcurrentSameIdentityResolvesToExactlyOneFreshClaim`, `internal/sessionlock`'s `TestLockByIDAndByUUID`), and every `sessionlifecycle` Manager integration/concurrency test (`TestManagerCreate_Integration*` including `_Concurrent`, `TestManagerJoin_Integration*` including `_ConcurrentJoinsForFinalSlot`/`_ConcurrentJoinRacingLeave`/`_ConcurrentOperationRacingLobbyExpiration`, `TestManagerLeave_Integration*`) now actually executed against a real PostgreSQL instance and passed. This empirically verifies, for the first time, the acceptance criteria that could previously only be reasoned about: the `xmax`-based idempotency claim mechanic (concurrent same-identity claims resolve to exactly one fresh claim), the `CreateJoinCode` collision-retry fix, per-Session `FOR UPDATE` serialization for concurrent Joins/Leaves racing for the final slot and racing lobby expiration, and concurrent-Create producing exactly one Session with a consistent replayed result.
- `gofmt -l` on the one file changed by this pass - clean.
- `go test ./... -count=1` (repository-wide, broader than this WORK's own required `./game/session/...` scope) surfaced one pre-existing, out-of-scope failure and one in-scope one, both only exposed now because a real Postgres was reachable for the first time in any sandbox used across this WORK's entire history:
  - **In-scope, fixed**: `game/game/usecases/getgamedefinition` (the new Game Management read capability this WORK added per "Game Management Dependency") - `TestRepoGetGameDefinitionByUUID` asserted exact string equality between the seeded JSON literal and the value read back from the `game_definitions.script JSONB` column; PostgreSQL's `jsonb` type re-serializes stored JSON in its own canonical text form (inserting a space after `:`/`,`), so a byte-exact comparison fails even though the data is semantically identical. Fixed by comparing `UUID` with `require.Equal` and `Script` with `require.JSONEq` (semantic JSON comparison) in `game/game/usecases/getgamedefinition/repo_test.go` - a test-assertion-only fix, no production code, migration, or behavior changed. Re-run after the fix: passes.
  - **Out of scope, not fixed, reported below**: `game/game/usecases/getgame` (pre-existing Game Management package, not touched by this WORK) has the identical pre-existing test-assertion defect in `TestRepoGetGameCurrentVersion` (exact string comparison of a `JSONB` column's round-tripped value). This is outside this WORK's authorized scope ("Any Game Management change beyond the one minimum read capability described in Game Management Dependency... is out of scope"), predates this WORK, and does not affect any Session Lobby Foundation acceptance criterion. Recorded here as a discovered, non-blocking, pre-existing test-quality defect for a separate follow-up; left unfixed and unmodified.
- After the one in-scope fix, `go test ./... -count=1` is clean repository-wide except for the one reported out-of-scope pre-existing failure above.

**Conclusion**: every acceptance criterion in this WORK's Verification/Acceptance Criteria sections that required real-Postgres execution is now empirically verified, not merely reasoned through. No implementation defect was found in Session Runtime code itself; the only code change this pass made was the one-line test-assertion fix described above, in the new Game Management capability this WORK itself introduced.

## Documentation Impact

### Accepted / Canonical Knowledge

- GAME-ADR-0022 (business declines as workflow outcomes, refining GAME-ADR-0021) and its `game/README.md` Join-paragraph impact were created/applied by this same 2026-09-17 documentation-only handoff - see Related decisions above. `docs/engineering/standards/{error-handling,idempotency,repositories,function-signatures}.md` were refined by the same handoff (transaction-callback contract, expected-outcome-vs-error classification, claim mechanism contract, row-vs-entity naming, timestamp ownership); see those standards directly rather than duplicating their content here. Otherwise: this work implements what GAME-ADR-0001 through GAME-ADR-0005 and `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` already accept; it does not change any other accepted architecture/domain fact.

### Current-State Documentation After Implementation

- `game/CURRENT_STATE.md` - the Session Runtime capability-status row should be updated, once this work reaches DONE, from "PARTIAL... `CreateRoom` and `JoinRoom` are stubs" to reflect working Create/Join/Leave; the "Known Gaps"/"Known Drift" sections should be revised to remove now-resolved items. **Already done ahead of formal closure**: as of 2026-09-11, `game/CURRENT_STATE.md` already reflects working Create/Join/Leave and lists no known drift.
- `game/docs/DATA_MODEL.md` - the Session Runtime Tables section should be updated, once DONE, to show the new `sessions`/`session_actors`/`session_participants`/`join_codes`/`session_requests` schema, replacing the old `sessions`/`session_players`/`session_states`/`join_codes` shape. **Already done ahead of formal closure**: as of 2026-09-11, `game/docs/DATA_MODEL.md` already shows the new schema.

### Intentionally Unchanged

- `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` is accepted-design documentation, not current-state; it is not modified by this implementation.
- Game Management's existing ownership, publish/lifecycle behavior, and persisted schema are unchanged; this work adds exactly one minimum read capability (see Game Management Dependency above) and nothing else under `game/game/**`.

## Data / Migration Impact

**Historical (as approved before implementation) - already executed as described below; preserved as the rationale for the chosen approach, not as an open task.** The `game/session` schema that existed before this WORK (`sessions`, `session_states`, `session_players`, `join_codes`, migrated by `20260817000000_session_states.go`/`20260817000001_sessions.go`) had never held real data: `CreateRoom`/`JoinRoom` were no-op stubs that never persisted anything, and the `game/session/...` package tree did not compile. Discarding that data was judged safe - there was nothing to preserve.

Proposed approach: add **new** migration files (do not edit or delete the two existing historical migration files) that:
1. Drop the old `session_states`, `session_players`, `sessions`, and `join_codes` tables (in dependency order).
2. Create the new `sessions`, `session_actors`, `session_participants`, `join_codes`, and `session_requests` tables per the Approved Design above, each with a `Rollback` that reverses it, following the existing `gormigrate` convention (`game/session/internal/storage/migrations/migration.go`'s registration list) exactly as already used for the current two migrations.

**Executed as of 2026-09-08**: `game/session/internal/storage/migrations/20260908000000_drop_legacy_session_schema.go` through `20260908000005_session_requests.go` implement exactly this plan (drop-then-recreate as new migrations, old historical migration files untouched).

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
- Exact `session_requests.status` representation (string enum vs. boolean) - any type distinguishing completed from non-completed is acceptable.

## Completion Record

Status: DONE, 2026-09-18.

**Implementation summary**: Create/Join/Leave for the Session lifecycle are implemented as one `Manager` workflow controller (`game/session/workflows/sessionlifecycle/`) with explicit-parameter methods, Manager-owned transaction/admission/expiration policy, a narrow persistence-oriented `internal/repo`, direct use of the shared `sessionlock`/`idempotency` mechanism packages, token-aware Join idempotency (GAME-ADR-0021), and business declines represented as result-value outcomes rather than errors (GAME-ADR-0022) - per the Approved Design above, including its 2026-09-16 and 2026-09-17 revisions. A minimum new Game Management read capability (`game/game/usecases/getgamedefinition`) loads an immutable Game Definition by its own pinned Definition/Version UUID, never through "current version". New migrations (`game/session/internal/storage/migrations/20260908000000` through `...000005`) implement the accepted `sessions`/`session_actors`/`session_participants`/`join_codes`/`session_requests` schema.

**Independent review**: Performed per `docs/ai/protocols/IMPLEMENTATION_REVIEW.md`. Verdict: **APPROVED** (no unresolved REQUIRED_FIX or DECISION_REQUIRED findings).

**Verification actually performed** (see the dated "Verification Performed So Far" subsections above for full detail and exact commands):
- `go build ./...` - clean.
- `go vet ./...` - clean.
- `go test ./game/session/... -count=1 -v` - **all tests pass against a real PostgreSQL instance**, including every repository-integration test and every concurrency test required by this WORK's Acceptance Criteria (concurrent Joins racing for the final slot, Join racing Leave, an operation racing lobby-expiration materialization, and concurrent Create sharing the same idempotency identity). This closes the one gate every prior implementation pass had left open (no sandbox before this session had a reachable Postgres/Docker engine).
- `gofmt -l` clean on the one file this closure pass changed.
- One pre-existing, out-of-scope test defect was discovered (`game/game/usecases/getgame`'s `TestRepoGetGameCurrentVersion`, a byte-exact-string assertion against a `JSONB` column's canonically-reformatted round-trip) and is recorded as a non-blocking follow-up - not part of this WORK's scope, not fixed here.

**Documentation synchronized**: `game/CURRENT_STATE.md`, `game/docs/DATA_MODEL.md`, and `game/docs/FLOWS.md` were already synchronized to implemented reality by prior passes (see Documentation Impact above); no further documentation changes were required by this closure pass.

**Approved material deviations**: None beyond the already-HUMAN-APPROVED 2026-09-16 and 2026-09-17 Design Revisions recorded above.

**Local implementation decisions made during this closure pass**: One - `game/game/usecases/getgamedefinition/repo_test.go`'s `TestRepoGetGameDefinitionByUUID` assertion was changed from exact-string equality to `require.Equal` on `UUID` plus `require.JSONEq` on `Script`, since PostgreSQL's `jsonb` column type re-serializes JSON in its own canonical text form (adding a space after `:`/`,`) and a byte-exact comparison of a round-tripped `JSONB` value is not a valid test assertion regardless of implementation correctness. Test-only change; no production code, migration, or documented behavior changed.

**Explicitly out-of-scope follow-up worth recording**: `game/game/usecases/getgame`'s `TestRepoGetGameCurrentVersion` has the identical pre-existing test-assertion defect described above. It predates this WORK, is outside this WORK's authorized Game Management scope, and does not affect any Session Lobby Foundation acceptance criterion - recommended as a small separate follow-up (fix the assertion the same way), not urgent.

**Known limitations**: None material. This sandbox does not support `go test -race` (`CGO_ENABLED=0`/no C toolchain reachable here), so the concurrency tests above were verified without the Go race detector; correctness is instead demonstrated by the DB-locking/idempotency-claim mechanism itself and by tests that assert on outcomes under real concurrent execution (see Acceptance Criteria), consistent with how every prior pass already verified this.
