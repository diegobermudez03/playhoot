# WORK-0048: MCP Tool Adapter For External AI Authoring

Status: PLANNED
Created: 2026-09-27
Last status change: 2026-09-29 (removed from `docs/projects/active/js-runtime-migration/`, now standalone under `docs/work/active/`, see Reparenting Note below)

Related decisions:
- `docs/decisions/architecture/ADR-0015-javascript-rule-execution-and-iframe-frontend-contract.md`

Canonical context:
- None yet.

## Reparenting Note (2026-09-29)

Removed from `docs/projects/active/js-runtime-migration/` by explicit human decision: that Project migrates the JavaScript execution runtime and its supporting backend capabilities, and must not carry Game creation/authoring capabilities — a distinct concern belonging to its own future Project (informally, "game creation"), not yet created. This WORK moves to standalone (`docs/work/active/`) pending that future Project's creation. Its Outcome/Constraints below are deliberately preserved as-is, recording the intended shape of this capability (external AI tools reach the same validated authoring capabilities/authorization as Playhoot's own integrated chat, no privileged path) for a future session designing the "game creation" Project's WORK. Still PLANNED, still not yet designed - this move changes ownership/grouping only.

## Outcome

Expose `WORK-0047`'s incremental authoring-session capabilities to an externally connected AI tool via an MCP (Model Context Protocol) adapter, so an author using an external AI assistant operates on the same authoring capabilities and validations as Playhoot's own integrated chat (`WORK-0049`) — no privileged/unvalidated path.

## Context

Not yet designed. Depends on `WORK-0047`.

## Scope

Not yet designed.

## Approved Design

Not yet designed.

## Constraints and Invariants

- Must not expose any authoring capability beyond what `WORK-0050`'s authorization model grants the calling credential.

## Acceptance Criteria

Not yet designed.

## Implementation Freedom

Not yet designed.

## Verification

Not yet designed.

## Documentation Impact

Not yet designed.

## Blockers

- Depends on `WORK-0047`, `WORK-0050`.

## Completion Record

Not yet started.
