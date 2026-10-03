# WORK-0047: Incremental Authoring Session Backend

Status: PLANNED
Created: 2026-09-27
Last status change: 2026-09-29 (removed from `docs/projects/active/js-runtime-migration/`, now standalone under `docs/work/active/`, see Reparenting Note below)

Related decisions:
- `docs/decisions/architecture/ADR-0015-javascript-rule-execution-and-iframe-frontend-contract.md`

Canonical context:
- `game/language/v1/program/DEFINITION.md`, `GAME_BRIEF.md` (the current single-shot AI-authoring flow this replaces)

## Reparenting Note (2026-09-29)

Removed from `docs/projects/active/js-runtime-migration/` by explicit human decision: that Project migrates the JavaScript execution runtime and its supporting backend capabilities, and must not carry Game creation/authoring capabilities — a distinct concern belonging to its own future Project (informally, "game creation"), not yet created. This WORK moves to standalone (`docs/work/active/`) pending that future Project's creation. Its Outcome/Context/Constraints below are deliberately preserved as-is, not stripped - they already record the intended shape of this capability (an incremental, resumable authoring session validated/simulated/rendered through the real execution path, shared by both AI-authoring modalities) so a future session designing the "game creation" Project's WORK does not have to reconstruct that thinking from scratch. Still PLANNED, still not yet designed - this move changes ownership/grouping only, not design maturity.

## Outcome

Build the backend capability for incremental, persistent game authoring: read documentation/an existing draft, edit the draft's files/components, validate it (`WORK-0037`'s lint plus `WORK-0039`'s protocol validation), simulate it, review a render (using the same real execution path production Sessions use and `WORK-0045`'s real contract, not an invented approximation), and correct — as a durable, resumable session, not a single JSON-generation call. This is the shared foundation both AI-authoring modalities (`WORK-0048` external tool integration, `WORK-0049` integrated chat) operate on, so neither gets a privileged path that skips validation.

## Context

Not yet designed. Depends on `WORK-0037`, `WORK-0044` existing enough to validate/simulate/render against, and on `WORK-0053`'s `Executor` port existing enough to simulate against the same real, separately deployed execution path production Sessions use (per `docs/decisions/architecture/ADR-0016-javascript-executor-separately-deployed-infrastructure-service.md`) — this WORK must not invent its own shortcut, in-process execution path merely because it is "just a simulation."

## Scope

Not yet designed.

## Approved Design

Not yet designed.

## Constraints and Invariants

- Simulations must use the same rules and relevant guarantees as real sessions (`ADR-0015`'s mandate) — no separate, weaker "preview" execution path.
- Renders must reflect the actual reviewed draft, never a fabricated/approximated image.

## Acceptance Criteria

Not yet designed.

## Implementation Freedom

Not yet designed.

## Verification

Not yet designed.

## Documentation Impact

### Accepted / Canonical Knowledge

- Successor authoring-facing documentation to `game/language/v1/program/DEFINITION.md`/`GAME_BRIEF.md`, reflecting the JavaScript authoring surface.

## Blockers

- Depends on `WORK-0035`, `WORK-0037`, `WORK-0044`.

## Completion Record

Not yet started.
