# Slice 1 — Session Lobby Foundation: APPROVED, READY For Implementation

Process: Feature Development (Slice 1 of `session-runtime-v1`; architecture is CLOSED, the broader initiative remains governed by `PLAN.md`)

Status: **RESOLVED**. There is no pending checkpoint or open question. `docs/work/active/WORK-0001-session-lobby-foundation.md` is **READY** for implementation. This file is retained as the record of what was approved, until the next checkpoint (implementation completion / independent review) produces its own.

## What Was Approved

You approved WORK-0001 subject to the following implementation-design corrections, all of which have been applied to the WORK specification:

1. **Pinned Game Definition is immutable for the Session.** `sessions.game_definition_uuid` is resolved exactly once, at Create, and never re-resolved against the game's "current version" by any later operation. A later publication of a new version must never change lobby capacity, Start behavior, or any other semantics of an already-created Session.
2. **A minimum new Game Management read capability is required** - loading an immutable Game Definition by its own Definition/Version UUID (not by Game UUID + "current"). The existing `getgame.GetPlayableGameWithCurrentVersion` stays Create-only; Join (and later Start) uses the new capability instead.
3. **Idempotency identity is `(user_uuid, operation, idempotency_key)`**, not `(operation, idempotency_key)` - two different users can never collide on the same opaque key.
4. **Per-operation idempotency payload fields are spelled out explicitly** (no generic JSON-diffing): CREATE compares GameUUID (+ the host identity already implied); JOIN compares JoinCode + UserUUID + display name; LEAVE compares SessionUUID + UserUUID.
5. **Concurrent Create is now explicitly correct**: the WORK requires claiming the idempotency identity (inserting the `session_requests` row) before any Session-creation effect happens, so two concurrent Creates with the same identity can never both create separate Sessions.
6. **Transient failures can't become permanent cached results**: a new `status` field on `session_requests` and explicit wording make clear that a rollback or infrastructure failure before commit leaves nothing behind to replay - a retry just runs fresh.
7. **The Session/HostActor circular-reference insertion is fully specified**: insert Session with `host_actor_id = NULL` → insert host actor → update Session's `host_actor_id` → commit, all in one transaction, with a strict invariant that no successfully committed Session is ever missing its host actor.
8. **New/updated acceptance criteria** cover all of the above: pinned-definition enforcement across a version change, cross-domain read correctness (pinned vs. "current"), idempotency-namespace non-collision across users, concurrent-Create producing exactly one Session, conflicting-payload rejection, and transient-failure non-replayability.

Everything else from the original DRAFT stands unchanged: the `sessions`-row DB lock with `FOR UPDATE`, reuse of the existing `utils.RunInDBTransaction` helper, a locking primitive shaped for reuse in Slice 2's RUNNING serialization, trusted `UserUUID` input with no Identity/Auth implementation, no externally-supplied `engine.Program`, Create/Join/Leave only (no Start, no RuntimeTurn, no WebSocket, no disconnect/reconnect, no inactivity expiration, no archival), and the pre-launch destructive-migration approach.

## No Remaining Review Questions

There is nothing further to approve. `WORK-0001-session-lobby-foundation.md` is READY. The next step is implementation - a Codebase Agent session following `docs/ai/protocols/IMPLEMENTATION_REVIEW.md` against the READY WORK. No code, tests, or migrations have been written yet.

**Historical note (added 2026-09-11, does not change the approval recorded above)**: this checkpoint predates implementation. A first implementation pass has since happened (see `AI_CONTEXT.md`'s 2026-09-08 and 2026-09-11 checkpoints and `docs/work/active/WORK-0001-session-lobby-foundation.md`'s Implementation Status Note) - WORK-0001 is now IMPLEMENTING, not READY-with-nothing-written. This file remains the historical record of the READY approval decision itself and is retained per this workspace's rules until the next human-facing checkpoint (independent review / closure) produces its own.

## Also Synchronized This Checkpoint

- `PLAN.md` - Slice 1 now shows WORK-0001 as READY; the approved 10-slice sequence (Lobby Foundation → Start+First RuntimeTurn → Interaction Response Processing → Thin Live Coordinator/WebSocket → Timer Obligations → Failure Diagnostics+Terminal Cleanup → Disconnect/Reconnect+Resync → Inactivity Expiration/Reaper → Keyed Timers → Archival) is unchanged and preserved, including Slice 2's mandatory immediate 20-Step enforcement and Slice 4's intentionally thin scope.
- `AI_CONTEXT.md` - resume header and body updated to reflect READY status; no stale "awaiting review"/"DRAFT" language remains for WORK-0001.
