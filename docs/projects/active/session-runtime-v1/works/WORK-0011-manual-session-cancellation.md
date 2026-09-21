# WORK-0011: Manual Session Cancellation

Status: PLANNED
Created: 2026-09-20
Last status change: 2026-09-20

Related decisions:
- GAME-ADR-0019 (RuntimeTurn execution bound and terminal cleanup)
- GAME-ADR-0002 (Live Session Coordinator responsibility boundary)

Canonical context:
- `docs/projects/active/session-runtime-v1/PROJECT.md`
- `game/docs/decisions/GAME-ADR-0004-session-lobby-contract.md` (Leave "does not automatically cancel the Session" - the gap this WORK closes)
- `game/language/v1/program/signal.go`/compiler catalog (`SessionCancelled` - already a real, usable `NamedSignalSource`)
- `docs/projects/active/session-runtime-v1/works/WORK-0007-session-termination-live-notification.md` (the generalized terminal-live consequence this WORK's resulting termination must reuse)

## Outcome

A host can explicitly end/cancel an active Session before it would otherwise reach a terminal state on its own. `SessionCancelled` enters Session Runtime and is delivered to the game as a signal; the game's own authored semantics determine how it reacts (an authored workflow may match on `SessionCancelled` via a `CancelControl`, per Game Language's existing engine tests); the Session becomes terminal; connected clients are notified/closed through WORK-0007's terminal-live mechanism.

Today, no such capability exists: `grep "Cancel"` across `game/session/` returns zero matches, and GAME-ADR-0004 explicitly confirms Leave does not cancel a Session. `SessionCancelled` is real and matchable in Game Language today, but nothing in Session Runtime ever emits it.

## Context

This is required for V1 per the reconciliation prompt that created this WORK: a host currently has no way to end a Session they no longer want to continue, other than every player leaving (which does not terminate it either, per GAME-ADR-0004) or waiting for inactivity expiration (WORK-0016, not yet implemented either).

This WORK's resulting termination must be a cause that reuses WORK-0007's generalized terminal-live-notification mechanism rather than reimplementing "notify connected clients on termination" a third time (WORK-0007 is itself being generalized under this migration precisely so causes like this one can plug into it).

## Scope

### In Scope (known required outcome; design not yet started)

- A host-authorized `Manager` capability that emits `SessionCancelled` into the currently-loaded runtime/workflow instance, drives a RuntimeTurn (or an equivalent forced-terminal path if the game does not itself model a transition on `SessionCancelled`), and terminalizes the Session.
- Live-transport exposure of this capability (host-only).
- Reuse of WORK-0007's terminal-live-consequence mechanism for the resulting notification/connection closure.

### Out of Scope

- Deciding what a specific authored game does in response to `SessionCancelled` - that is game-authoring content, not Session Runtime's concern.
- Host "kick a participant" - a different capability, not decided to be required for V1 (see `PROJECT.md`).

## Approved Design

Not yet designed. Open questions include: the exact command/API/wire contract (deliberately left open per the reconciliation prompt), whether a game that does not match on `SessionCancelled` still forces a terminal transition or requires the game to explicitly handle it, and how this interacts with GAME-ADR-0019's terminal-cleanup invariant (WORK-0014).

## Constraints and Invariants

- Only the Session's host may cancel it.
- The resulting terminal transition must satisfy the same terminal-cleanup invariant (no dangling `ACTIVE` interaction/timer obligation) that other terminal paths already establish or that WORK-0014 generalizes.
- Must reuse, not reimplement, WORK-0007's terminal-live-notification mechanism.

## Acceptance Criteria

Not yet defined - to be written when this WORK moves to DRAFT.

## Blockers

- Exact command/API/wire contract is an open design question for the DRAFT phase, not resolved here.

## Documentation Impact

Not yet assessed in detail; expected to touch `game/session/session.go` (new `TerminalReason`), `game/CURRENT_STATE.md`, `game/docs/FLOWS.md` once designed.

## Completion Record

Not started. PLANNED.
