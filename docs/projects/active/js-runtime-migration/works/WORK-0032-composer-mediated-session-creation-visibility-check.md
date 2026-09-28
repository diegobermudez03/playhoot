# WORK-0032: Composer-Mediated Session Creation (Game Visibility Composition)

Status: PLANNED
Created: 2026-09-27
Last status change: 2026-09-27 (reparented into `docs/projects/active/js-runtime-migration/`, see Reparenting note below)

Related decisions:
- `docs/decisions/architecture/ADR-0014-management-session-domain-split.md`
- `docs/decisions/architecture/ADR-0015-javascript-rule-execution-and-iframe-frontend-contract.md`

Canonical context:
- `ARCHITECTURE.md` (Cross-Domain Reads, Composer)
- `composer/README.md`
- `game/session/workflows/sessionlifecycle/step_create.go` (current direct-call shape being replaced)

## Reparenting Note (2026-09-27)

This WORK originated under `docs/projects/completed/management-session-domain-split/`, driven by `ADR-0014`. Its own goal (a real Composer-mediated home for `Create`'s visibility check, replacing a direct cross-domain call) is unaffected by `ADR-0015`/`GAME-ADR-0028` retiring Game Language for sandboxed JavaScript — visibility composition is independent of what rule-execution language a Session runs. It is moved here because the WORK it depended on (the old WORK-0031) is cancelled and superseded by this Project's own `WORK-0034`; every other reference to "WORK-0031" below now means `WORK-0034`.

## Outcome

Today, `sessionlifecycle.Manager.Create` calls Game Management directly (`gameCurrentVersionReader.GetPlayableGameWithCurrentVersion`) to resolve the Game's current playable version and enforce visibility before creating a Session. WORK-0034 removes that direct cross-domain call as part of dissolving the Game Management/Session Runtime coupling (ADR-0014). Something must still enforce "a Session may only be created for a currently-playable/visible Game" — this WORK is required so that check has a real home: Composer (`composer/`), per `ARCHITECTURE.md -> Cross-Domain Reads`, reading Game Management's visibility/current-version first and only then calling Session Runtime's `Create`.

This is a known-required future outcome, not a speculative idea: without it, either the visibility check silently disappears (a real product regression — anyone could create a Session for an unpublished/hidden Game) or WORK-0034 cannot actually remove the direct call it set out to remove. `docs/projects/README.md` Invariant 1 is why this exists as its own WORK now, even though its design has not started.

## Context

Not yet designed. `composer/` currently contains only a README stub describing its accepted responsibility — no concrete composition code exists yet in this codebase.

**Simplified (2026-09-28), per WORK-0034's own Scope Narrowing.** The entanglement this section previously described is resolved: `Create` resolves the current version's content entirely from Session Runtime's own tables (populated by a future publish path, `WORK-0033`), never from Game Management and never from data Composer passes in. Composer's role here is therefore only a visibility gate — read Game Management's visibility/current-version state, decide whether `Create` may proceed, and if so call `Create` with nothing more than the `game_uuid` it already takes today. Composer does not resolve or hand off any artifact content, and this WORK's design is no longer coupled to how WORK-0034's table gets its first row.

## Scope

Not yet designed.

## Approved Design

Not yet designed.

## Constraints and Invariants

- Must preserve the currently-enforced invariant: `Create` never succeeds for a Game whose current version is not playable/visible (today enforced by `businessservice.IsPlayableVisibility` inside `getgame`'s service — see `game/usecases/getgame/service.go`).
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

- ~~Whether `Create`'s own public signature changes...~~ — **Resolved 2026-09-28**, see Context above: `Create`'s signature is unchanged (`game_uuid` only); Composer passes no additional data.
- Sequencing against `session-runtime-v1`'s own live-transport WORK (WORK-0020 onward) — Composer needs some real entry point calling it; this WORK's design should confirm what that entry point is before design proceeds.

## Completion Record

Not yet started.
