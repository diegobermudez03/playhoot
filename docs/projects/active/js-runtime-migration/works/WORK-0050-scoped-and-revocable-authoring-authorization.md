# WORK-0050: Scoped & Revocable Authoring Authorization

Status: PLANNED
Created: 2026-09-27
Last status change: 2026-09-27

Related decisions:
- `docs/decisions/architecture/ADR-0015-javascript-rule-execution-and-iframe-frontend-contract.md`

Canonical context:
- `identity/README.md`, `identity/CURRENT_STATE.md` (Identity is not yet implemented — see Blockers)

## Outcome

Build authoring-access authorization that is scoped (limited to a specific project/draft, not blanket account-wide access), separates editing/testing from publishing/access to other projects, and is revocable. This applies to both AI-authoring modalities (`WORK-0048`, `WORK-0049`) and any future human co-authoring capability.

## Context

Not yet designed. Materially constrained by Identity/Auth not being implemented yet (`identity/CURRENT_STATE.md`) — this WORK cannot assume a real credential/authentication system exists and must state explicitly what it depends on Identity providing versus what it can build against today's simplified caller-supplied-identity assumption (the same assumption `session-runtime-v1` already operates under).

## Scope

Not yet designed.

## Approved Design

Not yet designed.

## Constraints and Invariants

- Publish authority must be separable from edit/test authority — an authorization scoped to edit/test must never implicitly grant publish.
- Must be revocable without requiring a broader account-level credential change.

## Acceptance Criteria

Not yet designed.

## Implementation Freedom

Not yet designed.

## Verification

Not yet designed.

## Documentation Impact

Not yet designed.

## Blockers

- Depends on how much of this can be built before real Identity/Auth exists — needs explicit human scoping (build against today's simplified identity assumption vs. wait).

## Completion Record

Not yet started.
