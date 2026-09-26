# WORK-0013: Keyed Timers

Status: CANCELLED (superseded - folded into WORK-0012)
Created: 2026-09-20
Last status change: 2026-09-25 (PLANNED -> CANCELLED, superseded by WORK-0012 - see "Superseded (2026-09-25)" below)

Related decisions:
- GAME-ADR-0012 (KeyedTimerSlot direction accepted; its own "Implemented by" note confirms the compiler/engine/program portion is DONE via WORK-0025)
- GAME-ADR-0024 (Replay-First Session Runtime Persistence - the same TimerExpired-occurrence framing WORK-0012 confirms applies equally to a keyed timer's expiration, carrying `engine_key`)

Canonical context:
- `docs/projects/active/session-runtime-v1/PROJECT.md`
- `game/docs/decisions/GAME-ADR-0012-*.md`
- `docs/projects/active/session-runtime-v1/works/WORK-0012-timer-obligations.md` (supersedes this WORK - now covers ordinary **and** keyed timer persistence/expiration together)
- `docs/projects/completed/game-language-flat-execution-model/works/WORK-0025-keyed-interaction-slots.md` (already implemented the compiler/engine/program portion this WORK's own "Outcome" originally described)

## Superseded (2026-09-25)

This WORK's premise is stale. Its own "Outcome"/"Context" (below, preserved unedited as historical record) states `KeyedTimerSlot` "does not exist in code anywhere today," confirmed at the time by "a whole-repo grep for `KeyedTimer`: zero Go source hits." That is no longer true: `program.KeyedTimerSlotDeclaration`, `ScheduleKeyedTimerOperation`/`CancelKeyedTimerOperation`, `engine.ScheduleKeyedTimerOutput`/`CancelKeyedTimerOutput`, and `SignalKindKeyedTimerExpired` are all fully implemented today - landed by `WORK-0025-keyed-interaction-slots.md` (now completed), as `GAME-ADR-0012`'s own "Implemented by" note already confirms.

The only piece of this WORK's originally-described scope that was never implemented - the Session Runtime persistence consequence, `session_timer_obligations.engine_key` - is exactly `WORK-0012-timer-obligations.md`'s own territory (it already owns designing/implementing `session_timer_obligations` for ordinary timers). Per explicit human direction (2026-09-25), rather than draft this WORK a second time against an already-mostly-complete premise, its remaining scope is folded into WORK-0012, which now covers ordinary **and** keyed timer persistence/expiration together, designed and implemented once against the same table/step/replay-reconstruction machinery.

This WORK is marked `CANCELLED` per `docs/work/README.md`'s status model (no "SUPERSEDED" status exists) and preserved as closed historical record. No implementation was ever started under this WORK. `docs/projects/active/session-runtime-v1/PROJECT.md`'s Phase 1 table is updated to reflect this (WORK-0013 removed as a separate ordered row; WORK-0012's own row now describes the merged scope).

---

*Everything below this line is preserved unedited from this WORK's original drafting, as the historical record of what was known/assumed at the time - it does not reflect current reality where it conflicts with "Superseded" above.*

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
