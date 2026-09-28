# WORK-0034: Session-Owned Executable Script Artifact & Package Restructuring

Status: PLANNED
Created: 2026-09-27
Last status change: 2026-09-27

Related decisions:
- `docs/decisions/architecture/ADR-0014-management-session-domain-split.md`
- `docs/decisions/architecture/ADR-0015-javascript-rule-execution-and-iframe-frontend-contract.md`
- `game/docs/decisions/GAME-ADR-0028-javascript-execution-replaces-game-language.md`

Canonical context:
- `ARCHITECTURE.md` (Accepted Business Boundaries, Dependency Principles)
- `game/README.md` (Internal Structure, Capability Persistence and Transaction Boundary)
- `docs/engineering/standards/domain-logic-placement.md`
- `game/session/workflows/sessionlifecycle/manager.go` and its `step_*.go` call sites
- `game/management/models.go`, `game/management/usecases/getgame/`, `game/management/usecases/getgamedefinition/`

## Outcome

Supersedes cancelled `docs/projects/completed/management-session-domain-split/works/WORK-0031-session-owned-executable-program-management-session-split.md` — see that WORK's own Completion Record. Its underlying outcome is conserved: Session Runtime stops depending on Game Management at runtime for anything beyond, at most, a one-time Composer-mediated Create-time visibility check (`WORK-0032`), and owns its own persisted copy of the executable artifact it runs, keyed by the same immutable definition/version identity already pinned at `Create`. What changes from WORK-0031's own design is the artifact's content type: two mandatory scripts (backend JavaScript rules and a stored frontend script) plus optional contract/asset metadata, per `WORK-0044`'s artifact model, not a compiled Game Language `program.Definition`.

This WORK also completes `ADR-0014`'s package-restructuring goal: Game Management and Session Runtime become independent top-level packages, no longer nested under a shared `game/` bounded-context root, and Game Language's successor (the JavaScript execution boundary) lives inside Session Runtime's own package tree, not importable from Game Management.

## Context

Not yet designed. `WORK-0044` is DONE — `game/docs/GAME_VERSION_ARTIFACT_MODEL.md` now defines the shape this WORK must persist: `DefinitionUUID`, mandatory `BackendScript` and `FrontendScript`, optional `GameContract`/`Assets`/`PlatformContractVersion`. Designing this table against WORK-0031's old (now-retired) artifact shape would have repeated WORK-0031's own mistake of designing against a shape about to change; that risk no longer applies.

## Scope

Not yet designed.

## Approved Design

Not yet designed.

## Constraints and Invariants

- `GAME-ADR-0001`'s version-immutability invariant (a Session's pinned artifact/version must remain semantically stable for that Session's lifetime) is unaffected and must continue to hold.
- No database transaction may span Game Management-owned and Session-Runtime-owned tables (`ARCHITECTURE.md -> State and Transaction Boundaries`).
- No direct call from Session Runtime into Game Management may remain for any pinned-artifact read.

## Acceptance Criteria

Not yet designed.

## Implementation Freedom

Not yet designed.

## Verification

Not yet designed.

## Documentation Impact

### Accepted / Canonical Knowledge

- `ARCHITECTURE.md` — Accepted Business Boundaries rewritten to state the new boundary directly.
- `game/README.md` — split into `management/README.md`/`session/README.md`, each following `docs/ai/templates/domain/`.
- `docs/ai/KNOWLEDGE_MAP.md` — Accepted Domain Documentation table updated.

### Current-State Documentation After Implementation

- `game/docs/DATA_MODEL.md` (or its new home) — add the new artifact table, remove Game Language's retired tables/entities.
- `game/CURRENT_STATE.md` (or its split successors) — reflect the removed Game Management dependency, new package layout, and retired Game Language.

## Blockers

- Same bootstrapping question WORK-0031 identified: something must supply the artifact's first row for a version Session Runtime has never seen, until `WORK-0033`'s publish composition exists — needs joint resolution with `WORK-0032`.
- Depends on `WORK-0044`'s artifact-model design maturing enough to fix this table's shape.

## Completion Record

Not yet started.
