# WORK-0008: Live Lobby / Session Bootstrap

Status: PLANNED
Created: 2026-09-20
Last status change: 2026-09-20

Related decisions:
- GAME-ADR-0002 (Live Session Coordinator responsibility boundary)
- GAME-ADR-0004 (Lobby contract, LOBBY serialization)
- GAME-ADR-0020 (post-commit delivery semantics, best-effort)

Canonical context:
- `docs/projects/active/session-runtime-v1/PROJECT.md`
- `game/README.md` (Session Runtime Lobby Lifecycle Contract, Actor and Lifecycle Model)
- `docs/projects/active/session-runtime-v1/works/WORK-0005-thin-live-coordinator.md` (the live Coordinator this WORK builds on; also owns the host/Participant connection-boundary question this WORK depends on being resolved)
- `play/README.md`, `api/README.md`

## Outcome

A real lobby UI can be built against a live backend surface: a client connecting before `Start` can see who else is in the lobby, learn when someone joins or leaves, learn the lobby's expiration deadline, leave the lobby itself through the live/application surface, and receive whatever bootstrap/resync information it needs on first connecting (or reconnecting while still in `LOBBY`) to render current lobby state without guessing.

Today, `Manager.Leave` exists at the Go-API level (WORK-0001) but has no live-transport exposure at all (`play.SessionRuntime`'s port has no `Leave` method) - a connected client cannot leave a Session except by disconnecting outright, and no connected client (including the host) currently learns about another participant joining or leaving in real time. `play.Coordinator` today only fans out `Output`s produced by `Start`/`AnswerInteraction` - both of which only happen after a lobby already exists and, for `Start`, after the lobby phase has already ended.

## Context

Every gap this WORK addresses is required before an actual lobby screen (join list, host "who's here" view, leave button, expiration countdown) can be built, and none of it is owned by any other current WORK: WORK-0005 proves the live transport itself works end-to-end for one interaction; WORK-0006/WORK-0007 concern in-game/termination fan-out, not the pre-`Start` lobby. This WORK is what a lobby frontend actually needs.

This WORK depends on WORK-0005's host/Participant connection-boundary Blocker being resolved first: if the mechanism for a host to obtain a live connection changes, this WORK's own roster/leave/bootstrap design must be consistent with whatever that mechanism turns out to be, not designed against a connection model that is about to change.

## Scope

### In Scope (known required outcomes; design not yet started)

- A live/application `Leave` capability reachable from a connected client (not only the existing Go-API `Manager.Leave`).
- Live roster-change notification (join/leave) to other currently-connected lobby occupants.
- Lobby phase/expiration-deadline visibility to a connected client.
- Bootstrap/resync information a client needs on first connecting (or reconnecting while still `LOBBY`) to render current lobby state, without assuming it already knows the roster from anywhere else.
- Host-relevant lobby state/controls needed to render a host's view of the lobby (this WORK does not itself decide what host controls exist beyond what's already accepted - e.g. it does not include manual cancellation, WORK-0011's scope, or kick/transfer-host, an open scope question - see `PROJECT.md`).

### Out of Scope

- Manual Session cancellation (WORK-0011).
- Anything RUNNING-phase (Presentation/Effects: WORK-0006; user intents: WORK-0010; timers: WORK-0012).
- Full RUNNING-phase resync/reconnect (WORK-0015) - this WORK's bootstrap concern is LOBBY-phase only.
- Host "kick participant" / "transfer host" - not yet decided to be required for V1; not designed here (see `PROJECT.md`'s open scope questions).

## Approved Design

Not yet designed. Left open pending PLANNED -> DRAFT: exact wire message shapes for roster/leave/expiration, whether roster-change fan-out reuses `play.Coordinator`'s existing per-recipient `Event`/`Deliver` mechanism or needs its own broadcast-style mechanism (compare WORK-0007's session-wide broadcast, added for a different reason), and the exact LOBBY bootstrap payload shape.

## Constraints and Invariants

- Must not introduce a new authoritative connection-presence field on Session Runtime persistence (GAME-ADR-0003), consistent with WORK-0005's existing constraint.
- Must not require the host to become a Participant merely to receive lobby state (depends on WORK-0005's Blocker 8 resolution).

## Acceptance Criteria

Not yet defined - to be written when this WORK moves to DRAFT.

## Blockers

- Depends on WORK-0005's host/Participant live-connection Blocker being resolved first (see Constraints).
- Whether "host kick a participant" / "transfer host" belongs in V1 is an open scope question this WORK does not resolve on its own (see `PROJECT.md`).

## Documentation Impact

Not yet assessed in detail; expected to touch `game/CURRENT_STATE.md`, `game/docs/FLOWS.md`, `play/README.md` once designed.

## Completion Record

Not started. PLANNED.
