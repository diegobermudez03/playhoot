# WORK-0025: Keyed Question, Ask Group, and Presentation Slots

Status: PLANNED
Created: 2026-09-24
Last status change: 2026-09-24

Related decisions:
- GAME-ADR-0026 (Flat Workflow Execution Model, Keyed Interaction Slots, and Engine-Owned Interaction Addressing - Decision 2)
- GAME-ADR-0012 (Game Language Keyed Timer Slots - accepted, still unimplemented; this WORK generalizes its concept, not its code, since no `KeyedTimerSlot` implementation exists yet either)

Canonical context:
- `game/docs/decisions/GAME-ADR-0026-flat-workflow-execution-model-and-keyed-interaction-slots.md`
- `game/docs/decisions/GAME-ADR-0012-game-language-keyed-timer-slots.md` (the accepted semantics this WORK extends: identity `(slot, key)`, atomic-failure-on-occupied-tuple, explicit-cancel-before-reschedule, key exposed to authored logic)
- `docs/projects/active/game-language-flat-execution-model/works/WORK-0024-remove-child-workflow-and-task-group.md` (must land first - see Ordering in this Project's `PROJECT.md`)

## Outcome

An author can declare a Question, Ask Group, or Presentation slot that holds independent, simultaneously-pending occurrences addressed by an authored key (typically a player, team, or object identity), instead of needing a separate workflow instance to get per-entity independent progress - this is what Task Group previously existed for, replaced per GAME-ADR-0026. This is required for the product's target range of games that need "the same simple mechanism, repeated independently per player/team/object" (an asynchronous self-paced quiz, a per-team negotiation, a per-object timer already accepted for Timers) without the removed nested-execution machinery.

## Context

Not yet designed in detail. GAME-ADR-0012 already accepted the semantic shape for the Timer case; this WORK's central design question is whether/how much of that shape (and any future implementation) can be shared across Timer/Question/AskGroup/Presentation rather than reimplemented four times, given none of the four have any existing implementation to reconcile with (GAME-ADR-0012 was accepted but never implemented).

## Scope

Not yet designed.

## Approved Design

Not yet designed.

## Constraints and Invariants

- Must match GAME-ADR-0012's already-accepted per-tuple semantics (atomic failure on an occupied `(slot, key)`, explicit cancel before reschedule, no implicit reset/replace/coalesce) for consistency across every keyed slot family, not just Timers.
- Must not reintroduce any form of nested execution/instance addressing - a keyed slot's multiple occurrences all belong to the one existing instance.

## Acceptance Criteria

Not yet defined - to be written when this WORK moves to DRAFT.

## Blockers

- Whether Timer's own keyed-slot implementation should happen as part of this WORK (since GAME-ADR-0012 predates this one and is still unimplemented) or remain separately tracked - open question for DRAFT.
- Concrete declaration/operation/signal-source shape per slot kind - open question for DRAFT.

## Documentation Impact

Not yet assessed in detail; expected to touch `game/language/v1/program/README.md`, `game/language/v1/engine/README.md`, `game/docs/decisions/GAME-ADR-0012-...md`'s Canonical Knowledge Impact cross-reference.

## Completion Record

Not started. PLANNED.
