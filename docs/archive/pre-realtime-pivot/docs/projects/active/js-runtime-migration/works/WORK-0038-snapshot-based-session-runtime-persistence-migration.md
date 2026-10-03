# WORK-0038: Snapshot-Based Session Runtime Persistence Migration

Status: DONE
Created: 2026-09-27
Last status change: 2026-09-29 (IMPLEMENTING -> DONE; independent review APPROVED, no findings requiring a fix)

Related decisions:
- `session/docs/decisions/SESSION-ADR-0025-snapshot-based-session-runtime-persistence.md`
- `session/docs/decisions/SESSION-ADR-0023-replay-first-session-runtime-persistence.md` (superseded in part by the above)

Canonical context:
- `session/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md`
- `docs/projects/completed/game-language-flat-execution-model/works/WORK-0028-engine-owned-turn-execution.md` (the implementation this WORK reverses the persistence-model portion of)
- `game/session/workflows/sessionlifecycle/internal/replay/replay.go` (current replay-based reconstruction being replaced as the live-correctness path)

## Outcome

Implement `SESSION-ADR-0025`: reintroduce a persisted current-state representation as the authoritative record for Session continuation (crash recovery, disconnect/reconnect, any operation needing "what is the Session's state right now"), so live correctness no longer depends on replaying the durable signal log. Retain the durable signal/event log's existing tables, repurposed as a best-effort reconstruction/audit input only. This is foundational for `WORK-0040`–`WORK-0043` (all of which read/write current state) and for `session-runtime-v1`'s own `WORK-0017` (Archival), which this WORK's design must coordinate with.

**Scope addition (2026-09-27, discovered while drafting `WORK-0035`):** this WORK also switches every one of Session Runtime's seven RuntimeTurn-capable call sites (`step_create.go`, `step_join.go`, `step_start.go`, `step_answer_interaction.go`, `step_submit_user_intent.go`, `step_cancel_session.go`, `step_expire_timer.go`) from `engineservice.Compile`/`StartTurn`/`AdvanceTurn` to the new execution boundary, rather than leaving that switch as an unassigned task. It is folded in here rather than given its own WORK because this WORK already must touch every one of those call sites to change what they persist; doing the engine-call swap in the same pass avoids touching each file twice.

**Revised (2026-09-27, per `docs/decisions/architecture/ADR-0016-javascript-executor-separately-deployed-infrastructure-service.md`):** those seven call sites must be switched to `WORK-0053`'s `Executor` port, not directly to `WORK-0035`'s package (`game/session/internal/jsengine`) — that package's direct local invocation is superseded by `ADR-0016`; see `WORK-0035`'s own Completion Record. This WORK now depends on `WORK-0053` in addition to `WORK-0035`.

## Context

This is an explicit architectural reversal of a fully implemented, independently reviewed, DONE migration (`WORK-0019`, implementing `SESSION-ADR-0023`) — per `SESSION-ADR-0025`'s own Consequences, that prior WORK/ADR text is not rewritten (Historical Immutability).

**Why this is structurally required, not optional polish:** the already-built `Executor` port (`session/internal/executor/executor.go`, DONE via `WORK-0035`/`WORK-0052`/`WORK-0053`) has no reconstruction mechanism at all — `Execute(ctx, ExecutionInput{..., PreviousState json.RawMessage, ...})` requires the caller to already hold current state in hand and pass it in explicitly; there is no analog of today's "replay the signal log to rebuild it" step anywhere in that contract. Today, **zero** `sessionlifecycle` call site invokes this port (confirmed: no caller besides tests exists in the repository). Today's actual mechanism (`session/workflows/sessionlifecycle/internal/replay/replay.go`'s `LoadPriorSignals`/`loadReplaySignal`, feeding `engineservice.AdvanceTurn`, which internally replays every prior signal against the compiled Game-Language program on every single call) is entirely Game-Language-specific and has no equivalent under the new contract. This WORK is therefore the thing that makes the JS execution boundary actually callable, not an independent persistence-model cleanup that happens to be scheduled nearby.

**Current replay-first mechanism (being replaced), confirmed from the actual code:**
- `session_runtime_turns` carries no state of any kind — pure ordered replay-input envelope (`id, session_id, sequence, source_kind, source_interaction_id, source_timer_obligation_id, source_cause_event_id, actor_id, created_at`).
- Every RUNNING-phase step (`step_start.go`, `step_answer_interaction.go`, `step_submit_user_intent.go`, `step_cancel_session.go`, `step_expire_timer.go`) follows the identical pattern: read the pinned definition via `pinnedGameReader.GetGameDefinition` -> `engineservice.Compile` (fresh, every call) -> (except Start) `replay.LoadPriorSignals` (loads `session_runtime_starts` + every `session_runtime_turns` row + whichever satellite table each row's `source_kind` points at, rebuilding each prior `engine.Signal`) -> `engineservice.AdvanceTurn(compiledProgram, input, priorSignals, newSignal, limits)`, which internally re-executes every prior signal before applying the new one. Start alone calls `engineservice.StartTurn` instead (no prior signals to replay).
- `engineservice.ErrReplayDivergence` is treated as a hard, unrecoverable data-integrity error on every call site today — exactly the failure mode `SESSION-ADR-0025` judges unacceptable once the engine is only best-effort-deterministic (sandboxed JS, not a closed DSL).
- `step_join.go` never executes the engine at all (LOBBY-phase only; reads the pinned definition only for `players.Max`).
- `SESSION-ADR-0025` (ACCEPTED) decides *that* current state must be persisted and be authoritative and *that* the signal/cause log is retained for reconstruction-only, but explicitly leaves the exact persisted-state column/table shape as this WORK's own design task — nothing below is dictated by the ADR beyond that.

## Scope

### In Scope

- A persisted current-state representation, authoritative for every operation needing "what is this Session's state right now" (see Approved Design).
- Switching every RUNNING-phase call site (`Start`, `AnswerInteraction`, `SubmitUserIntent`, `CancelSession`, `ExpireTimer`) from `engineservice.Compile`/`StartTurn`/`AdvanceTurn` to `session/internal/executor`'s `Executor` port, using `WORK-0039`'s Event vocabulary to build `ExecutionInput.Event` and its Command vocabulary (`ParseCommand`) to interpret `ExecutionOutput.RequestedCommands`. **Revised (2026-09-28, per `WORK-0039`'s vocabulary revision):** `AnswerInteraction` and `SubmitUserIntent` both now drive the identical `PLAYER_EVENT` Event — there is no longer a platform-level "open interaction" a response answers, only a game-defined player action. Whether `Manager`'s own public method surface collapses to one method or keeps two thin ones that both build a `PLAYER_EVENT` is this WORK's own call-site design decision (Implementation Freedom), not decided here; the wire vocabulary is fixed by `WORK-0039` regardless.
- Deciding `session_interactions`' own fate: with the platform's `OPEN_INTERACTION` concept retired (`WORK-0039`), the durable "an interaction is open/closed, of kind QUESTION/ASK_GROUP" model this table encodes no longer has a platform-level reason to exist as designed. This WORK must decide whether it is dropped outright (a `PLAYER_EVENT`'s own idempotency/dedup can likely reuse the existing `session_requests`-based claim mechanism already used by `Create`/`SubmitUserIntent`/`CancelSession`, the same way, rather than the interaction-row-state dedup `AnswerInteraction` uses today) or repurposed for something narrower — flagged as part of this WORK's own design, not pre-decided here.
- Switching each of those same call sites' pinned-artifact read from `gamePinnedDefinitionReader`/Game Management's `game_definitions` to Session Runtime's own `session_game_version_artifacts.backend_script` (already persisted by `WORK-0034`, currently read by nothing).
- Retaining `session_runtime_turns`/`session_interactions`/`session_timer_obligations`/`session_cause_events`/`session_runtime_starts` exactly as their own normalized durable-cause homes, now for reconstruction/audit purposes only, never the live-correctness path.
- Completing `ADR-0014`'s deferred Game-Language-internalization goal, now that this WORK is what removes Game Management's last real dependents on it: dropping `game_definitions`/`game_definition_histories` (tables + migrations), removing `program.Definition`/`VersionUUID` from `game.Game` and deleting the now-fully-orphaned `getgame.GetPlayableGameWithCurrentVersion`/`getgamedefinition` use cases, and deleting `game/language/v1/...` (`program`, `engine`, `engineservice`, `gameservice`) outright — per `GAME-ADR-0028`, retired, not relocated, since nothing calls it live once this WORK lands (Project Completion Criteria #3 and #6).

### Out of Scope

- Actually dispatching a parsed Command to its owning mechanism: timer scheduling/cancellation persistence (`WORK-0040`), per-player view computation (`WORK-0041`), effects delivery (`WORK-0042`), durable confirmed-turn-result delivery (`WORK-0043`). This WORK makes `Execute` callable and its output persisted/parsed; those WORK consume the parsed Commands.
- Redesigning `WORK-0017`'s (Archival, `session-runtime-v1`) own archive-payload shape — flagged as needing joint resolution (see Blockers), not resolved here.
- The eventual best-effort reconstruction/audit feature itself (replaying the retained signal log for a diagnostic rebuild) — `SESSION-ADR-0025` names this as a future capability, not this WORK's own deliverable.

## Approved Design

**Persisted state placement (proposed):** add `session_runtime_turns.new_state JSONB NOT NULL` — the authoritative state immediately after that Turn committed, literally `ExecutionOutput.NewState` stored opaque (Session Runtime never decodes/interprets it beyond passing it through). "Current state" for any operation is simply `session_runtime_turns.new_state` for the row `sessions.current_turn_id` already points at (existing column, existing logical pointer) — no second table to keep in sync, matching `SESSION-ADR-0025`'s own wording ("`session_runtime_turns`... gains back a persisted state/snapshot representation").

**Start's own two-entry-point split retires (proposed):** rather than keeping `StartTurn` as a distinct call, Start becomes an ordinary `Execute` call like every other step: a `SESSION_STARTED` Event (`WORK-0039`'s vocabulary), `PreviousState = null`, letting the authored script construct its own initial state as its reaction to that event — the same role `WorkflowStarted` played, just no longer a structurally distinct engine entry point.

**Every RUNNING-phase step's new pattern:**
1. Read the pinned artifact's `backend_script` from `session_game_version_artifacts` (by `sessions.game_definition_uuid`) — never `game_definitions`.
2. Read current state: `session_runtime_turns.new_state` for `sessions.current_turn_id` (Start: `PreviousState = null`, no prior Turn).
3. Build the step's Event via `WORK-0039`'s vocabulary.
4. Call `Executor.Execute(ctx, ExecutionInput{Script: {Source: backendScript}, PreviousState: currentState, Event: event, Context: {LogicalTime, RandomSeed, ActingActor}})`.
5. `*executor.ScriptRejectedError` -> the step's existing ordinary-decline path (no new Turn) — the same role `ErrSignalRejected`/`ErrInputRejected` played. `*executor.ExecutorError` -> the step's existing fatal path (`session_runtime_failures`, same as a compile failure does today).
6. On success: `WORK-0039`'s `ParseCommand` over every element of `RequestedCommands` (a parse/validation failure is itself treated as `ScriptRejectedError`-equivalent, per `WORK-0039`'s own Acceptance Criteria); persist a new `session_runtime_turns` row carrying `new_state = output.NewState`; dispatch each parsed Command to whichever mechanism owns it (`WORK-0040`–`WORK-0043`, out of this WORK's own scope beyond making a typed Command available); update `sessions.current_turn_id`; renew/advance lifecycle deadlines and terminal cleanup exactly as today, now driven by a `SESSION_COMPLETE`/`SESSION_FAIL` Command rather than an engine-internal completion signal.

**`ErrReplayDivergence` and `replay.go`/`engineservice`'s fate:** replay is no longer any part of the live-correctness path — there is nothing to diverge from during a live call, since `PreviousState` is read directly, never reconstructed. `replay.go`'s mechanism is retained only as raw input for the future, not-yet-implemented best-effort reconstruction/audit capability `SESSION-ADR-0025` names; `engineservice`/`game/language/v1/...` themselves are deleted outright once every call site stops needing them (see Scope), since `GAME-ADR-0028` retires them rather than keeping a parallel path.

## Constraints and Invariants

- The persisted state representation is authoritative; a future reconstruction attempt from the retained signal log diverging from it is a diagnostic finding, never a live-correctness failure.
- Must not silently break `session-runtime-v1`'s already-DONE work built against the replay-first model — re-audited per-WORK below, not assumed unaffected:
  - `WORK-0019` (defines the model being reversed) and `WORK-0012` (timers: adds a `TIMER_EXPIRED` case to `loadReplaySignal`, relies on replay for reload-before-advance) and `WORK-0010` (user intent: same reload pattern, plus argument validation against `program.Definition`/`engine.Type` — moot once the platform no longer validates a game-specific intent shape at all, since `PLAYER_EVENT.name`/`.payload` are opaque to Playhoot by `WORK-0039`'s own design; any such validation is now entirely the authored backend script's own job, not Session Runtime's) and `WORK-0011` (cancellation: its reject-path "no Turn, no cause event" asymmetry is explicitly justified today by "replaying a rejected signal would corrupt reconstruction" — that specific justification stops applying once nothing is ever replayed live again; whether "reject -> no committed Turn" is still the right behavior for an independent reason needs re-confirming, not silent carry-forward) — each needs its call site rewritten per Approved Design above, not merely "unaffected." `WORK-0010`/`WORK-0011` (`SubmitUserIntent`/`CancelSession`) both now build a `PLAYER_EVENT`/platform-fact Event per `WORK-0039`'s collapsed vocabulary, not their own distinct Signal kinds.
  - `WORK-0014` (failure diagnostics: `diagnostic_payload`'s minimal `{"schema_version":1}` shape and `mapExecutionErrorCode` are keyed to the current engine's specific error codes; both need re-auditing against whatever failure shape the Executor/sandbox boundary actually returns).
  - `WORK-0016`/`WORK-0030` (inactivity expiration): explicitly decoupled from the replay/state model already (touches `sessions.activity_expires_at` directly, never `session_runtime_turns`) — lowest-risk of the six, re-audit only its "RuntimeTurn-producing operation" trigger list for renaming, no mechanism change expected.

## Acceptance Criteria

- `session_runtime_turns` gains a `new_state` column (or the implementer's chosen equivalent placement — see Implementation Freedom), populated by every RuntimeTurn-producing call site.
- `Start`/`AnswerInteraction`/`SubmitUserIntent`/`CancelSession`/`ExpireTimer` each call `session/internal/executor`'s `Executor` port, not `engineservice.Compile`/`StartTurn`/`AdvanceTurn`.
- No call site loads current state via `replay.LoadPriorSignals`/`loadReplaySignal` on its live path; that mechanism remains only as (unused-until-a-future-WORK) input to a not-yet-implemented reconstruction feature.
- `game_definitions`/`game_definition_histories` no longer exist; `game.Game` carries no `Definition`/`VersionUUID` field; `game/language/v1/...` no longer exists in the repository; `go build ./...` succeeds with zero import of it from anywhere.
- `go test ./...` clean; existing tests for `WORK-0010`/`WORK-0011`/`WORK-0012`/`WORK-0014`'s own acceptance criteria re-verified against the new call pattern (rewritten, not merely left passing by accident).
- A test proving persisted `new_state` matches what a live `Execute` call actually produced (the snapshot-based analog of `WORK-0019`'s own replay-divergence test, inverted: prove no silent drift between "what was persisted" and "what the Executor returned").

## Implementation Freedom

- Exact column name/placement for persisted state (`session_runtime_turns.new_state` is proposed, not frozen) and exact migration sequencing for dropping `game_definitions`.
- Exact mechanics of deleting `game/language/v1/...` (one commit vs. staged).

## Verification

- `go test ./...` across every touched package.
- Real-Postgres integration coverage for the new `new_state` column/read path, per this codebase's established convention.
- Independent review per `docs/ai/protocols/IMPLEMENTATION_REVIEW.md`, given this WORK reverses an accepted persistence model and removes a cross-domain dependency.

## Documentation Impact

### Accepted / Canonical Knowledge

- `session/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` — replaces the replay-first sections with the snapshot-based model (its own Status line already flags it as stale pending this WORK).
- `session/README.md`/`game/README.md` — Session Runtime Turn And Persistence Model, Process-Agnostic Recovery sections updated; `game/README.md`'s "Does Not Own" section drops its Game-Language caveat entirely once this WORK lands.
- `ARCHITECTURE.md` — `ADR-0014`'s "not yet complete" annotation removed; Project Completion Criteria #3/#6 satisfied.
- `docs/decisions/architecture/ADR-0014-management-session-domain-split.md`'s header / `game/docs/decisions/GAME-ADR-0028`'s Consequences — annotated as fully realized.

### Current-State Documentation After Implementation

- `session/docs/DATA_MODEL.md` — `new_state` column added to `session_runtime_turns`; `game_definitions`/`game_definition_histories` rows removed from `game/docs/DATA_MODEL.md` entirely.

## Blockers

None remaining — all resolved by explicit human decision (2026-09-28), see Material Decisions below. Coordination with `session-runtime-v1`'s `WORK-0017` proceeds in parallel (neither WORK has started implementation), flagged in both Projects' own files, not a sequencing block. Still depends on `WORK-0039` implementing first (needs its Event/Command vocabulary to build the actual `ExecutionInput.Event`/parse `RequestedCommands`), but `WORK-0039` itself has no unresolved material decision either.

## Material Decisions — Resolved (2026-09-28, explicit human decision)

1. **Persisted-state column placement**: confirmed — `session_runtime_turns.new_state`, as proposed.
2. **Retiring Start's `StartTurn` two-entry-point split**: confirmed — one `Execute` call for every Turn, including the first, as proposed.
3. **`WORK-0011`'s reject-path asymmetry** ("no Turn on a rejected `SessionCancelled`"): **kept as-is** — a rejected event didn't change state, so there is no real transition to record; this holds for an independent reason (nothing happened) even though the original replay-corruption justification no longer applies. A `PLAYER_EVENT`/`SESSION_CANCELLED` rejection continues to produce no `session_runtime_turns` row, only (where applicable) `sessions.terminal_reason`.
4. **Sequencing against `session-runtime-v1`'s `WORK-0017`**: proceed in parallel — neither WORK has started implementation; flagged in both Projects' own files rather than blocking either on the other.
5. **`game_definitions`/`game_definition_histories`/`game/language/v1/...` removal**: confirmed as this WORK's own closure (see Scope/Acceptance Criteria above) — the original request this whole design thread started from.

No material decision remains unresolved. Ready for implementation once `WORK-0039` lands.

## Material Decisions — Resolved (2026-09-29, explicit human decision, discovered during implementation)

6. **`session.Output`/`session.Value`/`session.Type` (the Game-Language-shaped push-effect/presentation mirror types)**: retired outright, not preserved as an always-empty compatibility shell. Confirmed zero live callers outside this package's own tests. `WORK-0041`/`WORK-0042`/`WORK-0045` will define the new architecture's own two separate concepts (a pull-based `ClientState` from `project(...)`, and transient game-defined events via `SEND_EVENT`) fresh, not as a relocation of this type family.
7. **Structural participant min/max enforcement**: kept as a platform-level gate, not delegated to authored JavaScript. `ParticipantConstraints{Min, Max}` added to the Game Version Artifact model (`session/docs/GAME_VERSION_ARTIFACT_MODEL.md`, amending `WORK-0044`'s own already-DONE record via an addendum, not a rewrite) and persisted as `session_game_version_artifacts.participant_min`/`participant_max` (`Max` nullable, meaning unlimited). Authored JavaScript remains free to add further game-specific participant rules inside this structural bound.
8. **`session_interactions`' fate** (this WORK's own delegated design freedom, per Scope): dropped outright, not repurposed. The platform's closed Event/Command vocabulary has no `OPEN_INTERACTION` concept at all; a `PLAYER_EVENT`'s own idempotency/dedup reuses the `session_requests`-based claim mechanism `SubmitUserIntent`/`CancelSession`/`Create` already used, exactly as this WORK's own Scope suggested.
9. **`AnswerInteraction`/`SubmitUserIntent` method-surface collapse** (this WORK's own delegated Implementation Freedom): collapsed into one `Manager.SubmitPlayerEvent(sessionUUID, userUUID, name, payload, idempotencyKey)` method, since `session_interactions`' retirement leaves `AnswerInteraction`'s own `interactionUUID` parameter with nothing to resolve against.

## Completion Record

**Status: IMPLEMENTING, not DONE — independent review per `docs/ai/protocols/IMPLEMENTATION_REVIEW.md` has not yet run.**

### Implemented

- `session_runtime_turns.new_state` (opaque, Executor-returned state) added; is now "current state" for `sessions.current_turn_id` — no reconstruction step anywhere.
- `Start`/`SubmitPlayerEvent` (replacing `AnswerInteraction`+`SubmitUserIntent`)/`CancelSession`/`ExpireTimer` all call `session/internal/executor`'s `Executor` port exclusively; `Join`/every RUNNING-phase step resolve the pinned artifact from `session_game_version_artifacts` only, never Game Management.
- `platform.ParseCommand`/`ParseEvent` (`WORK-0039`) wired at every call site; a parse/validation failure is treated as an authored-script defect (ordinary decline), never an infrastructure failure.
- `internal/completion` rewritten to detect `SESSION_COMPLETE`/`SESSION_FAIL` from parsed `[]platform.Command` (replacing `engine.RunCompletedOutput` detection).
- `ParticipantConstraints{Min, Max}` added to the artifact model and enforced by `Join`/`Start` before any script executes.
- `session_interactions` (table, repo methods, capturer package) and the old `session.Output`/`Value`/`Type` mirror + `mapOutputs`/`internal/clientoutputs` translation layer deleted outright.
- `game_definitions`/`game_definition_histories` tables and `games.current_definition_id` dropped; `game.Game` carries no `Definition`/`VersionUUID`; `game/usecases/getgame`/`getgamedefinition` (confirmed fully orphaned — zero external callers) deleted; `game/language/v1/...` (`program`/`engine`/`engineservice`/`gameservice`, ~140 files) deleted outright.
- `internal/replay` (the live-path replay-reconstruction package) deleted; its former source-kind constants moved to a small `sourcekind.go`.
- New migrations: `session_runtime_turns.new_state`; `session_game_version_artifacts.participant_min`/`participant_max`; drop `session_interactions` (+ its `session_runtime_turns`/`session_runtime_failures` FK columns); drop `game_definitions`/`game_definition_histories`/`games.current_definition_id`.
- `runtime_failure.go`'s error-code catalog collapsed to the one stable code an `*executor.ExecutorError` can produce (`RuntimeFailureKindStateInvalid`/the old `engineservice.ExecutionErrorCode` catalog retired — no compile step exists under the new model, so there is no equivalent failure mode).

### Local Implementation Decisions (reported, not hidden)

- Session-side `RandomSeed` for each `Execute` call is drawn fresh via `crypto/rand` per call, not derived to reproduce a specific prior run — `SESSION-ADR-0025`'s own Alternatives Considered leaves strict deterministic replay explicitly open/not-required, and nothing depends on it.
- `ExpireTimer` passes `session_timer_obligations.engine_key` straight through as `TimerExpired.Data` (opaque bytes) rather than decoding it — the old engine-Value codec it was decoded through no longer exists, and no call site currently creates an obligation with a non-nil `engine_key` (dispatching `SCHEDULE_TIMER` remains `WORK-0040`'s own future scope, unchanged).
- `KnownActors` at each call site is built from `ListActiveParticipantsForRoster` (every currently-active Participant) — the same roster-resolution query `Start` already used, reused as the natural "this Session's addressable actors" set for `ParseEvent`/`ParseCommand`.

### Deviations from the approved WORK

- None beyond the two Material Decisions (6, 7) explicitly reported above, both discovered as genuine gaps in the approved design during implementation and resolved by explicit human decision before proceeding, not invented.

### Discoveries

- **Resolved before implementation** (see Material Decisions 6–9 above): the fate of `session.Output`/`Value`/`Type`, and the missing participant-capacity data source, were both reported as DISCOVERY and resolved by the human before the call-site rewrite continued.
- **New, unrelated to this WORK, found via real-Postgres verification**: `TestManagerJoin_Integration_ConcurrentOperationRacingLobbyExpiration` (`step_join_integration_test.go`) fails deterministically against a real Postgres instance — confirmed present on the pre-WORK-0038 baseline too (verified via `git stash`), so it is not a regression this WORK introduced. Root cause: `Join`'s own unlocked pre-lock `ResolveSessionForJoinCode` read can lose a race against a concurrent `Leave`'s lazy lobby-expiration materialization (which revokes the active JoinCode as part of its own transaction), producing `ErrJoinCodeInvalid` instead of the test's expected graceful `LobbyExpired` outcome. This is `Join`/`Leave`/lazy-lobby-expiration logic entirely outside this WORK's own scope (owned by `session-runtime-v1`), never previously exercised against a real database in this repository's history (every prior sandbox lacked reachable Postgres/Docker) — flagged here as a genuine, newly-surfaced pre-existing issue for `session-runtime-v1` to triage, not fixed by this WORK.
- **Pre-existing, unrelated**: `TestNoInternalDocCitationsInComments` still fails against `session/internal/storage/migrations/20260926000000_session_runtime_failures.go` (a file this WORK never touched, dated to the original `WORK-0014` implementation) — confirmed via `git diff` showing no changes to that file. This mirrors the same class of pre-existing, unrelated `comment_standard_test.go` failure `WORK-0034` already flagged and left unfixed as a separate follow-up.

### Verification Performed

- `go build ./...` / `go vet ./...` clean across the entire repository.
- `go test ./... -count=1`: clean except the one pre-existing, unrelated `TestNoInternalDocCitationsInComments` failure above (confirmed pre-existing via `git diff`).
- **Real-Postgres integration verification** (a genuine capability upgrade over this repository's own repeated "sandbox has no reachable Docker/Postgres" limitation — a reachable Postgres container was available this session): `go test ./... -count=1` against real Postgres passes for every test this WORK touched or added, including all rewritten `TestManagerStart_Integration`/`TestManagerSubmitPlayerEvent_Integration`/`TestManagerCancelSession_Integration`/`TestManagerExpireTimer_Integration`/`TestManagerJoin_Integration*`/`TestManagerCreate_Integration*` suites, concurrency/race tests, and the acceptance-criteria-required "persisted `new_state` matches what a live Execute call actually produced" assertions. The only real-Postgres failures are the two pre-existing, unrelated issues reported under Discoveries above (both confirmed present on the pre-WORK-0038 baseline).
- `gofmt -l` on every file this WORK touched: clean, except the repository's own known, pre-existing CRLF-vs-LF `gofmt` artifact already documented by prior WORK in this project (files are checked in with CRLF line endings under this repository's Windows-native convention; `gofmt`'s canonical output is LF-only) — not a real formatting defect, not introduced by this WORK.

### Documentation Synchronized

- `session/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` — "Replay-First Persistence Model" replaced with "Snapshot-Based Persistence Model"; `session_interactions` removed from the Runtime History Tables/Failure Diagnostics ER diagrams; Recovery/Process-Agnostic Recovery sections updated. Sections owned by `session-runtime-v1`'s own not-yet-built capabilities (archival, disconnect/reconnect authored processing, keyed-timer dispatch) are left as accepted-but-not-yet-implemented design, per this document's own long-standing convention, and still reference the retired model in places — flagged as a known remaining gap for that project to reconcile when it designs those capabilities against the new execution model, not fixed here.
- `session/README.md` — "Session Runtime Turn And Persistence Model", "Process-Agnostic Recovery", "Session Runtime Failure Classification And Diagnostic Persistence", "Terminal Cleanup", and the Lobby Lifecycle Contract's `players.max/min` references updated to the new model. The same caveat as above applies to this document's own disconnect/reconnect/archival/delivery sections, which remain `session-runtime-v1`'s own scope and were not rewritten.
- `game/README.md` — "Does Not Own" section's Game-Language caveat removed entirely.
- `session/docs/DATA_MODEL.md` — `new_state` added; `session_interactions` removed entirely; `session_game_version_artifacts.participant_min`/`participant_max` added.
- `game/docs/DATA_MODEL.md` — `game_definitions`/`game_definition_histories`/`games.current_definition_id` rows removed entirely.
- `ARCHITECTURE.md` — the ADR-0014/Game-Language "not yet complete" annotation removed.
- `docs/decisions/architecture/ADR-0014-management-session-domain-split.md` / `game/docs/decisions/GAME-ADR-0028-javascript-execution-replaces-game-language.md` — both annotated as fully realized via an appended dated addendum (their own accepted bodies left unrewritten, per this repository's ADR-immutability convention).

### Known Remaining Documentation Gaps (flagged, not fixed — outside this WORK's own explicit Documentation Impact list)

`session/CURRENT_STATE.md`, `session/docs/FLOWS.md`, `docs/architecture/SYSTEM_MAP.md`, and `docs/ai/KNOWLEDGE_MAP.md` still describe the retired Game-Language-based mechanism in detail (replay reconstruction, `session_interactions`, `AnswerInteraction`/`SubmitUserIntent`, `engineservice`). None of these were named in this WORK's own Documentation Impact section; they are flagged here for a follow-up documentation pass rather than silently rewritten under this WORK's own authorization.

### Known Limitations

- Real-Postgres verification was still sandbox-dependent on a Docker container happening to be available this session — a future session without one reverts to the documented graceful-skip behavior.
- `SEND_EVENT`/`SCHEDULE_TIMER`/`CANCEL_TIMER` platform Commands are parsed and validated at every call site but not dispatched to any owning mechanism (persisting a new timer obligation, delivering an effect) — explicitly out of this WORK's own scope, owned by `WORK-0040`/`WORK-0042`.

### Independent Review

APPROVED (fresh Codebase Agent, read-only, reasoning from the actual `git diff HEAD~1 HEAD` change-set, migrations, tests, and canonical docs, not from this Completion Record's own claims — confirmed the required "no drift between persisted `new_state` and what Execute returned" test genuinely exists and asserts the real property; independently re-verified both Discoveries above as true and pre-existing; confirmed the `ParticipantConstraints`/`session.Output` retirement decisions were implemented consistently and their amendments to `WORK-0044`/`ADR-0014`/`GAME-ADR-0028` used the addendum-not-rewrite convention correctly). No `REQUIRED_FIX`/`DECISION_REQUIRED` findings. Two `NON_BLOCKING` findings, not required for closure, left as follow-up: (1) `manager.go`'s package doc comment and `step_start_test.go`'s own comment still narrate the retired Game-Language-based mechanism (`engineservice`, `engine.Signal`, `Output`) despite the surrounding code no longer matching it; (2) `api/session/wire.go`'s pre-existing, untouched placeholder constants (`ANSWER_INTERACTION`, `INTERACTION_OPENED/CLOSED`) still name the retired interaction concept — confirmed out of this WORK's own scope (the whole `api/session` package is an untouched no-op transport stub with zero diff in this WORK), left for whichever future WORK actually wires the API layer.
