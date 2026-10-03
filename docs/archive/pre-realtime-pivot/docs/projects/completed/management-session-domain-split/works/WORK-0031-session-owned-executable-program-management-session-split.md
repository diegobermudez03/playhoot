# WORK-0031: Session-Owned Executable Game Definition — Management/Session Domain Split

Status: CANCELLED
Created: 2026-09-27
Last status change: 2026-09-27 (CANCELLED — superseded by WORK-0034 under `docs/projects/active/js-runtime-migration/`)

Related decisions:
- `docs/decisions/architecture/ADR-0014-management-session-domain-split.md`
- `game/docs/decisions/GAME-ADR-0001-game-capability-persistence-transaction-boundary.md` (superseded in part by ADR-0014)

Canonical context:
- `ARCHITECTURE.md` (Accepted Business Boundaries, Dependency Principles)
- `game/README.md` (Internal Structure, Capability Persistence and Transaction Boundary)
- `docs/engineering/standards/domain-logic-placement.md`
- `game/session/workflows/sessionlifecycle/manager.go`, `step_create.go`, `step_join.go`, `step_start.go`, `step_answer_interaction.go`, `step_submit_user_intent.go`, `step_cancel_session.go`, `step_expire_timer.go`
- `game/management/models.go`, `game/management/usecases/getgame/`, `game/management/usecases/getgamedefinition/`

## Outcome

Session Runtime stops depending on Game Management at runtime for anything beyond, at most, a one-time Create-time visibility check (see Blockers). Concretely:

- Session Runtime owns its own persisted copy of the executable Game Language definition it runs, keyed by the same immutable definition/version UUID already pinned at `Create` (`sessions.game_definition_uuid`).
- Every one of Session Runtime's six pinned-definition reads (`Join`, `Start`, `AnswerInteraction`, `SubmitUserIntent`, `CancelSession`, `ExpireTimer` — everything using today's `gamePinnedDefinitionReader`) reads from that Session-Runtime-owned table instead of calling Game Management.
- Game Language (`program`, `engine`, `engineservice`, and their supporting packages) becomes Session Runtime's own internal implementation, no longer importable from Game Management.
- Game Management and Session Runtime become independent top-level packages, no longer nested under a shared `game/` bounded-context root.

This is required because the review that produced ADR-0014 found Game Management's own exported `Game` type (`management.Game`) embeds `program.Definition` — a Game Language type Game Management does not conceptually own — so every consumer of Game Management's narrow read contract transitively depends on Game Language regardless of how narrow its own interface is. Fixing only that return-type shape would leave the deeper problem: six of Session Runtime's seven RuntimeTurn-capable operations call into Game Management on every single invocation, for the Session's entire lifetime, solely to re-read data that never changes after `Create`. See ADR-0014's Context/Rationale for the full reasoning.

## Context

Today, `sessionlifecycle.Manager` holds two Game Management read dependencies (`game/session/workflows/sessionlifecycle/manager.go:76-77`): `currentGameReader` (`gameCurrentVersionReader`, used only by `Create` to resolve the Game's *current* playable version) and `pinnedGameReader` (`gamePinnedDefinitionReader`, used by every other RuntimeTurn-capable step to re-read the *already-pinned, immutable* version by UUID). Both return `program.Definition` values sourced from `management.Game`/`management` usecases (`game/management/usecases/getgame`, `game/management/usecases/getgamedefinition`), which decode a stored JSON script via `gameservice.DecodeJSON`.

Game Management currently exposes only these two read use cases — no create/publish/authoring write path exists yet in this codebase. This matters for scope: this WORK cannot design "how does a newly authored Game version's script get into Session Runtime's new table," because the thing that would populate it (Game Management's authoring/publish flow) does not exist yet. See Blockers.

## Scope

### In Scope

- New Session-Runtime-owned migration/table for the executable Game Language definition, keyed by immutable definition/version UUID (not Game UUID — a Game has many authored versions over its lifetime; Session Runtime's own copy must be one row per immutable version, matching the granularity `sessions.game_definition_uuid` already pins at).
- A new internal repository method on `sessionlifecycle`'s own `internal/repo` (or a new sibling internal package, implementer's choice) reading that table, replacing `gamePinnedDefinitionReader.GetGameDefinition` at all six call sites (`step_join.go`, `step_start.go`, `step_answer_interaction.go`, `step_submit_user_intent.go`, `step_cancel_session.go`, `step_expire_timer.go`) and their tests/mocks.
- Removing `gamePinnedDefinitionReader` (interface, field, constructor parameter, mocks) from `sessionlifecycle` entirely.
- Moving `game/language/v1/...` to become Session-Runtime-internal (target: `game/session/internal/language/v1/...`, or the equivalent post-move path once `game/session` itself moves — see below), so it is compiler-enforced non-importable from Game Management, not merely conventionally private. Update every import path across `engineservice`, `program`, and every file currently importing `game/language/v1/...`.
- Moving `game/management` and `game/session` to become independent top-level packages (target: `management/`, `session/`), retiring `game/README.md`/`game/CURRENT_STATE.md`/`game/docs/` as a shared bounded-context root. Splitting `game/README.md` into `management/README.md` and `session/README.md` (each following `docs/ai/templates/domain/`), preserving `game/docs/decisions/` as Session Runtime's own decision family (its `GAME-ADR-*` identifiers/history are unaffected by this rename — renumbering/renaming existing ADR identifiers is out of scope, see `docs/decisions/README.md`'s historical-immutability rule) — implementer's call whether the directory itself is renamed or left at its current path with updated ownership text; do not rename existing `GAME-ADR-NNNN` filenames/identifiers regardless.
- Updating every canonical/current-state doc this move touches: `docs/ai/KNOWLEDGE_MAP.md`'s Accepted Domain Documentation table (add `Management`/`Session` rows, remove/retire the `Game` row), `ARCHITECTURE.md`'s Accepted Business Boundaries (replacing the superseded-in-part annotation with the actual new accepted state), `game/docs/DATA_MODEL.md`/`game/docs/FLOWS.md` equivalents under their new home.
- Removing Game Management's dependency on `program`/`gameservice`/`engine` (its `getgame`/`getgamedefinition` use cases currently call `gameservice.DecodeJSON` — see Blockers for what replaces script validation there, if anything, within this WORK's scope).

### Out of Scope

- The Composer-mediated Create-time visibility check replacing `Create`'s current direct call to `gameCurrentVersionReader` — tracked separately as WORK-0032, sequenced per this Project's Ordering/Dependencies.
- Any Game Management authoring/publish write path, and the future Orchestrator-mediated publish composition — tracked separately as WORK-0033.
- Any change to `sessionlifecycle`'s RuntimeTurn/replay/persistence model beyond the new table and removed dependency — `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md`'s existing accepted model is otherwise unaffected.
- Backfilling/migrating any existing production data — this codebase has no production Sessions yet; the migration only needs to work going forward.

## Approved Design

- New table (name/exact columns are the implementer's naming call within this shape; illustrative): one row per immutable definition/version UUID, storing at minimum `definition_uuid` (unique, matches `sessions.game_definition_uuid`), the encoded script/definition content in the same wire shape Game Management stores today, and `created_at`. No foreign key to Game Management's tables (per `ARCHITECTURE.md -> Domain Boundaries`).
- The new repository read method's signature/behavior should mirror today's `gamePinnedDefinitionReader.GetGameDefinition(ctx, gameDefinitionUUID) (*program.Definition, error)` exactly (including its `nil, nil`-if-missing shape), so the six call sites' surrounding step logic changes minimally — only the dependency being called changes, not each step's own control flow.
- Per `docs/engineering/standards/domain-logic-placement.md -> A Workflow's Public Contract Lives At The Domain Root`, this new repository method is `sessionlifecycle`-internal (`internal/repo` or a new sibling internal package) — it is not part of `game/session`'s zero-dependency root, the same way today's repository methods aren't.
- `program.Definition` itself remains the value type this new method returns — it does not need session's own mirror/translation type the way `engine.Output`/`engine.Value` got one in WORK-0029, because after this WORK, `program` is Session Runtime's own internal implementation (moved under `session/internal/language/`), not a third domain's type anymore. There is nothing left to decouple from once Game Management no longer touches it.

## Constraints and Invariants

- `GAME-ADR-0001`'s version-immutability invariant (a Session's pinned definition/version must remain semantically stable for that Session's lifetime) is unaffected and must continue to hold against the new table exactly as it holds today against Game Management's.
- No database transaction may span Game Management-owned and Session-Runtime-owned tables (`ARCHITECTURE.md -> State and Transaction Boundaries`, restated by ADR-0014) — the new table lives entirely inside Session Runtime's own transaction boundary.
- `ARCHITECTURE.md -> Domain Boundaries`: no direct call from Session Runtime into Game Management may remain for any of the six pinned-read call sites this WORK covers.

## Acceptance Criteria

- None of `step_join.go`, `step_start.go`, `step_answer_interaction.go`, `step_submit_user_intent.go`, `step_cancel_session.go`, `step_expire_timer.go` import `game/management` (or its post-move path) or reference `gamePinnedDefinitionReader` after this WORK.
- `sessionlifecycle.Manager` no longer has a `pinnedGameReader` field/constructor parameter.
- Game Management's package (post-move `management/`) does not import `program`/`gameservice`/`engine` (or their post-move paths) anywhere.
- `go build ./...` succeeds with `game/language/v1/...` moved under Session Runtime's internal tree, with no remaining import of its old path anywhere in the repository.
- Existing tests for all six touched steps pass against the new repository dependency (mocked the same way today's `MockgamePinnedDefinitionReader` is), and the new table's own repository method has integration test coverage against real Postgres, matching this codebase's existing repository-integration-test convention (see e.g. `game/session/workflows/sessionlifecycle/internal/repo/join_code_test.go`).

## Implementation Freedom

Exact table/column naming, exact new-package internal layout (`internal/repo` extension vs. a new sibling internal package for this specific read), and exact file-move mechanics are local implementation choices within the Operating Model's normal autonomy boundary. Whether `game/management`/`game/session` physically move in this same WORK or a follow-up purely-mechanical WORK is also implementer's call, given the review found no other material blocker to doing it together — but do not leave the repository in a state where the new table exists while `sessionlifecycle` still also depends on Game Management for the same data (no dual-read transitional state), since that would silently violate this WORK's own Acceptance Criteria.

## Verification

- `go test ./...` across every touched package.
- Real-Postgres repository-integration tests for the new table/repository method, per this codebase's established convention (see `docs/projects/active/session-runtime-v1/PROJECT.md`'s repeated "real-Postgres verification" completion gate for precedent).
- Independent review per `docs/ai/protocols/IMPLEMENTATION_REVIEW.md`, given this WORK changes public package structure and a persisted schema.

## Documentation Impact

### Accepted / Canonical Knowledge

- `ARCHITECTURE.md` — Accepted Business Boundaries rewritten to state the new boundary directly (no longer a superseded-in-part annotation over old text).
- `game/README.md` — split into `management/README.md`/`session/README.md` (or renamed in place, per Implementation Freedom above), each following `docs/ai/templates/domain/`.
- `docs/ai/KNOWLEDGE_MAP.md` — Accepted Domain Documentation table updated (Management/Session rows replace the Game row).

### Current-State Documentation After Implementation

- `game/docs/DATA_MODEL.md` (or its new home) — add the new table.
- `game/CURRENT_STATE.md` (or its split successors) — reflect the removed Game Management dependency and new package layout.

### Intentionally Unchanged

- `game/docs/decisions/*` (`GAME-ADR-NNNN` identifiers/filenames/history) are not renumbered or moved as part of this WORK, regardless of whether their containing directory's physical path changes.

## Blockers

- **Bootstrapping the new table's first row per version.** Game Management has no authoring/publish write path yet (WORK-0033, out of scope here), so nothing currently populates the new table when a Game version is first authored. Until WORK-0033 exists, something must still supply the script content for a definition/version UUID Session Runtime has never seen before — most plausibly, `Create` (still, today, the one operation that must learn "what does this Game's current version contain") receiving that content already resolved (by a future Composer per WORK-0032, or, in the interim, by `Create` itself still calling Game Management once, only to populate this new table on first pin) rather than every other operation reading it repeatedly. This WORK does not resolve which of those interim shapes `Create` uses — that is materially entangled with WORK-0032's own design and needs human decision before this WORK can move to READY.
- **Deployment sequencing with WORK-0032**, per this Project's own `PROJECT.md` Ordering/Dependencies section — whether this WORK is allowed to ship with `Create`'s direct Game Management call temporarily kept (see bootstrapping blocker above) versus requiring WORK-0032 to land in the same release. Needs human decision.
- **Exact new table name/columns** — no material design risk identified, but not yet fixed; a reasonable implementer default is proposed in Approved Design above and does not block READY on its own, only the two blockers above do.

## Completion Record

**Cancelled (2026-09-27), superseded, not implemented.** `docs/decisions/architecture/ADR-0015-javascript-rule-execution-and-iframe-frontend-contract.md` and `game/docs/decisions/GAME-ADR-0028-javascript-execution-replaces-game-language.md` retire Game Language (`program`/`engine` v1) as Session Runtime's execution mechanism, replacing it with sandboxed JavaScript. This WORK's Approved Design specifically persisted the executable artifact as encoded Game Language `program.Definition` content ("the same wire shape Game Management stores today") — an artifact type that no longer exists once Game Language is retired. Its underlying goal (Session Runtime owning its own persisted executable-artifact copy, independent of Game Management, per `ADR-0014`) is conserved, not abandoned: it is carried forward as `docs/projects/active/js-runtime-migration/works/WORK-0034-session-owned-executable-script-artifact-and-package-restructuring.md`, redesigned against a JavaScript-artifact shape instead of a compiled DSL row. No implementation or migration from this WORK was ever started; nothing is reverted.
