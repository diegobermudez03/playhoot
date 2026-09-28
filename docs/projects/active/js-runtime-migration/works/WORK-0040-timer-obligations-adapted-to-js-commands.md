# WORK-0040: Timer Obligations Adapted To JS Commands

Status: PLANNED
Created: 2026-09-27
Last status change: 2026-09-27

Related decisions:
- `game/docs/decisions/GAME-ADR-0028-javascript-execution-replaces-game-language.md`
- `game/docs/decisions/GAME-ADR-0008-session-runtime-v1-timer-recovery-simplification.md`
- `game/docs/decisions/GAME-ADR-0012-game-language-keyed-timer-slots.md`

Canonical context:
- `game/session/workflows/sessionlifecycle/internal/timers/` (existing durable timer-obligation persistence, `session_timer_obligations`)

## Outcome

Adapt Session Runtime's already-implemented durable timer-obligation mechanism (`session_timer_obligations`, ordinary and keyed, from `session-runtime-v1`'s DONE `WORK-0012`) to be driven by `WORK-0039`'s command vocabulary (schedule/cancel requested by JS output) instead of `engine.ScheduleTimerOutput`/`CancelTimerOutput`. Timer ownership itself (Playhoot schedules and fires; authored code only requests/reacts) is unchanged in principle per `GAME-ADR-0028`; this WORK is the concrete integration, not a redesign of timer semantics.

## Context

Not yet designed. Depends on `WORK-0039`'s command schema for schedule/cancel.

## Scope

Not yet designed.

## Approved Design

Not yet designed. `GAME-ADR-0008`'s relative-delay-only recovery model and the existing keyed-slot addressing scheme are expected to carry over largely unchanged; confirm rather than assume during design.

## Constraints and Invariants

- No timer obligation may remain ACTIVE after a Session reaches TERMINAL (restates the existing GAME-ADR-0019 terminal-cleanup invariant, applied to the new command-sourced timers).

## Acceptance Criteria

Not yet designed.

## Implementation Freedom

Not yet designed.

## Verification

Not yet designed.

## Documentation Impact

### Current-State Documentation After Implementation

- `game/docs/DATA_MODEL.md`/`game/docs/FLOWS.md` — timer-obligation source updated from engine Output to JS command.

## Blockers

- Depends on `WORK-0039`.

## Completion Record

Not yet started.
