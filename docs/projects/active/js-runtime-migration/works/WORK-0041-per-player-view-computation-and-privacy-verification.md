# WORK-0041: Per-Player View Computation & Privacy Verification

Status: PLANNED
Created: 2026-09-27
Last status change: 2026-09-27

Related decisions:
- `docs/decisions/architecture/ADR-0015-javascript-rule-execution-and-iframe-frontend-contract.md`
- `game/docs/decisions/GAME-ADR-0028-javascript-execution-replaces-game-language.md`

Canonical context:
- `game/language/v1/program/README.md` (the retired `Projection`/`View`/`Presentation` model, for precedent on what a privacy boundary must cover)
- `docs/projects/active/session-runtime-v1/works/WORK-0015-disconnect-reconnect-full-resync.md` (owns the live reconnect/resync capability this WORK's "retrieve on reconnect" phrasing borders - see Coordination Flag below)

## Outcome

Because authored JavaScript computes per-player views directly (there is no longer a Playhoot-trusted, compiler-enforced `Projection` guaranteeing purity), build the mechanism that verifies no player's `ClientState` leaks another player's or role's private data, and the mechanism to retrieve a player's current `ClientState` on initial load or reconnect directly from persisted state (`WORK-0038`), without requiring every prior transient `SEND_EVENT` to have been received. `ADR-0015` names this a required capability, not optional hardening: knowing all game state does not, by itself, guarantee an authored view is correct.

**Coordination Flag (2026-09-29): `session-runtime-v1`'s `WORK-0015`.** This WORK's own "on initial load or reconnect" phrasing is a pure backend capability only: given a persisted state and a viewer's identity/role, compute and privacy-verify that viewer's `ClientState`. It does not cover *when* that computation is invoked over a live connection, debounce/grace timing, or how a reconnecting client's transport session is resumed - that is `session-runtime-v1`'s own `WORK-0015` (Disconnect/Reconnect/Full Resync), still PLANNED, which owns resync over the live-connection layer `WORK-0020` rebuilds. `WORK-0015` should call this WORK's verified `project(state, viewer, context) -> ClientState` computation as its own projection step, not reimplement privacy verification itself; this WORK should not grow a delivery/connection-handling mechanism of its own. Flagged here and in both Projects' own `PROJECT.md`, not silently resolved in either - see `js-runtime-migration/PROJECT.md`'s own Coordination note.

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
