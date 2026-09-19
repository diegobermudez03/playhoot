# Slice 3 — Interaction Response Processing: DRAFT (Awaiting Review)

Process: Feature Development (Slice 3 of `session-runtime-v1`; architecture is CLOSED, the broader initiative remains governed by `PLAN.md`)

Status: **DRAFT PROPOSED, 2026-09-19.** `docs/work/active/WORK-0004-interaction-response-processing.md` was created as DRAFT after reconciling Slice 3's design against the actual post-Slice-2 codebase (GAME-ADR-0007/0018/0019, the engine's actual `OpenQuestionOutput`/`SignalKindQuestionAnswered`/`SignalKindAskGroupAnswered` API, and `step_start.go`'s actual implementation). This supersedes the prior checkpoint in this file (Slice 2's READY-approval record, now historical - see `docs/work/completed/WORK-0003-session-start-first-runtimeturn.md` for that record).

## What This Slice Delivers

A player can answer an open interaction against a `RUNNING` Session, entirely at the Go-API level: Session Runtime obtains RUNNING-phase per-Session serialization (GAME-ADR-0018, extending the existing LOBBY locking mechanism unchanged), reloads current state, applies the response through the engine, and persists the resulting RuntimeTurn while resolving the answered `session_interactions` row.

**A real gap was found while designing this**, not previously visible: no Turn-producing code today persists `session_interactions` at all - `step_start.go`'s own first Turn can open a question and nothing durable would ever record it. This WORK is also where that capture is introduced.

## Five Material Decisions Needing Your Resolution

WORK-0004's Blockers section has the full reasoning for each; summarized here for a quick decision:

1. **New sibling workflow package (`game/session/workflows/runtimeexecution`) vs. adding `AnswerInteraction` onto the existing `sessionlifecycle.Manager`.** Proposed: new sibling package - Create/Join/Leave/Start is an "admission into a running game" lifecycle; interaction/timer/presence processing is a materially distinct "progressing a running game" process with its own concurrency model, expected to grow (Slices 5/7 add `TimerExpired`/disconnect-reconnect to the same new package).
2. **Extract the shared RuntimeTurn Step-draining/bound execution logic now** (`game/session/internal/runtimeturn`), reversing Slice 2's deferral now that a second real caller needs it - exactly the condition you set when rejecting it at Slice 2. `step_start.go` would be refactored to call the extracted function; nothing about its behavior changes.
3. **Retrofit `step_start.go` to also capture interactions opened by its own first Turn**, so a game that opens a question immediately at Start actually works with this feature. Proposed: yes, in this WORK's scope, since leaving it out would silently break the most natural authored-game shape.
4. **How to determine whether an opened interaction is a `Question` or an `AskGroup`** (needed to construct the correct signal kind on response) - proposed: read it off the compiled `engine.Program`'s own slot declaration at capture time and persist it on the row. Flagged as not yet deeply verified against the compiler's actual internal shape.
5. **Narrow terminal-cleanup scope**: when this slice's own fatal path terminates a Session, should it close any still-`ACTIVE` interactions itself (a small, path-scoped instance of GAME-ADR-0019's general invariant), or is leaving that gap until Slice 6's general sweep acceptable? Proposed: implement the narrow case now - this is the first slice where a fatal failure could leave a real `ACTIVE` interaction behind.

## Consequence

`WORK-0004` remains **DRAFT** - no implementation authority - until you resolve the five items above (approve as proposed, or redirect, per item). Once resolved, the DRAFT is revised in place and moved to READY, exactly as WORK-0003 was.

## Next Human Action

Review and resolve the five Blockers above (or ask for more detail on any of them) so this can move DRAFT -> READY.
