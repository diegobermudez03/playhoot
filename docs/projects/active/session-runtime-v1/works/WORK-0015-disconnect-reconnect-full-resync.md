# WORK-0015: Disconnect / Reconnect / Full Resync

Status: PLANNED
Created: 2026-09-20
Last status change: 2026-09-20 (Part B/K reconciliation, same day: resync now recomputes via replay rather than a stored Snapshot; reconnect is per connection role; spectator resubscription noted as ephemeral - see "Scope Revision (Part B/K Reconciliation, 2026-09-20)" below)

Related decisions:
- GAME-ADR-0010 (resync read capability boundary)
- GAME-ADR-0011 (Game Language disconnect/reconnect authored semantics)
- GAME-ADR-0015 (phase-dependent disconnect consequences)
- GAME-ADR-0016 (atomic presence-transition-plus-processing rule)
- GAME-ADR-0024 (Replay-First Session Runtime Persistence - resync reconstructs Runtime state by replay, not by loading a stored Snapshot; the semantic-presence transition itself needs a durable, ordered replay-input representation)
- GAME-ADR-0025 (Role-Aware Live Connections - disconnect/reconnect is a PARTICIPANT-role concept; an ADMIN connection's physical loss is not a Game Language event)

Canonical context:
- `docs/projects/active/session-runtime-v1/PROJECT.md`
- `game/README.md` (Session Runtime Actor and Lifecycle Model - disconnect/reconnect responsibility split between Coordinator/Session Runtime/Game Language)
- `game/session/session.go` (`PresenceDisconnected` - exists but "not currently produced by any operation")
- `docs/projects/active/session-runtime-v1/works/WORK-0018-game-language-disconnect-reconnect-signal-support.md` (hard prerequisite - `UserDisconnected`/`UserReconnected` schema)
- `docs/projects/active/session-runtime-v1/works/WORK-0013-keyed-timers.md` (likely prerequisite for per-user disconnect-grace timing)
- `docs/projects/active/session-runtime-v1/works/WORK-0006-broaden-live-fanout-effects-presentations.md` (Presentation is derived from the current Snapshot, not replayed - resync must follow the same principle)
- `docs/projects/active/session-runtime-v1/works/WORK-0019-replay-first-session-runtime-persistence-migration.md` (owns the replay-reconstruction capability this WORK's resync builds its projection on top of, and the durable representation of the semantic-presence transition itself)
- `docs/projects/active/session-runtime-v1/works/WORK-0020-role-aware-live-connections.md` (the ADMIN/PARTICIPANT connection distinction this WORK's reconnect design must respect per role)
- `docs/projects/active/session-runtime-v1/works/WORK-0021-host-participant-spectator-view.md` (a consumer of this WORK's eventual resync/projection capability, and of this WORK's spectator-resubscription note below)

## Outcome

`session_actors.semantic_presence` exists and is maintained; phase-dependent LOBBY/RUNNING disconnect consequences apply (GAME-ADR-0015); a presence transition and its Game-Language-visible consequence apply atomically (GAME-ADR-0016); a resync read capability returns a player-facing projection - reconstructing current Session phase, player-facing state, active Interactions, active Presentations, and relevant manifest/version identity - for a client that missed live delivery or is reconnecting. WORK-0005's thin Coordinator gains exactly the operational complexity it deliberately excluded: transport grace/debounce, semantic-disconnect reporting, and reconnect handling - now understood per connection role (WORK-0020), not as one undifferentiated "the user reconnected" event.

Today: `grep` confirms no code sets semantic presence, no code emits `UserDisconnected`/`UserReconnected`, and `play/README.md` explicitly defers all of this. `PresenceDisconnected` exists as a constant with a doc comment stating it is not yet produced by any operation.

## Context

This WORK has a hard Game Language prerequisite: WORK-0018 must land first, since `UserDisconnected`'s current placeholder schema and the total absence of `UserReconnected` mean Session Runtime cannot deliver either signal correctly yet.

Historical UI Effects must NOT be replayed as part of reconnect. Per WORK-0006's accepted architecture, Presentation state is derived fresh from the current Snapshot, so resync recomputes current state rather than replaying history. **Under GAME-ADR-0024 (2026-09-20), the current Snapshot itself is also no longer a stored payload** - resync's "recompute, don't replay history" principle now applies one level deeper: resync calls WORK-0019's replay-reconstruction capability to obtain the current authoritative Snapshot (itself the result of deterministic replay of durable causes, not history-as-observed-by-a-client), then derives the player-facing projection from that reconstructed Snapshot exactly as it would have from a stored one. Historical Effects remain non-replayed, unchanged.

This WORK may depend on WORK-0013 (Keyed Timers) for disconnect-grace timing if a per-user grace period requires an independent timer per user - reevaluate this when WORK-0013 and this WORK are both designed in more detail.

This WORK depends on WORK-0020 (Role-Aware Live Connections): reconnect is not one undifferentiated event. A physical ADMIN connection loss must never itself produce a Game Language `UserDisconnected` signal - that remains a PARTICIPANT/runtime-membership concept exclusively (GAME-ADR-0025). If the same person holds both an ADMIN and a PARTICIPANT connection, losing one must not be treated as losing the other. This WORK's disconnect-grace/debounce/reconnect design must be built per connection role from the start, not retrofitted onto a single-connection-kind assumption.

## Scope

### In Scope (known required outcome; design not yet started)

- `session_actors.semantic_presence` persistence and maintenance.
- Phase-dependent LOBBY/RUNNING disconnect consequences (GAME-ADR-0015).
- Atomic presence-transition-plus-Game-Language-processing (GAME-ADR-0016).
- A durable, ordered representation of the semantic-presence transition itself (not merely its resulting current value), consistent with GAME-ADR-0024/WORK-0019's replay-input model - so a Session's disconnect/reconnect history remains reconstructible in commit order, not only its latest value.
- A resync read capability (GAME-ADR-0010) returning a derived player-facing projection, built on top of WORK-0019's replay-reconstruction capability - never a raw stored Snapshot (none exists) and never replayed history/Effects.
- Coordinator-level disconnect grace/debounce, semantic-disconnect reporting, reconnect handling, applied independently per connection role (ADMIN vs. PARTICIPANT, per WORK-0020) - extending WORK-0005's/WORK-0020's existing Coordinator, not a second one.

### Out of Scope

- Inactivity/reaper handling for an orphaned Session with no one reconnecting (WORK-0016) - a related but distinct concern.
- Historical UI Effect replay - explicitly excluded; resync recomputes current state.
- The ADMIN connection's own spectator-selection resubscription behavior (WORK-0021's scope) - this WORK only notes, for WORK-0021's benefit, that spectator selection is ephemeral Coordinator/session-view state, not durable Game state, and so may reasonably need to be re-selected after an ADMIN reconnect unless WORK-0021's own design chooses to preserve it some other way; this WORK does not design that mechanism.

## Approved Design

Not yet designed. Exact resync payload shape, exact grace/debounce timing per connection role, and whether disconnect-grace timing needs WORK-0013's keyed timers or can use WORK-0012's ordinary timer mechanism remain open.

## Constraints and Invariants

- Resync must recompute current state via WORK-0019's replay-reconstruction capability, never load a stored Snapshot (none exists under GAME-ADR-0024) and never replay historical Effects.
- Presence transition and its Game-Language-visible consequence must be atomic (GAME-ADR-0016).
- A physical ADMIN connection's loss/reconnect must never produce or consume a `UserDisconnected`/`UserReconnected` Game Language signal (GAME-ADR-0025) - only a PARTICIPANT connection's semantic disconnect/reconnect does.
- Depends on WORK-0018 landing first (Game Language signal support) and WORK-0020 (role-aware connections) for its per-role reconnect design.

## Acceptance Criteria

Not yet defined - to be written when this WORK moves to DRAFT.

## Blockers

- Hard prerequisite: WORK-0018 (Game Language `UserDisconnected`/`UserReconnected` signal support) must be DONE (or land alongside this WORK) before this WORK can deliver either signal.
- Depends on WORK-0020 (role-aware connections) for its per-role reconnect design, and WORK-0019 (replay-first persistence) for its resync-reconstruction foundation.
- Whether this WORK needs WORK-0013's keyed timers, or can proceed with WORK-0012's ordinary timer mechanism, is an open dependency question to resolve during DRAFT.

## Scope Revision (Part B/K Reconciliation, 2026-09-20)

A broader reconciliation session accepted GAME-ADR-0024 (replay-first persistence) and GAME-ADR-0025 (role-aware connections), both materially bearing on this previously-PLANNED WORK's own framing: (1) this WORK's resync capability was originally framed against "the current Snapshot" as if one were simply stored and loadable - under GAME-ADR-0024 there is no stored Snapshot, so resync must call WORK-0019's replay-reconstruction capability first and derive its projection from that result; (2) this WORK's disconnect/reconnect handling was originally framed as a single undifferentiated Coordinator concern - GAME-ADR-0025 requires it to be designed per connection role from the outset, since an ADMIN connection's physical loss is never a Game Language event. Neither changes this WORK's own central open design questions (exact resync payload, grace/debounce timing, keyed-timer dependency), which remain for DRAFT. No implementation was performed; this WORK's Status remains PLANNED.

## Documentation Impact

Not yet assessed in detail; expected to touch `game/README.md`, `game/CURRENT_STATE.md`, `game/docs/DATA_MODEL.md`, `game/docs/FLOWS.md`, `play/README.md` once designed.

## Completion Record

Not started. PLANNED.
