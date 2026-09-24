# WORK-0012: Timer Obligations

Status: PLANNED
Created: 2026-09-20
Last status change: 2026-09-20 (Part B reconciliation, same day: replay-input framing clarified - see "Scope Clarification (Part B Reconciliation, 2026-09-20)" below)

Related decisions:
- GAME-ADR-0008 (no durable due-at, full-configured-delay recovery tradeoff)
- GAME-ADR-0013 (process-agnostic timer recovery)
- GAME-ADR-0018 (RUNNING serialization - timer expiration contends for the same boundary as interaction responses)
- GAME-ADR-0024 (Replay-First Session Runtime Persistence - `session_timer_obligations` already gives a TimerExpired occurrence a durable home; this WORK confirms, does not newly design, that fit)

Canonical context:
- `docs/projects/active/session-runtime-v1/PROJECT.md`
- `game/language/v1/program/timer.go` (`TimerSlotDeclaration`, `ScheduleTimerOperation`, `CancelTimerOperation` - already implemented, ordinary single-pending-timer only)
- `game/language/v1/engine/output.go` (`ScheduleTimerOutput`/`CancelTimerOutput` - already produced by the engine, currently unhandled anywhere in Session Runtime)
- `docs/projects/active/session-runtime-v1/works/WORK-0020-role-aware-live-connections.md` (owns rebuilding the `play` Coordinator this WORK's physical scheduling extends. **2026-09-23 correction**: `play` does not currently exist - deleted in full by WORK-0005's Blocker 11 (2026-09-21); this WORK's durable-persistence half needs no `play` at all, but its physical scheduling/wakeup half can only be implemented once WORK-0020 (re)builds the Coordinator - this WORK was drafted before that deletion and never reconciled against it until now)

## Outcome

An authored Game Language `TimerSlot` obligation is durably scheduled, survives process loss/restart, and its expiration re-enters Session Runtime as a RuntimeTurn cause exactly like an interaction response does today.

Today, `ScheduleTimerOutput`/`CancelTimerOutput` are real engine outputs but `grep "Timer"` across `game/session/` returns zero matches - nothing schedules a real delayed delivery of a timer-expiration signal anywhere in Session Runtime.

## Context

This is required before any authored game that uses a timer (a countdown per question, a turn-time-limit, etc.) can actually run to completion in Session Runtime - without it, a `ScheduleTimerOutput` the engine produces is simply dropped, and the game logically waiting on that timer can never receive its expiration.

## Scope

### In Scope (known required outcome; design not yet started)

- Durable persistence for a pending timer obligation (`session_timer_obligations` per the accepted persistence model) - needs no `play`/transport code.
- Physical scheduling/wakeup added to the `play` Coordinator WORK-0020 (re)builds - not a mechanism this WORK invents independently of WORK-0020's own registry/lifecycle design.
- Cancellation/replacement semantics matching `CancelTimerOperation`'s authored meaning.
- Timer expiration as a RuntimeTurn cause, reusing the existing Step-draining/bound execution mechanism and contending correctly for the same per-Session RUNNING serialization boundary interaction responses already use (GAME-ADR-0018).
- Correctness after process loss/restart, using the accepted full-configured-delay recovery tradeoff (GAME-ADR-0008/0013) rather than a durable due-at timestamp.

### Out of Scope

- Keyed/per-key independent timers (WORK-0013) - this WORK covers only the existing single-pending-timer `TimerSlot` shape already in `program/timer.go`.
- Disconnect-driven timer usage (WORK-0015) - a consumer of this mechanism, not this WORK's own scope.

## Approved Design

Not yet designed. Scheduler mechanism, leasing/claiming approach if any, and exact recovery-on-restart procedure remain open, per GAME-ADR-0008/0013's already-accepted tradeoffs, deliberately left for the DRAFT phase.

## Constraints and Invariants

- No durable due-at timestamp is required to be exact (GAME-ADR-0008's accepted tradeoff); recovery uses the full configured delay, not a resumed countdown.
- Timer expiration must serialize correctly against a concurrent interaction response for the same Session (GAME-ADR-0018).

## Acceptance Criteria

Not yet defined - to be written when this WORK moves to DRAFT.

## Blockers

- Scheduler/recovery mechanism design is the material open question for DRAFT, not resolved here.

## Scope Clarification (Part B Reconciliation, 2026-09-20)

`game/docs/decisions/GAME-ADR-0024-replay-first-session-runtime-persistence.md` distinguishes two things this WORK must keep separate: the *physical scheduling mechanism* (Coordinator-owned, ephemeral, already out of scope for durable persistence per GAME-ADR-0008/GAME-ADR-0013 - ordinary operational/diagnostic logging about scheduling is fine, but it is never archived as replay-input truth) versus the *semantic `TimerExpired` occurrence* (a genuine RuntimeTurn-driving cause, whose ordering in the replay-input log matters). This WORK already gives the latter a durable home via `session_timer_obligations` (`engine_path`/`engine_slot`/`engine_key`) plus the causing Turn's own `source_timer_obligation_id` - this is confirmed sufficient by GAME-ADR-0024's audit, not a new requirement this WORK must additionally design for. This is a clarification of already-intended scope, not a new deliverable; no implementation was performed and this WORK's Status remains PLANNED.

## Documentation Impact

Not yet assessed in detail; expected to touch `game/CURRENT_STATE.md`, `game/docs/FLOWS.md`, `play/README.md` once designed.

## Completion Record

Not started. PLANNED.
