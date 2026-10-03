# WORK-0049: Integrated Authoring Chat Backend Orchestration

Status: PLANNED
Created: 2026-09-27
Last status change: 2026-09-29 (removed from `docs/projects/active/js-runtime-migration/`, now standalone under `docs/work/active/`, see Reparenting Note below)

Related decisions:
- `docs/decisions/architecture/ADR-0015-javascript-rule-execution-and-iframe-frontend-contract.md`

Canonical context:
- None yet.

## Reparenting Note (2026-09-29)

Removed from `docs/projects/active/js-runtime-migration/` by explicit human decision: that Project migrates the JavaScript execution runtime and its supporting backend capabilities, and must not carry Game creation/authoring capabilities — a distinct concern belonging to its own future Project (informally, "game creation"), not yet created. This WORK moves to standalone (`docs/work/active/`) pending that future Project's creation. Its Outcome/Constraints below are deliberately preserved as-is, recording the intended shape of this capability (budget-controlled, resumable chat orchestration over the same validated authoring capabilities) for a future session designing the "game creation" Project's WORK. Still PLANNED, still not yet designed - this move changes ownership/grouping only.

## Outcome

Build the backend orchestration for Playhoot's own integrated authoring chat (operating on the same `WORK-0047` capabilities `WORK-0048` exposes externally), including consumption/budget controls that prevent unbounded AI usage and preserve authoring progress if an operation is stopped mid-way. The chat's own visual interface is explicitly out of scope — it belongs to the future frontend repository; this WORK is backend orchestration only.

## Context

Not yet designed. Depends on `WORK-0047` and the AI provider/model decision — a separate business/vendor decision, out of this WORK's own scope to make but assumed as an interface by this WORK's design.

## Scope

### Out of Scope

- The chat's visual/UI implementation (future frontend repository).

## Approved Design

Not yet designed.

## Constraints and Invariants

- Must not allow unbounded AI-usage cost accumulation without an enforced budget/consumption control.
- Must preserve authoring progress already made when an operation is interrupted, rather than discarding it.

## Acceptance Criteria

Not yet designed.

## Implementation Freedom

Not yet designed.

## Verification

Not yet designed.

## Documentation Impact

Not yet designed.

## Blockers

- Depends on `WORK-0047`.
- AI provider/model/commercial terms are a separate business/vendor decision, not this WORK's own to make, but needed as at least a placeholder interface assumption.

## Completion Record

Not yet started.
