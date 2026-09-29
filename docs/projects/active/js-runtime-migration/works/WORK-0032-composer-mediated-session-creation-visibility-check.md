# WORK-0032: Composer-Mediated Session Creation (Game Visibility Composition)

Status: DRAFT
Created: 2026-09-27
Last status change: 2026-09-28 (PLANNED -> DRAFT; initial design pass, two material questions flagged below for human decision before READY)

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

`composer/` currently contains only a README stub describing its accepted responsibility — no concrete composition code exists yet in this codebase.

**Simplified (2026-09-28), per WORK-0034's own Scope Narrowing.** The entanglement this section previously described is resolved: `Create` resolves the current version's content entirely from Session Runtime's own tables (populated by a future publish path, `WORK-0033`), never from Game Management and never from data Composer passes in. Composer's role here is therefore only a visibility gate — read Game Management's visibility/current-version state, decide whether `Create` may proceed, and if so call `Create` with nothing more than the `game_uuid` it already takes today. Composer does not resolve or hand off any artifact content, and this WORK's design is no longer coupled to how WORK-0034's table gets its first row.

**Blocker resolved by inspection (2026-09-28): the real entry point is `POST /sessions`.** `api/session/handler.go`/`http.go` already register this route with real request decoding/logging, but `handleCreateSession` calls no domain package today — it unconditionally answers `501 Not Implemented` (its own doc comment: "there is nothing to call yet"). No other current WORK claims this wiring: `session-runtime-v1`'s `WORK-0008`/`WORK-0020` (still PLANNED/DRAFT) own the *live/WebSocket* lobby-projection experience post-Create, not the initial REST call. This WORK is therefore the natural, and only currently-planned, place to make `POST /sessions` functional end-to-end, since Composer has no other caller to be reached from.

**Discovery (2026-09-28): Game Management's existing visibility-read capability does more than a visibility check needs, and will not survive an all-JavaScript-published Game.** `getgame.GetPlayableGameWithCurrentVersion` (`game/usecases/getgame/service.go`) is exactly the capability this WORK's own Constraints already point to for enforcing "not currently playable/visible" — and, since `WORK-0034` removed `Create`'s only caller of it, it is currently unreferenced by any production code path (only its own test calls it). But it does not stop at visibility: it also decodes `g.Script` via `gameservice.DecodeJSON` into a full Game-Language `program.Definition` and fails with `game.ErrBrokenGame` if that decode fails. Once a Game's current version is ever published purely as JavaScript (`WORK-0033`, not yet designed), Game Management's own `game_definitions.script` for that version may contain no decodable Game-Language source at all, and this existing method would then reject an actually-visible, actually-playable Game as broken — a latent defect this WORK would inherit if it reused the method as-is. See Material Decisions below.

## Scope

### In Scope

- `composer.CreateSession(ctx, gameUUID, hostUserUUID, idempotencyKey) (session.CreatedSession, error)`: reads Game Management's visibility state for `gameUUID` first; if not visible/playable, returns without calling Session Runtime at all; otherwise calls `sessionlifecycle.Manager.Create` with the same arguments and returns its result unchanged. Composer owns no business entity and no retry/compensation logic of its own (`ARCHITECTURE.md -> Cross-Domain Reads`) — a plain sequential read-then-call.
- Wiring `api/session/handler.go`'s `handleCreateSession` to call `composer.CreateSession` instead of its current unconditional `501`, using the wire shapes (`createSessionRequest`/`createSessionResponse`) already defined in `api/session/ws.go`.
- Whatever narrow Game Management read capability Composer actually calls for the visibility check (see Material Decisions — exact shape depends on that decision).

### Explicitly Out Of Scope

- Anything WORK-0008/WORK-0020 own: live/WebSocket lobby roster, join/leave fan-out, bootstrap/resync. This WORK only makes the pre-lobby `POST /sessions` call itself functional.
- Redesigning `getgamedefinition`/the five untouched `sessionlifecycle` call sites' own pinned-definition read path — unaffected by this WORK.
- Solving how a Game's current version is actually published/becomes visible in the first place (`WORK-0033`).
- Host "kick"/"transfer host" — tracked in `session-runtime-v1`'s own open scope questions, unaffected here.

## Approved Design

Proposed, pending the Material Decisions below:

- A new Game Management use case (illustrative name: `getgamevisibility`), returning only whether `gameUUID`'s current version is playable/visible (and a not-found case), never a `program.Definition` or any Game-Language-decoded content — Composer needs no Game-Language-shaped data at all, mirroring `WORK-0034`'s own principle that a JS-era caller should never depend on decodable Game Language content existing.
- `getgame.GetPlayableGameWithCurrentVersion` itself is left exactly as-is (not modified, not removed) — it stays orphaned/test-only, a pre-existing condition this WORK did not create and is not obligated to clean up.
- `composer.CreateSession` composes the two calls above; on a not-visible/not-found result it returns `session.ErrGameNotFound` (Session Runtime's own existing public error, already what `Create` itself returns for its own not-found case today) rather than inventing a new Composer-level error type, so `handleCreateSession` needs only one not-found branch regardless of which domain produced it.
- `handleCreateSession` is edited in place: same decode/validate logic it already has, its `501` replaced by the real call and existing response-shape mapping.

## Constraints and Invariants

- Must preserve the currently-enforced invariant: `Create` never succeeds for a Game whose current version is not playable/visible (today enforced by `businessservice.IsPlayableVisibility` inside `getgame`'s service — see `game/usecases/getgame/service.go`).
- Per `ARCHITECTURE.md -> Cross-Domain Reads`, Composer is stateless with respect to business/domain state and does not own business entities or either domain's business rules — it only composes.

## Acceptance Criteria

Drafted, pending Material Decisions below:

- `POST /sessions` no longer unconditionally returns `501`; a valid request for a visible/playable Game's `gameUUID` creates a Session exactly as `sessionlifecycle.Manager.Create` does today when called directly.
- A request for a `gameUUID` that does not exist or is not currently playable/visible returns the same outcome `Create` itself already returns for "game not found" (no new distinct wire error class).
- No code path lets `handleCreateSession` call `sessionlifecycle.Manager.Create` without first going through the new visibility read — no bypass.
- Composer's new code owns no business entity/table of its own and contains no business rule beyond call sequencing (`ARCHITECTURE.md -> Cross-Domain Reads`).
- `go build ./...`/`go test ./...` clean; new unit coverage for `composer.CreateSession`'s visible/not-visible/not-found/downstream-error branches.

## Implementation Freedom

- Exact new Game Management use-case/package naming and its repo-level SQL shape (whether it's a new narrow query or a projection of an existing one).
- Exact `composer` package internal layout (single file vs. its own subpackage), consistent with `composer/README.md`'s stub.

## Verification

- `go test ./...` across `composer/`, the new Game Management use case, and `api/session`.
- A manual/integration-level check that `POST /sessions` returns a real `201`-shaped response for a seeded visible Game and the not-found/not-playable outcome for a hidden/draft one.

## Documentation Impact

### Accepted / Canonical Knowledge

- `composer/README.md` gains a concrete example (its first real composition code).

### Current-State Documentation After Implementation

- `game/CURRENT_STATE.md`/`game/docs/FLOWS.md` — note the new visibility-only read capability and that `GetPlayableGameWithCurrentVersion` remains unused/legacy pending `WORK-0033`.
- `session/CURRENT_STATE.md` — note `Create` is now reachable over `POST /sessions`, not only at the Go-API level.
- `api/README.md` (if it documents endpoint implementation status) — `POST /sessions` moves from stub to implemented.

## Blockers

None remaining once the Material Decisions below are confirmed.

## Material Decisions Needing Human Input

1. **Does this WORK also wire the `POST /sessions` HTTP endpoint, or only build `composer.CreateSession` itself and leave the endpoint stubbed for a separate WORK?** Proposed: yes, wire it here — no other current WORK claims that wiring, Composer has no other caller to be reached from, and the endpoint's own request/response shapes already exist waiting for exactly this call.
2. **Does Composer's visibility check call a new, narrower Game Management read capability, or reuse `getgame.GetPlayableGameWithCurrentVersion` as-is?** Proposed: add a new visibility-only capability (see Approved Design) rather than reuse the existing one, because that method also decodes a Game-Language `program.Definition` and would reject a real, visible Game whose current version was ever published purely as JavaScript (`WORK-0033`) as `ErrBrokenGame` — a defect this WORK would otherwise inherit rather than cause.

## Completion Record

Not yet started.
