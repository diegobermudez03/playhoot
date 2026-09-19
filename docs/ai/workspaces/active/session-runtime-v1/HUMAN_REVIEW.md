# Slice 2 — Start + First RuntimeTurn: READY (Human-Approved 2026-09-19)

Process: Feature Development (Slice 2 of `session-runtime-v1`; architecture is CLOSED, the broader initiative remains governed by `PLAN.md`)

Status: **RESOLVED.** `docs/work/active/WORK-0003-session-start-first-runtimeturn.md` moved DRAFT -> READY on 2026-09-19 after the human resolved all four Blockers below. This supersedes the prior checkpoint in this file (the DRAFT proposal awaiting review).

## Resolution Of The Four Material Decisions

1. **Where the common RuntimeTurn execution logic lives.** Proposed a new shared package, `game/session/internal/runtimeturn`. **Human decision: rejected.** Reason given: this is workflow execution logic that no other use case will call - it belongs inside `step_start.go`, the workflow step that actually performs the execution, not a horizontal shared package. A shared package is deferred until Slice 3 (interaction responses) actually needs to reuse this logic; WORK-0003's Approved Design and Scope were revised accordingly (RuntimeTurn Execution Logic subsection, inline in `step_start.go`).
2. **Fatal-failure classification split** (`RUNTIME_STATE_INVALID` for a pinned-Definition recompile failure; `RUNTIME_EXECUTION_FAILED` for everything else, including an outright rejection of Start's own initial signal). **Approved as proposed.**
3. **Replaying a fatal Start via idempotency** (`StartOutcomeRuntimeInitFailed` replays as a completed outcome on retry, never re-attempted). **Approved as proposed.**
4. **`players` root-roster ordering** (ascending `joined_at`, first to join is `players[0]`). **Approved as proposed.**

## Consequence

`WORK-0003` is now **READY**. A future Codebase Agent session may implement it following the READY specification and `docs/ai/protocols/IMPLEMENTATION_REVIEW.md`. No Slice 2 implementation has begun yet.

## Next Human Action

None required for this checkpoint. Implementation may proceed per the Feature Development protocol's Handoff step.
