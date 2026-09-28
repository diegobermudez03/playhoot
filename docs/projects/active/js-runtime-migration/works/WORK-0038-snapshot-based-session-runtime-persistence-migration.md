# WORK-0038: Snapshot-Based Session Runtime Persistence Migration

Status: PLANNED
Created: 2026-09-27
Last status change: 2026-09-27

Related decisions:
- `game/docs/decisions/GAME-ADR-0029-snapshot-based-session-runtime-persistence.md`
- `game/docs/decisions/GAME-ADR-0024-replay-first-session-runtime-persistence.md` (superseded in part by the above)

Canonical context:
- `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md`
- `docs/projects/completed/game-language-flat-execution-model/works/WORK-0028-engine-owned-turn-execution.md` (the implementation this WORK reverses the persistence-model portion of)
- `game/session/workflows/sessionlifecycle/internal/replay/replay.go` (current replay-based reconstruction being replaced as the live-correctness path)

## Outcome

Implement `GAME-ADR-0029`: reintroduce a persisted current-state representation as the authoritative record for Session continuation (crash recovery, disconnect/reconnect, any operation needing "what is the Session's state right now"), so live correctness no longer depends on replaying the durable signal log. Retain the durable signal/event log's existing tables, repurposed as a best-effort reconstruction/audit input only. This is foundational for `WORK-0040`–`WORK-0043` (all of which read/write current state) and for `session-runtime-v1`'s own `WORK-0017` (Archival), which this WORK's design must coordinate with.

**Scope addition (2026-09-27, discovered while drafting `WORK-0035`):** this WORK also switches every one of Session Runtime's seven RuntimeTurn-capable call sites (`step_create.go`, `step_join.go`, `step_start.go`, `step_answer_interaction.go`, `step_submit_user_intent.go`, `step_cancel_session.go`, `step_expire_timer.go`) from `engineservice.Compile`/`StartTurn`/`AdvanceTurn` to the new execution boundary, rather than leaving that switch as an unassigned task. It is folded in here rather than given its own WORK because this WORK already must touch every one of those call sites to change what they persist; doing the engine-call swap in the same pass avoids touching each file twice.

**Revised (2026-09-27, per `docs/decisions/architecture/ADR-0016-javascript-executor-separately-deployed-infrastructure-service.md`):** those seven call sites must be switched to `WORK-0053`'s `Executor` port, not directly to `WORK-0035`'s package (`game/session/internal/jsengine`) — that package's direct local invocation is superseded by `ADR-0016`; see `WORK-0035`'s own Completion Record. This WORK now depends on `WORK-0053` in addition to `WORK-0035`.

## Context

Not yet designed. This is an explicit architectural reversal of a fully implemented, independently reviewed, DONE migration — see `GAME-ADR-0029`'s own Consequences on why the prior WORK/ADR text is not rewritten.

## Scope

Not yet designed.

## Approved Design

Not yet designed.

## Constraints and Invariants

- The persisted state representation is authoritative; a reconstruction attempt from the retained signal log diverging from it is a diagnostic finding, never a live-correctness failure.
- Must not silently break `session-runtime-v1`'s already-DONE work built against the replay-first model (`WORK-0019`, `WORK-0012`, `WORK-0010`, `WORK-0011`, `WORK-0014`, `WORK-0016`) — each must be re-audited for what, if anything, changes for it, not assumed unaffected.

## Acceptance Criteria

Not yet designed.

## Implementation Freedom

Not yet designed.

## Verification

Not yet designed. Must include a test proving persisted state matches what live execution actually produced (mirroring WORK-0019's own precedent for its replay reconstruction, inverted).

## Documentation Impact

### Accepted / Canonical Knowledge

- `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` — replaces the replay-first sections with the snapshot-based model.
- `game/README.md` — Session Runtime Turn And Persistence Model, Process-Agnostic Recovery, History Archival Direction sections updated.

### Current-State Documentation After Implementation

- `game/docs/DATA_MODEL.md` — persisted-state column/table added; replay-only reconstruction logic marked as non-authoritative.

## Blockers

- Depends on `WORK-0035`'s execution contract (needs to know the exact `newState` shape being persisted).
- Must jointly resolve `session-runtime-v1`'s `WORK-0017` (Archival) redesign — flagged in that Project's own `PROJECT.md`, not resolved here in isolation.

## Completion Record

Not yet started.
