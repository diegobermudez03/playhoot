# WORK-0014: Runtime Failure Diagnostics + Terminal Cleanup

Status: DONE
Created: 2026-09-20
Last status change: 2026-09-26 (IMPLEMENTING -> DONE: independent re-review verdict APPROVED, no findings, after one fix/re-review round - first round found a genuine test-coverage gap (AC2, the RUNTIME_STATE_INVALID/recompile-failure path, had no test across any of the 5 fatal paths), fixed and verified against real Postgres; re-review confirmed the fix and traced the actual production code path it exercises, no further findings. See "Implementation Report"/"Fix Pass"/"Independent Re-Review" sections. Prior status change, same day: READY -> IMPLEMENTING: implemented against the Approved Design - see "Implementation Report" below. Prior status change, same day: DRAFT -> READY: human explicitly authorized implementation, confirming both recommended Blocker resolutions - omit `failed_step_index` as a column for now; keep `diagnostic_payload` a minimal envelope. See "Blockers" below for the resolved record. Prior status change, same day: PLANNED -> DRAFT: Approved Design filled in against the actual current implementation - see "Approved Design" and "Design Basis (2026-09-26)" below. Prior status change: 2026-09-20 Part B reconciliation, same day: Snapshot-availability assumption reconciled - see "Scope Clarification (Part B Reconciliation, 2026-09-20)" below)

Related decisions:
- GAME-ADR-0017 (failure taxonomy, diagnostic entity - HUMAN-APPROVED and canonically promoted, not yet implemented)
- GAME-ADR-0019 (RuntimeTurn execution bound and terminal cleanup)
- GAME-ADR-0024 (Replay-First Session Runtime Persistence - no durable Snapshot exists for this WORK's diagnostics to reference)

Canonical context:
- `docs/projects/active/session-runtime-v1/PROJECT.md`
- `game/README.md` (states GAME-ADR-0017 is "HUMAN-APPROVED and canonically promoted; not yet implemented")
- `game/session/workflows/sessionlifecycle/step_start.go` (`terminalizeStartFatal`), `step_answer_interaction.go` (`terminalizeAnswerInteractionFatal`) - the existing narrow fatal paths this WORK generalizes/enriches

## Outcome

Every deterministic runtime failure across every RuntimeTurn-producing path (Start, interaction response, and future timer expiration/user-intent/cancellation paths) produces a durable, queryable diagnostic record with a stable error code, distinguished from transient infrastructure failure, and every terminal Session - regardless of cause - retains no dangling `ACTIVE` interaction or timer obligation.

Today, two narrow fatal paths exist (`terminalizeStartFatal`/`terminalizeAnswerInteractionFatal`), each closing only the interactions its own path knows about. GAME-ADR-0017's fuller diagnostic entity (`session_runtime_failures`, stable error codes, queryability) has no implementation anywhere.

## Context

This WORK retrofits diagnostics/cleanup across all RuntimeTurn-producing paths that exist by the time it is designed (Start, AnswerInteraction, and whichever of WORK-0010/0011/0012 have landed by then), rather than each new path reinventing its own narrow version, as Start and AnswerInteraction currently each do independently.

## Scope

### In Scope (known required outcome; design not yet started)

- `session_runtime_failures` persistence (GAME-ADR-0017).
- Stable error codes, including at minimum `runtime_turn_step_limit_exceeded`.
- Atomic fatal materialization, generalized across every RuntimeTurn-producing path.
- The terminal-cleanup invariant (GAME-ADR-0019): a `TERMINAL` Session retains no `ACTIVE` interaction/timer obligation, closed atomically with `closed_by_turn_id = NULL` when not Turn-produced.

### Out of Scope

- Any new failure-causing path itself - this WORK enriches how existing/future paths report failure, it does not add new ways to fail.

## Design Basis (2026-09-26)

Before drafting, the actual current implementation was inspected (not assumed from the ADRs alone):

- **GAME-ADR-0019's terminal-cleanup invariant is already fully implemented today.** Every RuntimeTurn-producing path (`step_start.go`, `step_answer_interaction.go`, `step_submit_user_intent.go`, `step_cancel_session.go`, `step_expire_timer.go`) already calls the same two shared repo-level bulk methods - `CloseAllActiveInteractionsForSession` / `CancelAllActiveTimerObligationsForSession` (`internal/repo/interaction.go`, `internal/repo/timer_obligation.go`) - both on every existing fatal path and on every authored-completion/host-cancellation terminal path. What is genuinely missing is only the `session_runtime_failures` diagnostic entity (GAME-ADR-0017) itself; this WORK's terminal-cleanup half is therefore a verification/no-regression concern, not new construction.
- **Five fatal-path functions already exist** (`terminalizeStartFatal`, `terminalizeAnswerInteractionFatal`, `terminalizeSubmitUserIntentFatal`, `terminalizeCancelSessionFatal`, `terminalizeExpireTimerFatal`), each independently duplicating the same `SetSessionTerminal` -> close-interactions -> cancel-timers call sequence at the workflow-call-site level (the underlying repo methods are already shared; the call sequence around them is not). This resolves this WORK's own previously-recorded "shared mechanism vs. per-path" question: introduce one shared workflow-level helper for the sequence, since 5 already-landed, already-reviewed call sites duplicate it verbatim today - this is a straightforward internal refactor (private helper structure), not a new architectural pattern, so it is decided here rather than left as a Blocker.
- **`terminalizeCancelSessionForced` (WORK-0011's host-forced terminalization) and every authored-completion "if terminated" branch (WORK-0006/0007) are not runtime failures** and must not gain a `session_runtime_failures` row - GAME-ADR-0017 creates that entity only for failure classes B/C, never for an intentional host cancellation or an authored game ending.
- **The engine's actual failure contract is narrower than GAME-ADR-0017's aspirational diagnostic-payload description.** `engineservice.StartTurn`/`AdvanceTurn` return only `*engine.ExecutionError{Code, Message}` on failure - no per-Step index, no partial Step traces, no Snapshot (confirmed: `Trace` is only ever produced by a successful `Commit`; a failed/rejected Step never produces one). Populating `failed_step_index` or "traces from successful in-memory Steps" (both explicitly optional in GAME-ADR-0017 - "when known" / "may preserve") is not possible without an engine-contract change, which this WORK does not authorize (it "enriches how existing/future paths report failure, it does not add new ways to fail"). The design below reflects only what the engine's current contract actually exposes.
- **`source_kind` vocabulary already exists and is reused, not invented**: `internal/replay` already defines `AnswerInteractionSourceKind`/`TimerExpiredSourceKind`/`UserIntentSourceKind`/`SessionCancelledSourceKind`; only Start's own `"SESSION_START"` is still a package-local constant in `step_start.go` rather than promoted alongside the other four.
- **`base_turn_id`/`attempted_sequence` are already computed by every non-Start path** (`lockedSession.CurrentTurnID`, and `currentTurn.Sequence` from the `GetRuntimeTurn` call each step already makes) - no new query is needed to obtain them.

## Approved Design

**1. New table `session_runtime_failures`** (migration under `game/session/internal/storage/migrations/`), per GAME-ADR-0017's field list, narrowed per the point above:

`id`, `session_id` (FK `sessions.id`, indexed), `failure_kind` (`RUNTIME_EXECUTION` | `RUNTIME_STATE_INVALID`), `error_code` (text, indexed - see below), `error_message` (text), `base_turn_id` (nullable FK `session_runtime_turns.id`), `attempted_sequence` (int), `source_kind` (text, reusing the existing `internal/replay` vocabulary), `source_interaction_id` / `source_timer_obligation_id` / `actor_id` (nullable, mirroring `session_runtime_turns`' own existing nullable source columns), `diagnostic_payload` (JSONB), `created_at`.

`failed_step_index` is deliberately **not** added as a column now (see Blockers - flagged for confirmation, not silently dropped): the engine's current contract cannot populate it, and GAME-ADR-0017 itself already marks it optional/"when known." Adding it later is purely additive and does not require revisiting this design.

**2. Promote `"SESSION_START"` into `internal/replay` as `replay.SessionStartSourceKind`**, alongside the four already-shared constants, so the new shared failure helper (below) can accept one uniform `source_kind` type across all 5 call sites. Purely a naming/location move; no behavior change.

**3. Stable, engine-decoupled error-code catalog** - new file `game/session/workflows/sessionlifecycle/runtime_failure.go`:
- A `RuntimeFailureErrorCode` string type with one stable constant per `engine.ExecutionErrorCode` value that can actually reach a fatal path via `AdvanceTurn`/`StartTurn` (excludes `ExecutionErrorSignalRejected`/`ExecutionErrorInputRejected` - those are already intercepted upstream as class-A rejections before any `terminalize*Fatal` call, unchanged by this WORK), plus `RuntimeFailureErrorCodeDefinitionRecompileFailed` for the `RUNTIME_STATE_INVALID`/recompile-diagnostics path (which never produces an `engine.ExecutionError` at all), plus `RuntimeFailureErrorCodeUnknown` as a forward-compatible fallback for any future engine code this mapping has not been updated for.
- `ExecutionErrorStepChainExceeded` maps to `runtime_turn_step_limit_exceeded` exactly, matching GAME-ADR-0019's own named example.
- This mirrors WORK-0029's already-accepted precedent (`sessionlifecycle` owning its own closed `Value`/`Output` vocabulary decoupled from `engine`'s): a future engine `ExecutionErrorCode` renumbering must never silently change the meaning of an already-persisted `error_code` string.

**4. One shared atomic-materialization helper**, same file: `(m *Manager) materializeRuntimeFailure(ctx, tx, repo runtimeFailureRepoAPI, lockedSession, terminalAt, failureKind, errorCode, errorMessage, sourceKind, sourceInteractionID, sourceTimerObligationID, actorID) error`. In order, within the caller's existing transaction: `SetSessionTerminal` -> `CreateRuntimeFailure` (new repo method, `internal/repo/runtime_failure.go`) -> `CloseAllActiveInteractionsForSession` -> `CancelAllActiveTimerObligationsForSession`. `base_turn_id`/`attempted_sequence` are derived inside from `lockedSession.CurrentTurnID` (nil -> `base_turn_id=NULL`, `attempted_sequence=1`, Start's pre-first-Turn case) or the caller's already-loaded `currentTurn.Sequence+1`. `diagnostic_payload` is written as a minimal forward-compatible envelope (`{"schema_version": 1}`) - not a dumping ground for content the engine does not actually provide (see Design Basis). Each of the 5 existing narrow `xxxRepoAPI` interfaces gains the one new `CreateRuntimeFailure` method (matching this package's existing per-step-interface convention - no new interface-embedding pattern introduced).

**5. Refactor the 5 existing fatal functions** to call `materializeRuntimeFailure` instead of directly repeating `SetSessionTerminal`/close/cancel; each keeps building its own step-specific `Result` value and idempotency-claim completion exactly as today (unchanged - that part genuinely varies per step and stays local to each). `terminalizeCancelSessionForced` and every authored-completion "if terminated" branch are explicitly **not** touched - they call the existing (unchanged) `SetSessionTerminal`/close/cancel sequence directly, since they are not runtime failures.

## Constraints and Invariants

- Must not weaken the existing Step-bound guard (already enforced since WORK-0003/relocated into `engine.Limits` by WORK-0028) - this WORK adds richer diagnostics/cleanup around failures that guard already stops, it does not change when execution stops.
- Terminal cleanup must remain atomic with the terminal transition itself (already true today; must not regress).
- `error_code` must be a Session-Runtime-owned stable string, never the engine's raw `ExecutionErrorCode` int, and never exposed to players (GAME-ADR-0017's public/internal separation is unchanged).
- Must not reintroduce a durable Snapshot or per-Step trace under any name (GAME-ADR-0024) - `diagnostic_payload` stays a minimal envelope, not computed/derived content.
- `session_runtime_failures` is not created for class A (expected rejection), class D (transient infrastructure failure), authored game completion, or host cancellation - only for classes B/C.

## Acceptance Criteria

1. A deterministic execution failure on each of Start/AnswerInteraction/SubmitUserIntent/CancelSession/ExpireTimer persists exactly one `session_runtime_failures` row, atomically with the Session's existing TERMINAL transition and terminal cleanup, in the same transaction - proven per path (5 integration scenarios, extending each step's already-existing fatal-path test).
2. A pinned-Definition recompile failure (`RUNTIME_STATE_INVALID`) persists a `session_runtime_failures` row with `failure_kind=RUNTIME_STATE_INVALID` and `error_code=RuntimeFailureErrorCodeDefinitionRecompileFailed` - proven for at least one path.
3. `error_code` is engine-decoupled (a Session-Runtime-owned string, not the engine's int `ExecutionErrorCode`) and `ExecutionErrorStepChainExceeded` maps to exactly `runtime_turn_step_limit_exceeded`.
4. A transient infrastructure failure before commit produces no `session_runtime_failures` row and no TERMINAL Session (unchanged rollback behavior - regression test only, no new behavior).
5. An engine signal rejection (`ErrSignalRejected`/`ErrInputRejected`) produces no `session_runtime_failures` row; Session stays RUNNING as today (regression test only).
6. An authored-game-completion terminal outcome (WORK-0007) and a host `CancelSession` terminal outcome (WORK-0011) each produce no `session_runtime_failures` row (regression test only - neither is a runtime failure).
7. `base_turn_id`/`attempted_sequence` read `NULL`/`1` for a pre-first-Turn Start failure and the correct last-committed-Turn-derived values for every later path.
8. The GAME-ADR-0019 terminal-cleanup invariant (no `ACTIVE` interaction/timer obligation survives a `TERMINAL` Session) continues to hold post-refactor across all 5 paths - no regression versus already-passing existing tests.
9. `session_runtime_failures` has an index supporting lookup by `error_code` and by `session_id` (GAME-ADR-0017's failure-rate-by-error-code/by-game-definition queryability goal) - no query/read API is added by this WORK itself (matches the "no new Manager-level read API" precedent from WORK-0012's Blocker resolution); direct table query only.

## Blockers

Both resolved, 2026-09-26, human-confirmed (recommended option accepted for each):

1. **`failed_step_index` is omitted as a column for now** - the engine's current `AdvanceTurn`/`StartTurn` contract cannot populate it and GAME-ADR-0017 already marks it optional. May be added later, purely additively, only if/when the engine contract ever exposes a per-step failure position.
2. **`diagnostic_payload` stays a minimal envelope** (`{"schema_version": 1}`) - `error_code`/`error_message` already have their own columns, and the engine's contract provides no additional in-memory diagnostic content today. Enriching it would require a separate engine-contract change, out of this WORK's scope.

## Scope Clarification (Part B Reconciliation, 2026-09-20)

`game/docs/decisions/GAME-ADR-0024-replay-first-session-runtime-persistence.md` removes durable per-Turn Snapshot persistence. This WORK's `session_runtime_failures.diagnostic_payload` (already accepted by GAME-ADR-0017 as "a versioned internal diagnostic structure... that may preserve the attempted cause, the failed signal, and traces from successful in-memory Steps before the fatal Step") must not assume a durable Snapshot is available to reference or embed - any diagnostic content it captures must be self-contained (the in-memory state actually available at failure time, captured directly into the payload) rather than a pointer to a Snapshot row that no longer exists under the replay-first model. This WORK must not reintroduce Snapshot persistence under another name (for example, a "failure snapshot" column) merely to make diagnostics easier - if richer diagnostic context is genuinely needed, it is captured directly in `diagnostic_payload` at the moment of failure, consistent with GAME-ADR-0017's own already-accepted shape. No implementation was performed; this WORK's Status remains PLANNED.

## Documentation Impact

- `game/README.md`'s "Session Runtime Failure Classification And Diagnostic Persistence" section: status line changes from "HUMAN-APPROVED and canonically promoted; not yet implemented" to implemented.
- `game/CURRENT_STATE.md` / `game/docs/DATA_MODEL.md`: add `session_runtime_failures`.
- `GAME-ADR-0017` / `GAME-ADR-0019`: add an "Implemented by WORK-0014" addendum, matching this Project's established convention (e.g. GAME-ADR-0026/0027's own addenda).
- Flag for **WORK-0017 (Archival, PLANNED)**, not decided or implemented here: `session_runtime_failures` is excluded from hard-delete-after-archival per GAME-ADR-0017 - WORK-0017's own design should account for this when it is next drafted.

## Implementation Report (2026-09-26)

Implemented against the Approved Design above, in the same session as drafting/READY:

- **Migration**: `game/session/internal/storage/migrations/20260926000000_session_runtime_failures.go` creates `session_runtime_failures` (no `failed_step_index` column, per the resolved Blocker), plus `session_runtime_failures_session_idx`/`session_runtime_failures_error_code_idx` indexes; registered in `migration.go`.
- **Repo**: `game/session/workflows/sessionlifecycle/internal/repo/runtime_failure.go` - `CreateRuntimeFailure`, following the existing `CreateRuntimeTurn`/`CreateTimerObligation` explicit-parameter/insert-struct convention exactly.
- **Stable error-code catalog + shared helper**: new `game/session/workflows/sessionlifecycle/runtime_failure.go` - `RuntimeFailureKindExecution`/`RuntimeFailureKindStateInvalid`, a `RuntimeFailureErrorCode*` constant per `engineservice.ExecutionErrorCode` value reachable from a fatal path (`mapExecutionErrorCode`), `classifyExecutionError` (extracts code/message from a non-rejection `AdvanceTurn`/`StartTurn` error via `errors.As`), `formatDiagnostics` (joins a failed recompile's `engineservice.Diagnostics` into one message), the minimal `runtimeFailureDiagnosticPayload{SchemaVersion: 1}` envelope (per the resolved Blocker), and the shared `(*Manager).materializeRuntimeFailure` - `SetSessionTerminal` -> `CreateRuntimeFailure` -> `CloseAllActiveInteractionsForSession` -> `CancelAllActiveTimerObligationsForSession`, replacing the 5 fatal functions' own previously-duplicated 2-3-line sequence.
- **Promoted `replay.SessionStartSourceKind`** (was a package-local const in `step_start.go`) alongside the other 4 already-shared `internal/replay` source-kind constants, so the new shared helper accepts one uniform vocabulary across all 5 call sites.
- **Refactored all 5 fatal paths** (`terminalizeStartFatal`, `terminalizeAnswerInteractionFatal`, `terminalizeSubmitUserIntentFatal`, `terminalizeCancelSessionFatal`, `terminalizeExpireTimerFatal`) to call `materializeRuntimeFailure`, each now taking `failureKind, errorCode, errorMessage` plus `baseTurnID`/`attemptedSequence` (nil/1 for Start's pre-first-Turn case; the caller's already-loaded current Turn's id/`Sequence+1` otherwise - no new query added). Each of the 5 `xxxRepoAPI` interfaces gained the one new `CreateRuntimeFailure` method (mocks regenerated via `mockgen`/`go generate`). `terminalizeCancelSessionForced` (host-forced, not a runtime failure) and every authored-completion "if terminated" branch were **not** touched - neither creates a `session_runtime_failures` row, per Approved Design.

Local implementation decisions:
- `classifyExecutionError` returns `RuntimeFailureErrorCodeUnrecognized` for the (currently unreachable in normal fatal-path use) case where `errors.As` fails to extract an `*engineservice.ExecutionError` - purely defensive, matches the constant's own documented purpose.
- Start's own fatal path can be reached by an `ErrSignalRejected`-shaped error too (Start treats *any* `StartTurn` error as fatal, unlike every other step, which is Start's own pre-existing, unchanged design) - `classifyExecutionError` handles this gracefully via its defensive fallback rather than assuming every fatal-path error is an `*ExecutionError`.

Deviations from the approved WORK:
- None.

Discoveries:
- None.

Verification performed:
- `go build ./...`, `go vet ./...` clean repository-wide.
- `gofmt -l` clean for every new/changed file.
- `go test ./game/session/... -count=1` - every mocked-collaborator and real-Postgres repository-integration/concurrency test passes, including this WORK's own new assertions extending the existing Start/AnswerInteraction/SubmitUserIntent fatal-path integration tests (session_runtime_failures row content: `failure_kind`/`error_code`/`base_turn_id`/`attempted_sequence`/`source_kind`/`source_interaction_id`/`actor_id`) and new zero-row regression assertions on SubmitUserIntent's game-completion test and both of CancelSession's host-forced-terminal tests (AC6). New `runtime_failure_test.go` unit-tests `mapExecutionErrorCode` (every reachable engine code maps to a distinct, non-`Unrecognized` string; `ExecutionErrorStepChainExceeded` maps to exactly `runtime_turn_step_limit_exceeded`) and `classifyExecutionError`. Real Postgres was reachable this session (`playhoot-postgres-1` via the repo's own `docker-compose.yaml`) - every repository-integration/concurrency test ran for real, none skipped.
- CancelSession's/ExpireTimer's own `terminalizeXFatal` execution-error branch (post-`AdvanceTurn`, non-rejection) has no independent integration test forcing it, matching this Project's own already-accepted practice recorded in `step_cancel_session_integration_test.go`'s `already_terminal_for_a_different_reason_is_still_an_idempotent_no_op` test ("AC8 itself... is not independently forceable through a Definition fixture alone... verified by code review: `terminalizeCancelSessionFatal` is structurally identical to the already-reviewed `terminalizeSubmitUserIntentFatal`/`terminalizeAnswerInteractionFatal`") - both now call the exact same shared `materializeRuntimeFailure`, making that structural-identity argument stronger, not weaker.

Documentation synchronized:
- `game/README.md` - status line updated to implemented, with the two narrowings noted.
- `game/CURRENT_STATE.md` - Session Runtime capability row, Current Gaps, and Evidence bullets updated.
- `game/docs/DATA_MODEL.md` - new `session_runtime_failures` class diagram/prose/relationship lines.
- `GAME-ADR-0017`/`GAME-ADR-0019` - new "Implemented by" addenda.

Known limitations:
- None beyond the CancelSession/ExpireTimer fatal-branch test-coverage limitation noted above, itself a pre-existing, already-accepted Project practice, not new to this WORK.

Ready for independent review:
YES

## Fix Pass (2026-09-26, post independent review)

Independent review verdict: CHANGES_REQUIRED. One REQUIRED_FIX: AC2 (the `RUNTIME_STATE_INVALID`/recompile-failure path) had no test proving a `session_runtime_failures` row was persisted for any of the 5 paths - a genuine, undisclosed gap (the Implementation Report's "Known limitations" named only the separate, already-accepted CancelSession/ExpireTimer execution-error-branch gap, which read as if AC2 were otherwise covered). Fixed: added `fatal_recompile_failure_terminalizes_with_state_invalid_reason` to `step_start_integration_test.go`, reusing the existing `uncompilableDefinitionForTest()` fixture (already used by Create's own compile-rejection tests) via `stubStartPinnedGameReader`, proving `failure_kind = RuntimeFailureKindStateInvalid`, `error_code = RuntimeFailureErrorCodeDefinitionRecompileFailed`, `base_turn_id = NULL`, `attempted_sequence = 1`, `source_kind = SESSION_START`, and a non-empty compiler-diagnostic `error_message` - all atomic with the Session's `TERMINAL`/`RUNTIME_STATE_INVALID` transition. Re-verified: `go build ./...`/`go vet ./game/session/...` clean, `gofmt -l` clean, `go test ./game/session/... -count=1` against real Postgres - all pass, including the new test. The one NON_BLOCKING finding (CancelSession's/ExpireTimer's own execution-error branch has no independent integration test, a pre-existing, already-accepted Project limitation predating this WORK) was left as-is per its disposition - NON_BLOCKING findings are not required for DONE.

## Independent Re-Review (2026-09-26)

Verdict: APPROVED, no findings. Verified the fix pass against real evidence (not the Fix Pass narrative alone): confirmed only the new test case plus this WORK file changed since the first review (via file mtimes, no other production/mock/migration/doc file touched); read the new test in full and traced it against the actual `step_start.go` code path it exercises (`diagnostics.HasErrors()` -> `terminalizeStartFatal` with `RuntimeFailureKindStateInvalid`/`RuntimeFailureErrorCodeDefinitionRecompileFailed`); confirmed the assertions are against the real persisted `session_runtime_failures` row, not vacuous; re-ran `go build`/`go vet`/`gofmt -l`/the full real-Postgres test suite - all clean, the new test passes, no regression across all 11 sibling `TestManagerStart_Integration` subtests. Confirmed the one remaining NON_BLOCKING finding (CancelSession's/ExpireTimer's own execution-error branch has no independent integration test - a pre-existing, already-accepted Project limitation) was correctly left unresolved, since NON_BLOCKING findings do not block APPROVED.

## Completion Record

**DONE (2026-09-26).** `session_runtime_failures` (GAME-ADR-0017) is implemented, populated atomically alongside the Session's `TERMINAL` transition by every RuntimeTurn-producing path's fatal branch (Start/AnswerInteraction/SubmitUserIntent/CancelSession/ExpireTimer), through one shared `(*Manager).materializeRuntimeFailure` helper consolidating what were previously 5 independently-duplicated call sites - this also resolves this WORK's own originally-recorded "shared mechanism vs. per-path" open question, as a local implementation decision. `error_code` is a Session-Runtime-owned stable string catalog, decoupled from the engine's own `ExecutionErrorCode` int (mirrors WORK-0029's precedent). The GAME-ADR-0019 terminal-cleanup invariant was found already fully implemented before this WORK began - this WORK verified/consolidated it rather than newly building it. Two narrowings from GAME-ADR-0017's aspirational field list, both human-confirmed before READY: no `failed_step_index` column (the engine's contract cannot populate it); `diagnostic_payload` is a minimal forward-compatible envelope. Independent review: one fix/re-review round - first round found a genuine AC2 test-coverage gap (fixed, verified against real Postgres); re-review APPROVED with no findings. Documentation synchronized: `game/README.md`, `game/CURRENT_STATE.md`, `game/docs/DATA_MODEL.md`, `GAME-ADR-0017`/`GAME-ADR-0019` "Implemented by" addenda. Known, pre-existing, already-accepted limitation (not introduced by this WORK): CancelSession's/ExpireTimer's own deterministic-execution-error branch has no independent integration test forcing it (NON_BLOCKING).
