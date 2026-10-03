# WORK-0010: User Intent Runtime Path

Status: DONE
Created: 2026-09-20
Last status change: 2026-09-26 (independent re-review verdict APPROVED, no findings)

Related decisions:
- GAME-ADR-0018 (RUNNING serialization boundary, reload-after-lock, ordering-by-commit)
- GAME-ADR-0002 (Live Session Coordinator responsibility boundary)
- GAME-ADR-0024 (Replay-First Session Runtime Persistence - this WORK's UserIntent input must be durably, order-preservingly representable)
- GAME-ADR-0025 (Role-Aware Live Connections - UserIntent is a PARTICIPANT-only command)

Canonical context:
- `docs/projects/active/session-runtime-v1/PROJECT.md`
- `game/language/v1/program/signal.go` (`UserIntentSignalSource`), `game/language/v1/program/ui.go` (`EmitUserIntentAction`)
- `game/language/v1/engine/signal.go` (`SignalKindIntent`)
- `docs/projects/active/session-runtime-v1/works/WORK-0004-interaction-response-processing.md` (the existing RuntimeTurn/serialization pattern this WORK's runtime path reuses)
- `docs/projects/active/session-runtime-v1/works/WORK-0019-replay-first-session-runtime-persistence-migration.md` (owns the general replay-input model; this WORK is responsible for satisfying it for UserIntent specifically)
- `docs/projects/active/session-runtime-v1/works/WORK-0020-role-aware-live-connections.md` (the PARTICIPANT connection this WORK's live command rides)

## Outcome

A player can trigger an authored user intent (an unsolicited player action, not an answer to something Session Runtime already opened) and have it flow end-to-end: UI action -> transport command -> Session Runtime authorization/validation -> the correct runtime/workflow instance -> a RuntimeTurn -> persisted resulting state -> Presentation/Effect consequences.

`UserIntentSignalSource`/`EmitUserIntentAction`/`SignalKindIntent` are fully implemented, compiled, and engine-tested in Game Language today - but `grep "Intent"` across `game/session/`, `play/`, and `api/` returns zero matches. There is currently no way for a player to submit an intent through any live path; only answering an already-open interaction is wired (WORK-0004/WORK-0005).

## Context

This is a distinct runtime path from `AnswerInteraction`, not a variant of it: `AnswerInteraction` always targets a specific already-open `session_interactions` row Session Runtime itself opened. A user intent is unsolicited - the player initiates it, not Session Runtime - so there is no existing open-interaction row to correlate against. The open design question this WORK must resolve is routing context: an intent may need to target a particular workflow/runtime instance, and the client action needs enough context to route it correctly without exposing engine-internal slot/path identities (the same non-leakage principle WORK-0005/WORK-0006 already apply to Output translation).

## Scope

### In Scope (known required outcome; design not yet started)

- A `Manager`-level capability (parallel to `Start`/`AnswerInteraction`) that accepts a client-submitted intent, resolves it to the correct runtime/workflow instance, and drives a RuntimeTurn through the existing Step-draining/bound execution mechanism. This part needs only `sessionlifecycle.Manager` and no `play`/transport code.
- A live-transport command and Coordinator-side translation. **(2026-09-23 correction)** `play`/`play/sessionruntime` do not exist today - deleted in full by WORK-0005's Blocker 11 (2026-09-21) and not yet rebuilt; this part reuses whatever translation/fan-out shape `docs/projects/active/session-runtime-v1/works/WORK-0020-role-aware-live-connections.md` (re)builds, once it lands, not WORK-0005's original (deleted) implementation.
- Whatever Presentation/Effect consequences result, reusing WORK-0006's play-half fan-out mechanism (not yet drafted - see WORK-0006's own "Scope Correction (Domain/Play Split, 2026-09-23)") once it exists.

### Out of Scope

- Any change to Game Language's own intent compiler/engine support - already implemented and out of this WORK's scope.
- Manual Session cancellation (WORK-0011) - a distinct host-only operation, not a player intent.

## Drift Correction (2026-09-26): Routing-Context Question Resolved By GAME-ADR-0026

This WORK's stated "central open question" (how a client identifies which workflow/runtime instance an intent targets) predates GAME-ADR-0026 (accepted 2026-09-24, four days after this WORK was planned) and is resolved by it, by construction, not by any design decision made here. GAME-ADR-0026's Decision 7 states this exactly: "The previously open, never-actually-authorable question of 'which instance does a user intent target' is resolved by construction: there is only one. `program.EmitUserIntentAction` needs no addressing mechanism." `engine.Signal`'s own doc comment confirms it in code: "A Session runs exactly one workflow instance for its entire lifetime, so a Signal always targets that one instance - there is no addressing concept to resolve" (`game/language/v1/engine/signal.go`). There is nothing to design here; this section records the resolution so it is not re-opened.

## Scope Split (2026-09-26 drafting pass)

Per this Project's Restructuring and its established WORK-0006/0007/0012 precedent, this WORK now covers **only the domain half**: `sessionlifecycle.Manager`'s own new capability, requiring no `play`/transport code. The live-transport command and PARTICIPANT-only connection-layer authorization enforcement remain a not-yet-drafted play-half WORK depending on WORK-0020, per this Project's just-in-time practice - not designed here.

## Approved Design (2026-09-26)

**New Manager capability**, following the exact template `AnswerInteraction`/`ExpireTimer`/`Start` already establish (lock -> reload via replay -> compile -> `AdvanceTurn` -> persist -> capture -> map outputs):

```go
func (m *Manager) SubmitUserIntent(ctx context.Context, sessionUUID session.SessionUUID, userUUID session.UserUUID, intentName string, arguments json.RawMessage, idempotencyKey session.IdempotencyKey) (session.SubmitUserIntentResult, error)
```

Locks directly by `sessionUUID` (`sessionlock.LockByUUID`, the same shape `Start` uses - no separate unlocked resolve-by-a-different-UUID step is needed, unlike `AnswerInteraction`/`ExpireTimer`, which are addressed by an interaction/obligation UUID instead of the Session's own).

**Idempotency**: required (`session.ErrIdempotencyKeyRequired` if empty), via the existing shared `internal/idempotency` mechanism (`operationSubmitUserIntent = "SUBMIT_USER_INTENT"`), the same pattern `Start`/`Join`/`Leave` already use - not `AnswerInteraction`/`ExpireTimer`'s no-idempotency approach. Rationale: `AnswerInteraction`/`ExpireTimer` dedup for free against an already-existing target row's own mutable state (the interaction/obligation); `SubmitUserIntent` is genuinely unsolicited with no prior row to compare against, so nothing dedups a retried submission for free - a network retry without idempotency would double-submit a real gameplay action. This follows established precedent; it is not a new architectural decision.

**Step sequence** (mirrors `answerInteractionInTx`/`startSessionInTx`):
1. Decode `arguments` via `engineservice.DecodeValue` (empty/nil treated as `engine.RecordValue{}` for a zero-parameter intent) - same self-describing plain-JSON technique `answer` already uses (a JSON object keyed by parameter name).
2. `sessionlock.LockByUUID`; `idempotency.Claim` (same session-lock-then-claim ordering `Start` uses); an existing claim replays its persisted `SubmitUserIntentResult` outcome instead of re-executing.
3. Resolve the submitting actor via `FindActor(sessionID, userUUID)`, same as `AnswerInteraction`/`Start`; a missing actor is `SubmitUserIntentOutcomeRejected`, not a Go error (same "indistinguishable from unauthorized" reasoning `AnswerInteraction` already uses).
4. **Validate the intent name and arguments before ever calling the engine** (see "Argument Validation Boundary" below) - a validation failure completes the idempotency claim as `SubmitUserIntentOutcomeRejected`, never reaching `AdvanceTurn`.
5. Load pinned Definition, `engineservice.Compile` (recompile failure -> fatal terminalize, same as `AnswerInteraction`/`Start`).
6. `replay.LoadPriorSignals`; build `engine.Signal{Kind: engine.SignalKindIntent, Intent: intentName, Actor: engine.UserID(strconv...), Fields: <decoded RecordValue's fields as a map>}`.
7. `engineservice.AdvanceTurn`; `ErrReplayDivergence` -> alert + hard error; `ErrSignalRejected`/`ErrInputRejected` -> ordinary `SubmitUserIntentOutcomeRejected`; any other error -> fatal terminalize (`TerminalReasonRuntimeExecutionFailed`), identical to `AnswerInteraction`.
8. Persist the durable cause (see "Cause Persistence" below), then `CreateRuntimeTurn`, `interactions.Capture`/`timers.Capture`, `SetCurrentTurn`, `completion.Detect` + terminal cleanup, `mapOutputs`/`clientoutputs.ClientFacing` - identical tail to `AnswerInteraction`.
9. `idempotency.Complete` with the final outcome/marshaled result, mirroring `Start`.

**Cause Persistence (`session_cause_events`)**: this is the first capability to populate the table WORK-0019 provisioned but left unpopulated. Because `session_cause_events.runtime_turn_id` is `NOT NULL` with a 1:1 unique index (the row cannot exist before its owning Turn does - unlike `source_interaction_id`/`source_timer_obligation_id`, which reference rows that already existed *before* this Turn), persistence is three steps, not two:
1. `CreateRuntimeTurn(..., sourceCauseEventID: nil, ...)` - extend its existing signature with a fourth nullable id (`sourceCauseEventID *uint`), alongside `sourceInteractionID`/`sourceTimerObligationID`, mirroring the migration's own "third nullable cause pointer" framing. Every existing call site (`step_answer_interaction.go`, `step_expire_timer.go`, `step_start.go`) passes `nil` for it - mechanical, no behavior change.
2. `internal/repo/cause_event.go` (new, mirroring `timer_obligation.go`'s shape): `CreateCauseEvent(ctx, tx, sessionID, runtimeTurnID uint, causeKind string, actorID *uint, payload []byte) (uint, error)`; `GetCauseEventByID(ctx, tx, causeEventID uint) (*CauseEvent, error)` for replay. `causeKind = "USER_INTENT"`. `payload` is a small JSON envelope `{intent_name string; arguments []byte}` (`arguments` itself the `engineservice.EncodeValue`-encoded `RecordValue`, reusing `replay.EncodeRootParameters`'s exact technique rather than a second encoding).
3. A new `SetRuntimeTurnCauseEvent(ctx, tx, turnID, causeEventID uint) error` backfills the Turn's `source_cause_event_id` once the cause event row exists (parallel to how `CloseAnsweredInteraction`/`CreateTimerObligation` interleave with their owning Turn, just in the opposite order since this cause has no independent prior existence).

**Replay wiring** (`internal/replay/replay.go`): new `UserIntentSourceKind = "USER_INTENT"` constant; a new `loadReplaySignal` case requiring `turn.SourceCauseEventID != nil` (alert + hard error otherwise, matching the existing two cases' own defensive shape), loading the cause event by id, and `buildUserIntentSignal` decoding its payload envelope back into `engine.Signal{Kind: SignalKindIntent, Intent, Actor, Fields}` - the exact counterpart to `buildAnswerSignal`/`buildTimerExpiredSignal`.

**Argument Validation Boundary (genuine, scoped design decision - not left open, but flagged as a real residual limitation, see below):** `SignalKindIntent` is fully implemented/compiled/engine-tested per this WORK's own Context, and changing that engine/compiler support is explicitly Out of Scope. Investigation for this design found the engine does **not** validate a `SignalKindIntent` signal's `Fields` against the intent's declared `Parameters` at Step time (`signalSchemaFields`'s `SignalKindIntent` case copies `Fields` verbatim; `signalMatchesSource` only compares the `Intent` name) - unlike `SignalKindInteractionAnswered`, which the engine does validate against the question's declared response type. This is a pre-existing engine asymmetry, not introduced by this WORK, and every prior caller of `SignalKindIntent` supplied `Fields` as trusted Go literals (tests), never untrusted client JSON - this WORK is the first to change that. A structurally wrong-shaped/incomplete `Fields` map risks a nil-binding panic downstream in transition evaluation (a plain map read with no presence check), which would be a real availability problem once this path is live.
Because fixing the engine itself is out of scope, `SubmitUserIntent` validates defensively at this system boundary, within scope, using what is cheaply available without duplicating compiler internals:
- The intent name must exist in the pinned `program.Definition.UserIntents` (a simple name lookup Session Runtime can already do; unknown name -> `SubmitUserIntentOutcomeRejected`).
- The decoded arguments must be a `RecordValue` whose `Fields` include every declared `Parameter` name (presence check); a missing declared field -> `SubmitUserIntentOutcomeRejected`.
- For a declared `Parameter` whose `program.TypeReference` is a `BuiltinTypeReference` (user/bool/number/string), the corresponding field's value is checked with the engine's own exported `Value.Validate(Type)` (the same mechanism the engine itself uses internally for `InteractionAnswered`) - a mismatch -> `SubmitUserIntentOutcomeRejected`.
- **Residual, explicitly-recorded limitation**: a declared `Parameter` referencing a *named* type (record/union/enum/newtype, or a list/map/optional wrapping one) is not deeply validated here, since resolving a `program.TypeReference` to its compiled `engine.Type` requires the compiler's internal named-type resolution (`Types map[string]Type`), not exposed on `engine.Program` and out of this WORK's engine-scope boundary. A submission with a structurally-wrong named-typed argument therefore still risks the downstream panic described above. This is recorded as a genuine known limitation (see "Known Limitations" in the eventual Completion Record), not silently engineered around - a natural future engineering-standard/engine hardening candidate (exposing `Program.UserIntents` with compiled Parameter types, and/or the engine itself validating `SignalKindIntent.Fields` the same way it already validates `InteractionAnswered.Answer`) is worth flagging to `docs/engineering/ENGINEERING_RADAR.md`, not fixed here.

**Result shape** (`game/session/types.go`): `SubmitUserIntentOutcome` (`Accepted`/`Rejected`/`RuntimeExecutionFailed`, mirroring `AnswerInteractionOutcome`'s three non-replay-specific values - no `Answered`/`Conflict` analogue, since dedup is idempotency-key-based, not target-row-based) and `SubmitUserIntentResult{Outcome, SessionUUID, Outputs []Output `json:"-"`, TerminalReason string}`, matching `AnswerInteractionResult`'s shape/JSON tags exactly (`Outputs` excluded from idempotency-replay JSON, consistent with `AnswerInteractionResult`).

## Constraints and Invariants

- Must reuse the existing per-Session RUNNING serialization boundary (GAME-ADR-0018) - an intent-driven RuntimeTurn is subject to the same reload-after-lock/ordering-by-commit/Step-bound rules as `AnswerInteraction`.
- Must not expose engine-internal slot/path identities to the client. (The routing-context form this constraint originally anticipated is moot per the Drift Correction above; the constraint itself - no engine-internal leakage generally - still holds.)
- **(Part B reconciliation)** A submitted UserIntent's content (intent name, submitted arguments, actor) must be durably captured, in commit order, before or atomically with the RuntimeTurn it drives - satisfied by the `session_cause_events` design above. This WORK does not persist the resulting Presentation/Effect consequences for replay - those remain derived, per WORK-0006.
- **(Part J reconciliation)** UserIntent is a PARTICIPANT-connection command. This WORK's domain half enforces no connection-role check itself (`sessionlifecycle.Manager` has no notion of connection role) - PARTICIPANT-only enforcement happens at the connection layer, owned by the not-yet-drafted play-half WORK depending on WORK-0020, per the Scope Split above. Auth/Identity stays at WORK-0005's already-established trusted-`UserUUID` stance; this WORK adds no new authentication mechanism.
- No change to Game Language's own compiler/engine `SignalKindIntent`/`UserIntentSignalSource` support (already implemented, out of scope) - see the Argument Validation Boundary's own residual-limitation note for what this excludes.

## Acceptance Criteria

- AC1: `Manager.SubmitUserIntent` with a valid intent name, arguments matching every declared `Parameter`, and a matching pinned `program.Definition` commits a new RuntimeTurn via `engineservice.AdvanceTurn` with `engine.SignalKindIntent`, returns `SubmitUserIntentOutcomeAccepted`, and the Turn is durably replayable (a subsequent `replay.LoadPriorSignals` reconstructs the identical `engine.Signal`).
- AC2: An unknown intent name is rejected as `SubmitUserIntentOutcomeRejected` without reaching `engineservice.AdvanceTurn` (no RuntimeTurn created).
- AC3: Arguments missing a declared `Parameter`'s field, or a builtin-typed (`user`/`bool`/`number`/`string`) field whose decoded value fails `Value.Validate(Type)`, are rejected as `SubmitUserIntentOutcomeRejected` without reaching `engineservice.AdvanceTurn`.
- AC4: A caller not resolving to a current `session_actors` row for the target Session is rejected as `SubmitUserIntentOutcomeRejected`, mirroring `AnswerInteraction`'s actor-authorization handling.
- AC5: `engineservice.AdvanceTurn` returning `ErrSignalRejected`/`ErrInputRejected` (e.g. no matching transition/guard) yields `SubmitUserIntentOutcomeRejected` with no RuntimeTurn persisted; any other `AdvanceTurn` error terminalizes the Session (`TerminalReasonRuntimeExecutionFailed`), mirroring `AnswerInteraction`'s fatal path exactly.
- AC6: A retried call with the same `(UserUUID, "SUBMIT_USER_INTENT", IdempotencyKey)` replays the original persisted outcome without a second engine effect or a second RuntimeTurn; a same-key retry with a materially different request is a conflict, consistent with `Start`'s own idempotency-conflict handling.
- AC7: `CreateRuntimeTurn`'s extended signature (`sourceCauseEventID *uint`) does not change behavior at any existing call site (`step_start.go`, `step_answer_interaction.go`, `step_expire_timer.go`) - each continues to pass `nil` for it; full existing `sessionlifecycle` test suite passes unmodified.
- AC8: A game-completion Output produced by the same Turn terminalizes the Session under the matching `TerminalReasonGame*` value and closes other still-`ACTIVE` interactions/timer obligations, mirroring `AnswerInteraction`'s completion handling exactly.
- AC9: Effect/Presentation Outputs produced by the same Turn are returned via `SubmitUserIntentResult.Outputs`, mapped through the existing `mapOutputs`/`clientoutputs.ClientFacing` path, unchanged from how `AnswerInteraction` already does this.
- AC10 (real-Postgres, per this Project's established practice): a repository-integration test proves a `SubmitUserIntent`-driven RuntimeTurn survives process-loss reconstruction - `replay.LoadPriorSignals` against real Postgres rebuilds the identical `engine.Signal` a fresh execution would have produced, the same proof style `WORK-0019`/`WORK-0012` already established for their own new causes.
- AC11 (found during fix pass, see below): a `SubmitUserIntent` call against a Session already `TERMINAL` (including via the fatal `RuntimeExecutionFailed` path, which does not itself mark the underlying engine instance Completed) is rejected without reaching `AdvanceTurn` again and without committing a further RuntimeTurn.

## Blockers

None remaining for the domain half. (The residual argument-validation limitation is a recorded, accepted-scope limitation, not a blocker - see Approved Design.)

## Scope Addition (Part B/J Reconciliation, 2026-09-20)

A broader reconciliation session accepted GAME-ADR-0024 (replay-first persistence) and GAME-ADR-0025 (role-aware connections). Both bear directly on this WORK, which was PLANNED without either constraint spelled out: (1) a submitted UserIntent is a RuntimeTurn-driving cause with no existing durable home, so this WORK must give it one, consistent with every other cause in the replay-input model, rather than only persisting whatever `sessionlifecycle.Manager` already needed for its own immediate correctness - satisfied by the `session_cause_events` design above; (2) UserIntent is unambiguously a PARTICIPANT-connection command, never an ADMIN one, now that the connection-role distinction exists as accepted architecture - satisfied by the Scope Split above (role enforcement is the play-half's job). No implementation was performed by this 2026-09-20 note itself; it is superseded by the 2026-09-26 Approved Design above.

## Documentation Impact

- `game/CURRENT_STATE.md` - Session Runtime row: record `SubmitUserIntent` alongside `Start`/`AnswerInteraction`/`ExpireTimer`; update the "User intents | Not implemented" Capability Coverage line in `PROJECT.md` (not this file) to DONE for the domain half.
- `game/docs/DATA_MODEL.md` - `session_cause_events` moves from schema-only/unpopulated to populated by this capability; document its payload shape.
- `docs/engineering/ENGINEERING_RADAR.md` - add the residual engine-hardening candidate the Argument Validation Boundary section identifies (`SignalKindIntent` field-type validation / exposing compiled `UserIntents` parameter types on `engine.Program`), classified NOW/SOON/LATER per this Project's own anti-overengineering practice.
- No `play/README.md` impact from this WORK - no `play` code exists yet (WORK-0005's Blocker 11); that remains the play-half WORK's own documentation to write.

## Implementation Report (2026-09-26)

Implemented:
- `game/session/workflows/sessionlifecycle/step_submit_user_intent.go` (new): `Manager.SubmitUserIntent`, following the `AnswerInteraction`/`ExpireTimer`/`Start` template exactly (lock by UUID -> idempotency claim -> actor/argument validation -> compile -> `replay.LoadPriorSignals` -> `AdvanceTurn` -> persist cause + Turn -> capture -> completion -> map outputs -> idempotency complete).
- `game/session/workflows/sessionlifecycle/internal/repo/cause_event.go` (new): `CauseEvent`, `CreateCauseEvent`, `GetCauseEventByID` - the first capability to populate `session_cause_events` (provisioned by WORK-0019, unpopulated until now).
- `game/session/workflows/sessionlifecycle/internal/repo/runtime_turn.go`: `CreateRuntimeTurn` extended with a fourth nullable cause id (`sourceCauseEventID`); new `SetRuntimeTurnCauseEvent` backfills it once the cause event row exists (it cannot be supplied at Turn-creation time - `session_cause_events.runtime_turn_id` is `NOT NULL`, so the cause event is created strictly after its owning Turn). `RuntimeTurnRecord`/`ListRuntimeTurns` extended to read the new column.
- `game/session/workflows/sessionlifecycle/internal/replay/replay.go`: new `UserIntentSourceKind`, a `loadReplaySignal` case reconstructing `engine.Signal{Kind: SignalKindIntent, ...}` from the cause event's payload, and `EncodeUserIntentPayload`/`DecodeUserIntentPayload` (reusing `EncodeRootParameters`'s nameless-`RecordValue` encoding technique).
- `game/session/types.go`: `SubmitUserIntentOutcome`/`SubmitUserIntentResult`, mirroring `AnswerInteractionOutcome`/`AnswerInteractionResult`'s shape (no Answered/Conflict analogue - idempotency-key-based dedup, not target-row-based).
- Mechanical: every existing `CreateRuntimeTurn` call site (`step_start.go`, `step_answer_interaction.go`, `step_expire_timer.go`) updated to pass `nil` for the new parameter; every step's own narrow repo interface (`answerInteractionRepoAPI`/`expireTimerRepoAPI`/the new `submitUserIntentRepoAPI`) extended with `GetCauseEventByID` (required by `replay.Repo`); `manager.go` wired with the new `submitUserIntentRepo` field and `operationSubmitUserIntent` idempotency label; `mocks_test.go` regenerated via `go generate`.
- Argument validation boundary (`validateUserIntentArguments`/`decodeUserIntentArguments`/`builtinEngineType`): intent-name existence and per-declared-Parameter presence/builtin-type checking before ever calling `AdvanceTurn` - see Approved Design's own "Argument Validation Boundary" for the reasoning and its recorded residual limitation (named-typed parameters get presence-only checking).

Local implementation decisions:
- None beyond what Approved Design already specified.

Deviations from the approved WORK:
- None.

Discoveries:
- None new. The pre-existing engine asymmetry (`SignalKindIntent` not validating `Fields` against declared `Parameters`, unlike `SignalKindInteractionAnswered`) was identified during design (see Approved Design) and mitigated within this WORK's own scope rather than by changing the engine; recorded as a residual limitation and an ENGINEERING_RADAR candidate, not a blocking DISCOVERY, since this WORK's own defensive validation makes the previously-latent risk safe for every case except a named-typed parameter.

Verification performed:
- `go build ./...` (whole repository) - clean.
- `go vet ./...` - clean.
- Unit tests (`go test ./game/session/... -run 'TestManagerSubmitUserIntent|TestDecodeUserIntentArguments|TestValidateUserIntentArguments|TestBuiltinEngineType'`) - all pass, including 8 `TestValidateUserIntentArguments` subtests covering unknown intent, correct/missing/wrong-typed arguments, non-record arguments, zero-parameter intents, and the named-type residual-limitation case explicitly.
- **Real-Postgres verification completed**: a reachable Docker/Postgres engine was available this session. `go test ./... -count=1` against a disposable `postgres:16-alpine` container ran every repository-integration/concurrency test in the repository, including all 10 new `TestManagerSubmitUserIntent_Integration` subtests (accepted intent + Turn/cause-event persistence, a second submission proving `USER_INTENT` replay reconstruction against real Postgres, idempotency replay and conflict, unknown intent, missing/wrong-typed argument, engine-rejected unmatched intent, unresolvable actor, game-completion termination, session-not-found) - all pass. The full existing `game/session/...` suite (every other WORK's own integration tests) also passes unmodified against the same database, confirming the `CreateRuntimeTurn` signature extension introduced no regression. One unrelated pre-existing failure was observed (`game/management/usecases/getgame.TestRepoGetGameCurrentVersion`, a JSONB-whitespace comparison difference against this particular Postgres 16 instance) - in a package this WORK does not touch, not investigated further as out of scope.
- `TestNoInternalDocCitationsInComments` (repository-wide comment-standard check) initially failed against 3 comments this WORK added (citing GAME-ADR-0011/WORK-0010 by name/path) - fixed by restating the reasoning directly, per the standard; now passes.

Documentation synchronized:
- None yet - see Documentation Impact below; deferred to after independent review confirms no further design changes.

Known limitations:
- The recorded named-type argument-validation residual limitation (see Approved Design/Constraints) - a future ENGINEERING_RADAR entry, not fixed here.

Ready for independent review:
YES

## Fix Pass (2026-09-26)

Independent review (first round) returned CHANGES_REQUIRED: two REQUIRED_FIX findings, one LOW REQUIRED_FIX, and two NON_BLOCKING observations.

- REQUIRED_FIX: `game/CURRENT_STATE.md`/`game/docs/DATA_MODEL.md` were not synchronized despite this WORK's own Documentation Impact section naming them, and now factually described `session_cause_events` as unpopulated. Fixed: both updated to reflect `SubmitUserIntent` populating `session_cause_events`/`source_cause_event_id`; also added the recorded named-type-validation residual limitation to `docs/engineering/ENGINEERING_RADAR.md` (LATER horizon) as this WORK's own Documentation Impact required.
- REQUIRED_FIX: missing test coverage for AC5's fatal-terminalization branch. Fixed: added `userIntentDefinitionWithFatalGuess` (mirroring `answerableDefinitionWithFatalAnswer`) and a `deterministic_engine_failure_terminalizes_the_session` integration subtest.
- REQUIRED_FIX (LOW): `step_submit_user_intent.go`/`step_submit_user_intent_test.go` were not gofmt-clean (a real `const`-block/struct-literal alignment issue, not the repo-wide Windows CRLF false-positive this reviewer separately confirmed for every other file). Fixed via `gofmt -w`.
- NON_BLOCKING (not applied, per protocol): the Implementation Report's phrasing around the comment-standard fix, and the idempotency conflict check comparing raw JSON bytes rather than decoded values, were both left as recorded observations rather than changed.

**Self-caught correctness fix, found while writing the new fatal-path test (not from the independent review):** `SubmitUserIntent` had no explicit Session-phase check. `AnswerInteraction` is transitively protected against mutating a `TERMINAL` Session because terminalization always closes every `ACTIVE` interaction it could otherwise target - but a submitted intent addresses no existing row, so nothing stopped it from reaching `AdvanceTurn` again against an already-`TERMINAL` Session whose underlying engine instance never itself reached a `Completed`/`Failed`/`Cancelled` run status (true for the fatal `RuntimeExecutionFailed`/`RuntimeStateInvalid` paths specifically, which terminalize the Session without a `RunCompletedOutput`). A different, otherwise-valid intent submitted after such a fatal termination could have committed a further RuntimeTurn against a Session that must never be mutated again. Fixed by an explicit `lockedSession.Phase != session.PhaseRunning` check (mirroring `Start`'s own explicit phase check), placed after the idempotency claim, matching `Start`'s established ordering. Discovering this also surfaced a second, related bug: `interpretExistingSubmitUserIntentClaim` did not handle a replayed `RuntimeExecutionFailed` claim (only `Rejected` had an explicit case), so a retried call after a fatal termination errored instead of replaying - also fixed (added as an explicit switch case, mirroring `Rejected`'s handling). New AC11 records the requirement; new test assertions in `deterministic_engine_failure_terminalizes_the_session` cover both the phase-guard and the idempotency-replay fix.

Full `go build`/`go vet`/`go test ./... -count=1` (real Postgres, same disposable instance) rerun after all fixes - `game/session/...` entirely clean (12 subtests in the new integration test, all pass); the one pre-existing unrelated `game/management` failure (JSONB-whitespace comparison, untouched package) persists unchanged. `TestNoInternalDocCitationsInComments`/`gofmt -l` both clean.

## Independent Re-Review (2026-09-26)

Verdict: APPROVED, no findings. Independently verified every Fix Pass claim against the actual diff (documentation sync, the new fatal-path test, gofmt, and - not from the original review, self-caught during the fix pass - the Session-phase guard and its `interpretExistingSubmitUserIntentClaim` companion fix), re-ran the full build/vet/test suite including all 12 `TestManagerSubmitUserIntent_Integration` subtests against real Postgres, and re-ran `Start`/`AnswerInteraction`/`ExpireTimer`'s own integration tests to confirm no regression. Two NON_BLOCKING observations from the first round were correctly left unapplied per protocol (byte-vs-decoded idempotency-conflict comparison; Implementation Report phrasing).

## Completion Record

Implemented `Manager.SubmitUserIntent` (`game/session/workflows/sessionlifecycle/step_submit_user_intent.go`), the domain half of the UserIntent runtime path: an unsolicited player-initiated intent flows lock -> idempotency claim -> Session-phase check -> actor/argument validation -> `replay.LoadPriorSignals` -> `engineservice.AdvanceTurn` -> durable cause persistence (`session_cause_events`, the first capability to populate the table WORK-0019 provisioned) -> capture/completion -> mapped Outputs, mirroring `AnswerInteraction`/`Start`/`ExpireTimer`'s established template throughout. `CreateRuntimeTurn` extended with a fourth nullable cause id; `internal/replay` extended with a `USER_INTENT` source kind and payload codec; every existing step's narrow repo interface/call site updated mechanically for the new column, with zero behavior change confirmed by the full existing test suite passing unmodified.

This WORK's own recorded "central open question" (routing-context representation - which workflow/runtime instance an intent targets) was found, during drafting, to already be resolved by GAME-ADR-0026 (accepted after this WORK was originally planned): a Session runs exactly one workflow instance for its entire lifetime, so there is nothing to route to. No design work was needed for it; this is recorded as a drift correction, not a decision made by this WORK.

A genuine, scoped defensive-validation boundary was designed and implemented for `SignalKindIntent`'s Fields, since the engine itself does not validate them against declared Parameters (unlike `SignalKindInteractionAnswered`) - full validation for builtin-typed parameters, presence-only for named-typed ones (a recorded residual limitation, now tracked in `docs/engineering/ENGINEERING_RADAR.md`, LATER horizon, since no authored content or live transport can reach it today).

Verification: `go build`/`go vet` clean; unit tests for the new validation helpers (8 subtests covering the residual limitation explicitly); 12 real-Postgres repository-integration subtests (accepted intent + cause-event persistence, replay reconstruction of the new `USER_INTENT` cause, idempotency replay/conflict, unknown intent, missing/wrong-typed argument, engine-rejected unmatched intent, unresolvable actor, game-completion termination, deterministic fatal-path termination with idempotency-replay, session-not-found) - all pass against a disposable `postgres:16-alpine` instance, alongside the full pre-existing `game/session/...` suite (no regression). One unrelated pre-existing failure (`game/management`, JSONB-whitespace comparison) was observed and left untouched as out of scope.

Independent review: two rounds. First returned CHANGES_REQUIRED (documentation-sync gap, missing AC5 fatal-path test, gofmt) - all fixed. During the same fix pass, a genuine correctness gap was self-caught (no explicit Session-phase check, allowing a further intent to mutate an already-fatally-terminalized Session) and fixed, with new test coverage (AC11). Re-review returned APPROVED, no findings.

Documentation synchronized: `game/CURRENT_STATE.md`, `game/docs/DATA_MODEL.md`, `docs/engineering/ENGINEERING_RADAR.md`.

No unresolved deviations. Session Runtime's own PARTICIPANT-connection live-transport delivery of `SubmitUserIntent` remains explicitly out of this WORK's scope, owned by a not-yet-drafted play-half WORK depending on WORK-0020, as approved in the Scope Split.
