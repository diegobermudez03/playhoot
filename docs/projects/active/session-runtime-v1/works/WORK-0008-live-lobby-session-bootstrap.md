# WORK-0008: Live Lobby / Session Bootstrap

Status: PLANNED
Created: 2026-09-20
Last status change: 2026-09-20 (Part F reconciliation, same day: role-specific ADMIN/PARTICIPANT lobby projections required, dependency moved from WORK-0005's Blocker 8 to WORK-0020 - see "Scope Revision (Part F Reconciliation, 2026-09-20)" below)

Related decisions:
- GAME-ADR-0002 (Live Session Coordinator responsibility boundary)
- GAME-ADR-0004 (Lobby contract, LOBBY serialization)
- GAME-ADR-0020 (post-commit delivery semantics, best-effort)
- GAME-ADR-0025 (Role-Aware Live Connections - the ADMIN/PARTICIPANT distinction this WORK's lobby projections must follow)

Canonical context:
- `docs/projects/active/session-runtime-v1/PROJECT.md`
- `game/README.md` (Session Runtime Lobby Lifecycle Contract, Actor and Lifecycle Model)
- `docs/projects/active/session-runtime-v1/works/WORK-0005-thin-live-coordinator.md` (the live Coordinator this WORK builds on)
- `docs/projects/active/session-runtime-v1/works/WORK-0020-role-aware-live-connections.md` (the ADMIN/PARTICIPANT connection model this WORK depends on and whose projections it fills in)
- `play/README.md`, `api/README.md`

## Outcome

A real lobby UI can be built against a live backend surface: a client connecting before `Start` can see who else is in the lobby (role-appropriately, see below), learn when someone joins or leaves, learn the lobby's expiration deadline, leave the lobby itself through the live/application surface, and receive whatever bootstrap/resync information it needs on first connecting (or reconnecting while still in `LOBBY`) to render current lobby state without guessing.

Today, `Manager.Leave` exists at the Go-API level (WORK-0001) but has no live-transport exposure at all (`play.SessionRuntime`'s port has no `Leave` method) - a connected client cannot leave a Session except by disconnecting outright, and no connected client (including the host) currently learns about another participant joining or leaving in real time. `play.Coordinator` today only fans out `Output`s produced by `Start`/`AnswerInteraction` - both of which only happen after a lobby already exists and, for `Start`, after the lobby phase has already ended.

## Context

Every gap this WORK addresses is required before an actual lobby screen (join list, host "who's here" view, leave button, expiration countdown) can be built, and none of it is owned by any other current WORK: WORK-0005 proves the live transport itself works end-to-end for one interaction; WORK-0006/WORK-0007 concern in-game/termination fan-out, not the pre-`Start` lobby. This WORK is what a lobby frontend actually needs.

This WORK depends on WORK-0020 (Role-Aware Live Connections) landing first: its roster/leave/bootstrap design must target the ADMIN/PARTICIPANT two-connection model WORK-0020 establishes, not a single undifferentiated connection kind.

## Scope

### In Scope (known required outcomes; design not yet started)

- A live/application `Leave` capability reachable from a connected `PARTICIPANT` client (not only the existing Go-API `Manager.Leave`).
- Live roster-change notification (join/leave) to other currently-connected lobby occupants, projected per role (see below).
- Lobby phase/expiration-deadline visibility to a connected client, either role.
- Bootstrap/resync information a client needs on first connecting (or reconnecting while still `LOBBY`) to render current lobby state, without assuming it already knows the roster from anywhere else - role-appropriate content per below.
- **ADMIN lobby projection**: full/current Participant roster (exact per-participant metadata is this WORK's own design task), live participant count, join events, leave events, lobby phase/deadline, and whatever state the host's already-accepted controls need (at minimum, Start-eligibility).
- **PARTICIPANT lobby projection**: at minimum, live participant count and count updates on join/leave, lobby phase/deadline as needed for the frontend, and its own `Leave` capability. Full roster/participant identities are not exposed to an ordinary Participant merely because ADMIN needs them - this is a role-specific projection, not the same payload with fields hidden client-side.

### Out of Scope

- Manual Session cancellation (WORK-0011).
- Anything RUNNING-phase (Presentation/Effects: WORK-0006; user intents: WORK-0010; timers: WORK-0012).
- Full RUNNING-phase resync/reconnect (WORK-0015) - this WORK's bootstrap concern is LOBBY-phase only.
- Host "kick participant" / "transfer host" - not yet decided to be required for V1; not designed here (see `PROJECT.md`'s open scope questions).
- The ADMIN/PARTICIPANT connection model itself (WORK-0020's own scope; this WORK depends on it and fills in the lobby-projection content for each role).

## Approved Design

Not yet designed. Left open pending PLANNED -> DRAFT: exact wire message shapes for each role's roster/leave/expiration projection, whether roster-change fan-out reuses `play.Coordinator`'s existing per-recipient `Event`/`Deliver` mechanism or needs its own broadcast-style mechanism per role (compare WORK-0007's session-wide broadcast, added for a different reason), and the exact LOBBY bootstrap payload shape for each of ADMIN and PARTICIPANT.

## Constraints and Invariants

- Must not introduce a new authoritative connection-presence field on Session Runtime persistence (GAME-ADR-0003), consistent with WORK-0005's existing constraint.
- Must not require the host to become a Participant merely to receive lobby state - satisfied by WORK-0020's ADMIN connection, which this WORK depends on rather than re-solves.
- Must not expose full Participant roster/identities to an ordinary PARTICIPANT connection merely because the ADMIN projection needs them (GAME-ADR-0025) - the two projections are genuinely different payloads, not the same one filtered client-side.

## Acceptance Criteria

Not yet defined - to be written when this WORK moves to DRAFT.

## Blockers

- Depends on WORK-0020 (Role-Aware Live Connections) landing first (see Constraints and Context).
- Whether "host kick a participant" / "transfer host" belongs in V1 is an open scope question this WORK does not resolve on its own (see `PROJECT.md`).

## Scope Revision (Part F Reconciliation, 2026-09-20)

This WORK originally depended on WORK-0005's own Blocker 8 (a host/Participant connection-boundary gap) being resolved, without specifying role-differentiated lobby projections as an explicit requirement. A broader reconciliation session accepted `game/docs/decisions/GAME-ADR-0025-role-aware-live-connections.md`, which both (a) moves the actual host-connection-boundary fix to a new dedicated WORK, WORK-0020, and (b) makes explicit that ADMIN and PARTICIPANT must receive genuinely different lobby projections (full roster vs. count-only), not merely that a host needs *some* connection. This WORK's Scope/Constraints above have been revised to state both changes explicitly. No implementation was performed; this WORK's Status remains PLANNED.

## Documentation Impact

Not yet assessed in detail; expected to touch `game/CURRENT_STATE.md`, `game/docs/FLOWS.md`, `play/README.md` once designed.

## Completion Record

Not started. PLANNED.
