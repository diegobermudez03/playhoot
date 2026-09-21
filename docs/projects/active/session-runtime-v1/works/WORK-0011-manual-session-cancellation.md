# WORK-0011: Manual Session Cancellation

Status: PLANNED
Created: 2026-09-20
Last status change: 2026-09-20 (Part B/I reconciliation, same day: ADMIN-only command surface and durable-cause requirement made explicit - see "Scope Addition (Part B/I Reconciliation, 2026-09-20)" below)

Related decisions:
- GAME-ADR-0019 (RuntimeTurn execution bound and terminal cleanup)
- GAME-ADR-0002 (Live Session Coordinator responsibility boundary)
- GAME-ADR-0024 (Replay-First Session Runtime Persistence - the cancellation cause must be durably representable)
- GAME-ADR-0025 (Role-Aware Live Connections - Cancel is an ADMIN-only command)

Canonical context:
- `docs/projects/active/session-runtime-v1/PROJECT.md`
- `game/docs/decisions/GAME-ADR-0004-session-lobby-contract.md` (Leave "does not automatically cancel the Session" - the gap this WORK closes)
- `game/language/v1/program/signal.go`/compiler catalog (`SessionCancelled` - already a real, usable `NamedSignalSource`)
- `docs/projects/active/session-runtime-v1/works/WORK-0007-session-termination-live-notification.md` (the generalized terminal-live consequence this WORK's resulting termination must reuse)
- `docs/projects/active/session-runtime-v1/works/WORK-0019-replay-first-session-runtime-persistence-migration.md` (owns the general replay-input model; this WORK is responsible for satisfying it for SessionCancelled specifically)
- `docs/projects/active/session-runtime-v1/works/WORK-0020-role-aware-live-connections.md` (the ADMIN connection this WORK's command rides, exclusively)

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

- Only the Session's host may cancel it, authority coming from the connection role (ADMIN) plus Manager's own authoritative `host_actor_id` check together, never from either alone (GAME-ADR-0025) - a host who also holds a PARTICIPANT connection does not gain cancellation authority through that connection.
- The resulting terminal transition must satisfy the same terminal-cleanup invariant (no dangling `ACTIVE` interaction/timer obligation) that other terminal paths already establish or that WORK-0014 generalizes.
- Must reuse, not reimplement, WORK-0007's terminal-live-notification mechanism.
- **(Part B reconciliation)** The cancellation cause (at minimum, the issuing host's actor identity and the fact that cancellation - not some other terminal reason - occurred) must be durably representable in commit order, per GAME-ADR-0024/WORK-0019's replay-input model, so a Session's terminal history remains reconstructible/understandable after the fact.

## Acceptance Criteria

Not yet defined - to be written when this WORK moves to DRAFT.

## Blockers

- Exact command/API/wire contract is an open design question for the DRAFT phase, not resolved here.
- Depends on WORK-0020 (ADMIN connection) landing first.

## Scope Addition (Part B/I Reconciliation, 2026-09-20)

A broader reconciliation session accepted GAME-ADR-0024 (replay-first persistence) and GAME-ADR-0025 (role-aware connections), both bearing on this previously-PLANNED WORK: (1) cancellation is explicitly an ADMIN-only command - a host's separate PARTICIPANT connection (if any) never gains cancellation authority merely because the same person holds it; (2) the cancellation cause needs a durable, ordered representation consistent with every other RuntimeTurn-driving cause, not only whatever `Manager` needed for its own immediate terminalization logic. Neither changes this WORK's own open design questions (exact command/wire contract, forced-terminal-vs-authored-handling semantics), which remain for DRAFT. No implementation was performed; this WORK's Status remains PLANNED.

## Documentation Impact

Not yet assessed in detail; expected to touch `game/session/session.go` (new `TerminalReason`), `game/CURRENT_STATE.md`, `game/docs/FLOWS.md` once designed.

## Completion Record

Not started. PLANNED.
