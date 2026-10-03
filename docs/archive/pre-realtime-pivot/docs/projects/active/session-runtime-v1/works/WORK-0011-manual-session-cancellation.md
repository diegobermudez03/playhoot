# WORK-0011: Manual Session Cancellation

Status: DONE
Created: 2026-09-20
Last status change: 2026-09-26 (IMPLEMENTING -> DONE: independent re-review verdict APPROVED after one fix/re-review round - first round found a genuine missing-test gap (AC7's conflicting-retry scenario was wrongly claimed untestable), fixed and re-verified against real Postgres; re-review confirmed the fix and found no further issues. See this WORK's own "Fix Pass"/"Independent Re-Review"/"Completion Record" sections. Prior status change, same day: READY -> IMPLEMENTING: implementation started against the Approved Design below. Prior status change, same day: DRAFT -> READY: human explicitly authorized implementation of the Approved Design below, including the force-terminal-always decision and the Scope Split. Prior status change, same day: PLANNED -> DRAFT: Approved Design filled in, following the `AnswerInteraction`/`SubmitUserIntent`/`ExpireTimer` domain-half template exactly, per this Project's own "Current Work" sequencing (WORK-0011 next in Phase 1). Scope split for real, matching the correction already applied to WORK-0006/0007/0010/0012: live-transport/wire-contract exposure moves out to a not-yet-drafted play-half WORK depending on WORK-0020; this WORK's own "Depends on WORK-0020" Blocker is corrected accordingly - the domain half depends on nothing beyond the Foundational tier plus WORK-0019's durable-cause model, same as its Phase-1 siblings. One genuinely open product/domain question this WORK's own Approved Design (below, at PLANNED time) already flagged - whether a host's cancel forces the Session terminal even when the authored game does not itself react to `SessionCancelled` - was escalated to and resolved by explicit human decision: **force-terminal always**. Prior status change: 2026-09-20 (Part B/I reconciliation, same day: ADMIN-only command surface and durable-cause requirement made explicit - see "Scope Addition (Part B/I Reconciliation, 2026-09-20)" below)

Related decisions:
- GAME-ADR-0019 (RuntimeTurn execution bound and terminal cleanup)
- GAME-ADR-0002 (Live Session Coordinator responsibility boundary)
- GAME-ADR-0024 (Replay-First Session Runtime Persistence - the cancellation cause must be durably representable)
- GAME-ADR-0025 (Role-Aware Live Connections - Cancel is an ADMIN-only command)

Canonical context:
- `docs/projects/active/session-runtime-v1/PROJECT.md`
- `game/docs/decisions/GAME-ADR-0004-session-lobby-contract.md` (Leave "does not automatically cancel the Session" - the gap this WORK closes)
- `game/language/v1/program/signal.go`/compiler catalog (`SessionCancelled` - already a real, usable `NamedSignalSource`)
- `docs/projects/active/session-runtime-v1/works/WORK-0007-session-termination-live-notification.md` (the generalized terminal-live consequence this WORK's resulting termination must reuse)
- `docs/projects/active/session-runtime-v1/works/WORK-0019-replay-first-session-runtime-persistence-migration.md` (owns the general replay-input model; this WORK is responsible for satisfying it for SessionCancelled specifically)
- `docs/projects/active/session-runtime-v1/works/WORK-0020-role-aware-live-connections.md` (the ADMIN connection this WORK's command rides, exclusively)

## Outcome

A host can explicitly end/cancel an active Session before it would otherwise reach a terminal state on its own. `SessionCancelled` enters Session Runtime and is delivered to the game as a signal; the game's own authored semantics determine how it reacts (an authored workflow may match on `SessionCancelled` via a `CancelControl`, per Game Language's existing engine tests); the Session becomes terminal; connected clients are notified/closed through WORK-0007's terminal-live mechanism.

Today, no such capability exists: `grep "Cancel"` across `game/session/` returns zero matches, and GAME-ADR-0004 explicitly confirms Leave does not cancel a Session. `SessionCancelled` is real and matchable in Game Language today, but nothing in Session Runtime ever emits it.

## Context

This is required for V1 per the reconciliation prompt that created this WORK: a host currently has no way to end a Session they no longer want to continue, other than every player leaving (which does not terminate it either, per GAME-ADR-0004) or waiting for inactivity expiration (WORK-0016, not yet implemented either).

This WORK's resulting termination must be a cause that reuses WORK-0007's generalized terminal-live-notification mechanism rather than reimplementing "notify connected clients on termination" a third time (WORK-0007 is itself being generalized under this migration precisely so causes like this one can plug into it).

## Scope Split (2026-09-26, matching WORK-0006/0007/0010/0012's own correction)

This WORK mixes a Session-Runtime-domain concern with a `play`/`api` (live-transport) concern in one document, exactly like its Phase-1 siblings did before each was drafted for real. Per this Project's established just-in-time practice, the split happens now that this WORK is actually being drafted:

- **Domain half (this WORK, Phase 1)**: a host-authorized `Manager.CancelSession` capability - no wire contract, no connection-role enforcement (`sessionlifecycle.Manager` has no notion of connection role, same stance as every other domain-half WORK in this phase).
- **Play half (not yet drafted, Phase 2, depends on WORK-0020)**: live-transport exposure of the command (ADMIN-connection-only per GAME-ADR-0025) and delivering the resulting Outputs/terminal notification to connected clients, reusing WORK-0007's mechanism once it exists on the live-transport side.

## Scope

### In Scope (domain half)

- `Manager.CancelSession`: a host-authorized capability that delivers `SessionCancelled` into the Session's current runtime/workflow instance, and terminalizes the Session - either via the game's own authored reaction (reusing WORK-0007's `completion.Detect`/`TerminalReasonGame*` mechanism) or, absent one, via a forced terminal path (see Approved Design) - so the Session always ends once this capability commits, regardless of whether the authored game itself models a transition on `SessionCancelled`.
- Durable, replayable persistence of the cancellation cause where the engine actually accepts the signal, per WORK-0019/GAME-ADR-0024 (see Approved Design's "Cause Persistence").
- Host-only authorization at the domain layer (`sessions.host_actor_id` check, the same mechanism `Start` already uses) - independent of, and in addition to, the play-half's future ADMIN-connection-role check (GAME-ADR-0025: authority comes from both together, never either alone).

### Out of Scope

- Live-transport exposure of this capability (host-only command routing, ADMIN-connection-role enforcement) and delivering the resulting notification/connection closure to clients - both owned by a not-yet-drafted play-half WORK depending on WORK-0020, per the Scope Split above.
- Deciding what a specific authored game does in response to `SessionCancelled` - that is game-authoring content, not Session Runtime's concern.
- Cancelling a Session still in `LOBBY` (before `Start`) - there is no running engine instance yet to deliver a signal to. `Manager.CancelSession` rejects this case (`CancelSessionOutcomeNotRunning`) rather than silently no-op'ing or inventing a different lobby-abandonment mechanism; a host wanting to end an unstarted lobby is left to existing lobby-lifecycle behavior (natural expiration, WORK-0016) for now. If ending an unstarted lobby on demand is later found to be required, it needs its own WORK (see `PROJECT.md`'s Material Decisions, which already tracks the adjacent "kick a participant"/host-administration gap the same way).
- Host "kick a participant" - a different capability, not decided to be required for V1 (see `PROJECT.md`).

## Approved Design (2026-09-26)

**Human decision (escalated during drafting, not invented here):** when a host cancels, the Session always becomes terminal, even if the authored game does not itself react to `SessionCancelled` (no matching transition). This differs from every other signal-driven capability in this Project (`AnswerInteraction`/`SubmitUserIntent`/`ExpireTimer`), where an engine-rejected signal is an ordinary decline that leaves the Session `RUNNING` - a host cancel command is an administrative override, not gameplay input, so it must not be a silent no-op merely because a game author never authored a `CancelControl` transition for it.

**New Manager capability**, following the `AnswerInteraction`/`SubmitUserIntent`/`ExpireTimer`/`Start` template (lock -> idempotency claim -> phase/authority checks -> reload via replay -> compile -> `AdvanceTurn` -> persist -> capture -> map outputs -> idempotency complete):

```go
func (m *Manager) CancelSession(ctx context.Context, sessionUUID session.SessionUUID, userUUID session.UserUUID, idempotencyKey session.IdempotencyKey) (session.CancelSessionResult, error)
```

Locks directly by `sessionUUID` (`sessionlock.LockByUUID`), the same shape `Start`/`SubmitUserIntent` use - `CancelSession` addresses the Session itself, not a separate interaction/obligation row.

**Idempotency**: required (`session.ErrIdempotencyKeyRequired` if empty), via the existing shared `internal/idempotency` mechanism (`operationCancelSession = "CANCEL_SESSION"`), mirroring `Start`/`SubmitUserIntent`. Rationale, following the same precedent those two already established: `CancelSession` is a genuinely unsolicited host command with no pre-existing target row to naturally dedup against (unlike `AnswerInteraction`/`ExpireTimer`); `sessions.phase` alone is not sufficient the same way it is not sufficient for `Start` (which also keeps an idempotency key despite its own phase checks) - a network retry must replay the exact prior outcome, not merely re-derive "already terminal."

**Step sequence** (mirrors `startSessionInTx`):
1. `sessionlock.LockByUUID`; `idempotency.Claim`; an existing claim replays its persisted `CancelSessionResult` outcome (`interpretExistingCancelSessionClaim`), mirroring `Start`.
2. Phase-based short circuits, evaluated before host authorization (mirrors `Start`'s own ordering): `Phase == PhaseTerminal` -> `CancelSessionOutcomeAlreadyTerminal` (idempotent no-op regardless of which reason originally terminalized it - cancelling an already-ended Session is harmless and consistent, the same reasoning `Start`'s own already-`RUNNING` short circuit uses); `Phase == PhaseLobby` -> `CancelSessionOutcomeNotRunning` (see Scope's "Cancelling a Session still in LOBBY" note).
3. Resolve the calling actor via `FindActor(sessionID, userUUID)`; `actor == nil || lockedSession.HostActorID == nil || actor.ID != *lockedSession.HostActorID` -> `CancelSessionOutcomeNotHost`, the identical authorization check `Start` already uses (a missing actor is indistinguishable from "not the host," same established reasoning).
4. Load pinned Definition, `engineservice.Compile` (recompile failure -> fatal terminalize under the existing `TerminalReasonRuntimeStateInvalid`, identical to every other capability's fatal path).
5. `replay.LoadPriorSignals`; build `engine.Signal{Kind: engine.SignalKindNamed, Name: "SessionCancelled"}` (empty `Fields` - the compiler's own `namedLifecycleSignals` catalog already declares `"SessionCancelled": {}`, an empty schema, so no payload decoding is needed, unlike `SubmitUserIntent`'s `Fields`).
6. `engineservice.AdvanceTurn`. Three outcomes:
   - **Engine error** (`ErrReplayDivergence` or anything else unexpected) -> alert + fatal terminalize (`TerminalReasonRuntimeExecutionFailed`), identical to every other capability's fatal path. Unaffected by the force-terminal decision above - the Session was already going to end via the existing fatal mechanism.
   - **Accepted** (the authored game has a transition matching `SessionCancelled`, e.g. via `CancelControl`, per Game Language's existing engine tests): commit the durable cause and RuntimeTurn as usual (see "Cause Persistence" below), capture Outputs, `SetCurrentTurn`. Then `completion.Detect`: if it finds a `RunCompletedOutput` -> terminalize under the matching existing `TerminalReasonGame*` value (reusing WORK-0007 exactly, as Scope requires). If it does **not** (the authored transition moved state without ending the run) -> per the force-terminal decision, terminalize anyway, under a **new** `session.TerminalReasonSessionCancelledByHost`, running the identical terminal-cleanup steps (`SetSessionTerminal`, `CloseAllActiveInteractionsForSession`, `CancelAllActiveTimerObligationsForSession`) every other terminal path already runs.
   - **Rejected** (`ErrSignalRejected`/`ErrInputRejected` - no transition matches `SessionCancelled` at all): per the force-terminal decision, still terminalize the Session under `TerminalReasonSessionCancelledByHost` - but **no RuntimeTurn/session_cause_events row is created** for this case (see "Why the rejected path creates no RuntimeTurn" below). This mirrors the shape of `terminalizeAnswerInteractionFatal`/`terminalizeExpireTimerFatal` (direct `SetSessionTerminal` + terminal cleanup, no Turn, `current_turn_id` unchanged) - the difference is only in *why* it happens (a deliberate host action forcing a decline into a terminal outcome, not an error condition), not in its persistence shape.
7. `idempotency.Complete` with the final outcome/marshaled result, mirroring `Start`/`SubmitUserIntent`.

**Why the rejected path creates no RuntimeTurn (persistence-shape reasoning, not itself asked of the human - a consequence of applying the force-terminal decision consistently with the already-accepted replay model):** `session_cause_events`/RuntimeTurn only ever represent a signal the engine actually and durably accepted - `engineservice.AdvanceTurn`'s replay reconstruction re-executes every prior signal in the log and expects each one to succeed identically (`ErrReplayDivergence` is treated as a data-integrity alert precisely because "every element of priorSignals already succeeded once - it is durable specifically because it did," per `ExpireTimer`'s own established reasoning). Recording a *rejected* signal as if it were a durable cause would break that invariant on replay: reconstructing state would attempt to replay a signal the engine never actually accepted, and correctly reject it again, which the existing machinery cannot distinguish from genuine divergence/corruption. The rejected-path terminalization therefore uses the same "no Turn, direct terminal bookkeeping" shape already established for fatal paths - `sessions.terminal_reason` (a new, distinct, durable value) is sufficient to explain *that* the Session was force-cancelled; it does not need a replayed signal, since nothing about a rejected signal is ever replayed forward. The issuing host's identity for this path is captured via the same structured request logging (`logging.LogFields`) every other command already uses, not a new persisted column - consistent with how a `NOT_HOST`/rejected `Start` attempt is handled today.

**New `TerminalReasonSessionCancelledByHost`** (`game/session/session.go`): distinct from `TerminalReasonGameCancelled` (the game's own authored `CancelControl`-driven abandonment) - this WORK's own comment on `TerminalReasonGameCancelled` already anticipates exactly this ("distinct from any future session-lifecycle-level cancellation a host or operator initiates directly").

**Cause Persistence (accepted-path only, `session_cause_events`)**: identical shape to `SubmitUserIntent`'s own (`cause_kind = "SESSION_CANCELLED"`, matching `replay.SessionCancelledSourceKind`), except the payload envelope is empty (`SessionCancelled` carries no Fields) - the cause event's `actor_id` (the issuing host) is its only meaningful content. `internal/replay/replay.go` gets a new `SessionCancelledSourceKind = "SESSION_CANCELLED"` constant and a `loadReplaySignal` case requiring `turn.SourceCauseEventID != nil` (alert + hard error otherwise, matching the existing cases' defensive shape) and reconstructing `engine.Signal{Kind: SignalKindNamed, Name: "SessionCancelled"}` directly (no payload decode needed).

**Result shape** (`game/session/types.go`): `CancelSessionOutcome` (`Cancelled`/`AlreadyTerminal`/`NotRunning`/`NotHost`/`RuntimeExecutionFailed`) and `CancelSessionResult{Outcome, SessionUUID, Outputs []Output` (excluded from idempotency-replay JSON via `json:"-"`)`, TerminalReason string}`, matching `SubmitUserIntentResult`'s shape/JSON tags exactly.

## Constraints and Invariants

- Only the Session's host may cancel it, authority coming from the connection role (ADMIN) plus Manager's own authoritative `host_actor_id` check together, never from either alone (GAME-ADR-0025) - a host who also holds a PARTICIPANT connection does not gain cancellation authority through that connection. This WORK's domain half enforces only the `host_actor_id` half; ADMIN-connection-role enforcement is the play-half's job (see Scope Split).
- The resulting terminal transition must satisfy the same terminal-cleanup invariant (no dangling `ACTIVE` interaction/timer obligation) that other terminal paths already establish or that WORK-0014 generalizes - satisfied identically on every terminal branch (game-driven, forced-after-accepted, and forced-after-rejected) by reusing `CloseAllActiveInteractionsForSession`/`CancelAllActiveTimerObligationsForSession`.
- Must reuse, not reimplement, WORK-0007's terminal-live-notification mechanism - a play-half concern once it exists (see Scope Split); this domain half only needs to guarantee a `TerminalReason` is always set once `CancelSession` commits.
- **(Part B reconciliation)** The cancellation cause (at minimum, the issuing host's actor identity and the fact that cancellation - not some other terminal reason - occurred) must be durably representable in commit order, per GAME-ADR-0024/WORK-0019's replay-input model, so a Session's terminal history remains reconstructible/understandable after the fact. Satisfied by `session_cause_events` on the accepted-signal path; satisfied by the new distinct `TerminalReasonSessionCancelledByHost` value alone on the rejected path, where no signal is ever replayed forward (see Approved Design's "Why the rejected path creates no RuntimeTurn").
- **(Human decision, 2026-09-26)** `CancelSession` always terminalizes the Session once it reaches the point of calling the engine, whether or not the authored game itself reacts to `SessionCancelled` - it is never a silent no-op.

## Acceptance Criteria

- AC1: `Manager.CancelSession` called by the Session's host while `RUNNING`, where the authored game has a transition matching `SessionCancelled` that itself reaches a terminal run status (e.g. via `CancelControl`), commits a new RuntimeTurn (`SessionCancelledSourceKind`), returns `CancelSessionOutcomeCancelled` with `TerminalReason` set to the matching existing `TerminalReasonGame*` value (reusing `completion.Detect` unchanged), and the Turn is durably replayable (`replay.LoadPriorSignals` reconstructs the identical `engine.Signal{Kind: SignalKindNamed, Name: "SessionCancelled"}`).
- AC2: Same as AC1, but the authored game's matching transition does not itself reach a terminal run status - `CancelSession` still commits the RuntimeTurn/cause event, then forcibly terminalizes the Session under the new `session.TerminalReasonSessionCancelledByHost`, returns `CancelSessionOutcomeCancelled`, and runs the same terminal-cleanup steps (closes other `ACTIVE` interactions/timer obligations).
- AC3: `Manager.CancelSession` called while `RUNNING`, where the authored game has no transition matching `SessionCancelled` at all (`engineservice.AdvanceTurn` returns `ErrSignalRejected`/`ErrInputRejected`), forcibly terminalizes the Session under `TerminalReasonSessionCancelledByHost` without creating any RuntimeTurn (`current_turn_id` unchanged), and returns `CancelSessionOutcomeCancelled`.
- AC4: A caller not resolving to a current `session_actors` row for the target Session, or resolving to one that is not `sessions.host_actor_id`, is rejected as `CancelSessionOutcomeNotHost` without reaching `engineservice.AdvanceTurn` (no RuntimeTurn created, Session not terminalized), mirroring `Start`'s own host-authorization handling.
- AC5: `Manager.CancelSession` called while the Session is still `LOBBY` returns `CancelSessionOutcomeNotRunning` without reaching `engineservice.AdvanceTurn` and without any state change.
- AC6: `Manager.CancelSession` called while the Session is already `TERMINAL` (any `terminal_reason`, including a prior `CancelSession` call, `RuntimeExecutionFailed`, or a `TerminalReasonGame*` value) returns `CancelSessionOutcomeAlreadyTerminal` idempotently, without reaching `engineservice.AdvanceTurn` and without any further state change.
- AC7: A retried call with the same `(UserUUID, "CANCEL_SESSION", IdempotencyKey)` replays the original persisted outcome without a second engine effect or a second terminalization attempt; a same-key retry against a different Session (the claim's own scoping identity is `(UserUUID, operation, IdempotencyKey)` alone - `SessionUUID` is not part of it, so the same host reusing a key across two Sessions is a materially different request under the same identity) is a conflict, consistent with `Start`'s own `rejects_conflicting_payload_under_same_token` handling.
- AC8: `engineservice.AdvanceTurn` returning any error other than `ErrSignalRejected`/`ErrInputRejected` (e.g. `ErrReplayDivergence`) terminalizes the Session under the existing `TerminalReasonRuntimeExecutionFailed`, mirroring every other capability's fatal path exactly - not the new `TerminalReasonSessionCancelledByHost`.
- AC9: `CreateRuntimeTurn`'s existing signature/every other existing call site is unaffected by this WORK; full existing `sessionlifecycle` test suite passes unmodified.
- AC10: Effect/Presentation Outputs produced by an accepted `SessionCancelled` transition (AC1/AC2) are returned via `CancelSessionResult.Outputs`, mapped through the existing `mapOutputs`/`clientoutputs.ClientFacing` path, unchanged from how `AnswerInteraction`/`SubmitUserIntent` already do this.
- AC11 (real-Postgres, per this Project's established practice): repository-integration tests prove each of AC1/AC2/AC3's distinct terminal branches survives process-loss reconstruction - `replay.LoadPriorSignals` against real Postgres reconstructs the identical prior-signal log for the accepted-path Turns (AC1/AC2), and the rejected-path terminal Session (AC3) reconstructs with no additional Turn and the correct `terminal_reason`, the same proof style `WORK-0019`/`WORK-0010`/`WORK-0012` already established for their own new causes/terminal paths.

## Blockers

None remaining for the domain half. Exact live-transport command/API/wire contract remains open, but is now explicitly the play-half WORK's own concern (see Scope Split), not a blocker on this domain-half WORK.

## Scope Addition (Part B/I Reconciliation, 2026-09-20)

A broader reconciliation session accepted GAME-ADR-0024 (replay-first persistence) and GAME-ADR-0025 (role-aware connections), both bearing on this previously-PLANNED WORK: (1) cancellation is explicitly an ADMIN-only command - a host's separate PARTICIPANT connection (if any) never gains cancellation authority merely because the same person holds it; (2) the cancellation cause needs a durable, ordered representation consistent with every other RuntimeTurn-driving cause, not only whatever `Manager` needed for its own immediate terminalization logic. Neither changes this WORK's own open design questions (exact command/wire contract, forced-terminal-vs-authored-handling semantics), which remain for DRAFT. No implementation was performed; this WORK's Status remains PLANNED.

## Documentation Impact

- `game/session/session.go` - new `TerminalReasonSessionCancelledByHost` constant, doc comment cross-referencing (in reasoning, not citation) `TerminalReasonGameCancelled`'s own comment, which already anticipated it.
- `game/CURRENT_STATE.md` - Session Runtime row: record `CancelSession` alongside `Start`/`AnswerInteraction`/`SubmitUserIntent`/`ExpireTimer`; update `PROJECT.md`'s "Manual cancellation | Not implemented" Capability Coverage line to DONE for the domain half.
- `game/docs/DATA_MODEL.md` - document the new `SESSION_CANCELLED` `session_cause_events.cause_kind` value (empty payload).
- No `play/README.md` impact from this WORK - no `play` code exists yet; that remains the play-half WORK's own documentation to write.

## Implementation Report (2026-09-26)

Implemented:
- `game/session/workflows/sessionlifecycle/step_cancel_session.go` (new): `Manager.CancelSession`, following the `Start`/`SubmitUserIntent`/`ExpireTimer` template exactly (lock by UUID -> idempotency claim -> phase short-circuits (`AlreadyTerminal`/`NotRunning`) -> host-authorization check (`NotHost`) -> compile -> `replay.LoadPriorSignals` -> `AdvanceTurn` against `engine.Signal{Kind: SignalKindNamed, Name: "SessionCancelled"}` -> three-way branch on the result). Accepted: persists the cause (`session_cause_events`, `cause_kind SESSION_CANCELLED`) + RuntimeTurn, captures interactions/timers, then force-terminalizes under the game's own `TerminalReasonGame*` (via `completion.Detect`) if it reached one, or the new `TerminalReasonSessionCancelledByHost` if it did not. Rejected (`ErrSignalRejected`/`ErrInputRejected`): force-terminalizes directly under `TerminalReasonSessionCancelledByHost` with no RuntimeTurn created (`terminalizeCancelSessionForced`) - deliberately never durably represented as a replayed signal, since the engine never accepted it (see the WORK's own Approved Design reasoning). Engine error: existing `terminalizeCancelSessionFatal` fatal path, identical in shape to `terminalizeSubmitUserIntentFatal`.
- `game/session/types.go`: `CancelSessionOutcome`/`CancelSessionResult`, mirroring `SubmitUserIntentOutcome`/`SubmitUserIntentResult`'s shape (`Cancelled`/`AlreadyTerminal`/`NotRunning`/`NotHost`/`RuntimeExecutionFailed`).
- `game/session/session.go`: new `TerminalReasonSessionCancelledByHost` constant.
- `game/session/workflows/sessionlifecycle/internal/replay/replay.go`: new `SessionCancelledSourceKind` constant, a `loadReplaySignal` case, and `buildSessionCancelledSignal` (no payload decode needed - `SessionCancelled` carries no Fields per the compiler's own `namedLifecycleSignals` catalog).
- Mechanical: `manager.go` wired with the new `cancelSessionRepo` field, `operationCancelSession` idempotency label, and the mockgen directive; `mocks_test.go` regenerated via `go generate`.
- Tests: `step_cancel_session_test.go` (idempotency-key-required check, mirroring `TestManagerSubmitUserIntent`'s own pre-transaction-only shape) and `step_cancel_session_integration_test.go` (11 real-Postgres subtests covering AC1-AC7/AC9/AC10/session-not-found, plus an `AlreadyTerminal`-for-a-different-reason case); `testutil_test.go` gained `sessionCancelledDefinition`/`sessionCancelledStaysDefinition` fixtures.

Local implementation decisions:
- AC8's genuinely-distinct `AdvanceTurn`-error branch (`ErrReplayDivergence`/unexpected error) is not independently forceable through a Definition fixture alone once past Start, the same limitation every other capability's own fatal-path tests already accept; verified by code review instead (`terminalizeCancelSessionFatal` is structurally identical to the already-reviewed `terminalizeSubmitUserIntentFatal`/`terminalizeAnswerInteractionFatal`).

Deviations from the approved WORK:
- None.

Discoveries:
- None new beyond the two Local implementation decisions above (both test-construction limitations, not behavioral gaps).

Verification performed:
- `go build ./...` (whole repository) - clean.
- `go vet ./...` - clean.
- `gofmt -l` on every changed file - clean.
- **Real-Postgres verification completed**: a reachable Docker/Postgres engine (`playhoot-postgres-1`) was available this session. `go test ./... -count=1` ran the whole repository's test suite, including all 11 new `TestManagerCancelSession_Integration` subtests - all pass. The full existing `game/session/...` suite (every other WORK's own integration tests) also passes unmodified against the same database, confirming this WORK introduced no regression.
- `TestNoInternalDocCitationsInComments` (repository-wide comment-standard check): initially failed against one comment this WORK added (citing WORK-0007 by name in a test-fixture doc comment) - fixed by restating the reasoning directly, per the standard; now passes.
- One unrelated pre-existing failure was observed (`game/management/usecases/getgame.TestRepoGetGameCurrentVersion`, a JSONB-whitespace comparison difference against this particular Postgres instance) - in a package this WORK does not touch, the same failure WORK-0010 already recorded as out of scope, not investigated further.

Documentation synchronized:
- `game/CURRENT_STATE.md` - Session Runtime row/evidence bullets: `CancelSession` recorded alongside `Start`/`AnswerInteraction`/`SubmitUserIntent`/`ExpireTimer`; `session_cause_events` narrative updated to describe both populated `cause_kind` values.
- `game/docs/DATA_MODEL.md` - `session_cause_events`/`session_runtime_turns.source_cause_event_id` narrative and relationship notes updated for the new `SESSION_CANCELLED` cause_kind (accepted-signal path only).
- `docs/projects/active/session-runtime-v1/PROJECT.md` - WORK-0011's own Capability Coverage row and Work table status.

Known limitations:
- None beyond the two recorded Local implementation decisions above.

Ready for independent review:
YES

## Fix Pass (2026-09-26)

Independent review (first round) returned CHANGES_REQUIRED: one REQUIRED_FIX, one NON_BLOCKING.

- REQUIRED_FIX: AC7's "conflicting retry not independently testable" claim was factually incorrect - the idempotency claim's own scoping identity is `(UserUUID, operation, IdempotencyKey)` alone (`SessionUUID` is not part of it, per `idempotency.fetchExisting`'s own query), so the same host reusing an idempotency key across two different Sessions is a real, constructible conflict, exactly like `Start`'s own already-existing `rejects_conflicting_payload_under_same_token` test. Fixed: added `rejects_conflicting_payload_under_same_token` to `step_cancel_session_integration_test.go` (two Sessions, same host, same key, second call asserts `session.ErrIdempotencyConflict`) - passes against real Postgres. AC7's own wording and the now-removed incorrect Local Implementation Decision note above were both corrected.
- NON_BLOCKING (not applied, per protocol): `interpretExistingCancelSessionClaim`'s decline-outcome branches return a zero-value `SessionUUID` on a replayed retry while the first-call paths populate it - a pre-existing pattern shared by `Start`/`SubmitUserIntent` alike, left as a recorded observation for a possible future cross-cutting fix rather than changed here.

Full `go build`/`go vet`/`go test ./... -count=1` (real Postgres, same disposable instance) rerun after the fix - `game/session/...` entirely clean (11 subtests in the integration test, all pass, including the new conflicting-retry case); the one pre-existing unrelated `game/management` failure (JSONB-whitespace comparison, untouched package) persists unchanged. `TestNoInternalDocCitationsInComments`/`gofmt -l` both clean.

## Independent Re-Review (2026-09-26)

Verdict: APPROVED. Independently verified the fix against the actual diff and its own reasoning (confirmed `idempotency.fetchExisting`'s query really does scope only by `user_uuid`/`operation`/`idempotency_key`, not `SessionUUID`, so the added conflict test exercises a real path, not a fabricated one), confirmed the NON_BLOCKING item was correctly left unapplied (`interpretExistingCancelSessionClaim`'s decline branches still return a zero-value `SessionUUID`, matching `Start`/`SubmitUserIntent`'s own pattern), and re-ran the full build/vet/test suite (real Postgres) including all 11 `TestManagerCancelSession_Integration` subtests - no regressions. One cosmetic-only observation (a subtest-count narrative slip in this WORK's own Fix Pass prose, "12" vs the actual 11) was corrected in place; no re-fix required.

## Completion Record

Implemented `Manager.CancelSession` (`game/session/workflows/sessionlifecycle/step_cancel_session.go`), the domain half of manual Session cancellation: a host-authorized command that delivers `SessionCancelled` and always terminalizes the Session once past authorization - reusing the existing `completion.Detect`/`TerminalReasonGame*` mechanism (WORK-0007) when the authored game itself reacts and reaches a terminal run status, or the new `session.TerminalReasonSessionCancelledByHost` otherwise, including when the engine rejects the signal outright (in which case, deliberately, no RuntimeTurn/`session_cause_events` row is created at all - only `sessions.terminal_reason` - to avoid ever asking replay to reconstruct a signal the engine never actually accepted). Follows the `Start`/`SubmitUserIntent`/`ExpireTimer` template throughout: lock -> idempotency claim -> phase short-circuits (`AlreadyTerminal`/`NotRunning`) -> host authorization (`NotHost`, `sessions.host_actor_id` only - the play-half ADMIN-connection-role check is explicitly out of this WORK's scope) -> compile -> `replay.LoadPriorSignals` -> `AdvanceTurn` -> branch on accepted/rejected/error.

This WORK's own central open product/domain question (recorded since PLANNED) - whether an engine-rejected cancel should force-terminate or leave the Session RUNNING - was escalated to and resolved by explicit human decision (force-terminal always) rather than silently decided during drafting. The Scope Split correction already applied to WORK-0006/0007/0010/0012 (domain half here; live-transport/wire-contract exposure deferred to a not-yet-drafted play-half WORK depending on WORK-0020) was applied identically.

Verification: `go build`/`go vet`/`gofmt -l` clean; the repository-wide comment-standard check (`TestNoInternalDocCitationsInComments`) clean after one same-session fix; 11 real-Postgres repository-integration subtests (game-driven terminal reason, forced-terminal-after-accepted, forced-terminal-after-rejected-with-no-Turn, host-authorization rejection, LOBBY-phase rejection, already-terminal idempotency for two different underlying reasons, same-key retry replay, cross-Session same-key conflict, mapped Effect Outputs, session-not-found) all pass against a real disposable Postgres instance, alongside the full pre-existing `game/session/...` suite (no regression). One unrelated pre-existing failure (`game/management`, JSONB-whitespace comparison) was observed and left untouched as out of scope, consistent with WORK-0010's own prior record of the same failure.

Independent review: two rounds. First returned CHANGES_REQUIRED - one REQUIRED_FIX (AC7's "conflicting retry untestable" claim was factually wrong; a real cross-Session conflict test was missing and constructible, mirroring `Start`'s own precedent) and one NON_BLOCKING (a pre-existing zero-value-`SessionUUID`-on-replay pattern shared with `Start`/`SubmitUserIntent`, left unapplied). The REQUIRED_FIX was fixed and verified same-session; re-review returned APPROVED, one cosmetic-only observation, no further findings.

Documentation synchronized: `game/CURRENT_STATE.md`, `game/docs/DATA_MODEL.md`, `docs/projects/active/session-runtime-v1/PROJECT.md`.

No unresolved deviations. Session Runtime's own ADMIN-connection live-transport delivery of `CancelSession`, and the resulting client notification, remain explicitly out of this WORK's scope, owned by a not-yet-drafted play-half WORK depending on WORK-0020, as approved in the Scope Split.
