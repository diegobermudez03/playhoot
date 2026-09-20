# Slice 3 — Interaction Response Processing: READY

Process: Feature Development (Slice 3 of `session-runtime-v1`; architecture is CLOSED, the broader initiative remains governed by `PLAN.md`)

Status: **READY, 2026-09-19.** `docs/work/active/WORK-0004-interaction-response-processing.md` moved DRAFT -> READY the same day it was drafted: the human reviewed and resolved all five Blockers below. This supersedes the prior checkpoint in this file (the DRAFT-proposal record).

## What This Slice Delivers

A player can answer an open interaction against a `RUNNING` Session, entirely at the Go-API level: Session Runtime obtains RUNNING-phase per-Session serialization (GAME-ADR-0018, extending the existing LOBBY locking mechanism unchanged), reloads current state, applies the response through the engine, and persists the resulting RuntimeTurn while resolving the answered `session_interactions` row.

This also closes a real gap found while designing it: no Turn-producing code today persists `session_interactions` at all - `step_start.go`'s own first Turn can open a question and nothing durable would ever record it. This WORK is also where that capture is introduced.

## Five Material Decisions - Resolved

1. **New sibling workflow package vs. a step on the existing `sessionlifecycle.Manager`.** **Resolved: no sibling package.** `AnswerInteraction` is a new step directly on `sessionlifecycle.Manager` (`step_answer_interaction.go`), following the same pattern as `Create`/`Join`/`Leave`/`Start`. Whether `TimerExpired` (Slice 5) or disconnect-reconnect (Slice 7) eventually warrant splitting into a separate package is left for those slices to decide when they materialize, not decided now.
2. **Extract the shared RuntimeTurn Step-draining/bound execution logic now.** **Resolved: extract, but not as a session-level package.** The original proposal (`game/session/internal/runtimeturn`, sibling to `sessionlock`/`idempotency`) is rejected - that treats workflow-internal execution logic as if it were a horizontal Session Runtime mechanism. Instead it stays scoped to the `sessionlifecycle` workflow itself: either a plain shared helper inside the `sessionlifecycle` package, or, if a package is warranted, `game/session/workflows/sessionlifecycle/internal/runtimeturn` - never at session level. `step_start.go` is refactored to call it; its behavior doesn't change.
3. **Retrofit `step_start.go` to also capture interactions opened by its own first Turn.** **Approved as proposed**, in this WORK's scope.
4. **How to determine whether an opened interaction is a `Question` or an `AskGroup`.** **Approved as proposed** - read off the compiled `engine.Program`'s own slot declaration at capture time and persisted on the row.
5. **Narrow terminal-cleanup scope for this slice's own fatal path.** **Approved as proposed, now** - this slice's own fatal path atomically closes any still-`ACTIVE` interactions it leaves behind, in the same transaction; the fully general sweep stays with Slice 6.

## Consequence

`WORK-0004` is now **READY** - the DRAFT was revised in place per the resolutions above. Implementation may begin.

## Next Human Action

None required to proceed with implementation. Independent review will surface anything further once implementation completes.
