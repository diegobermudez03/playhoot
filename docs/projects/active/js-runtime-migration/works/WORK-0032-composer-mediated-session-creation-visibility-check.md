# WORK-0032: Orchestrator-Mediated Session Creation (Game Visibility Composition)

Status: DONE
Created: 2026-09-27
Last status change: 2026-09-29 (DRAFT -> READY -> IMPLEMENTING -> DONE, same day. Both Material Decisions resolved by explicit human decision, see "Material Decisions" below. Title/design updated: this is a Cross-Domain Write per `ARCHITECTURE.md`, so the coordination layer is Orchestrator, not Composer. Independent review returned APPROVED with no findings - see Completion Record.)

Related decisions:
- `docs/decisions/architecture/ADR-0014-management-session-domain-split.md`
- `docs/decisions/architecture/ADR-0015-javascript-rule-execution-and-iframe-frontend-contract.md`

Canonical context:
- `ARCHITECTURE.md` (Cross-Domain Reads, Cross-Domain Writes, Composer, Orchestrator)
- `orchestrator/README.md`
- `session/workflows/sessionlifecycle/step_create.go` (current direct-call shape being replaced)

## Reparenting Note (2026-09-27)

This WORK originated under `docs/projects/completed/management-session-domain-split/`, driven by `ADR-0014`. Its own goal (a real Composer-mediated home for `Create`'s visibility check, replacing a direct cross-domain call) is unaffected by `ADR-0015`/`GAME-ADR-0028` retiring Game Language for sandboxed JavaScript — visibility composition is independent of what rule-execution language a Session runs. It is moved here because the WORK it depended on (the old WORK-0031) is cancelled and superseded by this Project's own `WORK-0034`; every other reference to "WORK-0031" below now means `WORK-0034`.

## Outcome

Today, `sessionlifecycle.Manager.Create` resolves `session_games`/`session_game_version_artifacts` entirely from Session Runtime's own tables (WORK-0034) and no longer calls Game Management at all. Something must still enforce "a Session may only be created for a currently-playable/visible Game" — this WORK is required so that check has a real home.

**Revised (2026-09-29):** creating a Session is a write (it inserts a Session/JoinCode/idempotency-claim row), so the composition this WORK builds — read Game Management's visibility, then conditionally call Session Runtime's `Create` — is a Cross-Domain Write workflow (`ARCHITECTURE.md -> Cross-Domain Writes`), not a Cross-Domain Read. Its home is Orchestrator (`orchestrator/`), not Composer (`composer/`) — Composer is reserved for read-only composition and is stateless with respect to any resulting write. This corrects the WORK's original framing, made before Orchestrator's write/read distinction was applied to this specific case.

This is a known-required future outcome, not a speculative idea: without it, either the visibility check silently disappears (a real product regression — anyone could create a Session for an unpublished/hidden Game) or WORK-0034 could not have safely removed the direct call it removed. `docs/projects/README.md` Invariant 1 is why this exists as its own WORK now, even though its design started later.

## Context

`orchestrator/` currently contains only a README stub describing its accepted responsibility — no concrete workflow code exists yet in this codebase. (`composer/` is likewise still a stub; this WORK does not add code there — see Outcome above for why.)

**Simplified (2026-09-28), per WORK-0034's own Scope Narrowing.** `Create` resolves the current version's content entirely from Session Runtime's own tables, never from Game Management and never from data the new coordination layer passes in. Its role here is therefore only a visibility gate — read Game Management's visibility state, decide whether `Create` may proceed, and if so call `Create` with nothing more than the `game_uuid` it already takes today. It does not resolve or hand off any artifact content.

**Blocker resolved by inspection (2026-09-28), endpoint wiring confirmed in scope (2026-09-29): the real entry point is `POST /sessions`.** `api/session/handler.go`/`http.go` already register this route with real request decoding/logging, but `handleCreateSession` calls no domain package today — it unconditionally answers `501 Not Implemented`. No other current WORK claims this wiring: `session-runtime-v1`'s `WORK-0008`/`WORK-0020` (still PLANNED/DRAFT) own the *live/WebSocket* lobby-projection experience post-Create, not the initial REST call. This WORK wires it, since no other caller reaches the new coordination layer otherwise.

**Discovery (2026-09-28), resolved (2026-09-29): Game Management's old visibility-read capability (`getgame.GetPlayableGameWithCurrentVersion`) no longer exists.** `WORK-0038` (landed 2026-09-29, after this discovery was written) deleted `game/usecases/getgame`/`getgamedefinition` entirely along with `game_definitions`/`game_definition_histories`/`games.current_definition_id` — Game Management now owns no version/definition concept of its own at all (`game/docs/DATA_MODEL.md` already documents the current, version-free `games` schema). This WORK therefore builds a brand-new, narrow visibility-only read capability against the `games` table alone (`uuid`, `visibility` — no join, no decode of any script/definition). It also removes the dead Go-level leftovers this discovery originally warned about: `game/internal/storage/tables.go` still declared `gameDefinition`/`gameDefinitionHistory` structs and a `game.CurrentDefinitionID` field mirroring the already-dropped columns/tables — unreferenced by any repo code (no `game/internal/storage` repo existed at all pre-this-WORK) and inconsistent with `game/docs/DATA_MODEL.md`'s already-accurate current-state description. Removed as part of this WORK's own new repo code touching that same file, not a separate cleanup WORK.

## Scope

### In Scope

- `orchestrator.Orchestrator.CreateSession(ctx, gameUUID, hostUserUUID, idempotencyKey) (session.CreatedSession, error)`: reads Game Management's visibility state for `gameUUID` first; if not visible/playable (or not found), returns `session.ErrGameNotFound` without calling Session Runtime at all; otherwise calls `sessionlifecycle.Manager.Create` with the same arguments and returns its result unchanged. Orchestrator owns no business entity of its own, no retry/compensation/saga state for this workflow (none is needed — a failed write is simply reported, not compensated) — a plain sequential read-then-call (`ARCHITECTURE.md -> Cross-Domain Writes`).
- Wiring `api/session/handler.go`'s `handleCreateSession` to call the Orchestrator instead of its current unconditional `501`, using the wire shapes (`createSessionRequest`/`createSessionResponse`) already defined in `api/session/ws.go`, and mapping `session.ErrGameNotFound`/`ErrIdempotencyKeyRequired`/`ErrIdempotencyConflict`/`ErrIdempotencyInFlight` to appropriate HTTP statuses.
- A new Game Management read capability (`game/usecases/checkvisibility`) the Orchestrator calls for the visibility check: resolves only `games.visibility` by `games.uuid`, never a version/definition/script concept (see Material Decisions).
- End-to-end wiring so `POST /sessions` is reachable against a real database and a real Executor connection: `main.go` gains the Executor's gRPC target as required configuration, dials a `session/internal/executor.GRPCClient`, and constructs the real `sessionlifecycle.Manager`/`checkvisibility.Service`/`orchestrator.Orchestrator`/`api.Server` chain. Today none of this wiring exists (`api.NewServer()` takes no arguments and `main.go` never constructs a Manager) — this WORK is what first makes it exist, not a pre-existing seam it merely reconnects.
- Removing the dead `gameDefinition`/`gameDefinitionHistory`/`CurrentDefinitionID` Go-level leftovers in `game/internal/storage/tables.go` (see Context's second Discovery).

### Explicitly Out Of Scope

- Anything WORK-0008/WORK-0020 own: live/WebSocket lobby roster, join/leave fan-out, bootstrap/resync. This WORK only makes the pre-lobby `POST /sessions` call itself functional.
- Redesigning the five untouched `sessionlifecycle` call sites' own pinned-definition read path — unaffected by this WORK.
- Solving how a Game's current version is actually published/becomes visible in the first place (`WORK-0033`).
- Host "kick"/"transfer host" — tracked in `session-runtime-v1`'s own open scope questions, unaffected here.
- Any Game Management write/authoring path (create/edit/publish a Game) — still not implemented anywhere in this repository; out of scope here.

## Approved Design

- A new Game Management use case, `game/usecases/checkvisibility`, returning only whether `gameUUID` exists and is currently playable/visible — resolved from `games.visibility` alone (no join, no script/definition decode of any kind).
- `orchestrator.Orchestrator.CreateSession` composes the visibility check above with `sessionlifecycle.Manager.Create`; on a not-visible/not-found result it returns `session.ErrGameNotFound` (Session Runtime's own existing public error, already what `Create` itself returns for its own not-found case today) rather than inventing a new Orchestrator-level error type, so `handleCreateSession` needs only one not-found branch regardless of which domain produced it.
- `handleCreateSession` is edited in place: same decode/validate logic it already has, its `501` replaced by the real call and existing response-shape mapping, plus explicit status mapping for `session.ErrGameNotFound` (404), `ErrIdempotencyKeyRequired` (400), `ErrIdempotencyConflict`/`ErrIdempotencyInFlight` (409), default (500).
- `main.go`/`api.NewServer` gain the real dependency chain (DB-backed `checkvisibility.Service`, a `session/internal/executor.GRPCClient` dialed against a new required `EXECUTOR_ADDR` env var, `sessionlifecycle.New(db, exec)`, `orchestrator.New(...)`) — none of this wiring existed before this WORK; `api.NewServer()` took no arguments and `main.go` never constructed a Manager.

## Constraints and Invariants

- Must preserve the currently-enforced invariant: `Create` never succeeds for a Game whose current version is not playable/visible (enforced via `businessservice.IsPlayableVisibility`, called by the new `checkvisibility` use case).
- Per `ARCHITECTURE.md -> Cross-Domain Writes`, Orchestrator does not own the underlying business rules of Game Management or Session Runtime — it only sequences the read and the write.

## Acceptance Criteria

- `POST /sessions` no longer unconditionally returns `501`; a valid request for a visible/playable Game's `gameUUID` creates a Session exactly as `sessionlifecycle.Manager.Create` does today when called directly.
- A request for a `gameUUID` that does not exist or is not currently playable/visible returns the same outcome `Create` itself already returns for "game not found" (no new distinct wire error class).
- No code path lets `handleCreateSession` call `sessionlifecycle.Manager.Create` without first going through the new visibility read — no bypass.
- Orchestrator's new code owns no business entity/table of its own and contains no business rule beyond call sequencing (`ARCHITECTURE.md -> Cross-Domain Writes`).
- `go build ./...`/`go vet ./...` clean; `go test ./...` clean (integration/DB tests may skip without a reachable test database, consistent with the rest of the repository); new unit coverage for `Orchestrator.CreateSession`'s visible/not-visible/not-found/downstream-error branches.

## Implementation Freedom

- Exact new Game Management use-case package naming (`checkvisibility`) and its repo-level SQL shape.
- Exact `orchestrator` package internal layout (single file vs. its own subpackage), consistent with `orchestrator/README.md`'s stub.
- Exact env var name/shape for the Executor's gRPC target and dial credentials (no TLS material exists anywhere in this repository yet; insecure transport credentials are used, consistent with the JS Executor having no other deployed caller yet).

## Verification

- `go test ./...` across `orchestrator/`, the new Game Management use case, and `api/session`/`api`.
- A manual/integration-level check that `POST /sessions` returns a real `201`-shaped response for a seeded visible Game and the not-found/not-playable outcome for a hidden/draft one, where a real database and Executor are reachable.

## Documentation Impact

### Accepted / Canonical Knowledge

- `orchestrator/README.md` gains a concrete example (its first real workflow code).

### Current-State Documentation After Implementation

- `game/CURRENT_STATE.md`/`game/docs/FLOWS.md` — note the new visibility-only read capability.
- `session/CURRENT_STATE.md` — note `Create` is now reachable over `POST /sessions`, not only at the Go-API level.
- `api/README.md` (if it documents endpoint implementation status) — `POST /sessions` moves from stub to implemented.

## Blockers

None.

## Material Decisions (Resolved 2026-09-29)

1. **Does this WORK also wire the `POST /sessions` HTTP endpoint, or only build the coordination call itself and leave the endpoint stubbed for a separate WORK?** Resolved: yes, wire it here — no other current WORK claims that wiring, the new coordination layer has no other caller to be reached from, and the endpoint's own request/response shapes already exist waiting for exactly this call.
2. **Does the visibility check call a new, narrower Game Management read capability, or reuse `getgame.GetPlayableGameWithCurrentVersion` as-is?** Resolved: a new capability, `checkvisibility`, reading only `games.visibility` by `games.uuid` — no version/definition concept at all. (`getgame.GetPlayableGameWithCurrentVersion` no longer exists in this codebase; `WORK-0038` deleted it along with `game_definitions`/`current_definition_id` before this decision was made.) Separately decided in the same exchange: this composition is a Cross-Domain **Write** (creating a Session), so it is built in `orchestrator/`, not `composer/` — see Outcome's "Revised (2026-09-29)" note.

## Completion Record

Implemented 2026-09-29, same day as READY.

**Game Management:**
- `game/usecases/checkvisibility` (`service.go`, `service_test.go`, `repo_mock_test.go`): `Service.IsVisible(ctx, gameUUID) (visible, found bool, err error)`, resolving only `games.visibility` by `games.uuid` through a new narrow `repoAPI` (mockgen-generated).
- `game/internal/storage/repo.go` (new): `Repo.ResolveVisibility` - a raw-SQL, single-column read, excluding soft-deleted rows.
- `game/internal/storage/repo_test.go` (new): real-Postgres integration tests (visible/not-found/soft-deleted), skipping gracefully without a reachable test database per this repository's existing convention.
- `game/internal/storage/tables.go`: removed the dead `gameDefinition`/`gameDefinitionHistory` structs and the `game` struct's `CurrentDefinitionID` field - all three mirrored columns/tables `WORK-0038` already dropped from the database, were unreferenced by any repository code (no repo existed in this package before this WORK), and were already inconsistent with `game/docs/DATA_MODEL.md`'s accurate current-state description.

**Orchestrator:**
- `orchestrator/createsession.go` (new): `Orchestrator.CreateSession`, composing `visibilityChecker`/`sessionCreator` (both narrow, mockgen-generated interfaces) exactly per Approved Design.
- `orchestrator/createsession_test.go` (new): visible/not-visible/not-found/visibility-error/downstream-Create-error branches, all mocked.

**API wiring:**
- `api/session/handler.go`: `Handler` now holds a `SessionCreator` (an exported interface this package itself declares, satisfied structurally by `*orchestrator.Orchestrator`); `NewHandler` takes it.
- `api/session/http.go`: `handleCreateSession` calls `SessionCreator.CreateSession` and maps its result/errors to `201`/`404`/`400`/`409`/`500`.
- `api/server.go`: `NewServer` now takes a `sessionCreator apisession.SessionCreator` and forwards it; the file's own doc comment explaining the prior "no arguments" state was removed since it no longer applies.
- `api/server_test.go`: replaced `TestCreateSession_NotImplemented` with `TestCreateSession_Created`/`TestCreateSession_GameNotFound`, both against a hand-written `fakeSessionCreator` (the interface has one method, so a mock generator added no value); the two WebSocket tests were updated to pass the same fake, since `api.NewServer` now requires it.

**End-to-end wiring (main.go):**
- `session/workflows/sessionlifecycle/grpc_manager.go` (new): `NewWithGRPCExecutor(db, executorAddr) (*Manager, func() error, error)` - the supported entry point for a caller outside the `session/` tree (like `main.go`) that needs a real Executor connection, since `session/internal/executor` is unexported outside it. Uses insecure gRPC transport credentials (no TLS material exists anywhere in this repository yet).
- `main.go`: added a required `EXECUTOR_ADDR` env var; constructs `sessionlifecycle.NewWithGRPCExecutor` -> `checkvisibility.New` -> `orchestrator.New` -> `api.NewServer`, closing the Executor connection on shutdown.
- `.env` (gitignored, not committed): added `EXECUTOR_ADDR=localhost:50051` for local development.

**Verification:**
- `go build ./...`/`go vet ./...` clean.
- `go test ./...` clean except two pre-existing, unrelated failures already documented in `WORK-0038`'s own Completion Record: `TestNoInternalDocCitationsInComments` (an untouched file, `session/internal/storage/migrations/20260926000000_session_runtime_failures.go`, confirmed via `git log`) and `TestManagerJoin_Integration_ConcurrentOperationRacingLobbyExpiration` (the known `session-runtime-v1`-owned Join/Leave race, confirmed pre-existing there).
- A reachable Postgres container was available this session; ran the full suite against it (`TEST_DATABASE_*` set) with the same two pre-existing failures and nothing new.
- Manual end-to-end verification against that same real Postgres container (no `jsexecutor` needed - `Create` never calls the Executor): started the real binary (`go run .`) with real env vars, seeded a `public` Game plus its `session_games`/`session_game_version_artifacts` rows directly via `psql`, then exercised `POST /sessions` with `curl`: a visible/playable Game produced a real `201` with a genuine persisted Session/JoinCode/`lobby_expires_at`; a `draft`-visibility Game produced `404`; a nonexistent Game UUID produced `404`. All seeded rows were cleaned up afterward and the manually-started server process was stopped.

**Documentation synchronized:** `orchestrator/README.md` (first concrete example), `game/CURRENT_STATE.md`, `game/docs/FLOWS.md` (replaced its own stale `getgame`-era flow, which predated this WORK and was never updated when `WORK-0038` deleted that use case), `session/CURRENT_STATE.md` (narrow note that `POST /sessions` now calls through for real; the rest of that file's known Game-Language-era staleness is `WORK-0038`'s own already-flagged gap, not rewritten here), `api/README.md`. `PROJECT.md`'s Work table/Capability Coverage/narrative updated to DONE and to the Orchestrator (not Composer) framing.

**Independent review:** performed by a fresh, read-only agent per `docs/ai/protocols/IMPLEMENTATION_REVIEW.md`, reasoning from the actual uncommitted diff/code/tests/docs rather than this Completion Record's own claims. Verdict: **APPROVED, no findings.** The reviewer independently re-ran `go build`/`go vet`/`go test ./...` (including the real-Postgres integration tests, a reachable container being available), independently re-confirmed both pre-existing unrelated failures (`TestNoInternalDocCitationsInComments` on an untouched file; the pre-existing `session-runtime-v1`-owned Join/Leave race) via `git log`/`git diff`, independently re-verified the dead-code-removal claim via repo-wide grep, confirmed no bypass of the visibility gate (only `orchestrator/createsession.go` calls `sessionlifecycle.Manager.Create`), confirmed Cross-Domain Write/Orchestrator placement against `ARCHITECTURE.md`, and confirmed all five Documentation Impact targets were both updated and accurate against the real code.
