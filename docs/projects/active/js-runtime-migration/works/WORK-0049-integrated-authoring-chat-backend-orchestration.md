# WORK-0049: Integrated Authoring Chat Backend Orchestration

Status: PLANNED
Created: 2026-09-27
Last status change: 2026-09-27

Related decisions:
- `docs/decisions/architecture/ADR-0015-javascript-rule-execution-and-iframe-frontend-contract.md`

Canonical context:
- None yet.

## Outcome

Build the backend orchestration for Playhoot's own integrated authoring chat (operating on the same `WORK-0047` capabilities `WORK-0048` exposes externally), including consumption/budget controls that prevent unbounded AI usage and preserve authoring progress if an operation is stopped mid-way. The chat's own visual interface is explicitly out of scope — it belongs to the future frontend repository; this WORK is backend orchestration only.

## Context

Not yet designed. Depends on `WORK-0047` and the AI provider/model decision (`PROJECT.md` Material Decisions #6, out of this Project's scope to make but assumed as an interface by this WORK's design).

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
- AI provider/model/commercial terms are out of this Project's scope but needed as at least a placeholder interface assumption.

## Completion Record

Not yet started.
