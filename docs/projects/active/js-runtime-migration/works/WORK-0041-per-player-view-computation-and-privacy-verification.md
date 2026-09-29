# WORK-0041: Per-Player View Computation & Privacy Verification

Status: PLANNED
Created: 2026-09-27
Last status change: 2026-09-27

Related decisions:
- `docs/decisions/architecture/ADR-0015-javascript-rule-execution-and-iframe-frontend-contract.md`
- `game/docs/decisions/GAME-ADR-0028-javascript-execution-replaces-game-language.md`

Canonical context:
- `game/language/v1/program/README.md` (the retired `Projection`/`View`/`Presentation` model, for precedent on what a privacy boundary must cover)

## Outcome

Because authored JavaScript computes per-player views directly (there is no longer a Playhoot-trusted, compiler-enforced `Projection` guaranteeing purity), build the mechanism that verifies no player's `ClientState` leaks another player's or role's private data, and the mechanism to retrieve a player's current `ClientState` on initial load or reconnect directly from persisted state (`WORK-0038`), without requiring every prior transient `SEND_EVENT` to have been received. `ADR-0015` names this a required capability, not optional hardening: knowing all game state does not, by itself, guarantee an authored view is correct.

## Context

Not yet designed. This is a genuinely new capability with no direct current-implementation precedent (the retired `Projection` compiler enforced this by construction; nothing enforces it for JS-computed views yet).

**Vocabulary update (2026-09-28):** `WORK-0039`'s revised design fixes the mechanism this WORK verifies: the backend script exposes a second, pure entry point, `project(state, viewer, context) -> ClientState`, separate from `execute` — `project` cannot mutate authoritative state and cannot emit Commands (`WORK-0039`'s own Constraints). This WORK's privacy-verification mechanism operates specifically on `project`'s output for a given `viewer`, not on anything returned from `execute`'s Commands (`REQUEST_VIEW` as a command no longer exists). `ClientState`/`PlayerProjection` is the correct term for this WORK's own subject matter — never "View," since it is authorized game data, not a UI instruction, and must never itself be assumed to contain presentation concepts.

## Scope

Not yet designed.

## Approved Design

Not yet designed.

## Constraints and Invariants

- A per-player/role view must be verifiable against what that player/role is authorized to see before delivery, not merely trusted because the script produced it.
- Retrieving current view must not depend on replaying prior messages/effects — it derives from `WORK-0038`'s persisted state plus the requesting player's identity/role.

## Acceptance Criteria

Not yet designed.

## Implementation Freedom

Not yet designed.

## Verification

Not yet designed. Must include an adversarial-authored-script test (a script that deliberately or accidentally includes another player's private data in a view) proving the verification mechanism catches it.

## Documentation Impact

### Accepted / Canonical Knowledge

- A new package-local contract document recording the privacy-verification mechanism and its limits.

## Blockers

- Depends on `WORK-0039` (command/view vocabulary) and `WORK-0038` (persisted state to derive views from).

## Completion Record

Not yet started.
