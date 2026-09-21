# WORK-0013: Keyed Timers

Status: PLANNED
Created: 2026-09-20
Last status change: 2026-09-20 (Part B reconciliation, same day: replay-input framing note added)

Related decisions:
- GAME-ADR-0012 (KeyedTimerSlot direction accepted; explicitly states no compiler/engine/program/migration/Session Runtime/WORK code is authorized by that record alone)
- GAME-ADR-0024 (Replay-First Session Runtime Persistence - the same TimerExpired-occurrence framing WORK-0012 confirms applies equally to a keyed timer's expiration, carrying `engine_key`)

Canonical context:
- `docs/projects/active/session-runtime-v1/PROJECT.md`
- `game/docs/decisions/GAME-ADR-0012-*.md`
- `docs/projects/active/session-runtime-v1/works/WORK-0012-timer-obligations.md` (the ordinary-timer mechanism this WORK generalizes)
- `docs/projects/active/session-runtime-v1/works/WORK-0015-disconnect-reconnect-full-resync.md` (a primary motivating consumer - independent per-player disconnect-grace timers)

## Outcome

Game Language supports multiple logically independent timer instances per Session (`KeyedTimerSlot<Key>`), for scenarios needing simultaneous independent timers - most notably an independent per-player disconnect-grace timeout, needed by WORK-0015.

`KeyedTimerSlot` does not exist in code anywhere today (confirmed by a whole-repo grep for `KeyedTimer`: zero Go source hits, only design-doc mentions). GAME-ADR-0012 accepts the direction but explicitly authorizes no implementation.

## Context

This is Game Language/compiler/engine work (not Session Runtime application logic) consumed by WORK-0012's timer mechanism and, in turn, by WORK-0015's disconnect-grace design. It is tracked under this Project because it directly gates a Session Runtime V1 capability, the same way the original plan already treated it as part of this initiative's own scope rather than a separate Game Language initiative.

**Reordering note**: the original plan sequenced Keyed Timers after Disconnect/Reconnect. This Project reorders it before Disconnect/Reconnect (WORK-0015), because disconnect-grace handling plausibly needs independent per-user timers itself - see `PROJECT.md`'s ordering rationale.

## Scope

### In Scope (known required outcome; design not yet started)

- `KeyedTimerSlot<Key>` compiler support (`game/language/v1/program`).
- Engine support for scheduling/matching/cancelling a keyed timer instance.
- The `session_timer_obligations.engine_key` persistence consequence in Session Runtime, extending WORK-0012's mechanism rather than building a second one.

### Out of Scope

- Disconnect/reconnect's own consumption of keyed timers (WORK-0015's own scope).

## Approved Design

Not yet designed - this is itself the Game Language prerequisite work; its compiler/engine design is the DRAFT-phase task.

## Constraints and Invariants

- Must extend WORK-0012's timer-obligation mechanism, not introduce a parallel one.

## Acceptance Criteria

Not yet defined - to be written when this WORK moves to DRAFT.

## Blockers

- None yet beyond needing WORK-0012's mechanism to exist (or be designed concurrently) to extend.

## Documentation Impact

Not yet assessed in detail; expected to touch `game/language/v1/program/README.md`, `game/language/v1/engine/LOGICAL_CONTRACT.md`, `game/CURRENT_STATE.md` once designed.

## Completion Record

Not started. PLANNED.
