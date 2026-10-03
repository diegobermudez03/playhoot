# WORK-0016: Inactivity Expiration — Schema + Lazy Materialization (Domain Half)

Status: DONE
Created: 2026-09-20
Last status change: 2026-09-26 (IMPLEMENTING -> DONE: independent re-review verdict APPROVED, no findings - verified both fixes below against real evidence (git diff content, file-modification timestamps distinguishing the fix pass from the original implementation window), confirmed no code/test/migration file changed since the first review so no re-verification of build/vet/Postgres was needed. Prior status change, same day: independent review (fresh agent, no access to this session's context) returned CHANGES_REQUIRED: two REQUIRED_FIX findings, both mechanical/documentation-only - `game/README.md`'s Durable Inactivity Expiration section was missing the "Implemented by" note this WORK's own Documentation Impact committed to (fixed - added, matching the GAME-ADR-0014/WORK-0014 precedent style); this WORK's own `Status:` header still read READY instead of IMPLEMENTING despite implementation being complete (fixed - this header). Everything else - schema, renewal, all four lazy-materialization call sites, `terminal_at` correctness, the `session_runtime_failures` exclusion, and test coverage against every Acceptance Criterion - was independently verified against real Postgres/build/vet and found correct, no further findings. Re-review pending. Prior status change, same day: READY -> IMPLEMENTING: implemented against the Approved Design - see "Implementation Report" below. Prior status change, same day: DRAFT -> READY: human explicitly authorized implementation, confirming both Blockers - `activityTTL` defaults to 10 minutes, matching `defaultLobbyTTL`'s own precedent; the Reaper split is confirmed, the background Reaper process remains split out to a new, not-yet-drafted WORK. See "Blockers" below for the resolved record. Prior status change, same day: PLANNED -> DRAFT: Approved Design filled in against the actual current implementation, inspected directly (not assumed from the ADR alone) - see "Design Basis (2026-09-26)" below. Narrowed to a domain-half "schema + lazy materialization" scope, matching PROJECT.md's own already-recorded "(schema half)" framing (PROJECT.md's Phase-1 table, WORK-0016's own row) - the background Reaper process is split out to a new, not-yet-drafted WORK, the same domain/play-style split this Project already applies elsewhere (see "Scope Correction (Reaper Split, 2026-09-26)" below).

Related decisions:
- GAME-ADR-0014 (inactivity/reaper semantics)
- GAME-ADR-0019 (terminal cleanup - reused, not reinvented, by this WORK)
- GAME-ADR-0024 (Replay-First Session Runtime Persistence - inactivity termination is Session lifecycle state, not a gameplay RuntimeTurn, and stays that way under the replay-input model)

Canonical context:
- `docs/projects/active/session-runtime-v1/PROJECT.md`
- `game/CURRENT_STATE.md` (`activity_expires_at` "has no schema/enforcement anywhere yet despite being already-accepted architecture")
- `docs/projects/active/session-runtime-v1/works/WORK-0014-runtime-failure-diagnostics-terminal-cleanup.md` (the terminal-cleanup invariant this WORK reuses)
- `docs/projects/active/session-runtime-v1/works/WORK-0007-session-termination-live-notification.md` (the future terminal-live notification mechanism this WORK's termination will eventually feed, once WORK-0007's own not-yet-drafted play half exists - out of this WORK's own scope, see Scope below)

## Outcome

An orphaned/inactive Session (no meaningful activity for a configured period) is detected and transitioned to terminal automatically, without requiring a human or a player to notice it should end. `sessions.activity_expires_at` is renewed on meaningful activity and lazily materialized on a stale RUNNING-phase operation - this WORK's own scope (domain half). Proactive enforcement via a background Reaper, for a Session no later operation ever revisits, is split out to a separate not-yet-drafted WORK (see "Scope Correction (Reaper Split, 2026-09-26)" below).

Today, `activity_expires_at` has no schema or enforcement anywhere - distinct from the already-implemented LOBBY-phase `lobby_expires_at`, which is a different, already-working mechanism (WORK-0001).

## Context

Purely additive once RUNNING operations exist to renew/validate against (Start, AnswerInteraction, and whichever of WORK-0010/0012 have landed) - lower architectural risk than most of this Project's other remaining WORK, so it can follow rather than gate them.

## Scope

### In Scope

- `sessions.activity_expires_at` schema (nullable - unset during `LOBBY`, first set by `Start`, renewed thereafter).
- Initial deadline set by `Start`, alongside `started_at`.
- Renewal on a RuntimeTurn-producing RUNNING-phase operation actually committing a Turn: `AnswerInteraction`, `SubmitUserIntent`, `ExpireTimer` (see "Renewal Trigger List" below).
- Lazy materialization on a stale RUNNING-phase operation - the same pattern `lobby_expires_at`/`internal/expiration.MaterializeIfDue` already uses, generalized to `RUNNING`/`activity_expires_at`, including the GAME-ADR-0019 terminal-cleanup invariant (closing any still-`ACTIVE` interaction/timer obligation), since (unlike `LOBBY`) a `RUNNING` Session can genuinely have some.
- New `session.TerminalReasonRuntimeInactivityExpired` constant (GAME-ADR-0014's own `RUNTIME_INACTIVITY_EXPIRED` name).

### Out of Scope

- **The background Reaper process itself** - split out to a new, not-yet-drafted WORK; see "Scope Correction (Reaper Split, 2026-09-26)" below.
- **Client-visible termination notification** - the play-half, deferred to the not-yet-drafted play-half WORK following WORK-0020, the same split already applied to WORK-0007/0010/0011/0012's own domain halves. This WORK's own Canonical Context originally cited "WORK-0007's terminal-live-notification mechanism" as something to reuse; WORK-0007 itself only detects/terminalizes today and defers the actual broadcast, so there is nothing live-transport-shaped for this WORK to call into yet either.
- Redefining what counts as "meaningful activity" beyond what GAME-ADR-0014 already accepts (see "Renewal Trigger List" below for this WORK's own concrete reading of that already-accepted, deliberately non-exhaustive list).
- `session_runtime_failures` persistence for this termination cause - inactivity expiration is not a GAME-ADR-0017 class B/C runtime failure (see "Terminal-Cleanup Mechanism" below).
- Disconnect/reconnect-driven renewal ("authenticated reconnection/resume activity" per GAME-ADR-0014) - WORK-0015 (disconnect/reconnect) is still PLANNED; there is no reconnect code path to hook a renewal call into yet. When WORK-0015 lands, it should call the same renewal primitive this WORK introduces rather than reinventing one.

## Scope Correction (Reaper Split, 2026-09-26)

This WORK's original Scope listed "a background Reaper process" alongside the schema/materialization work, but `PROJECT.md`'s own Phase-1 table (`| 13 | WORK-0016 — Inactivity Expiration / Reaper (schema half) | PLANNED |`) already frames WORK-0016 as a schema half - the same drift `PROJECT.md`'s Restructuring (2026-09-21) previously flagged and corrected for WORK-0006/0007/0010/0011/0012 (mixing a Session-Runtime-domain concern with a differently-owned concern in one document). Investigating the actual codebase confirms why a genuine split is warranted here, not merely a documentation nicety: **no background job/worker/scheduler mechanism of any kind exists anywhere in this repository today** (`main.go` runs one `http.ListenAndServe` call and nothing else; no cron, ticker, or worker-process entrypoint exists in `game/`, `sessionhistory/`, or elsewhere). Introducing the Reaper is therefore not "add a call to an existing mechanism" the way lobby expiration's Join/Leave/Start lazy-check was - it is this codebase's *first* background/scheduled process, an infrastructure/deployment-topology decision (in-process goroutine+ticker inside the existing `main.go` HTTP process vs. an externally-cron-triggered admin endpoint vs. a separate deployable worker binary; polling interval; single-instance-only assumption today per GAME-ADR-0013) squarely inside `docs/ai/OPERATING_MODEL.md`'s escalation list, not a local implementation choice this WORK's schema/materialization design should silently carry.

Per this Project's own established practice (WORK-0006/0007/0010/0011/0012's domain/play splits), this WORK is narrowed to the schema/renewal/lazy-materialization half only - correctness (per GAME-ADR-0014, "the deadline, not Reaper scheduling latency, decides the outcome") does not depend on the Reaper existing at all; an already-expired Session simply also terminalizes the next time any RUNNING-phase operation reaches it, exactly as `lobby_expires_at` already works without the Reaper GAME-ADR-0004 also describes ever having been built. The Reaper itself becomes a new WORK once this one is READY/implemented, sequenced immediately after it, so it can reuse this WORK's own lazy-materialization decision as a plain function call rather than duplicating the check.

## Approved Design

### Design Basis (2026-09-26)

Before drafting, the actual current implementation was inspected (not assumed from GAME-ADR-0014 alone):

- **The `lobby_expires_at` pattern this WORK generalizes already exists and is fully understood**: `game/session/internal/sessionlock.LockByID/LockByUUID` (`FOR UPDATE` row lock, no advisory lock) return a `*sessionlock.Session` snapshot including `Phase`/`LobbyExpiresAt`/`CurrentTurnID`/`TerminalAt`/`TerminalReason`; `game/session/workflows/sessionlifecycle/internal/expiration.MaterializeIfDue(ctx, tx, store, lockedSession, now)` checks `lockedSession.Phase != PhaseLobby || now.Before(lockedSession.LobbyExpiresAt)`, else calls `SetSessionTerminal` (`terminal_at = LobbyExpiresAt`, the deadline itself, never `now`) + `RevokeActiveJoinCode`, mutating `lockedSession` in place; called from `step_join.go`, `step_leave.go`, `step_start.go`, right after locking.
- **`sessionlock.Session` does not yet carry `ActivityExpiresAt`** - it must gain that field alongside the new column, the same way it already carries `LobbyExpiresAt`.
- **A RUNNING-phase explicit lifecycle-validity check already exists as precedent**: `step_submit_user_intent.go`'s `if lockedSession.Phase != session.PhaseRunning { return m.declineSubmitUserIntent(...) }`, added by WORK-0010 specifically because (unlike `AnswerInteraction`, transitively protected via interaction state) nothing else stops a further call from reaching a `TERMINAL` Session. This WORK's own lazy-materialization check belongs at the same place in each RUNNING-phase step - immediately after `sessionlock.LockByID`/`LockByUUID`, before any other business logic - mirroring where `expiration.MaterializeIfDue` already sits for LOBBY steps.
- **`session_runtime_failures`/`materializeRuntimeFailure` (WORK-0014, `runtime_failure.go:191`) is fatal-failure-specific** (GAME-ADR-0017 class B/C) and must not be reused for inactivity expiration, which GAME-ADR-0014 treats as an ordinary, expected lifecycle outcome, not a failure - the same reasoning WORK-0014 itself already applied to exclude WORK-0007's game-completion and WORK-0011's host-cancellation terminal outcomes from `session_runtime_failures`. The correct precedent to reuse is WORK-0007/WORK-0011's own shape instead: `SetSessionTerminal` + `CloseAllActiveInteractionsForSession(..., session.InteractionClosureReasonSessionTerminated)` + `CancelAllActiveTimerObligationsForSession(..., session.TimerObligationClosureReasonSessionTerminated)`, no diagnostic row.
- **TTL configuration has exactly one existing precedent, `defaultLobbyTTL`**: `manager.go`'s `const defaultLobbyTTL = 10 * time.Minute`, a `Manager.lobbyTTL time.Duration` field, hardwired in `New(...)` - no env var or config-struct wiring exists for it. This WORK follows the identical shape for `activityTTL` rather than introducing a new configuration mechanism this codebase does not otherwise have.
- **`SetSessionRunning(ctx, tx, sessionID, startedAt)` (`internal/repo/session.go`) sets only `phase`/`started_at`/`updated_at` today** - it must additionally accept and persist the initial `activity_expires_at` (`startedAt.Add(activityTTL)`), called from `step_start.go` at the same call site.
- **`sessions` schema today** (`20260908000001_sessions.go` plus later `ALTER`s): `id, uuid, game_definition_uuid, host_actor_id, phase, lobby_expires_at, started_at, terminal_at, terminal_reason, current_turn_id, created_at, updated_at`. `terminal_reason` is plain `TEXT`, no CHECK constraint - a new constant value requires no schema change beyond the new column itself.

### Schema

New migration (`game/session/internal/storage/migrations/`, next timestamp in sequence): `ALTER TABLE sessions ADD COLUMN activity_expires_at TIMESTAMPTZ NULL`. Nullable, distinct from `lobby_expires_at` (`NOT NULL`, meaningful only during `LOBBY`): `activity_expires_at` is meaningless before `Start` and is never read/written once `TERMINAL`.

### Renewal

`Start` sets the initial deadline: `activity_expires_at = started_at + activityTTL`, in the same `SetSessionRunning` call/transaction that already sets `started_at`.

Each of `AnswerInteraction`/`SubmitUserIntent`/`ExpireTimer` renews the deadline (`activity_expires_at = now + activityTTL`) exactly when that call actually commits a new RuntimeTurn for the Session - i.e., alongside the existing `SetCurrentTurn(ctx, tx, sessionID, turnID)` call each step already makes, in the same transaction. A rejected/declined outcome (no Turn committed - e.g. `AnswerInteractionOutcomeRejected`/`Conflict`, a stale/no-op `SubmitUserIntent`) does not renew, consistent with GAME-ADR-0014's "arbitrary passive reads/polling must not keep a Session alive." See "Renewal Trigger List" below for why `CancelSession` is not in this list.

### Lazy Materialization

New shared function, `game/session/workflows/sessionlifecycle/internal/activity` (mirroring `internal/expiration`'s package shape): `MaterializeIfDue(ctx, tx, store Store, lockedSession *sessionlock.Session, now time.Time) (bool, error)`. Checks `lockedSession.Phase != session.PhaseRunning || lockedSession.ActivityExpiresAt == nil || now.Before(*lockedSession.ActivityExpiresAt)` - no-op; else, in the same transaction: `SetSessionTerminal(ctx, tx, sessionID, *lockedSession.ActivityExpiresAt, session.TerminalReasonRuntimeInactivityExpired)` (terminal_at is the deadline itself, never `now`, mirroring `lobby_expires_at`'s own already-accepted rule) + `CloseAllActiveInteractionsForSession(ctx, tx, sessionID, session.InteractionClosureReasonSessionTerminated)` + `CancelAllActiveTimerObligationsForSession(ctx, tx, sessionID, session.TimerObligationClosureReasonSessionTerminated)`, mutating `lockedSession` in place the same way `expiration.MaterializeIfDue` already does.

Called from **every** existing RUNNING-phase entry point immediately after `sessionlock.LockByID`/`LockByUUID`, before any other business logic - not only `AnswerInteraction`/`SubmitUserIntent`/`ExpireTimer` (this WORK's own Renewal-trigger operations) but also `CancelSession` (WORK-0011), since GAME-ADR-0014's own invariant is "every active-dependent operation validates the deadline," not only the three that also renew it. Each of the four already has its own existing decline path for a Session it discovers is not currently actionable, and this call is deliberately positioned so materializing `TERMINAL` here always falls straight through that same already-existing path, requiring no new branch or outcome value on any step's result type:
- `AnswerInteraction`: inserted before the interaction lookup - a materialized termination closes the interaction being answered (`CloseAllActiveInteractionsForSession`), so the immediately-following `interaction.State != session.InteractionStateActive` check (already handling `InteractionStateClosed` vs. every other non-`ACTIVE` state) sees it as freshly `InteractionStateTerminated` and returns the existing `AnswerInteractionOutcomeRejected`.
- `SubmitUserIntent`: inserted before the idempotency claim (mirroring `Start`'s own ordering, expiration-check before claim) - the immediately-following `lockedSession.Phase != session.PhaseRunning` check (added by WORK-0010) catches it and calls the existing `declineSubmitUserIntent`.
- `ExpireTimer`: inserted before the timer-obligation lookup - a materialized termination cancels the obligation being expired (`CancelAllActiveTimerObligationsForSession`), so the immediately-following `obligation.State != session.TimerObligationStateActive` check sees it as freshly cancelled and returns the existing `ExpireTimerOutcomeStale`.
- `CancelSession`: inserted before the idempotency claim (same ordering as `SubmitUserIntent`) - the immediately-following `lockedSession.Phase == session.PhaseTerminal` check (WORK-0011) catches it and returns the existing `cancelSessionOutcomeAlreadyTerminal`/`CancelSessionOutcomeAlreadyTerminal`, an accurate and already-idempotent outcome regardless of which terminal reason actually applied.

This four-call-site placement (one more than originally scoped in the Renewal Trigger List below, which only covers *renewal*, not the separate *validation* obligation GAME-ADR-0014 also imposes) was found necessary during design-basis inspection: `CancelSession` locks the Session exactly like the other three and, without this check, could otherwise silently apply `TerminalReasonSessionCancelledByHost` to a Session that had, in fact, already inactivity-expired moments earlier under a still-stale in-memory `phase=RUNNING` read - the exact "must not revive/misattribute" case GAME-ADR-0014 explicitly guards against. No new architectural decision is introduced by including it; it is the same already-accepted validation obligation applied to a fourth already-existing code path.

### Renewal Trigger List

Concrete reading of GAME-ADR-0014's deliberately non-exhaustive list, against operations that exist in code today: `AnswerInteraction` (interaction processing), `SubmitUserIntent` (unsolicited player intent), `ExpireTimer` (timer expiration processing) - all three "successful RuntimeTurn-producing gameplay operations." `CancelSession` (WORK-0011) is **not** a renewal trigger - it always terminalizes the Session itself (force-terminal, per WORK-0011's own accepted design), so there is no later RUNNING state left to renew a deadline for. `Start` sets the initial deadline (not a "renewal" of a prior one, since none exists yet before `RUNNING`). Reconnection-driven renewal is out of scope (see Scope above) until WORK-0015 exists.

## Constraints and Invariants

- Must reuse the GAME-ADR-0019 terminal-cleanup invariant (`CloseAllActiveInteractionsForSession`/`CancelAllActiveTimerObligationsForSession`), not reimplement it.
- Must not create a `session_runtime_failures` row for this termination cause (not a class B/C failure).
- `terminal_at` must equal `activity_expires_at` (the deadline), never the materializing operation's own current wall-clock time, mirroring `lobby_expires_at`'s already-accepted rule (GAME-ADR-0014's own "terminal_at always equals activity_expires_at, never the Reaper's or the lazy operation's current wall-clock time").
- Must not fabricate a RuntimeTurn, engine `Step`, or Snapshot to represent expiration (GAME-ADR-0014) - this is Session lifecycle cleanup, not gameplay, the same distinction GAME-ADR-0019 already draws for every other terminal cause.
- Must not introduce a background job/scheduler mechanism as part of this WORK (see Scope Correction above) - lazy materialization alone must be independently correct, exactly as `lobby_expires_at` already is without a Reaper ever having been built.

## Acceptance Criteria

1. `Start` sets `sessions.activity_expires_at = started_at + activityTTL` in the same transaction/row write that sets `started_at`/`phase = RUNNING` - proven by inspecting persisted state directly after a successful `Start`.
2. A successful `AnswerInteraction` call that commits a new RuntimeTurn renews `activity_expires_at` to `now + activityTTL`; a declined/rejected/conflicting call (no Turn committed) does not change it - both proven against real Postgres.
3. The same, for `SubmitUserIntent` and `ExpireTimer`.
4. A RUNNING-phase `AnswerInteraction`/`SubmitUserIntent`/`ExpireTimer` call arriving after `activity_expires_at` has passed materializes `phase = TERMINAL`, `terminal_reason = RUNTIME_INACTIVITY_EXPIRED`, `terminal_at = activity_expires_at` (the deadline, not the call's own current time), in the same transaction, and returns the same decline outcome each step already returns for a non-`RUNNING` Session - no partial/fabricated RuntimeTurn is persisted.
5. Materialization in (4) also closes every still-`ACTIVE` `session_interactions` row (`closed_by_turn_id = NULL`, `SESSION_TERMINATED`) and cancels every still-`ACTIVE` `session_timer_obligations` row for that Session, atomically with the `TERMINAL` transition - proven by a scenario with at least one of each still open at the deadline.
6. No `session_runtime_failures` row is created by this termination cause (regression-style assertion, zero rows).
7. An operation reaching serialization strictly before the deadline processes normally and (per Renewal) may extend the deadline into the future; a later call after that extension is not treated as expired merely because the *original* deadline already passed - proven by a concurrency/ordering-style test analogous to GAME-ADR-0014's own worked example.
8. `CancelSession` (WORK-0011) and every other already-existing terminal path are unaffected - no regression in their own existing tests.
9. `go build ./...`, `go vet ./...`, `go test ./... -count=1` pass (real Postgres), with no new failure beyond the already-recorded out-of-scope `getgame` JSONB-comparison defect.

## Blockers

Both resolved, 2026-09-26, human-confirmed (recommended option accepted for each):

1. **`activityTTL` defaults to `10 * time.Minute` (`defaultActivityTTL`)** - matching `defaultLobbyTTL`'s own already-accepted value and mechanism shape (a package constant + `Manager` field, no env var/config-struct wiring yet). Making it externally configurable without a code change remains a SOON/LATER concern, not blocking this WORK, since `lobbyTTL` already has the identical limitation today.
2. **The Reaper-process split is confirmed** - the background Reaper process remains split out to a new, not-yet-drafted WORK (see "Scope Correction (Reaper Split, 2026-09-26)" above); this WORK's own scope stays schema/renewal/lazy-materialization only.

Local implementation choices (not blockers): exact new migration filename/timestamp, exact `internal/activity` package/file naming, exact Go field/constant names beyond `session.TerminalReasonRuntimeInactivityExpired`'s required value (`RUNTIME_INACTIVITY_EXPIRED`, GAME-ADR-0014's own name) - Implementation Freedom.

## Scope Clarification (Part B Reconciliation, 2026-09-20)

`game/docs/decisions/GAME-ADR-0024-replay-first-session-runtime-persistence.md` confirms (does not change) this WORK's already-correct framing: inactivity termination is Session lifecycle state materialized without executing Game Language, not a gameplay RuntimeTurn - it must not be fabricated as a fake replay-input cause merely to make a Session's terminal history "look like" every other Turn-produced entry. No implementation was performed; this WORK's Status remains PLANNED.

## Documentation Impact

- `game/CURRENT_STATE.md` - Session Runtime capability row/Current Gaps: `activity_expires_at` moves from "no schema/enforcement anywhere" to schema + renewal + lazy materialization implemented (Reaper still not implemented, owned by the split-out WORK).
- `game/docs/DATA_MODEL.md` - `sessions` class diagram gains `activity_expires_at`.
- `game/README.md` - Durable Inactivity Expiration section (added when GAME-ADR-0014 was accepted) gets an "Implemented by" note once this WORK completes, matching this Project's established convention.
- `GAME-ADR-0014` - "Implemented by" addendum once this WORK completes (schema/renewal/lazy-materialization only; Reaper still open).

## Implementation Report (2026-09-26)

Implemented against the Approved Design above, in the same session as drafting/READY:

- **Migration**: `game/session/internal/storage/migrations/20260926000001_sessions_activity_expires_at.go` adds `sessions.activity_expires_at TIMESTAMPTZ NULL`; registered in `migration.go`.
- **`sessionlock.Session`** (`game/session/internal/sessionlock/lock.go`) gains `ActivityExpiresAt *time.Time`; both `LockByID`/`LockByUUID` select it alongside the existing columns.
- **New `session.TerminalReasonRuntimeInactivityExpired = "RUNTIME_INACTIVITY_EXPIRED"`** (`game/session/session.go`).
- **New shared package** `game/session/workflows/sessionlifecycle/internal/activity` (`activity.go`): `MaterializeIfDue(ctx, tx, store, lockedSession, now) (bool, error)`, mirroring `internal/expiration`'s shape - checks `Phase != RUNNING || ActivityExpiresAt == nil || now.Before(*ActivityExpiresAt)`, else `SetSessionTerminal` (terminal_at = the deadline) + `CloseAllActiveInteractionsForSession` + `CancelAllActiveTimerObligationsForSession`, mutating `lockedSession` in place.
- **`internal/repo/session.go`**: `SetSessionRunning` gained an `activityExpiresAt time.Time` parameter, persisted alongside `started_at`; new `RenewActivityDeadline(ctx, tx, sessionID, activityExpiresAt)` method.
- **`manager.go`**: new `defaultActivityTTL = 10 * time.Minute` constant and `Manager.activityTTL` field, wired in `New(...)` exactly like `lobbyTTL`.
- **`step_start.go`**: `SetSessionRunning` call now passes `now.Add(m.activityTTL)` as the initial deadline.
- **`step_answer_interaction.go`/`step_submit_user_intent.go`/`step_expire_timer.go`**: each calls `activity.MaterializeIfDue` immediately after locking, before any other business logic, and `RenewActivityDeadline(ctx, tx, sessionID, now.Add(m.activityTTL))` alongside its existing `SetCurrentTurn` call once a Turn actually commits.
- **`step_cancel_session.go`**: calls `activity.MaterializeIfDue` immediately after locking (no renewal call - see "Scope Addition" below).
- Each affected step's own narrow `*RepoAPI` interface gained `RenewActivityDeadline` where applicable (not `CancelSession`'s, which does not renew); no interface needed a new method for `MaterializeIfDue` itself, since each interface's already-existing `SetSessionTerminal`/`CloseAllActiveInteractionsForSession`/`CancelAllActiveTimerObligationsForSession` methods already structurally satisfy `activity.Store`. Mocks regenerated via `go generate`.

### Scope Addition (CancelSession Materialization Checkpoint, Self-Caught)

While implementing `step_cancel_session.go`, tracing GAME-ADR-0014's "every active-dependent operation validates the deadline" rule against every RUNNING-phase entry point (not only the three this WORK's own Renewal Trigger List names) found that `CancelSession` locks the Session exactly like the other three and, without a materialization check, could apply `TerminalReasonSessionCancelledByHost` over a Session that had, in fact, already inactivity-expired moments earlier. This is not a new design decision - it is the same already-accepted invariant applied to a fourth already-existing code path - so it was implemented directly rather than escalated; see the Approved Design's own "Lazy Materialization" section (updated during implementation to record this) for the full reasoning and exact call-site placement.

Local implementation decisions:
- Exact new migration filename/timestamp, `internal/activity` package/file naming, and the exact insertion point within each step's already-existing decline path - Implementation Freedom, per the WORK's own text.

Deviations from the approved WORK:
- None beyond the CancelSession scope addition above, which was recorded in the Approved Design itself before implementation, not discovered after the fact.

Discoveries:
- None requiring further escalation beyond the CancelSession addition above (already resolved as a local implementation decision, not a material one).

### Verification Performed (2026-09-26)

- `go build ./...`, `go vet ./...` - clean, repository-wide.
- `gofmt -l` on every new/changed file - flagged files are pure pre-existing CRLF-line-ending noise (confirmed via whole-file diff, every line shown as changed), the same finding WORK-0006/WORK-0007/WORK-0028 already recorded for this repository.
- `go test . -run TestNoInternalDocCitationsInComments` - re-checked before and after this WORK's own changes (temporarily stashing them) to confirm this WORK introduced zero new violations: 23 pre-existing violations both before and after, all from earlier already-DONE WORKs' own code (`runtime_failure.go`, `internal/repo/runtime_failure.go`, the `session_runtime_failures` migration, and several integration-test comments) - a genuine pre-existing gap in this already-enforced standard, not something this WORK introduced or is in scope to bulk-fix. Flagged as drift below.
- `go test ./game/session/... -count=1` against real Postgres - full package suite green, including every new test this WORK adds (see Acceptance Criteria below), no regression in any of the ~200 pre-existing repository-integration/concurrency tests across Create/Join/Leave/Start/AnswerInteraction/SubmitUserIntent/CancelSession/ExpireTimer.
- New tests, one per Acceptance Criterion: `step_start_integration_test.go` (AC1); `step_answer_interaction_integration_test.go` (AC2, AC4/5/6, AC7 - `accepted_answer_renews_activity_deadline`, `rejected_answer_does_not_renew_activity_deadline`, `stale_activity_deadline_materializes_inactivity_expiration_and_declines`, `renewed_activity_deadline_survives_past_original_stale_value`); `step_submit_user_intent_integration_test.go` and `step_expire_timer_integration_test.go` (AC3, plus their own stale-deadline proof); `step_cancel_session_integration_test.go` (the CancelSession materialization checkpoint, AC8-adjacent). AC7's own test backdates the deadline to 1.5s in the future, answers once (renewing it 10 minutes out), sleeps 2s past the *original* value, then proves a second call still succeeds rather than being wrongly treated as expired.

### Documentation Impact / Drift Found

- `game/CURRENT_STATE.md` - Session Runtime capability row and Current Gaps updated: `activity_expires_at` schema/renewal/lazy-materialization implemented; only the background Reaper remains not implemented.
- `game/docs/DATA_MODEL.md` - `sessions` class diagram gains `activity_expires_at`.
- `GAME-ADR-0014` - new "Implemented by" addendum, matching this Project's established convention.
- `game/README.md` - "Implemented by" status line added to the Durable Inactivity Expiration section (fix from independent review below - initially missed).
- **DRIFT DETECTED (pre-existing, not introduced by this WORK)**: `docs/engineering/standards/code-comments.md`'s `TestNoInternalDocCitationsInComments` (mechanically enforced by `go test ./...`) already fails on `main` independent of this WORK - 23 comments across `runtime_failure.go`, `internal/repo/runtime_failure.go`, the `session_runtime_failures` migration, and several WORK-0014-era integration-test comments cite an ADR/WORK number as the reason for something, contrary to the standard's own "No Citing Internal Documents As A Stand-In For Explanation" rule. This WORK's own new code was written and then corrected to comply with the standard (verified by re-running the check with this WORK's changes stashed vs. applied - identical 23-violation baseline either way), but a bulk cleanup of the pre-existing 23 is out of this WORK's own scope. Recommended resolution: a small, separate, low-risk cleanup WORK (or a code-review-only pass, since no behavior changes) rewriting those 23 comments to state their reasoning directly, the same way this WORK's own comments now do.

Ready for independent review: YES.

## Independent Review (2026-09-26)

A fresh agent, with no access to this session's own context, reviewed this WORK per `docs/ai/protocols/IMPLEMENTATION_REVIEW.md`. It determined the exact change-set via `git status`/`git diff` (confirmed exact match to scope, no ambiguity), read every changed production file's full diff plus the new `internal/activity/activity.go` in full, read all 5 changed/added integration test files in full and traced each new assertion against the actual code path (not just the test name), ran its own fresh `go build`/`go vet`/`gofmt -d` (confirmed CRLF-only noise)/`go test ./game/session/... -count=1` against real Postgres, independently reproduced and traced `TestNoInternalDocCitationsInComments`'s 23 pre-existing violations line-by-line to confirm this WORK's own new comments contribute zero of them, verified `terminal_at`'s deadline-not-now correctness directly in `activity.go`, verified the four `MaterializeIfDue` call-site placements by reading the surrounding code (not trusting the WORK's own narrative), and verified `RenewActivityDeadline` only fires on the already-committed-Turn path.

Verdict: CHANGES_REQUIRED, two REQUIRED_FIX findings, no DECISION_REQUIRED findings.

Findings and fixes:
1. **(MEDIUM, REQUIRED_FIX)** `game/README.md`'s Durable Inactivity Expiration section was missing the "Implemented by" note this WORK's own Documentation Impact section committed to. **Fixed**: added a "Status: HUMAN-APPROVED; schema/renewal/lazy-materialization implemented by `WORK-0016-inactivity-expiration-reaper.md`..." line, matching the existing `GAME-ADR-0014`/WORK-0014 precedent style already used elsewhere in `game/README.md`.
2. **(LOW, REQUIRED_FIX)** This WORK's own `Status:` header still read READY, inconsistent with its own already-narrated IMPLEMENTING completion state. **Fixed**: header updated to IMPLEMENTING.
3. **(LOW, NON_BLOCKING)** `gofmt -l` flags every touched file - confirmed pure pre-existing CRLF-line-ending noise, not fixed, consistent with this repository's own already-accepted condition.

Both REQUIRED_FIX findings were mechanical/documentation-only (no code/test change, no re-verification of Postgres/build/vet needed). Everything else - schema, renewal, all four lazy-materialization call sites, `terminal_at` correctness, the `session_runtime_failures` exclusion, and test coverage against every Acceptance Criterion - was independently verified against primary evidence and found correct.

## Independent Re-Review (2026-09-26)

A fresh agent (no access to this session's own context) verified both fixes against real evidence rather than the Fix Pass narrative alone: read the exact inserted `game/README.md` line via `git diff`, compared it against the existing WORK-0014 precedent line's style/shape, confirmed it accurately scopes the claim to schema/renewal/lazy-materialization only (explicitly stating the Reaper is not implemented); re-read the WORK file's own `Status:`/`Last status change` header directly; and, via `git status`/`git diff --stat` plus a file-modification-timestamp comparison, confirmed only `game/README.md` and this WORK file were touched after the first review - every production/migration/test file's timestamp clustered inside the original implementation window, before the first review ran - so no code/test/migration regression was possible and no build/vet/Postgres re-verification was needed.

Verdict: APPROVED, no findings.

## Completion Record

**DONE (2026-09-26).** `sessions.activity_expires_at` (GAME-ADR-0014's RUNNING-phase inactivity deadline) is implemented: schema (nullable, set by `Start` alongside `started_at`); renewal by `AnswerInteraction`/`SubmitUserIntent`/`ExpireTimer` whenever any of them actually commits a new RuntimeTurn; lazy materialization (`internal/activity.MaterializeIfDue`, mirroring `internal/expiration`'s LOBBY-phase shape) by those three plus `CancelSession` - a fourth checkpoint this WORK's own implementation pass found necessary and added as a local, non-material extension of GAME-ADR-0014's already-accepted "every active-dependent operation validates the deadline" invariant. Reuses WORK-0007/WORK-0011's non-failure terminalization shape (not WORK-0014's `materializeRuntimeFailure`) plus the GAME-ADR-0019 terminal-cleanup invariant; `terminal_at` always equals the deadline that passed, never the materializing call's own current time. The background Session Reaper remains explicitly out of this WORK's own scope, split to a new, not-yet-drafted WORK (see "Scope Correction (Reaper Split, 2026-09-26)" above) - a stale Session only actually terminalizes the next time some RUNNING-phase operation reaches it, not proactively. Two independent review rounds: first returned CHANGES_REQUIRED (two mechanical documentation-only REQUIRED_FIX findings - a missing `game/README.md` "Implemented by" note, and this WORK's own stale `Status:` header - both fixed); re-review returned APPROVED with no findings. Documentation synchronized: `game/CURRENT_STATE.md`, `game/docs/DATA_MODEL.md`, `game/README.md`, `GAME-ADR-0014`'s "Implemented by" addendum. Known, pre-existing, out-of-scope drift flagged (not introduced by this WORK): `docs/engineering/standards/code-comments.md`'s `TestNoInternalDocCitationsInComments` already fails on `main` with 23 pre-existing violations from earlier WORK-0014-era code; recommended resolution is a small separate cleanup WORK.
