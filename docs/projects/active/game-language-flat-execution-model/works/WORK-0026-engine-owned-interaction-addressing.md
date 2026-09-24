# WORK-0026: Engine-Owned Interaction Addressing

Status: PLANNED
Created: 2026-09-24
Last status change: 2026-09-24

Related decisions:
- GAME-ADR-0026 (Flat Workflow Execution Model, Keyed Interaction Slots, and Engine-Owned Interaction Addressing - Decisions 3, 4, and 5)

Canonical context:
- `game/docs/decisions/GAME-ADR-0026-flat-workflow-execution-model-and-keyed-interaction-slots.md`
- `docs/projects/active/game-language-flat-execution-model/works/WORK-0025-keyed-interaction-slots.md` (must land first - a caller-facing ID scheme should account for keyed occurrences from the start)
- `game/session/workflows/sessionlifecycle/interaction_capture.go` (`resolveInteractionKind` - the exact consumer-side leak this WORK removes: reaching into the compiled `Program` to classify an already-opened interaction by looking up which slot collection its name belongs to)

## Outcome

Every opened Question or Ask Group occurrence (keyed or not) receives a unique, monotonically increasing `InteractionID`, assigned as part of the engine's own deterministic state, carried on the Output that opens it alongside an explicit `Kind` (Question vs. AskGroup). Answering it submits only `{InteractionID, Respondent, Answer}` through one unified signal shape - never a `Slot`, `Key`, or the caller's own classification of what kind of interaction it is. This removes the last piece of engine-internal addressing (`Slot`/`Key` classification) that today leaks into `sessionlifecycle`'s own code, and directly enables WORK-0027.

## Context

Not yet designed in detail. The central design question is where the `InteractionID` counter lives (a `Snapshot`-level monotonic counter, analogous to `Snapshot.Sequence`) and how it survives replay identically (GAME-ADR-0024's replay-first model requires this to be part of the engine's own deterministic state, never assigned by a caller or a persistence-layer auto-increment).

## Scope

Not yet designed.

## Approved Design

Not yet designed.

## Constraints and Invariants

- The `InteractionID` sequence must be part of `Snapshot`'s own deterministic state, so that replaying the same signal sequence against the same starting Snapshot reproduces identical IDs - a caller-assigned or persistence-assigned ID would not satisfy this.
- Must not require the caller to already know whether it is answering a Question or an Ask Group - the engine resolves this from the ID alone (GAME-ADR-0026 Decision 4).

## Acceptance Criteria

Not yet defined - to be written when this WORK moves to DRAFT.

## Blockers

- Concrete `InteractionID` type/persistence-encoding shape - open question for DRAFT.
- Exact unified answer-signal Go shape - open question for DRAFT.

## Documentation Impact

Not yet assessed in detail; expected to touch `game/language/v1/engine/README.md`, `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` (flagging the GAME-ADR-0007 follow-up WORK-0027 will need).

## Completion Record

Not started. PLANNED.
