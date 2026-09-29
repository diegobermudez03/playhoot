# WORK-0034: Session-Owned Executable Script Artifact & Package Restructuring

Status: DONE
Created: 2026-09-27
Last status change: 2026-09-28 (IMPLEMENTING -> DONE, independent review APPROVED with no findings)

Related decisions:
- `docs/decisions/architecture/ADR-0014-management-session-domain-split.md`
- `docs/decisions/architecture/ADR-0015-javascript-rule-execution-and-iframe-frontend-contract.md`
- `game/docs/decisions/GAME-ADR-0028-javascript-execution-replaces-game-language.md`

Canonical context:
- `ARCHITECTURE.md` (Accepted Business Boundaries, Dependency Principles)
- `game/README.md` (Internal Structure, Capability Persistence and Transaction Boundary)
- `docs/engineering/standards/domain-logic-placement.md`
- `session/docs/GAME_VERSION_ARTIFACT_MODEL.md` (the artifact shape this WORK persists)
- `game/session/workflows/sessionlifecycle/manager.go` and `step_create.go` (the only call site this WORK touches)
- `game/management/models.go`, `game/management/usecases/getgame/`, `game/management/usecases/getgamedefinition/`
- `docs/projects/completed/management-session-domain-split/works/WORK-0031-session-owned-executable-program-management-session-split.md` (CANCELLED precedent this WORK's design reuses/narrows)
- `docs/projects/active/js-runtime-migration/works/WORK-0038-snapshot-based-session-runtime-persistence-migration.md` (owns the remaining five call sites' engine swap, explicitly out of scope here)

## Outcome

Supersedes cancelled `docs/projects/completed/management-session-domain-split/works/WORK-0031-session-owned-executable-program-management-session-split.md` — see that WORK's own Completion Record. Its underlying outcome is conserved but **narrowed in scope** (see Scope Narrowing below, human-decided 2026-09-28): `Create` stops depending on Game Management at runtime and resolves the Game's current playable version entirely from Session Runtime's own persisted artifact tables, keyed by the immutable definition/version identity already pinned at `Create` (`sessions.game_definition_uuid`). What changes from WORK-0031's own design is the artifact's content type: two mandatory scripts (backend JavaScript rules and a stored frontend script) plus optional contract/asset metadata, per `WORK-0044`'s artifact model, not a compiled Game Language `program.Definition`.

This WORK also completes **part** of `ADR-0014`'s package-restructuring goal: Game Management and Session Runtime become independent top-level packages (`game/`, `session/`), no longer nested under a shared `game/` bounded-context root. It does **not** yet relocate Game Language (`game/language/v1/...`) into Session Runtime's own package tree, and does **not** remove Game Management's dependency on it — see Scope Narrowing.

## Scope Narrowing (human-decided 2026-09-28)

Two material questions were resolved by explicit human decision before drafting further, rather than inferred:

1. **Bootstrapping `Create`'s data.** `Create` receives only the Game's public `game_uuid` (its own signature already does this — no change needed). It resolves the current version to pin *entirely from Session Runtime's own tables* — never from Game Management, and never by receiving pre-resolved content from a caller. Composer (`WORK-0032`) validates visibility against Game Management first and only then calls `Create`; it passes no Game Management data into Session Runtime. How Session Runtime's own tables first get populated for a version (the publish path) is `WORK-0033`'s concern and is explicitly assumed-already-solved by this WORK — this WORK does not design that write path, only the read path `Create` needs and the storage shape a future publish path will write into.
2. **Which call sites this WORK actually moves.** WORK-0031's original design moved all six of Session Runtime's pinned-definition reads (`Join`, `Start`, `AnswerInteraction`, `SubmitUserIntent`, `CancelSession`, `ExpireTimer`) off Game Management, in addition to `Create`. That is not safe to do here: those five call sites still execute gameplay through the *old* Game Language engine (`engineservice.Compile`/`StartTurn`/`AdvanceTurn` against `program.Definition`), and `WORK-0038` — not this WORK — owns switching them to the new JS Executor (`WORK-0038`'s own Outcome: "switches every one of Session Runtime's seven RuntimeTurn-capable call sites... from `engineservice.Compile`/`StartTurn`/`AdvanceTurn` to the new execution boundary"). Handing those five call sites a JS-shaped artifact now, before they can execute JS, would break them. **Decision: this WORK moves only `Create`.** The other five call sites are explicitly left unchanged — still reading Game Management via `gamePinnedDefinitionReader`, still running the old engine — as a recorded temporary gap, the same shape ADR-0014's own Consequences already anticipated for the Create/Composer visibility gap. `WORK-0038` closes this gap when it switches those five call sites at the same time it wires in the Executor.

Consequence: `ADR-0014`'s full goal (Game Management importing nothing from Game Language; Game Language relocated into Session Runtime's own package tree) is **not** fully achieved by this WORK alone. Game Management's `getgamedefinition`/`getgame` use cases still need `program`/`gameservice` (`gameservice.DecodeJSON`) because the five untouched call sites still depend on `management.Game.Definition` transitively through `gamePinnedDefinitionReader`. `game/language/v1/...` therefore also stays at its current import path for now — physically relocating it under Session Runtime's internal tree must wait until Game Management no longer needs it, which is `WORK-0038`'s closing condition, not this WORK's. This is recorded explicitly so it is not mistaken for an oversight when this WORK closes with `ADR-0014` only partially realized.

## Context

`WORK-0044` is DONE — `session/docs/GAME_VERSION_ARTIFACT_MODEL.md` defines the shape this WORK persists: `DefinitionUUID`, mandatory `BackendScript` and `FrontendScript`, optional `GameContract`/`Assets`/`PlatformContractVersion`. That contract does not itself include a `game_uuid`/"current version" concept — it describes one immutable version's own content, not how a caller finds the current one. Resolving "given a `game_uuid`, which version is current" is Session Runtime's own need (per the Scope Narrowing decision above) and is this WORK's own addition, not a modification of `WORK-0044`'s accepted artifact contract.

Today, `sessionlifecycle.Manager.Create` (`game/session/workflows/sessionlifecycle/step_create.go`) calls `m.currentGameReader.GetPlayableGameWithCurrentVersion(ctx, gameUUID)` (`gameCurrentVersionReader`, backed by Game Management's `getgame` use case), which returns `*management.Game` embedding `program.Definition`. `Create` then calls `engineservice.Compile(playableGame.Definition)` as a fail-fast validation before creating the Session, alerting monitoring on failure. Neither of these is meaningful once the pinned artifact is JavaScript: there is no `program.Definition` to compile, and no equivalent runtime-side validation is designed yet (`WORK-0037`, static-analysis/lint enforcement, is the accepted future owner of authored-script validation — it runs at authoring/publish time, not at `Create`). This WORK removes the compile-time validation with no replacement; it is not this WORK's job to invent one.

## Scope

### In Scope

- Two new Session-Runtime-owned tables (illustrative naming, exact names are implementer's choice per Implementation Freedom):
  - `session_games` — one row per Game (`game_uuid`, unique), pointing at its current version (`current_definition_uuid`, nullable until a version has ever been published).
  - `session_game_version_artifacts` — one row per immutable version (`definition_uuid`, unique — matches `sessions.game_definition_uuid`), storing `game_uuid` (correlating back to `session_games`, Session-owned, not Game Management's identity), `backend_script` (required), `frontend_script` (required), `game_contract` (optional), `assets` (optional), `platform_contract_version` (optional), `created_at`.
  - Both tables live entirely inside Session Runtime's own transaction/persistence boundary; `current_definition_uuid`/`game_uuid` correlation between them is a real, same-domain DB-enforced foreign key (both tables are Session-owned), not a logical cross-domain reference.
- A new internal repository method on `sessionlifecycle`'s own persistence layer, resolving `game_uuid -> current_definition_uuid`, called only by `Create`.
- Removing `Create`'s dependency on Game Management entirely: `gameCurrentVersionReader` (interface, field, constructor parameter, mocks) removed from `sessionlifecycle.Manager`/`New`; `step_create.go` no longer imports `game/management` or `game/language/v1/engine/engineservice`.
- Removing `Create`'s `engineservice.Compile` validation step, with no replacement (see Context).
- Moving `game/management` and `game/session` to become independent top-level packages (`game/`, `session/`), siblings to `identity/`, `composer/`, `orchestrator/`. `game/session/jsexecutor` (already relocated there by `WORK-0052`) moves along with `game/session` automatically.
- Splitting `game/README.md` into `game/README.md`/`session/README.md`, each following `docs/ai/templates/domain/`, scoped to what each domain actually now owns.
- Updating `docs/ai/KNOWLEDGE_MAP.md`'s Accepted Domain Documentation table (replace the single `Game` row with `Management`/`Session` rows) and `ARCHITECTURE.md`'s Accepted Business Boundaries (state the new boundary directly, no longer a superseded-in-part annotation).

### Explicitly Out Of Scope (see Scope Narrowing)

- Any change to `step_join.go`, `step_start.go`, `step_answer_interaction.go`, `step_submit_user_intent.go`, `step_cancel_session.go`, `step_expire_timer.go` — they keep calling `gamePinnedDefinitionReader`/Game Management exactly as today. `WORK-0038` owns switching them, together with the old-engine-to-Executor swap.
- Removing Game Management's dependency on `program`/`gameservice`/`engine` — it is still needed by `getgamedefinition` for the five untouched call sites above. `WORK-0038` closes this.
- Physically relocating `game/language/v1/...` into Session Runtime's internal package tree — it stays at its current import path until Game Management no longer needs it.
- The publish/authoring write path that populates `session_games`/`session_game_version_artifacts` for a version Session Runtime has never seen (`WORK-0033`). This WORK's own tests populate fixture rows directly, the same way existing tests populate Game Management fixture rows directly today.
- The Composer-mediated visibility check itself (`WORK-0032`) — this WORK only removes the direct call `WORK-0032` was going to replace; `WORK-0032`'s own design is unaffected and, per the human decision above, materially simpler than its own file currently anticipates (no data flows from Composer into `Create` beyond `game_uuid`, which `Create` already receives today).
- Backfilling/migrating any existing production data — this codebase has no production Sessions yet.

## Approved Design

- `Create`'s new resolution flow: look up `session_games` by `game_uuid`; if no row exists or `current_definition_uuid` is null, return `session.ErrGameNotFound` (same public outcome as today's "game not found"/unresolvable-version case). Otherwise pin `sessions.game_definition_uuid = current_definition_uuid` and proceed exactly as today (create Session, host SessionActor, JoinCode, in one transaction). `Create` does not need to load `session_game_version_artifacts`' script content at all — it only needs the `definition_uuid` to pin; the DB-enforced FK between the two tables is what guarantees the pinned UUID actually has artifact content once written.
- The new repository read method is `sessionlifecycle`-internal (`internal/repo` or a new sibling internal package, implementer's choice), per `docs/engineering/standards/domain-logic-placement.md -> A Workflow's Public Contract Lives At The Domain Root` — it is not part of `session`'s zero-dependency root package, the same way today's repository methods aren't.
- `Create`'s own public signature is unchanged (`gameUUID session.GameUUID`, already the only Game-identifying input it takes) — the Scope Narrowing decision confirms no signature change is needed or wanted.
- `gamePinnedDefinitionReader`/`pinnedGameReader` and the field on `Manager` are left entirely alone; `New`'s signature loses only its `currentGameReader` parameter.

## Constraints and Invariants

- `GAME-ADR-0001`'s version-immutability invariant (a Session's pinned artifact/version must remain semantically stable for that Session's lifetime) is unaffected and must continue to hold.
- No database transaction may span Game Management-owned and Session-Runtime-owned tables (`ARCHITECTURE.md -> State and Transaction Boundaries`).
- No direct call from Session Runtime into Game Management may remain in `Create`'s own path after this WORK.
- No dual-read transitional state for `Create` specifically: `Create` must resolve exclusively from Session Runtime's own tables, never falling back to Game Management, even temporarily.

## Acceptance Criteria

- `step_create.go` imports neither `github.com/diegobermudez03/playhoot/game/management` (or its post-move `game/` path) nor `.../game/language/v1/engine/engineservice`.
- `sessionlifecycle.Manager`/`New` no longer has a `currentGameReader`/`gameCurrentVersionReader` field or constructor parameter.
- `step_join.go`, `step_start.go`, `step_answer_interaction.go`, `step_submit_user_intent.go`, `step_cancel_session.go`, `step_expire_timer.go` are unchanged and still compile/pass against `gamePinnedDefinitionReader` exactly as before this WORK.
- New migration creates the two Session-owned tables, with a real FK between them and no FK to any Game-Management-owned table.
- `game/` and `session/` exist as independent top-level Go packages; no remaining reference anywhere in the repository to the old `game/management`/`game/session` import paths.
- `go build ./...` succeeds.
- Existing `step_create` unit/integration tests pass against the new repository dependency (mocked the same way `MockgameCurrentVersionReader` is today); new repository-integration test coverage (real Postgres) exists for `session_games`/`session_game_version_artifacts`' read path used by `Create`.

## Implementation Freedom

Exact table/column naming, exact new internal package layout for the new repository method, and exact file-move mechanics for `game/`/`session/` are local implementation choices within the Operating Model's normal autonomy boundary. Whether `game/docs/decisions/` (the `GAME-ADR-*` family) moves alongside `game/session` or stays at its current path is also implementer's call — existing `GAME-ADR-NNNN` filenames/identifiers are never renumbered or renamed regardless (`docs/decisions/README.md`'s historical-immutability rule).

## Verification

- `go test ./...` across every touched package.
- Real-Postgres repository-integration tests for the two new tables/repository method, per this codebase's established convention.
- Independent review per `docs/ai/protocols/IMPLEMENTATION_REVIEW.md`, given this WORK changes public package structure and a persisted schema.

## Documentation Impact

### Accepted / Canonical Knowledge

- `ARCHITECTURE.md` — Accepted Business Boundaries rewritten to state the new boundary directly; annotated that `ADR-0014`'s Game-Language-internalization goal remains partially open pending `WORK-0038`.
- `game/README.md` — split into `game/README.md`/`session/README.md`, each following `docs/ai/templates/domain/`. Session Runtime's own document should note that Game Language execution is still, for now, reached through the old engine for everything except `Create`.
- `docs/ai/KNOWLEDGE_MAP.md` — Accepted Domain Documentation table updated (`Management`/`Session` rows replace the `Game` row).

### Current-State Documentation After Implementation

- `game/docs/DATA_MODEL.md` (or its split `game/docs/DATA_MODEL.md` / `session/docs/DATA_MODEL.md`) — add `session_games`/`session_game_version_artifacts`; existing tables otherwise unchanged (the five other call sites' tables/behavior are untouched by this WORK).
- `game/CURRENT_STATE.md` (or its split successors) — reflect `Create`'s new resolution path and the new package layout; explicitly note the remaining five call sites are unchanged pending `WORK-0038`.

## Blockers

None remaining. Both blockers inherited from the cancelled `WORK-0031` are resolved by explicit human decision (2026-09-28, see Scope Narrowing): the bootstrapping question is resolved by `Create` never needing Game Management data at all (Session Runtime's own tables are the only source, populated by a future publish path this WORK does not design), and the WORK-0032 sequencing question is resolved by narrowing this WORK to `Create` only, which also simplifies `WORK-0032` itself (Composer passes no artifact data to `Create`, only gates on visibility).

## Completion Record

**Implementation pass complete (2026-09-28); not yet DONE — independent review per `docs/ai/protocols/IMPLEMENTATION_REVIEW.md` has not run.**

Implemented:
- `Create` (`session/workflows/sessionlifecycle/step_create.go`) resolves the Game's current pinnable version via a new `createRepoAPI.ResolveCurrentGameDefinitionUUID` method (`session/workflows/sessionlifecycle/internal/repo/game_version.go`), reading only Session Runtime's own tables. `gameCurrentVersionReader` (interface, field, constructor param) is removed from `Manager`/`New`; `step_create.go` no longer imports `management` or `game/language/v1/engine/engineservice`; the `engineservice.Compile` validation and `session.ErrDefinitionDoesNotCompile` are removed with no replacement, per the approved design.
- Two new Session-owned tables/migrations: `session_games` (`game_uuid` -> `current_definition_uuid`) and `session_game_version_artifacts` (one row per immutable version: `definition_uuid`, `game_uuid`, `backend_script`, `frontend_script`, `game_contract`, `assets`, `platform_contract_version`), with a real FK pair between them (`session/internal/storage/migrations/20260928000000_session_games.go`, `.../20260928000001_session_game_version_artifacts.go`).
- `game/management` and `game/session` moved to independent top-level packages `game/` and `session/` (`git mv`, ~135 files), including `session/jsexecutor` moving along with `session/`. All Go import paths repository-wide updated accordingly; `game/language/v1/...` and `game/docs/decisions/` deliberately left at their current paths (implementer's freedom, per the WORK's own scope narrowing — Game Management and the five untouched call sites still need them).
- Test fixtures: `session/internal/testfixtures.SeedCurrentGameVersion` seeds both new tables directly (no publish path exists yet); `step_create_test.go`/`step_create_integration_test.go` rewritten against the new dependency; `testutil_test.go`'s now-dead `stubCurrentGameReader` removed. Mocks regenerated (`mockgen`) — `MockgameCurrentVersionReader` removed, `MockcreateRepoAPI` gained `ResolveCurrentGameDefinitionUUID`.
- Documentation synchronized: `ARCHITECTURE.md` (Accepted Business Boundaries rewritten to state the new boundary directly, including that Game Language internalization remains incomplete pending `WORK-0038`), `docs/ai/KNOWLEDGE_MAP.md` (Accepted Domain Documentation table split into Management/Session rows; Specialized Documentation paths corrected), `game/README.md`/`game/CURRENT_STATE.md`/`game/docs/{DATA_MODEL,FLOWS}.md` split into `game/{README,CURRENT_STATE}.md` + `game/docs/{DATA_MODEL,FLOWS}.md` and `session/{README,CURRENT_STATE}.md` + `session/docs/{DATA_MODEL,FLOWS}.md`, each corrected to current reality and cross-referencing the other; `docs/architecture/SYSTEM_MAP.md` and `docs/engineering/standards/domain-logic-placement.md` (its own `gameCurrentVersionReader` worked example, now removed, corrected) updated for the new package layout.
- `docs/projects/active/js-runtime-migration/works/WORK-0032-composer-mediated-session-creation-visibility-check.md` lightly synced: its own Context/Blockers now reflect that its design is simplified (visibility gate only, no data handoff) as a direct consequence of this WORK's Scope Narrowing decision.

Local implementation decisions:
- `session_games`/`session_game_version_artifacts` table/column names and the two-insert-then-update seeding sequence (mirroring `CreateSessionWithHost`'s existing insert-then-assign pattern) — both within Implementation Freedom.
- Regenerated `session/jsexecutor/proto/{executor.pb.go,executor_grpc.pb.go}` via `protoc` after correcting `executor.proto`'s `go_package` option — required because a naive path-string find/replace across the repository's `.go` files had corrupted the generated file's embedded serialized `FileDescriptorProto` bytes (the `go_package` string's byte-length changed but the wire-format length prefix framing it did not, causing a `slice bounds out of range` panic on package init); caught by `go test ./...` before this pass closed, fixed by regenerating from the corrected `.proto` source rather than hand-patching the generated file.

Deviations from the approved WORK: None.

Discoveries: None unresolved. (Two were surfaced and resolved by human decision *before* implementation began — see the WORK's own Scope Narrowing section — not during implementation.)

Verification performed:
- `go build ./...`, `go vet ./...`, `gofmt -l` (all changed/new `.go` files): clean.
- `go test ./... -count=1`: all packages pass except two pre-existing `comment_standard_test.go` checks, both confirmed unrelated to this WORK via `diff` against the pre-move file content (only the import path differs): `TestExportedDocCommentsStayAtPublicContract` (23 findings - `runtime_failure.go`'s `engineservice.*` references, `manager.go`'s own doc comment mentioning `gorm.DB`) and `TestNoInternalDocCitationsInComments` (23 findings, all `GAME-ADR-*`/`WORK-0014`/`WORK-0029` citations in files this WORK never touched: `runtime_failure.go`, `step_cancel_session.go`, three `step_*_integration_test.go` files, `session_runtime_failures.go`'s migration). Neither fixed here (outside this WORK's approved scope); both flagged for a separate follow-up. Note: this same `TestNoInternalDocCitationsInComments` check caught 6 real violations in code *this WORK itself added* (ADR-0014/WORK-0034/WORK-0033 citations in new doc comments) - human-flagged during review, fixed as a LOCAL FIX (comments rewritten to state the reasoning directly instead of citing the doc) before this checkpoint.
- Real-Postgres repository-integration tests (including the new `session_games`/`session_game_version_artifacts` coverage and `TestManagerCreate_Integration*`) compile-check clean and skip gracefully (no reachable Docker/Postgres in this sandbox) — the same known limitation recorded throughout this repository's history. Not yet run against a real database.

Documentation synchronized: see Implemented, above. `session/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md`/`session/docs/GAME_VERSION_ARTIFACT_MODEL.md` intentionally left at their current `game/docs/` path (implementer's freedom).

Known limitations:
- Real-Postgres verification of the new tables/`Create` path has not run (sandbox limitation, not a design gap).
- `comment_standard_test.go`'s one pre-existing failure (unrelated to this WORK) remains unfixed; recorded as a discovered, non-blocking, out-of-scope defect for a future follow-up.

Human review feedback:
- Doc comment in `step_create.go` cited `ADR-0014`/`WORK-0034`, violating the internal-citation comment standard -> LOCAL FIX — applied, and swept for the same pattern across every file this WORK touched (6 instances total, across `step_create.go`, `manager.go`, `step_create_integration_test.go`, `testfixtures.go`, and two migration files); verified via `go test -run TestNoInternalDocCitationsInComments` that none remain in code this WORK added.
- Directly requested: rename `management/` to `game/`, reclaiming the name the old shared "Game" bounded context used, and park Game Language (`game/language/v1/...`) inside it "for now" until it is eventually retired -> applied as a further package-layout refinement within this WORK's already-approved package-restructuring scope, not a new material decision: `management/*` merged into the existing `game/` directory (which already held `language/v1/` and `docs/`), `package management` renamed to `package game` throughout, every `management.Xxx`/`playhoot/management` reference repository-wide updated (all confirmed confined to the package's own former tree plus `migrations.go` - no external caller existed to update, consistent with `Create` no longer depending on it). `go build`/`go vet`/`go test ./...` re-verified clean (same two pre-existing failures, no new ones). Documentation re-synchronized: `game/README.md` (new note explaining `game/` now names Game Management alone and that Game Language's presence there is a parking spot, not ownership), `ARCHITECTURE.md`'s package-layout block, `KNOWLEDGE_MAP.md`, `SYSTEM_MAP.md`, `domain-logic-placement.md`'s worked example, and this WORK's own Scope/Approved Design/Acceptance Criteria/Completion Record text updated throughout to `game/`.

Ready for independent review: YES.

**Independent review (2026-09-28)**: Performed per `docs/ai/protocols/IMPLEMENTATION_REVIEW.md` by a fresh reviewer, reasoning from the actual implementation commit's diff, current code, migrations, and documentation rather than this report. Verdict: **APPROVED**, no findings. Verification independently repeated: `go build ./...`/`go vet ./...` clean; `go test ./... -count=1` clean except the same two pre-existing `comment_standard_test.go` failures, independently confirmed pre-existing (unaffected by this WORK) by diffing the flagged files against their pre-move content; `gofmt -l`'s repository-wide hits independently confirmed to be a pre-existing CRLF/LF checkout artifact, not a regression (files this WORK added/rewrote are absent from that list). Acceptance criteria checked directly against the diff and a repository-wide (all file types) search for stale `game/management`/`game/session` import paths, which returned zero live hits. Real-Postgres integration coverage remains unexercised (same disclosed sandbox limitation).

Status: DONE.
