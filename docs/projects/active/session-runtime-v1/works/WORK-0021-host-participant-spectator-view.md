# WORK-0021: Host Participant Spectator View

Status: PLANNED
Created: 2026-09-20
Last status change: 2026-09-20

Related decisions:
- GAME-ADR-0025 (Role-Aware Live Connections - the ADMIN connection this WORK's spectator selection attaches to, and the read-only-visual-mirroring principle)
- GAME-ADR-0020 (post-commit client delivery semantics - the same best-effort delivery rule this WORK's mirrored Presentation/Effect stream is subject to)

Canonical context:
- `docs/projects/active/session-runtime-v1/PROJECT.md`
- `game/docs/decisions/GAME-ADR-0025-role-aware-live-connections.md` ("Host spectator view (RUNNING)" section)
- `docs/projects/active/session-runtime-v1/works/WORK-0020-role-aware-live-connections.md` (the ADMIN connection this WORK depends on)
- `docs/projects/active/session-runtime-v1/works/WORK-0006-broaden-live-fanout-effects-presentations.md` (the Presentation/Effect fan-out mechanism this WORK mirrors read-only, and the "derived, never persisted" Presentation principle a spectator's initial-state reconstruction must reuse rather than duplicate)

## Outcome

An `ADMIN` connection can select a Participant to spectate and later switch the selection, receiving that Participant's currently-active Presentation state immediately on subscription/switch and ongoing Presentation/Effect updates for as long as the selection remains active - a read-only mirror of what the selected Participant sees, with no ability to answer or otherwise act on that Participant's behalf. The selection itself is ephemeral Coordinator/session-view state, never persisted as Session Runtime truth, and never affects the observed Participant's runtime state or Game Language semantics.

## Context

Decision 15/16 of the reconciliation prompt that created this WORK settle the product/architecture direction (read-only visual mirroring, ephemeral selection, no actionable Interaction authority) - this WORK's own design task is narrower: the concrete subscription/selection mechanism, and specifically how to reconstruct a spectator's *initial* current Presentation state on subscribe/switch without duplicating logic. `deriveActivePresentations` (`game/language/v1/engine/internal/runtime/presentation.go`) already recomputes "what's currently mounted for a given player" fresh from the current Snapshot on demand (WORK-0006's own established principle) - this WORK's initial-reconstruction path should reuse that same recomputation, ideally through whatever generic player-facing projection/resync capability WORK-0015 eventually builds, rather than inventing a second, spectator-specific recomputation path. Because WORK-0015 is itself PLANNED and not yet designed, this WORK is left PLANNED rather than DRAFT - committing to a concrete spectator-reconstruction mechanism before knowing WORK-0015's actual resync shape risks building something WORK-0015 then has to reconcile against, rather than reuse.

## Scope

### In Scope (known required outcome; design not yet started)

- A subscription/selection mechanism on the `ADMIN` connection: choose a Participant to spectate, switch to a different one, stop spectating.
- Initial current-Presentation-state delivery on subscribe/switch, reusing rather than duplicating the engine's existing `deriveActivePresentations` recomputation (and, once it exists, WORK-0015's generic resync/projection capability).
- Ongoing mirrored delivery of Presentation updates and presentation-only Effects for the currently-selected Participant, reusing WORK-0006's existing per-recipient `Event`/`Deliver` translation mechanism (the spectating `ADMIN` connection becomes an additional delivery target for the selected Participant's own Events, not a new translation path).
- Read-only enforcement: the spectator stream never carries an actionable `interaction_id`, and an `ADMIN` connection can never submit `ANSWER_INTERACTION` on the spectated Participant's behalf (already true by construction once WORK-0020 lands, since `ANSWER_INTERACTION` is rejected on any `ADMIN` connection regardless of spectator selection).

### Out of Scope

- The `ADMIN` connection/registry itself (WORK-0020's own scope; this WORK depends on it).
- Presentation/Effect fan-out to the actually-participating Participant (WORK-0006's own scope; this WORK only adds a second, read-only delivery target).
- A generic player-facing resync/projection capability (WORK-0015's own scope); this WORK reuses it once it exists rather than building its own.
- Any authoritative Game-state change caused by spectating - spectating never mutates runtime state, by design, not merely by omission.

## Approved Design

Not yet designed. Left open pending PLANNED -> DRAFT, and explicitly deferred until WORK-0015's resync/projection shape is known (see Context): the exact subscription/selection wire messages, whether the `ADMIN` registry entry itself carries the current selection or a separate small map does, and the precise reuse boundary between this WORK's initial-reconstruction path and WORK-0015's eventual resync capability.

## Constraints and Invariants

- Spectator selection is ephemeral Coordinator/session-view state - never persisted as Session Runtime truth, never a Session Runtime column/table.
- Selecting or switching a spectator target must never mutate the observed Participant's runtime state or Game Language semantics.
- The spectator stream never exposes an actionable `interaction_id` or grants answer authority over the observed Participant's open interaction.
- Depends on WORK-0020 (ADMIN connection) and WORK-0006 (Presentation/Effect live delivery mechanism) landing first; should reuse rather than duplicate whatever generic resync/projection capability WORK-0015 eventually builds for initial-state reconstruction.

## Acceptance Criteria

Not yet defined - to be written when this WORK moves to DRAFT.

## Blockers

- Depends on WORK-0020 (ADMIN connection) existing first.
- Depends on WORK-0006 (Presentation/Effect delivery) existing first.
- Initial-state-reconstruction design should wait for (or be designed jointly with) WORK-0015's generic resync/projection capability to avoid duplicating logic - an open sequencing question for whoever schedules this WORK's DRAFT phase, not resolved here.

## Documentation Impact

Not yet assessed in detail; expected to touch `play/README.md`, `game/CURRENT_STATE.md`, `game/docs/FLOWS.md` once designed.

## Completion Record

Not started. PLANNED.
