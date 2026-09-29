# WORK-0038: Snapshot-Based Session Runtime Persistence Migration

Status: DRAFT
Created: 2026-09-27
Last status change: 2026-09-28 (PLANNED -> DRAFT; initial design pass grounded in the actual current replay-first implementation and the already-built Executor contract, see Context)

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

- Must jointly resolve `session-runtime-v1`'s `WORK-0017` (Archival) redesign — its own archive-payload premise assumes replay-input-log-only content and must be redesigned to also carry final persisted state; flagged in that Project's own `PROJECT.md` Coordination Flag, not resolved here. Sequencing (does this WORK block on WORK-0017's redraft, or proceed and let WORK-0017 catch up, since it hasn't started implementation either) needs an explicit human call before READY.
- Depends on `WORK-0039` reaching READY first (needs its Event/Command vocabulary to build the actual `ExecutionInput.Event`/parse `RequestedCommands`).

## Material Decisions Needing Human Input

1. **Persisted-state column placement** (`session_runtime_turns.new_state`, proposed) vs. an alternative shape.
2. **Retiring Start's `StartTurn` two-entry-point split** in favor of one `Execute` call for every Turn, including the first — a real (if small) contract simplification, not merely an implementation detail.
3. **`WORK-0011`'s reject-path asymmetry** ("no Turn on a rejected `SessionCancelled`") — its original replay-corruption justification stops applying; confirm whether the same outcome is still wanted for an independent reason, or whether a rejected signal should now also commit a Turn (state genuinely didn't change either way, so the practical difference is only whether a "nothing happened" Turn exists in the log).
4. **Sequencing against `session-runtime-v1`'s `WORK-0017`** (Archival) — block on it, or proceed in parallel.
5. **Confirms the original ask this whole design thread started from**: yes, `game_definitions`/`game_definition_histories`/`game/language/v1/...` are fully removed as part of *this* WORK's own closure (not `WORK-0032`'s), once every call site no longer needs them — see Scope/Acceptance Criteria above.

## Completion Record

Not yet started.
