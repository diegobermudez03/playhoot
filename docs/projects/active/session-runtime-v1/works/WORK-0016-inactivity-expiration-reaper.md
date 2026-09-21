# WORK-0016: Inactivity Expiration / Reaper

Status: PLANNED
Created: 2026-09-20
Last status change: 2026-09-20

Related decisions:
- GAME-ADR-0014 (inactivity/reaper semantics)
- GAME-ADR-0019 (terminal cleanup - reused, not reinvented, by this WORK)

Canonical context:
- `docs/projects/active/session-runtime-v1/PROJECT.md`
- `game/CURRENT_STATE.md` (`activity_expires_at` "has no schema/enforcement anywhere yet despite being already-accepted architecture")
- `docs/projects/active/session-runtime-v1/works/WORK-0014-runtime-failure-diagnostics-terminal-cleanup.md` (the terminal-cleanup invariant this WORK reuses)
- `docs/projects/active/session-runtime-v1/works/WORK-0007-session-termination-live-notification.md` (the terminal-live consequence this WORK's termination must also reuse, for the rare case a client is still connected to an otherwise-inactive Session)

## Outcome

An orphaned/inactive Session (no meaningful activity for a configured period) is detected and transitioned to terminal automatically, without requiring a human or a player to notice it should end. `sessions.activity_expires_at` is renewed on meaningful activity, lazily materialized on a stale operation, and enforced by a background Reaper.

Today, `activity_expires_at` has no schema or enforcement anywhere - distinct from the already-implemented LOBBY-phase `lobby_expires_at`, which is a different, already-working mechanism (WORK-0001).

## Context

Purely additive once RUNNING operations exist to renew/validate against (Start, AnswerInteraction, and whichever of WORK-0010/0012 have landed) - lower architectural risk than most of this Project's other remaining WORK, so it can follow rather than gate them.

## Scope

### In Scope (known required outcome; design not yet started)

- `sessions.activity_expires_at` schema.
- Renewal on meaningful activity.
- Lazy materialization on a stale operation (the same pattern `lobby_expires_at` already uses).
- A background Reaper process.
- Reuse of WORK-0014's terminal-cleanup invariant and WORK-0007's terminal-live-notification mechanism for the resulting termination.

### Out of Scope

- Redefining what counts as "meaningful activity" beyond what GAME-ADR-0014 already accepts.

## Approved Design

Not yet designed. Reaper mechanism (polling interval, distribution across instances if ever more than one, exact activity-renewal trigger list) remains open.

## Constraints and Invariants

- Must reuse WORK-0014's terminal-cleanup invariant, not reimplement it.
- Must reuse WORK-0007's terminal-live-notification mechanism for any currently-connected client.

## Acceptance Criteria

Not yet defined - to be written when this WORK moves to DRAFT.

## Blockers

- None yet beyond ordinary design questions for DRAFT.

## Documentation Impact

Not yet assessed in detail; expected to touch `game/CURRENT_STATE.md`, `game/docs/DATA_MODEL.md` once designed.

## Completion Record

Not started. PLANNED.
