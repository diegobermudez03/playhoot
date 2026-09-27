# WORK-0032: Composer-Mediated Session Creation (Game Visibility Composition)

Status: PLANNED
Created: 2026-09-27
Last status change: 2026-09-27

Related decisions:
- `docs/decisions/architecture/ADR-0014-management-session-domain-split.md`

Canonical context:
- `ARCHITECTURE.md` (Cross-Domain Reads, Composer)
- `composer/README.md`
- `game/session/workflows/sessionlifecycle/step_create.go` (current direct-call shape being replaced)

## Outcome

Today, `sessionlifecycle.Manager.Create` calls Game Management directly (`gameCurrentVersionReader.GetPlayableGameWithCurrentVersion`) to resolve the Game's current playable version and enforce visibility before creating a Session. WORK-0031 removes that direct cross-domain call as part of dissolving the Game Management/Session Runtime coupling (ADR-0014). Something must still enforce "a Session may only be created for a currently-playable/visible Game" — this WORK is required so that check has a real home: Composer (`composer/`), per `ARCHITECTURE.md -> Cross-Domain Reads`, reading Game Management's visibility/current-version first and only then calling Session Runtime's `Create`.

This is a known-required future outcome, not a speculative idea: without it, either the visibility check silently disappears (a real product regression — anyone could create a Session for an unpublished/hidden Game) or WORK-0031 cannot actually remove the direct call it set out to remove. `docs/projects/README.md` Invariant 1 is why this exists as its own WORK now, even though its design has not started.

## Context

Not yet designed. `composer/` currently contains only a README stub describing its accepted responsibility — no concrete composition code exists yet in this codebase. This WORK's design must also account for WORK-0031's own "bootstrapping the new table's first row" Blocker: Composer resolving the current version's content (not just its UUID/visibility) may be exactly the mechanism that supplies Session Runtime's new table its first row for a not-yet-seen version, which materially couples this WORK's design to how WORK-0031's own Blockers are ultimately resolved.

## Scope

Not yet designed.

## Approved Design

Not yet designed.

## Constraints and Invariants

- Must preserve the currently-enforced invariant: `Create` never succeeds for a Game whose current version is not playable/visible (today enforced by `businessservice.IsPlayableVisibility` inside `getgame`'s service — see `game/management/usecases/getgame/service.go`).
- Per `ARCHITECTURE.md -> Cross-Domain Reads`, Composer is stateless with respect to business/domain state and does not own business entities or either domain's business rules — it only composes.

## Acceptance Criteria

Not yet designed.

## Implementation Freedom

Not yet designed.

## Verification

Not yet designed.

## Documentation Impact

### Accepted / Canonical Knowledge

- `composer/README.md` likely gains a concrete example once real composition code exists there for the first time.

### Current-State Documentation After Implementation

- Not yet designed.

## Blockers

- Whether `Create`'s own public signature changes (e.g. to accept an already-resolved definition-version-UUID and/or its content from Composer, rather than resolving it itself) is a public-contract decision this WORK must make explicitly, not silently — see WORK-0031's own "bootstrapping" Blocker, which this WORK's design must resolve jointly with it.
- Sequencing against `session-runtime-v1`'s own live-transport WORK (WORK-0020 onward) — Composer needs some real entry point calling it; this WORK's design should confirm what that entry point is before design proceeds.

## Completion Record

Not yet started.
