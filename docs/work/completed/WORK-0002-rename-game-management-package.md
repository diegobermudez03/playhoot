# WORK-0002: Rename `game/game` Package To `game/management`

Status: DONE
Created: 2026-09-18
Last status change: 2026-09-18

Related decisions:
- None. This is a pure physical package/directory rename with no product/architecture/domain behavior change; it does not require a new ADR/PDR.

Canonical context:
- `ARCHITECTURE.md` (Game bounded context; Game Management/Session Runtime/Game Language capabilities)
- `game/README.md` (Game Management capability description)
- `docs/architecture/SYSTEM_MAP.md` (current physical package diagram)
- `game/CURRENT_STATE.md`, `game/docs/FLOWS.md` (current-state docs referencing the physical path)
- `docs/engineering/standards/domain-logic-placement.md` (references `game/game/internal/businessservice` by path)

## Outcome

`game/game` (the Game Management capability's physical Go package/directory) is renamed to `game/management`, so the physical package layout under the Game bounded context (`game/`) reads as `management/` (Game Management), `session/` (Session Runtime), `language/` (Game Language) - matching the already-accepted conceptual capability model instead of the historical `game/game` directory name that predates that model being made explicit.

## Context

The human directly specified this exact rename, its intended meaning, its scope, and its constraints in this session's request. This is a pure structural/naming refactor with no product, architecture, or domain-boundary change - `ARCHITECTURE.md` and `game/README.md` already describe Game Management/Session Runtime/Game Language as the three capabilities of the Game bounded context; only the physical directory/package name for Game Management (`game/game`) has historically lagged that model.

Requested immediately after WORK-0001 (Session Lobby Foundation, Slice 1) reached DONE, and explicitly before Slice 2 begins. It is not part of the `session-runtime-v1` initiative's slice sequence and does not change Slice 2's scope/design in any way.

## Scope

### In Scope

- `git mv game/game game/management`.
- Renaming the top-level Go package declared directly inside that directory (`package game` in `errors.go`/`models.go`) to `package management`, consistent with the existing repository convention that a capability's top-level package name matches its directory name (e.g. `game/session` declares `package session`). Subpackages that already have their own distinct names (`getgame`, `getgamedefinition`, `businessservice`, `repo`, `migration`, `migrations`, `testdb`) are unaffected by this - only their import path changes, not their package name.
- Updating every Go import path referencing `github.com/diegobermudez03/playhoot/game/game...` to `github.com/diegobermudez03/playhoot/game/management...`, repository-wide.
- Updating every unaliased call site that references the renamed top-level package's exported identifiers (`game.Game`, `game.VisibilityType`, `game.Public`/`Private`/`Hidden`/`Draft`, `game.ErrBrokenGame`, `game.ErrNonPlayableGame`) to `management.X`, since this is a direct, mechanical consequence of the package name change - not a public-API redesign.
- Regenerating the `mockgen`-generated files whose source package moved (`game/management/usecases/getgame/repo_mock_test.go`, `game/management/usecases/getgamedefinition/repo_mock_test.go`, `game/session/workflows/sessionlifecycle/mocks_test.go`) via the existing `//go:generate mockgen` directives already present in each package, so generated headers/imports match the new path without hand-editing generated code.
- Updating current-state documentation that names the physical path (`game/CURRENT_STATE.md`, `game/docs/FLOWS.md`, `docs/architecture/SYSTEM_MAP.md`) and the one still-current engineering standard that names it as a live example (`docs/engineering/standards/domain-logic-placement.md`).
- Searching the repository afterward for any remaining `game/game` reference and resolving or explicitly accounting for each one.

### Out of Scope

- Any change to Game Management's, Session Runtime's, or Game Language's behavior, public contract, or domain semantics beyond the identifier changes strictly required by the package rename itself.
- Renaming `getgame`/`getgamedefinition`/any other subpackage that does not share the top-level directory's old name.
- Rewriting historical decision records (ADRs) or the completed `docs/work/completed/WORK-0001-session-lobby-foundation.md` to reflect the new path - those documents describe state as it was at specific points in time and are governed by `docs/decisions/README.md`'s Historical Immutability rule and `docs/work/README.md`'s Completed Work Immutability rule respectively. `docs/decisions/architecture/ADR-0001-intra-domain-responsibility-boundary.md` and `game/docs/decisions/GAME-ADR-0001-game-capability-persistence-transaction-boundary.md` reference `game/game/...` as historical-rationale evidence and are intentionally left unchanged.
- Rewriting past dated checkpoint narrative entries in `docs/ai/workspaces/active/session-runtime-v1/AI_CONTEXT.md`/`PLAN.md` that describe `game/game` as it existed at that checkpoint's time - only their live/current-facing sections are updated if they name the physical path as current fact.
- Starting, designing, or scoping Slice 2 (Start + First RuntimeTurn) in any way.
- Any opportunistic cleanup, refactor, or behavior change unrelated to the rename (including the previously-recorded, explicitly out-of-scope `game/game/usecases/getgame` test-assertion defect - unaffected by this rename beyond its own path update).

## Approved Design

No design beyond the mechanical rename described in Scope above. The physical package name is set to match the repository's existing convention (top-level capability package name equals its directory name, as already true for `game/session`); no other package structure changes.

## Constraints and Invariants

- No product/architecture/domain behavior change.
- No change to persisted schema, migrations' logical content, or transaction/consistency semantics - only the Go import path of the migration-runner packages changes.
- No change to any exported type/function/constant's meaning or shape - only the package-qualified name callers use to reference it, where that change is strictly required by the rename.
- `go build ./...`, `go vet ./...`, and `go test ./... -count=1` must remain clean after the rename (same pass/skip status as immediately before this WORK, module for the already-known out-of-scope `getgame` test-assertion defect recorded by WORK-0001, which this rename does not need to fix).
- No stale `game/game` reference should remain anywhere it would mislead a future reader about current physical structure (current-state docs, live engineering standards, Go source) - historical/decision-record references are the explicit, judged exception (see Out of Scope).

## Acceptance Criteria

- `game/game/` no longer exists; `game/management/` exists with the same file set (relocated, not duplicated).
- `grep -r "game/game"` across the repository returns only: (a) historical decision-record rationale (ADR-0001, GAME-ADR-0001), (b) the completed `WORK-0001` file, (c) past dated checkpoint narrative entries in the `session-runtime-v1` workspace - each judged and left as historical per Out of Scope above. No current-state doc, live engineering standard, or Go source file contains a stale `game/game` reference.
- `go build ./...` succeeds repository-wide.
- `go vet ./...` is clean repository-wide.
- `go test ./... -count=1` shows the same pass/fail/skip shape as before this WORK (all currently-passing tests still pass; the previously-recorded out-of-scope `getgame` test-assertion defect, if still unresolved, is unaffected by the rename itself).
- Every `mockgen`-generated file affected by the moved import path is regenerated (not hand-patched) and compiles.

## Implementation Freedom

- Exact `git mv`/edit ordering.
- Whether import aliases already present (e.g. `gamemigrations`, `gamemigration`) are kept as-is or renamed for clarity - keeping them is acceptable since they still describe Game Management migrations accurately and changing them is not required by the rename.
- Minor doc wording adjustments strictly necessary to keep an updated path reference grammatically coherent.

## Verification

- `go build ./...`
- `go vet ./...`
- `go test ./... -count=1` (using the same real-Postgres `TEST_DATABASE_*` setup already available in this sandbox where applicable)
- `gofmt -l` on every file this WORK changes.
- Repository-wide search for remaining `game/game` references, with each remaining occurrence accounted for.

## Documentation Impact

### Accepted / Canonical Knowledge

- None. `ARCHITECTURE.md`/`game/README.md` already describe the capability model conceptually and do not name the physical path.

### Current-State Documentation After Implementation

- `game/CURRENT_STATE.md` - update `game/game/...` path references to `game/management/...`.
- `game/docs/FLOWS.md` - update `game/game/...` path references to `game/management/...`.
- `docs/architecture/SYSTEM_MAP.md` - update the `game/game` node label to `game/management`.
- `docs/engineering/standards/domain-logic-placement.md` - update its live `game/game/internal/businessservice` path reference.

### Intentionally Unchanged

- `docs/decisions/architecture/ADR-0001-intra-domain-responsibility-boundary.md`, `game/docs/decisions/GAME-ADR-0001-game-capability-persistence-transaction-boundary.md` - historical decision rationale, governed by Historical Immutability.
- `docs/work/completed/WORK-0001-session-lobby-foundation.md` - completed work spec, governed by Completed Work Immutability.
- Past dated checkpoint entries in `docs/ai/workspaces/active/session-runtime-v1/AI_CONTEXT.md`/`PLAN.md` - historical narrative describing state at each checkpoint's time.

## Blockers

- None.

## Completion Record

Status: DONE, 2026-09-18.

**Implementation summary**: `game/game` (the Game Management capability) was renamed to `game/management` via `git mv`, with the top-level Go package declaration changed from `package game` to `package management` (matching the repository's existing convention that a capability's top-level package name equals its directory name, as already true for `game/session`'s `package session`). Every Go import path referencing the old `github.com/diegobermudez03/playhoot/game/game...` was updated to `.../game/management...` across `game/management/**` itself, `game/session/workflows/sessionlifecycle/{step_create.go,step_create_test.go,testutil_test.go,mocks_test.go}`, and the root `migrations.go`. Call sites qualifying the renamed package's exported identifiers (`game.Game`, `game.Public`/`Private`/`Hidden`/`Draft`, `game.VisibilityType`, `game.ErrBrokenGame`, `game.ErrNonPlayableGame`) were updated to `management.X` in `game/management/internal/businessservice/service.go`, `game/management/usecases/getgame/{service.go,service_test.go}`, and the three `sessionlifecycle` files above. Three `mockgen`-generated files were regenerated/patched to reflect the new import path: `game/management/usecases/getgamedefinition/repo_mock_test.go` and `game/session/workflows/sessionlifecycle/mocks_test.go` were cleanly regenerated via their existing `//go:generate mockgen` directives; `game/management/usecases/getgame/repo_mock_test.go` was hand-patched (header comment only) instead of regenerated, because the installed `mockgen` version would have renamed pre-existing exported test symbols (`MockRepoAPI` -> `MockrepoAPI`) as an unrelated side effect - reverted that regeneration and applied only the one-line `Source:` path fix, to keep the diff mechanical per this WORK's own constraint.

**Independent review**: Performed per `docs/ai/protocols/IMPLEMENTATION_REVIEW.md` by a fresh reviewer. First pass: **CHANGES_REQUIRED** - one REQUIRED_FIX (gofmt-broken import ordering in `step_create.go`/`step_create_test.go`/`testutil_test.go`, where the `game/management` import landed alphabetically before `game/language/v1/...` instead of after). Fixed via `gofmt -w` on the three files. Re-review verdict: **APPROVED** - fix verified, no new issues, no regressions.

**Verification actually performed**:
- `go build ./...` - clean.
- `go vet ./...` - clean.
- `gofmt -l` on every file this WORK touched - clean (after the import-order fix); the many other repository files `gofmt -l` flags are a pre-existing, environment-wide CRLF/LF artifact unrelated to this WORK (confirmed via `gofmt -d` showing pure line-ending diffs), consistent with what WORK-0001's verification passes already documented.
- `go test ./... -count=1` (against the real PostgreSQL instance already running in this sandbox, `TEST_DATABASE_*` exported) - identical pass/fail shape to immediately before this WORK: every package passes except `game/management/usecases/getgame`'s `TestRepoGetGameCurrentVersion`, the pre-existing, already-recorded (in WORK-0001's Completion Record) JSONB-canonical-serialization test-assertion defect that predates this WORK and remains explicitly out of its scope to fix.
- Repository-wide `grep -rn "game/game"` - remaining hits are exactly the ones judged historical in Out of Scope above (two ADRs, the completed WORK-0001, past dated `session-runtime-v1` workspace checkpoint narrative) plus this WORK's own spec describing the rename; no current-state doc, live standard, or Go source retains a stale reference.

**Documentation synchronized**: `game/CURRENT_STATE.md`, `game/docs/FLOWS.md`, `docs/architecture/SYSTEM_MAP.md`, `docs/engineering/standards/domain-logic-placement.md` updated to `game/management`.

**Approved material deviations**: None - this is a non-material mechanical rename; no product/architecture/domain decision was made or required.

**Local implementation decisions made**: (1) Full package-name rename (`game` -> `management`), not merely an import alias, per the request's explicit "package/directory" phrasing and to match the existing `game/session` convention. (2) Hand-patched rather than regenerated `getgame/repo_mock_test.go`'s header comment, to avoid an unrelated mockgen-version-drift rename of exported test-only mock symbols.

**Explicitly out-of-scope follow-up carried forward, unaffected by this WORK**: `game/management/usecases/getgame`'s `TestRepoGetGameCurrentVersion` JSONB-comparison test-assertion defect (see WORK-0001's Completion Record) - still unfixed, still not this WORK's concern.

**Known limitations**: None material.
