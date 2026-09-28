# WORK-0046: Frontend Package Serving & Versioned Asset Delivery

Status: PLANNED
Created: 2026-09-27
Last status change: 2026-09-27

Related decisions:
- `docs/decisions/architecture/ADR-0015-javascript-rule-execution-and-iframe-frontend-contract.md`

Canonical context:
- `docs/projects/active/session-runtime-v1/works/WORK-0009-client-safe-game-ui-manifest.md` (overlapping concern — see Reconciliation below)

## Outcome

Build the backend capability to store and serve a Session's pinned frontend package/assets (per `WORK-0044`'s artifact model) so Playhoot's own (separately built) frontend can load the correct iframe-delivered package for a given Session's version. This directly overlaps `session-runtime-v1`'s own `WORK-0009` (Client-Safe Game UI Manifest, PLANNED) — that WORK's original premise (a manifest describing Game Language's declarative UI tree) is retired along with Game Language itself; its actual need (some client-safe, version-pinned way to know what to render) is superseded by this WORK's frontend-package-serving capability. This reconciliation is flagged here and in `session-runtime-v1`'s own `PROJECT.md`, not silently resolved by duplicating both.

## Context

Not yet designed. Depends on `WORK-0044` and `WORK-0045`.

## Scope

Not yet designed.

## Approved Design

Not yet designed.

## Constraints and Invariants

- Must serve exactly the version a Session is pinned to (`GAME-ADR-0001`'s immutability invariant), never a Game's current/latest version.

## Acceptance Criteria

Not yet designed.

## Implementation Freedom

Not yet designed.

## Verification

Not yet designed.

## Documentation Impact

### Current-State Documentation After Implementation

- `session-runtime-v1`'s `PROJECT.md` — `WORK-0009` marked superseded by this WORK once this WORK is DRAFT/READY.

## Blockers

- Depends on `WORK-0044`, `WORK-0045`.
- Reconciliation with `session-runtime-v1`'s `WORK-0009` needs explicit human confirmation (cancel WORK-0009 as superseded, vs. keep a narrower WORK-0009 for something this WORK doesn't cover) before either moves to DRAFT.

## Completion Record

Not yet started.
