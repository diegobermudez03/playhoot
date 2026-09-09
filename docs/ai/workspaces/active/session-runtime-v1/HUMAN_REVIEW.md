# Slice 1 — Session Lobby Foundation: DRAFT WORK Review

Process: Feature Development (Slice 1 of `session-runtime-v1`; architecture is CLOSED, the broader initiative remains governed by `PLAN.md`)

Status: awaiting your READY decision on `docs/work/active/WORK-0001-session-lobby-foundation.md`. It is currently DRAFT and has no implementation authority - nothing will be built until you approve it.

## What This Checkpoint Did

1. Recorded your approved implementation sequence into `PLAN.md` (see below).
2. Repaired three stale, contradictory sentences left over from early Architecture Discussion checkpoints in `AI_CONTEXT.md` (labeled as historical, not deleted).
3. Graduated Slice 1 into Feature Development: created `WORK-0001-session-lobby-foundation.md` as DRAFT.

No code, tests, or migrations were touched. Nothing is READY.

## Approved Sequencing (Recorded In PLAN.md)

1. Session Lobby Foundation *(this WORK)*
2. Start + First RuntimeTurn
3. Interaction Response Processing
4. **Thin Live Coordinator / WebSocket** *(moved earlier - see below)*
5. Timer Obligations
6. Failure Diagnostics + Terminal Cleanup
7. Disconnect / Reconnect + Resync
8. Inactivity Expiration / Reaper
9. Keyed Timers
10. Archival

The Coordinator/WebSocket slice moving to position 4 is intentional: after Slices 1-3, Session Runtime can create/join/start a session and answer one interaction, but nobody has proven that works from a real connected client. A thin Coordinator there validates the live path early instead of building nearly the whole backend first. It's kept deliberately narrow - no disconnect grace, no semantic-presence recovery, no timer scheduling, no inactivity reaper, no advanced reconnect - all of that is explicitly pushed to Slice 7, where it belongs.

Slice 2 now explicitly must enforce the 20-Step RuntimeTurn limit from day one (no window where Start can run unboundedly); the richer failure-diagnostics persistence layer is deferred to Slice 6, but the guard itself is not.

## WORK-0001: What You're Approving

**Scope.** A Session can be created, joined, left, and reconstructed from durable storage. No game execution, no WebSocket - just a correct, durable LOBBY. This replaces the current `game/session/...` code, which **does not currently compile** (`step_create_room.go` declares a function with no body) and contradicts the accepted design in two concrete ways: it accepts an already-compiled `engine.Program` from the caller (Session Runtime is supposed to resolve/compile it itself), and it uses raw owner/player UUID strings instead of the accepted SessionActor/Participant model.

**Identity boundary.** Per your instruction, this WORK does **not** touch Identity/Auth. Every operation just takes an already-trusted `UserUUID` as a parameter - test code and internal callers supply it directly. When a real auth/transport layer exists later (Slice 4+), it plugs in without anything here needing to change.

**Locking design.** The proposed mechanism locks the `sessions` row itself (`SELECT ... FOR UPDATE`) inside a transaction, reusing an existing repository-wide helper (`utils.RunInDBTransaction`) that's already used elsewhere in the codebase but not yet in Session Runtime. One repository method does the locking; Join, Leave, and lazy lobby-expiration all go through it - and it's shaped so Slice 2 can reuse the exact same mechanism for RUNNING serialization instead of needing a second one later.

**Idempotency.** `session_requests` stores each CREATE/JOIN/LEAVE by `(operation, idempotency_key)`. A retry with the same key and the same meaningful fields (e.g., for Join: code + user + display name) replays the stored result; a retry with the same key but different fields is rejected as conflicting. This is deliberately narrow - no generic JSON-diffing framework, just explicit per-operation field comparison.

**Game Management dependency.** Already satisfied - no change needed there. `getgame.GetPlayableGameWithCurrentVersion` already returns the decoded game definition, and `engineservice.Compile` already validates it. WORK-0001 just wires these in.

**Migration approach.** The current session tables have never held real data (Create/Join are no-op stubs), so discarding them is safe. Proposed approach: add *new* migrations that drop the old tables and create the new ones, rather than editing the two existing migration files in place (safer regardless of whether any environment has ever actually run them). This assumes no production data-preservation requirement exists yet for this schema - flag me if that assumption is wrong.

**Tests required.** Repository tests against a real disposable test database (same pattern already used for Game Management), service-level tests with mocks, and at least one concurrency test that actually proves two competing Joins for the last slot resolve correctly via the DB lock rather than by lucky timing.

## Codebase Discoveries Worth Knowing

- The `game/session` package tree currently fails to compile - this isn't a design choice being second-guessed, it's a pre-existing bug this WORK fixes as a side effect of replacing the scaffolding.
- No Coordinator/WebSocket code exists anywhere in the repository - not even a placeholder file. The directory that would most plausibly host it (`play/`) is completely empty. This doesn't affect Slice 1 but is worth knowing heading into Slice 4.
- A generic transaction helper (`utils.RunInDBTransaction`) already exists in the codebase and has never been used yet - Slice 1 becomes its first real consumer.

## Open Questions Left To Implementation (Not Architecture)

These are flagged in WORK-0001 as implementation-time choices, not blockers - listed here only so you know they exist and aren't hiding a bigger decision:

- Exact lobby-expiration TTL value (a simple constant, no existing config convention to reuse).
- Whether `players.max` is re-checked against Game Management on every Join, or snapshotted once at Create (recommendation: re-check, simpler).
- Exact package layout - recommendation is to match Game Management's `usecases/<name>` convention instead of the current `workflows/sessionlifecycle` shape.

## Next Human Action

Read `docs/work/active/WORK-0001-session-lobby-foundation.md` if you want the full detail, or approve/question based on the summary above. Say explicitly whether it's **READY** for implementation, or tell me what needs to change first. Nothing is built until then.
