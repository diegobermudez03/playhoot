# GAME-ADR-0008: V1 Game Language Timer Schedule Recovery Simplification

Status: ACCEPTED
Created: 2026-09-07
Last status change: 2026-09-07
Supersedes: None
Superseded by: None
Legacy ID: ADR-0011

## Context

GAME-ADR-0002 established that timer obligations affecting game semantics are durable Session Runtime state, while the Coordinator owns the physical timer and detects elapsed wall-clock time. GAME-ADR-0007 introduces `session_timer_obligations` with `delay_ms` and `state`, created and closed by RuntimeTurns.

A pressure remained: should Session Runtime also persist an absolute deadline (`due_at`) so that, after a process crash, the exact remaining wall-clock time can be restored? Doing so would make Session Runtime itself time-aware and would need to reconcile stored deadlines against real elapsed downtime, which is a materially larger correctness surface than durably recording that a timer obligation exists.

## Decision

Session Runtime persists only the timer obligation and its configured `delay_ms`; it does not calculate or persist an absolute deadline for Game Language timers. The Coordinator remains the time-aware layer and owns physical timers in memory, consistent with GAME-ADR-0002.

The previously proposed Coordinator-owned `live_timer_schedules` table is rejected for V1 and is not part of the accepted schema.

If the process/system dies, physical schedules may be lost. On recovery, active timer obligations may be rescheduled using their full configured `delay_ms` from the new scheduling moment. Preserving elapsed wall-clock time across a full process restart is explicitly not required for V1. This is an accepted simplicity/failure-tolerance tradeoff.

`lobby_expires_at` is not affected by this decision. It is a previously accepted Session lifecycle deadline (GAME-ADR-0003, GAME-ADR-0004) and is a separate concern from Game Language timer scheduling.

## Rationale

Recording only the obligation and its configured delay keeps Session Runtime's durability guarantee simple and honest: an obligation cannot be silently lost, but its precise remaining time is not a correctness property V1 promises. Adding a durable absolute-deadline table now would require deciding clock-skew handling, missed-wakeup catch-up semantics, and reconciliation with the Coordinator's own recovery pass - complexity with no current concrete driver.

Rejecting `live_timer_schedules` avoids introducing a second durable timer representation (obligation plus schedule) before there is a concrete requirement that elapsed time must survive a full process restart.

## Alternatives Considered

### Persist an absolute `due_at`/deadline on the timer obligation

Rejected for V1. It would require Session Runtime to own precise wall-clock reconciliation logic and decide catch-up semantics for time missed during downtime, which is materially more complex than the current requirement justifies.

### Introduce a durable Coordinator-owned `live_timer_schedules` table

Rejected for V1. It duplicates the timer obligation with a second durable schedule representation before a concrete need for cross-restart elapsed-time preservation exists.

### Use an external durable scheduler/job queue for timer physical scheduling

Rejected for V1. It introduces an additional infrastructure dependency to solve a problem the accepted full-delay-reset tradeoff already handles acceptably for the current single-process modular monolith.

## Consequences

- Timer obligation recovery after a crash uses the full configured `delay_ms` from the new scheduling moment, not the original deadline.
- Game Language timer semantics must not assume exact elapsed-time preservation across a full process restart in V1.
- `lobby_expires_at` remains an authoritative Session lifecycle deadline, unaffected by this decision.
- Reevaluation trigger: if an accepted game/product requirement needs precise timer elapsed-time preservation across a full process restart, this tradeoff must be revisited before that feature can be considered correctly supported.

## Canonical Knowledge Impact

- `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` - confirms `live_timer_schedules` is excluded from the accepted schema and records the accepted recovery behavior.
- `game/README.md` - references this ADR alongside the Session Runtime Turn/persistence model.

## Implementation Impact

Future Coordinator/Session Runtime implementation must reschedule active timer obligations using their full configured delay on recovery rather than attempting to reconstruct an absolute deadline. No implementation, migration, or WORK is authorized by this record.
