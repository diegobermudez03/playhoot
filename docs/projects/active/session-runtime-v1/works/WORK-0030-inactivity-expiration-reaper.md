# WORK-0030: Inactivity Expiration Reaper

Status: DRAFT
Created: 2026-09-26
Last status change: 2026-09-26 (created directly as DRAFT, split out of WORK-0016 - see that WORK's own "Scope Correction (Reaper Split, 2026-09-26)" section for the full reasoning. Approved Design filled in against the actual current implementation; not yet READY - see "Blockers" below.)

Related decisions:
- GAME-ADR-0014 (inactivity/Reaper semantics - Reaper role, separate from Archive Worker)
- GAME-ADR-0013 (Session Runtime is process-agnostic; no process-ownership lease/heartbeat mechanism)
- GAME-ADR-0019 (terminal cleanup - reused, not reinvented, by this WORK)

Canonical context:
- `docs/projects/active/session-runtime-v1/works/WORK-0016-inactivity-expiration-reaper.md` (DONE) - implements `sessions.activity_expires_at` schema, renewal, and lazy materialization (`internal/activity.MaterializeIfDue`), which this WORK reuses as a plain function call rather than duplicating the check. This WORK is the "Reaper" half that WORK-0016 split out.
- `game/README.md`'s "Session Runtime Durable Inactivity Expiration" section - the Reaper's accepted role: proactive discovery only, never the source of expiration correctness.
- `main.go` - the repository's single process entrypoint (`http.ListenAndServe`, nothing else) - this WORK introduces the first background/scheduled process this codebase has ever had.

## Outcome

A Session whose `activity_expires_at` deadline has already passed, but that no later RUNNING-phase operation ever revisits (an abandoned Session with no more player activity), is still proactively discovered and terminalized within a bounded time, instead of remaining durably `RUNNING` forever until some hypothetical future caller happens to reach it.

Today (post-WORK-0016), expiration is *correct* the moment any RUNNING-phase operation reaches an already-expired Session, but nothing proactively visits Sessions nobody ever calls again.

## Context

WORK-0016 deliberately made this WORK's own correctness non-dependent on it: "the deadline, not Reaper scheduling latency, decides the outcome" (GAME-ADR-0014). This WORK is therefore purely an availability/hygiene improvement - closing `RUNNING` rows that would otherwise dangle indefinitely, freeing whatever resources depend on `phase = RUNNING` being an accurate signal (future abuse/resource accounting, operator dashboards, GAME-ADR-0009's eventual Archive Worker input).

This is also this repository's first background/scheduled process of any kind. No cron, ticker, or worker-process entrypoint exists anywhere in `game/`, `sessionhistory/`, `main.go`, or elsewhere - the concrete mechanism is a genuine infrastructure/deployment-topology decision, not a local implementation choice, and is recorded as a Blocker below rather than invented.

## Scope

### In Scope

- A `Manager`-level Reaper step reusing `internal/activity.MaterializeIfDue` per candidate Session, under the same per-Session locking/serialization every other step already uses.
- A repository query finding candidate Sessions (`phase = RUNNING AND activity_expires_at <= now()`).
- A scheduling mechanism that periodically invokes the above (exact shape gated by Blocker 1 below).
- Re-validation under lock before materializing (a legitimate operation may have already renewed the deadline between the candidate query and the Reaper's own attempt - the Reaper must recheck, not trust its own stale read).

### Out of Scope

- Anything about `activity_expires_at` schema, renewal, or lazy materialization itself - already DONE (WORK-0016).
- Any change to `internal/activity.MaterializeIfDue`'s own logic - reused unchanged.
- Client-visible termination notification - the same not-yet-drafted play-half WORK following WORK-0020 that every other terminal cause in this Project already defers to.
- Multi-instance coordination/distributed leader election for the Reaper - GAME-ADR-0013 already rejects process-ownership/heartbeat mechanisms for Session Runtime generally, and V1 assumes a single running instance (see Blocker 1's own options).
- The Archive Worker (GAME-ADR-0009) - a separate, unrelated background concern with its own not-yet-drafted WORK; this WORK's Reaper must not be generalized into a shared "background job runner" for both concerns.

## Approved Design

### Design Basis (2026-09-26)

- **`internal/activity.MaterializeIfDue(ctx, tx, store, lockedSession, now) (bool, error)`** (`game/session/workflows/sessionlifecycle/internal/activity/activity.go`) already implements the exact materialization this WORK needs to invoke - it takes an already-locked `*sessionlock.Session` and, if due, atomically terminalizes plus performs GAME-ADR-0019 terminal cleanup. This WORK's own Reaper step does not reimplement any of that logic - it only needs to (1) find candidate Session IDs, (2) for each, lock it as every other step does (`sessionlock.LockByID`), and (3) call `activity.MaterializeIfDue`.
- **No candidate-listing query exists yet.** Every existing repository method reads a Session by known id/uuid; nothing today lists Sessions by `(phase, activity_expires_at)`. A new narrow repository method is required (e.g. `ListInactiveRunningSessionIDs(ctx, tx, now, limit) ([]uint, error)`), returning bounded batches, not an unbounded scan.
- **No background job/scheduler mechanism exists anywhere in this repository.** `main.go` is the sole process entrypoint - one `http.ListenAndServe` call, nothing else. Introducing a periodic sweep is this codebase's first scheduled/background process ever, which is why the concrete mechanism is a Blocker, not assumed here.
- **GAME-ADR-0013's already-accepted process-agnostic model applies directly**: the Reaper must not become a process-ownership/leader-election mechanism. Under a single running instance (V1's own current deployment reality, per every prior WORK in this Project), a straightforward periodic sweep is sufficient; if a second concurrent Reaper instance ever ran, both would attempt the same candidate Sessions, but per-Session row locking plus the same re-validate-under-lock rule `internal/activity.MaterializeIfDue` already enforces makes a redundant concurrent attempt a harmless no-op (the second one sees the Session already `TERMINAL` and does nothing), not a correctness hazard - this WORK does not need to invent coordination to be correct under accidental duplication, only under deliberate horizontal scaling, which V1 does not have.

### Reaper Step (`Manager` method)

A new `sessionlifecycle.Manager` method, conceptually `ReapInactiveSessions(ctx context.Context, now time.Time, limit int) (reaped int, err error)`:

1. Call the new repository method to list up to `limit` candidate Session ids (`phase = RUNNING AND activity_expires_at <= now`), outside any single Session's own transaction (a plain read).
2. For each candidate id, in its own transaction (`utils.RunInDBTransaction`): `sessionlock.LockByID`, then `activity.MaterializeIfDue(ctx, tx, repo, lockedSession, now)`. A candidate that no longer qualifies under lock (already renewed, already terminal for another reason) is silently skipped - the candidate list is a hint, not an authoritative claim.
3. Return how many Sessions were actually materialized this pass (diagnostic/observability value only - not otherwise consumed).

Each candidate's own transaction failing must not abort the batch - log/alert and continue to the next candidate, the same "one bad row does not block everything else" principle already implicit in per-Session serialization elsewhere in this workflow.

### Scheduling Mechanism

**Gated by Blocker 1.** Whichever option is chosen invokes `Manager.ReapInactiveSessions` on a fixed interval (Blocker 2); the invocation itself is a thin wrapper regardless of which scheduling mechanism triggers it.

## Constraints and Invariants

- Must reuse `internal/activity.MaterializeIfDue` unchanged - no reimplementation of the materialization/terminal-cleanup logic.
- Must re-validate under the same per-Session lock before materializing - a candidate list read is never itself sufficient grounds to terminalize.
- Must not introduce a process-ownership lease, heartbeat, or takeover-fencing mechanism (GAME-ADR-0013).
- Must not block/serialize with ordinary Session mutations any longer than one Session's own already-existing lock scope - the Reaper must not hold any lock across multiple Sessions at once.
- Must not persist a `session_runtime_failures` row or fabricate a RuntimeTurn for a Reaper-materialized expiration, exactly as WORK-0016 already established for the lazy-materialization path.

## Acceptance Criteria

1. A `RUNNING` Session whose `activity_expires_at` has already passed, with no later operation ever reaching it, is discovered and terminalized (`phase = TERMINAL`, `terminal_reason = RUNTIME_INACTIVITY_EXPIRED`, `terminal_at = activity_expires_at`) within one scheduling interval, proven against real Postgres by seeding an already-expired `RUNNING` Session and invoking the Reaper step directly (not necessarily waiting on the real scheduler for the test).
2. A candidate Session whose deadline was renewed by a legitimate concurrent operation between the candidate query and the Reaper's own lock acquisition is left untouched (no incorrect termination) - proven by a concurrency-style test analogous to GAME-ADR-0014's own "operation wins the race, Reaper rechecks and does nothing" example.
3. A Session already `TERMINAL` for an unrelated reason, or still genuinely active (deadline not yet passed), is never touched by a Reaper pass.
4. The GAME-ADR-0019 terminal-cleanup invariant (no `ACTIVE` interaction/timer obligation survives) holds for every Reaper-materialized termination, identically to the lazy-materialization path WORK-0016 already proved.
5. No `session_runtime_failures` row is created by a Reaper-driven termination.
6. A batch containing one Session whose own transaction fails for an unrelated reason (simulated) does not prevent other candidates in the same pass from being correctly reaped.
7. `go build ./...`, `go vet ./...`, `go test ./... -count=1` pass (real Postgres), with no new failure beyond the already-recorded out-of-scope `getgame` JSONB-comparison defect.

## Blockers

Both require explicit human confirmation before READY - neither is a local implementation choice:

1. **Scheduling mechanism.** This is this repository's first background/scheduled process; no existing pattern to follow. Options:
   - **(a) In-process goroutine + `time.Ticker`, started from `main.go` alongside the existing HTTP server** (Recommended). Simplest possible V1 shape; no new deployment artifact, no new infra config, no new auth boundary to design. Consistent with GAME-ADR-0013's single-instance V1 reality and this WORK's own "redundant concurrent attempt is a harmless no-op" analysis above. Downside: the Reaper only runs while the HTTP process is alive; if that process is ever intentionally scaled to multiple replicas, every replica runs its own redundant sweep (harmless per the analysis above, but wasted work) - worth revisiting if/when horizontal scaling actually happens (SOON/LATER, not now).
   - **(b) An internal/admin HTTP endpoint triggered by an external scheduler** (e.g. a Kubernetes CronJob or cloud scheduler hitting `POST /internal/reap`). Decouples reaping cadence from the HTTP process's own lifecycle and would naturally dedupe across replicas (only the external scheduler's own trigger runs it), but requires designing an internal-only auth boundary this codebase does not have yet, and depends on a deployment topology (this session cannot inspect current production infra) this repository's own code does not describe.
   - **(c) A separate deployable worker binary** (a second `cmd/`-style entrypoint). Cleanest separation of concerns long-term, but this repository has exactly one binary today (root `main.go`) and no established multi-binary convention - introducing one is a larger infrastructure change than this WORK's own outcome (a hygiene sweep) justifies for V1.
2. **Polling interval and batch size.** Recommended: a 1-minute interval, 100-Session batch limit per pass - conservative relative to the 10-minute `activityTTL` (many chances to catch an expired Session well within an operator-visible timeframe), and small enough that a single pass's worth of per-Session transactions cannot meaningfully compete with foreground traffic. Both would be code constants (mirroring `defaultLobbyTTL`/`defaultActivityTTL`'s own precedent), not externally configurable in V1.

Local implementation choices (not blockers): exact new repository method name/signature beyond the shape described above, exact `Manager` method name, exact file/package placement for the scheduling wrapper (e.g. a small `game/session/reaper` package `main.go` imports, vs. inlining the ticker loop directly in `main.go`) - Implementation Freedom.

## Documentation Impact

- `game/CURRENT_STATE.md` - Session Runtime Current Gaps: the background Reaper moves from "not implemented" to implemented, once done.
- `game/README.md` - Durable Inactivity Expiration section's "Status:" line (added by WORK-0016) updated to also note the Reaper is now implemented.
- `GAME-ADR-0014` - "Implemented by" addendum extended to name this WORK for the Reaper role specifically.

## Completion Record

Not started. DRAFT - see "Blockers" above for what must be resolved before READY.
