# WORK-0027: Session Runtime Interaction-Addressing Rework

Status: PLANNED
Created: 2026-09-24
Last status change: 2026-09-24

Related decisions:
- GAME-ADR-0026 (Flat Workflow Execution Model, Keyed Interaction Slots, and Engine-Owned Interaction Addressing)
- GAME-ADR-0007 (Session Runtime Turn Architecture and Persistence Model - `session_interactions`' `(engine_path, engine_slot)` identity is superseded in part by this WORK; a follow-up note/update is owned here, not a rewrite of GAME-ADR-0007 itself)

Canonical context:
- `docs/projects/active/game-language-flat-execution-model/works/WORK-0026-engine-owned-interaction-addressing.md` (must land first)
- `game/session/workflows/sessionlifecycle/interaction_capture.go` (`captureInteractions`, `resolveInteractionKind`, `encodeEnginePath`/`decodeEnginePath` - all reworked or removed by this WORK)
- `game/session/workflows/sessionlifecycle/replay.go` (`buildAnswerSignal` - reconstructs an `engine.Signal` from persisted `engine_path`/`engine_slot`; becomes an `InteractionID`-based reconstruction instead)
- `docs/projects/active/session-runtime-v1/PROJECT.md` (Phase 1 is paused pending this Project - see its own "Paused (2026-09-24)" section)

## Outcome

`game/session/workflows/sessionlifecycle` consumes the new engine contract (WORK-0024/0025/0026) directly: `session_interactions` is keyed by the engine's own `InteractionID` instead of encoding/decoding `(engine_path, engine_slot)`, `captureInteractions` no longer needs the compiled `Program` to classify an opened interaction's kind, and answering constructs the new unified signal. This is what unblocks `session-runtime-v1`'s Phase 1 (WORK-0006 onward) to resume.

## Context

Not yet designed in detail. This WORK also needs a migration decision for `session_interactions`' existing `engine_path`/`engine_slot` columns (replace outright vs. add `interaction_id` alongside and deprecate the old columns) - flagged as a Material Decision in this Project's `PROJECT.md`, not resolved here.

## Scope

Not yet designed.

## Approved Design

Not yet designed.

## Constraints and Invariants

- Must preserve GAME-ADR-0024's replay-first guarantee: replaying a Session's durable causes must reconstruct the exact same `InteractionID` sequence live execution produced, the same way it already reconstructs `Snapshot.Sequence`/`RandomState` today.
- Must not reopen or reinterpret any already-DONE Session Runtime WORK's historical scope - this is new work against a new engine contract.

## Acceptance Criteria

Not yet defined - to be written when this WORK moves to DRAFT.

## Blockers

- `session_interactions` migration shape (see this Project's `PROJECT.md` Material Decisions) - open question for DRAFT.

## Documentation Impact

Not yet assessed in detail; expected to touch `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md`, `game/docs/DATA_MODEL.md`, `game/CURRENT_STATE.md`, `game/docs/FLOWS.md`, and `docs/projects/active/session-runtime-v1/PROJECT.md` (resolving its pause note).

## Completion Record

Not started. PLANNED.
