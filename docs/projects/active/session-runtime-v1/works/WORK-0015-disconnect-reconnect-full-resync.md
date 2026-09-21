# WORK-0015: Disconnect / Reconnect / Full Resync

Status: PLANNED
Created: 2026-09-20
Last status change: 2026-09-20

Related decisions:
- GAME-ADR-0010 (resync read capability boundary)
- GAME-ADR-0011 (Game Language disconnect/reconnect authored semantics)
- GAME-ADR-0015 (phase-dependent disconnect consequences)
- GAME-ADR-0016 (atomic presence-transition-plus-processing rule)

Canonical context:
- `docs/projects/active/session-runtime-v1/PROJECT.md`
- `game/README.md` (Session Runtime Actor and Lifecycle Model - disconnect/reconnect responsibility split between Coordinator/Session Runtime/Game Language)
- `game/session/session.go` (`PresenceDisconnected` - exists but "not currently produced by any operation")
- `docs/projects/active/session-runtime-v1/works/WORK-0018-game-language-disconnect-reconnect-signal-support.md` (hard prerequisite - `UserDisconnected`/`UserReconnected` schema)
- `docs/projects/active/session-runtime-v1/works/WORK-0013-keyed-timers.md` (likely prerequisite for per-user disconnect-grace timing)
- `docs/projects/active/session-runtime-v1/works/WORK-0006-broaden-live-fanout-effects-presentations.md` (Presentation is derived from the current Snapshot, not replayed - resync must follow the same principle)

## Outcome

`session_actors.semantic_presence` exists and is maintained; phase-dependent LOBBY/RUNNING disconnect consequences apply (GAME-ADR-0015); a presence transition and its Game-Language-visible consequence apply atomically (GAME-ADR-0016); a resync read capability returns a player-facing projection - reconstructing current Session phase, player-facing state, active Interactions, active Presentations, and relevant manifest/version identity - for a client that missed live delivery or is reconnecting. WORK-0005's thin Coordinator gains exactly the operational complexity it deliberately excluded: transport grace/debounce, semantic-disconnect reporting, and reconnect handling.

Today: `grep` confirms no code sets semantic presence, no code emits `UserDisconnected`/`UserReconnected`, and `play/README.md` explicitly defers all of this. `PresenceDisconnected` exists as a constant with a doc comment stating it is not yet produced by any operation.

## Context

This WORK has a hard Game Language prerequisite: WORK-0018 must land first, since `UserDisconnected`'s current placeholder schema and the total absence of `UserReconnected` mean Session Runtime cannot deliver either signal correctly yet.

Historical UI Effects must NOT be replayed as part of reconnect. Per WORK-0006's accepted architecture, Presentation state is derived fresh from the current Snapshot (`deriveActivePresentations`), so resync recomputes current state rather than replaying history - this WORK's resync capability must follow that same principle, not invent a second, history-replaying mechanism.

This WORK may depend on WORK-0013 (Keyed Timers) for disconnect-grace timing if a per-user grace period requires an independent timer per user - reevaluate this when WORK-0013 and this WORK are both designed in more detail.

## Scope

### In Scope (known required outcome; design not yet started)

- `session_actors.semantic_presence` persistence and maintenance.
- Phase-dependent LOBBY/RUNNING disconnect consequences (GAME-ADR-0015).
- Atomic presence-transition-plus-Game-Language-processing (GAME-ADR-0016).
- A resync read capability (GAME-ADR-0010) returning a derived player-facing projection, not raw Snapshot/history.
- Coordinator-level disconnect grace/debounce, semantic-disconnect reporting, reconnect handling - extending WORK-0005's existing Coordinator, not a second one.

### Out of Scope

- Inactivity/reaper handling for an orphaned Session with no one reconnecting (WORK-0016) - a related but distinct concern.
- Historical UI Effect replay - explicitly excluded; resync recomputes current state.

## Approved Design

Not yet designed. Exact resync payload shape, exact grace/debounce timing, and whether disconnect-grace timing needs WORK-0013's keyed timers or can use WORK-0012's ordinary timer mechanism remain open.

## Constraints and Invariants

- Resync must recompute current state from the current Snapshot/persisted state, never replay historical Effects.
- Presence transition and its Game-Language-visible consequence must be atomic (GAME-ADR-0016).
- Depends on WORK-0018 landing first (Game Language signal support).

## Acceptance Criteria

Not yet defined - to be written when this WORK moves to DRAFT.

## Blockers

- Hard prerequisite: WORK-0018 (Game Language `UserDisconnected`/`UserReconnected` signal support) must be DONE (or land alongside this WORK) before this WORK can deliver either signal.
- Whether this WORK needs WORK-0013's keyed timers, or can proceed with WORK-0012's ordinary timer mechanism, is an open dependency question to resolve during DRAFT.

## Documentation Impact

Not yet assessed in detail; expected to touch `game/README.md`, `game/CURRENT_STATE.md`, `game/docs/DATA_MODEL.md`, `game/docs/FLOWS.md`, `play/README.md` once designed.

## Completion Record

Not started. PLANNED.
