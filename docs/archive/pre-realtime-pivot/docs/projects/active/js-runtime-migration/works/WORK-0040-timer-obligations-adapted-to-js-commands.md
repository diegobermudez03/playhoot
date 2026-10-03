# WORK-0040: Timer Obligations Adapted To JS Commands

Status: DONE
Created: 2026-09-27
Last status change: 2026-09-29 (IMPLEMENTING -> DONE: independent review APPROVED after one fix/re-review round)

Related decisions:
- `game/docs/decisions/GAME-ADR-0028-javascript-execution-replaces-game-language.md`
- `session/docs/decisions/SESSION-ADR-0007-session-runtime-v1-timer-recovery-simplification.md`
- `session/docs/decisions/SESSION-ADR-0011-game-language-keyed-timer-slots.md` (superseded in substance by this WORK - see Context)

Canonical context:
- `session/workflows/sessionlifecycle/internal/repo/timer_obligation.go` (existing durable timer-obligation persistence, `session_timer_obligations` - read directly, not assumed, during drafting)
- `session/workflows/sessionlifecycle/internal/platform/command.go` (the already-accepted `ScheduleTimer`/`CancelTimer` command shape this WORK persists)

## Outcome

Adapt Session Runtime's already-implemented durable timer-obligation mechanism (`session_timer_obligations`, from `session-runtime-v1`'s DONE `WORK-0012`) to be driven by `WORK-0039`'s already-accepted command vocabulary (`SCHEDULE_TIMER`/`CANCEL_TIMER`, requested by a script's own output) instead of the retired `engine.ScheduleTimerOutput`/`CancelTimerOutput`. Timer ownership itself (Playhoot schedules and fires; authored code only requests/reacts) is unchanged in principle per `GAME-ADR-0028`; this WORK is the concrete integration, not a redesign of timer semantics.

## Context

Drafting checked the actual current code rather than trusting this WORK's own original assumption. Two things were confirmed, not assumed:

1. **The gap is real and exactly where expected.** `session_timer_obligations` (schema, `internal/repo/timer_obligation.go`'s persistence methods) still exists from the retired engine era, and `step_expire_timer.go` already reads an obligation to build a `TIMER_EXPIRED` event (reusing the old `EngineSlot`/`EngineKey` columns as the new `Timer`/`Data` fields, informally, with a comment acknowledging it). But nothing writes a *new* obligation: `CreateTimerObligation`/`CancelActiveTimerObligation` are defined but have zero callers anywhere in `sessionlifecycle`. Every RUNNING-phase step that calls `Execute` (`Start`, `SubmitPlayerEvent`, `CancelSession`, `ExpireTimer`) already parses `[]platform.Command` and passes it to `completion.Detect` (`SESSION_COMPLETE`/`SESSION_FAIL` only) - a `SCHEDULE_TIMER`/`CANCEL_TIMER` command is silently dropped everywhere today.
2. **This WORK's own flagged vocabulary question is already answered by `WORK-0039`'s existing accepted design, not still open.** `platform.ScheduleTimer`/`CancelTimer` (`command.go`) carry exactly one opaque `Timer` string (plus `DelayMS`/opaque `Data` on schedule) - there is no second "key" field at the wire/command level at all. `SESSION-ADR-0011`'s Game-Language-specific `KeyedTimerSlot<Key>` concept (a second addressing dimension, `(slot, key)`) has no equivalent here: an author who needs independently addressable concurrent timers (per-player, per-team) encodes that directly into their own single opaque `Timer` string (for example `"disconnect_timeout:P1"`), exactly as this WORK's own prior note already guessed. `SESSION-ADR-0011` is therefore superseded in substance, not merely pending re-confirmation - see its own dated Addendum.

`SESSION-ADR-0007`'s relative-delay-only recovery model (persist `delay_ms`, never an absolute deadline; reschedule from the full delay on recovery) is unaffected and still the accepted model - this WORK does not reopen it.

## Scope

### In Scope

- Rename `session_timer_obligations.engine_slot` -> `timer`, `engine_key` -> `data` (a migration; both already, informally, hold exactly this data per point 1 above - this makes it their real name, not merely their real meaning under a stale name), and simplify the `WHERE state = 'ACTIVE'` uniqueness constraint from `(session_id, engine_slot, COALESCE(engine_key, ...))` to `(session_id, timer)` - `data` is an opaque echoed payload, never part of a timer's identity, so it must not be part of the uniqueness tuple (the current index is subtly wrong on this point once `engine_key` genuinely means "data").
- A new shared mechanism (mirroring `internal/completion.Detect`'s own shape exactly: a package-level function scanning already-parsed `[]platform.Command`) that, given a Session's ID and the just-created RuntimeTurn's ID: creates a new ACTIVE timer obligation for each `*platform.ScheduleTimer` command, and cancels the matching ACTIVE obligation for each `*platform.CancelTimer` command.
- Wiring that mechanism into all four RUNNING-phase steps that already parse commands (`Start`, `SubmitPlayerEvent`, `CancelSession`, `ExpireTimer`), called after the RuntimeTurn is created (so `created_by_turn_id` is available) and before each step's existing `completion.Detect`/terminal-cleanup block (so the already-accepted "no timer obligation survives TERMINAL" invariant holds automatically: a Turn that both schedules a timer and completes the game has that same timer cancelled again by the unconditional terminal cleanup that already runs afterward - no special-casing needed).
- Renaming the Go-side mirror types (`internal/repo.TimerObligation.EngineSlot`/`EngineKey` -> `Timer`/`Data`) and the two existing, currently-uncalled repo methods' parameter names to match.

### Out of Scope

- Physical timer scheduling/wakeup (an actual process that calls `Manager.ExpireTimer` when a delay elapses) - `session-runtime-v1`'s `WORK-0020` owns building the Coordinator that would do this; `session/CURRENT_STATE.md` already records this gap and this WORK does not close it.
- `SendEvent` command delivery - `WORK-0042`'s own scope.
- Any change to `ExpireTimer`'s own already-implemented read/expire/close path beyond the rename above - it already works correctly against the existing columns under their old names.

## Approved Design

- **Schema**: one migration renaming the two columns and replacing the unique index, described above. No new table, no new column beyond the rename.
- **Reschedule semantics (a deliberate, flagged divergence from the retired `SESSION-ADR-0011`)**: issuing `SCHEDULE_TIMER` for a `Timer` identifier that already has an ACTIVE obligation **implicitly replaces it** (the existing ACTIVE obligation for that identifier is cancelled, then the new one is created, both inside the same already-open transaction) - not rejected the way the retired Game-Language engine atomically failed the whole transition on an occupied slot. This is a deliberate design choice, not an oversight: the old rejection model depended on a closed DSL's own compile-time slot concept and an engine that could atomically fail an entire transition before committing anything; JS commands are validated and applied after the script has already run, so an equivalent "reject the whole Turn" would require re-validating against database state before ever creating the RuntimeTurn, and rolling back cleanly if execution had already committed - real additional machinery for a case an author can already avoid by choosing distinct `Timer` identifiers, or embrace by relying on replace-on-reschedule as an intentional pattern (a countdown that resets itself, for example). `CANCEL_TIMER` against a Timer identifier with no matching ACTIVE obligation remains an ordinary no-op, unchanged from today's existing `CancelActiveTimerObligation` behavior and from `command.go`'s own already-documented contract.
- **New package**: `session/workflows/sessionlifecycle/internal/timers/timers.go`, exposing `Apply(ctx, tx, repo, sessionID, turnID uint, commands []platform.Command) error` against a narrow `repoAPI` (`CreateTimerObligation`, `CancelActiveTimerObligation` - already-existing methods, only their parameter names change). Each step's own existing repo interface gains these two methods (already present in `timer_obligation.go`, simply not yet part of any step's own interface).

## Constraints and Invariants

- No timer obligation may remain ACTIVE after a Session reaches TERMINAL (restates the existing `SESSION-ADR-0018` terminal-cleanup invariant, applied unchanged to command-sourced timers - already satisfied by ordering `timers.Apply` before each step's existing terminal-cleanup block, per Approved Design).
- `data` must never participate in a timer obligation's uniqueness/identity - only `(session_id, timer)` does.
- No absolute deadline (`due_at`) is persisted - `SESSION-ADR-0007` continues to apply unchanged.

## Acceptance Criteria

- A script's `SCHEDULE_TIMER` command, in any of `Start`/`SubmitPlayerEvent`/`CancelSession`/`ExpireTimer`'s own output, persists a new ACTIVE `session_timer_obligations` row identified by `(session_id, timer)`, stamped with the creating RuntimeTurn's ID.
- A script's `CANCEL_TIMER` command cancels the matching ACTIVE obligation for that `(session_id, timer)`; a `CANCEL_TIMER` with no matching ACTIVE obligation is a no-op, not an error.
- A `SCHEDULE_TIMER` for a `Timer` identifier that already has an ACTIVE obligation replaces it (the old obligation is CANCELLED, a new ACTIVE one is created) rather than erroring or leaving two ACTIVE rows.
- A Turn whose commands include both a timer command and `SESSION_COMPLETE`/`SESSION_FAIL` leaves no ACTIVE obligation afterward.
- `ExpireTimer`'s own existing behavior (reading an obligation, building `TIMER_EXPIRED`, closing it) is unaffected by the column rename - same behavior, new names.

## Implementation Freedom

- Exact file/function names inside the new `internal/timers` package, beyond the `Apply` shape above.
- Exact migration mechanics (single combined migration vs. one per rename) are the implementer's choice.

## Verification

- `go test ./session/...` covering every Acceptance Criterion above with fixture scripts/commands, following this package's existing integration-test conventions (`step_start_integration_test.go` and siblings already exercise a real Postgres container when reachable).
- `go build ./...` / `go vet ./...` clean repository-wide.

## Documentation Impact

### Accepted / Canonical Knowledge

- `session/docs/decisions/SESSION-ADR-0011-game-language-keyed-timer-slots.md` - dated Addendum recording that its `KeyedTimerSlot<Key>` concept is superseded in substance by the single-opaque-`Timer`-identifier model this WORK implements; not rewritten.

### Current-State Documentation After Implementation

- `session/docs/DATA_MODEL.md`/`session/docs/FLOWS.md` - timer-obligation source updated from engine Output to JS command; column names updated.
- `session/CURRENT_STATE.md` - "Current Gaps" bullet about timer obligations updated: persistence-side scheduling from JS commands is now implemented; physical wakeup remains `session-runtime-v1`'s `WORK-0020` gap, unchanged.

## Blockers

- None. (`WORK-0039` is DONE.)

## Completion Record

Implemented as designed. Migration `20260929000004_session_timer_obligations_timer_data` renames `session_timer_obligations.engine_slot`/`engine_key` to `timer`/`data` and replaces the active-obligation unique index with `(session_id, timer)` only. `internal/repo/timer_obligation.go`'s `TimerObligation` struct and `CreateTimerObligation`/`CancelActiveTimerObligation` methods renamed to match (`CancelActiveTimerObligation` also dropped its now-meaningless `engineKey` matching parameter). A dead, orphaned method discovered in the same file - `GetTimerObligationByID`, left over from the deleted replay mechanism, zero callers anywhere in the repository - was removed as a local cleanup, not left with a doc comment citing a package (`internal/replay`) that no longer exists.

New package `session/workflows/sessionlifecycle/internal/timers` (`timers.go`, mirroring `internal/completion.Detect`'s own shape): `Apply(ctx, tx, repo, sessionID, turnID, commands)` scans already-parsed commands and, for each `ScheduleTimer`, cancels any existing ACTIVE obligation for that `Timer` identifier before creating the new one (the approved replace-on-reschedule semantics), and for each `CancelTimer`, cancels the matching ACTIVE obligation (a no-op if none matches, per that command's own contract). Wired into all four RUNNING-phase steps that already parse commands (`Start`, `SubmitPlayerEvent`, `CancelSession`, `ExpireTimer`), called after each step's own RuntimeTurn is created and before its existing terminal-cleanup block - so the already-accepted "no timer obligation survives TERMINAL" invariant holds automatically, with no special-casing, exactly as designed. `step_expire_timer.go`'s existing `platform.NewTimerExpired(obligation.EngineSlot, obligation.EngineKey)` call updated to the renamed fields; its own stale comment explaining the informal old-field reuse was removed since it no longer applies.

A real, previously-undiscovered regression was caught during verification, not left for review to find: `session/internal/testfixtures/testfixtures.go`'s `SeedTimerObligation` fixture helper still inserted against the old `engine_slot` column name, breaking `TestManagerExpireTimer_Integration`'s six subtests against a real Postgres database the moment the migration ran. Fixed (renamed to `timer`, and its own doc comment - which had said "since nothing yet dispatches a SCHEDULE_TIMER platform Command," no longer true - corrected) before this WORK was considered complete, not reported as a pre-existing failure it was not.

Verification: `go build ./...`/`go vet ./...` clean repository-wide. `go test ./...` run twice, both against Postgres (a reachable container found in this session, credentials read from `docker inspect`) and without it - clean except the same two confirmed pre-existing, unrelated failures this Project's history already documents repeatedly (`comment_standard_test.go`'s three citations in an untouched migration file; `TestManagerJoin_Integration_ConcurrentOperationRacingLobbyExpiration`, the already-documented `session-runtime-v1`-owned Join/Leave race - confirmed via `git status` showing neither file touched by this WORK). New test coverage: `internal/timers/timers_test.go` (8 unit tests covering every Acceptance Criterion directly against a hand-written fake, including error-propagation/stop-on-first-failure and that unrelated commands are ignored); 5 new integration subtests in `step_submit_player_event_integration_test.go` (schedule persists ACTIVE, cancel cancels, cancel-with-no-match is a no-op, reschedule replaces with both old/new rows visible, a Turn that both schedules and completes leaves nothing ACTIVE) against a real Postgres database; 1 new integration subtest in `step_expire_timer_integration_test.go` proving the common "chain into the next timer" pattern (a timer's own expiration reaction schedules the next one) works end to end through `ExpireTimer`'s own call site specifically, distinct from `SubmitPlayerEvent`'s.

Documentation synchronized: `session/docs/decisions/SESSION-ADR-0011-game-language-keyed-timer-slots.md` (dated Addendum, not rewritten), `session/docs/DATA_MODEL.md` (column names/description, uniqueness constraint), `session/CURRENT_STATE.md` (Current Gaps' timer-obligation clause updated to name this WORK rather than the superseded `WORK-0012`). `session/docs/FLOWS.md` was checked and needed no change - its own timer mention is already accurate and unaffected by this WORK (no live Coordinator exists yet regardless of which mechanism populates the table, unchanged). The pre-existing, already-known-stale `session/CURRENT_STATE.md` "Session Runtime | PARTIAL" capability row and its "Evidence" bullets (describing the fully-retired Game Language/engine-Output mechanism in detail) were deliberately left untouched - out of this WORK's own committed Documentation Impact, already flagged by `WORK-0038`'s own review as a separate, not-yet-scheduled documentation-debt item.

No caller outside `sessionlifecycle` is affected: `session.TimerObligationUUID`/`ExpireTimerResult` and every other public contract type are unchanged. Physical timer scheduling/wakeup remains explicitly out of scope, unowned by this WORK, per its own Scope section - `session-runtime-v1`'s `WORK-0020` still owns that gap.

Independent review ran two rounds. Round 1 CHANGES_REQUIRED (one REQUIRED_FIX - `session/CURRENT_STATE.md`'s own Current Gaps clause asserted this WORK "DONE" while its own `Status:` header still read `IMPLEMENTING`, inconsistent with `PROJECT.md`/`AI_CONTEXT.md`'s correct "pending independent review" phrasing in the same uncommitted change), fixed same day by rewording to "pending independent review." Round 2 **APPROVED, no findings** - the same reviewer independently re-verified every claim from round 1 still held (nothing else in the diff changed) and confirmed the fix directly against the file. One NON_BLOCKING observation across both rounds - this WORK's own Completion Record being filled in while still `IMPLEMENTING` rather than only at Closure - was accepted as this Project's own established, harmless style (matching WORK-0032/0034/0036/0037's own history) rather than a defect.
