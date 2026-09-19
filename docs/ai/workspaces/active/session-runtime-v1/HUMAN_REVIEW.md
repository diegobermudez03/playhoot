# Slice 2 — Start + First RuntimeTurn: DRAFT Proposed, Awaiting Review

Process: Feature Development (Slice 2 of `session-runtime-v1`; architecture is CLOSED, the broader initiative remains governed by `PLAN.md`)

Status: **CHECKPOINT PENDING.** `docs/work/active/WORK-0003-session-start-first-runtimeturn.md` has been drafted and needs your review before it can move DRAFT -> READY. This supersedes the prior checkpoint in this file (Slice 1's READY approval, now historical - Slice 1 is DONE, see `docs/work/completed/WORK-0001-session-lobby-foundation.md`).

## What Happened Since Slice 1 Closed

1. Slice 1 (`WORK-0001`) reached DONE on 2026-09-18 - independent review APPROVED, real-Postgres verification complete.
2. You separately requested an isolated rename, `game/game` -> `game/management` (`WORK-0002`, DONE same day) - a pure structural change with no design impact on Slice 2.
3. This session reconciled tracking (confirmed clean - see below) and reevaluated Slice 2 against the real post-Slice-1 codebase, then drafted `WORK-0003` for your review. No Slice 2 code was written.

## Reconciliation Findings (No Action Needed)

- Slice 1's WORK is DONE and correctly located under `docs/work/completed/`.
- The `session-runtime-v1` workspace (`PLAN.md`/`AI_CONTEXT.md`) already correctly reflected Slice 1 as complete before this session.
- `game/management` is the current physical path; `game/game` no longer exists. A repository-wide search found no stale current-state/live-standard/Go-source reference to the old path - the only remaining `game/game` mentions are in places explicitly meant to preserve history (two ADRs' rationale, the completed WORK-0001 file, and `PLAN.md`'s own section labeled HISTORICAL describing pre-Slice-1 state) - none of these are being presented as current.

## Proposed Slice 2 Scope (WORK-0003)

Start transitions a `LOBBY` Session to `RUNNING` by resolving the pinned Game Definition, building the `players: list<user>` root roster from active Participants, compiling and initializing the Game Language engine, and executing the first `RuntimeTurn` - draining `Commit.InternalSignals` under a hard 20-Step bound - persisting Turn 1/`session_runtime_state`/`session_runtime_steps` atomically together with `phase = RUNNING`, `started_at`, and JoinCode revocation. A deterministic initialization failure (including the Step-bound) terminalizes the Session directly `LOBBY -> TERMINAL` instead of leaving it retryable; a transient infrastructure failure leaves it unchanged and retryable.

This slice also introduces a new shared mechanism, `game/session/internal/runtimeturn` - the common RuntimeTurn executor (Step-draining + the 20-Step bound + Turn/Step/State persistence) that Slice 3 (interaction responses) and Slice 5 (timer expirations) are expected to reuse unchanged, per the initiative's approved sequencing.

Explicitly out of scope: interactions, timers, the Coordinator/WebSocket, the richer `session_runtime_failures` diagnostic entity (Slice 6), disconnect/reconnect, inactivity expiration, keyed timers, archival - none of these are needed for Start to correctly execute and persist Turn 1.

## Material Decisions Requiring Your Confirmation

None of these require new architecture - each is a narrow, direct extension of already-ACCEPTED GAME-ADRs, proposed here with a specific reading rather than silently decided, because each shapes either Slice 2's own durable schema/behavior or a pattern Slices 3/5/6 will inherit unchanged.

1. **Where the common RuntimeTurn executor lives.** Proposal: a new shared internal package, `game/session/internal/runtimeturn`, parallel to the existing `sessionlock`/`idempotency` packages (justified the same way - a genuinely shared protocol/invariant, per `repositories.md`'s Sharing Rule). Introducing it now, at Slice 2, commits Slices 3/5/6 to calling this same mechanism rather than each inlining their own Step-loop. Alternative: inline the loop into `step_start.go` now and extract a shared package only when Slice 3 actually needs it - simpler today, but risks a slightly different shape emerging under time pressure later.

2. **Fatal-failure classification split.** Proposal: a pinned-Definition recompile failure at Start (data-integrity anomaly - the Definition already compiled once at Create) is classified `RUNTIME_STATE_INVALID`; everything else in Start's initialization chain - a non-rejection execution error, the 20-Step bound, or even an outright rejection of Start's own generated initial signal - is classified `RUNTIME_EXECUTION_FAILED`. Neither GAME-ADR-0017 nor GAME-ADR-0019 spells out the rejection case explicitly; the proposed reading is that LOBBY has no notion of "an active RuntimeTurn the rejection leaves untouched" the way a later RUNNING-phase rejection does, so treating it as fatal (rather than a silent no-op that leaves the Session in LOBBY forever unstarted) seemed like the only coherent option. Flagging in case a different reading was intended.

3. **Replaying a fatal Start via idempotency.** Proposal: once a Start attempt has fatally terminalized a Session, a same-token retry replays that recorded decline (`StartOutcomeRuntimeInitFailed`) rather than re-attempting engine initialization against a Session that is now `TERMINAL` - the same replay treatment `LOBBY_FULL`/`ALREADY_JOINED` already get for Join. This seems like the natural consequence of the existing idempotency standard, but it hasn't been explicitly stated anywhere for a decline this consequential (one that ends the Session, not just declines an admission), so it's called out for confirmation.

4. **`players` root-roster ordering.** Proposal: active Participants ordered by `joined_at` ascending (first to join is `players[0]`). This can matter to gameplay (e.g., "who goes first" in an authored script indexing into `players`), and no existing product/architecture decision specifies an ordering. If you have a different intent (random order, host-first, something else), this is the moment to say so - it's cheap to change now and potentially awkward to change once games are authored assuming a particular order.

## What Happens If You Approve

`WORK-0003` moves DRAFT -> READY (only you can authorize this transition) and a future Codebase Agent session implements it following the READY specification and `docs/ai/protocols/IMPLEMENTATION_REVIEW.md`.

## Next Human Action

Review `docs/work/active/WORK-0003-session-start-first-runtimeturn.md` in full (its Approved Design section has the complete reasoning behind each item above) and either:

- approve as-is (or with adjustments) and authorize DRAFT -> READY, or
- direct changes to any of the four items above (or anything else in the WORK) before approval.
