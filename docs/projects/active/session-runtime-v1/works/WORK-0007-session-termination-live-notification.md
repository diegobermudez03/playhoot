# WORK-0007: Game-Completion Termination Detection — Domain Half

Status: DONE
Created: 2026-09-20
Last status change: 2026-09-24 (IMPLEMENTING -> DONE, independent review APPROVED, three NON_BLOCKING findings, two fixed same-session (see "Independent Review" below). Prior update, same day: READY -> IMPLEMENTING, human-approved. Scope additionally gained a small `engine` API rename, done and verified before the rest of this WORK's own implementation - see "Scope Addition (Engine Output/Outcome Renaming, 2026-09-24)" below. Prior update, same day: Domain/Play Split, human-directed: narrowed to its domain half only, mirroring WORK-0006's own split - `Manager` detecting and terminalizing on a `RunCompletedOutput`, no broadcast/transport code. Blocker 1 resolved (three distinct `TerminalReason` values, no durable outcome-payload persistence); Blocker 3 confirmed in-scope (additive termination info on `StartResult`/`AnswerInteractionResult`); Blockers 2/4 (broadcast mechanism, transport send-then-close) deferred to a new, not-yet-drafted play-half WORK following WORK-0020, exactly like WORK-0006's own client-delivery half - see "Scope Correction (Domain/Play Split, 2026-09-24)" below. Prior update: 2026-09-20 Part H reconciliation, same day: broadcast scope generalized to both connection roles - see "Scope Addition (Part H Reconciliation, 2026-09-20)" below)

Related decisions:
- GAME-ADR-0002 (Live Session Coordinator responsibility boundary)
- GAME-ADR-0019 (RuntimeTurn execution bound and terminal cleanup - the fatal-path/terminal-cleanup invariant this WORK adds a live-notification consequence to, without changing the invariant itself)
- GAME-ADR-0020 (post-commit client delivery semantics, best-effort)

Canonical context:
- `docs/projects/active/session-runtime-v1/works/WORK-0005-thin-live-coordinator.md` (the live Coordinator this WORK extends)
- `docs/projects/active/session-runtime-v1/works/WORK-0020-role-aware-live-connections.md` (the second, ADMIN, registry this WORK's broadcast must also reach - see "Scope Addition (Part H Reconciliation, 2026-09-20)" below)
- `docs/projects/active/session-runtime-v1/works/WORK-0006-broaden-live-fanout-effects-presentations.md` (the "Question/Presentation/Effect only cross the wire" principle this WORK's own termination message follows, as a generic UI-shaped signal rather than a raw domain event)
- `game/session/workflows/sessionlifecycle/step_answer_interaction.go` (`terminalizeAnswerInteractionFatal`), `game/session/workflows/sessionlifecycle/step_start.go` (`terminalizeStartFatal`) - the existing fatal paths this WORK's notification covers
- `game/session/workflows/sessionlifecycle/internal/repo/interaction.go` (`CloseAllActiveInteractionsForSession` - confirms closed rows from these paths have `closed_by_turn_id = NULL`, structurally excluded from the existing live-event query)
- **(2026-09-23 correction)** `play`/`play/sessionruntime` do not exist today - deleted in full by WORK-0005's Blocker 11 (2026-09-21). This WORK's own domain half (below) needs none of it; its broadcast mechanism (`Deliver`'s per-recipient fan-out is the thing it must differ from - see Scope) is a play-half concern that can only be implemented once WORK-0020 has rebuilt `play` from scratch. This WORK was drafted 2026-09-20, one day before that deletion, and was never reconciled against it until now.
- **(2026-09-24 correction)** `game/language/v1/engine/output.go` (`RunCompletedOutput` no longer carries a `Path` field at all - `game/docs/decisions/GAME-ADR-0026-flat-workflow-execution-model-and-keyed-interaction-slots.md` removed it along with Child Workflows/Task Groups: "every occurrence of this Output is about the one instance that exists, and is always actionable the way a root completion already was." This WORK originally cited `Path == nil` as "the root instance" - stale, from before GAME-ADR-0026 removed the field. There is exactly one workflow instance per Session, ever, so `game/session` has no "workflow" concept left to disambiguate from; every `RunCompletedOutput` a Turn produces means the Session's own game execution ended. `Outcome.Kind` distinguishes `RunOutcomeCompleted`/`RunOutcomeFailed`/`RunOutcomeCancelled` - see Scope below for why this WORK's own `TerminalReason` values are named for the game's own outcome ("Game...") rather than "Workflow" or "Session", to stay unambiguous both from an engine-internal term this domain no longer needs and from a future session-lifecycle-level cause (e.g. WORK-0011's host-initiated manual cancellation) that is not the same thing as the game's own authored `CancelControl`.
- `game/language/v1/engine/instance.go` (`WorkflowInstance.Outcome` - nil while running, set only once a `Complete`/`Fail`/`Cancel` control applies and "no further transition may apply" - confirms a completed instance can structurally never produce anything else, so treating it as terminal is forced, not a judgment call)
- `game/language/v1/program/control.go` (`CompleteControl`/`FailControl`/`CancelControl` - the three authored transition controls whose `RunOutcomeKind` this WORK's three new `TerminalReason` values mirror)

## Outcome

`sessionlifecycle.Manager`'s `Start`/`AnswerInteraction` recognize a `RunCompletedOutput` produced during their own drain - the game's one workflow instance reaching `Completed`/`Failed`/`Cancelled` - and terminalize the Session on it, mirroring the existing fatal path's shape (`sessions.phase = TERMINAL`, closing any still-`ACTIVE` interactions via the existing `CloseAllActiveInteractionsForSession`) but recorded under one of three new, non-fatal `TerminalReason` values distinguishing which outcome the game itself reached. The call's result additively reports that termination happened and why, so a future live-transport Coordinator (WORK-0020 onward) can act on it without re-deriving the fact from outcome strings. This WORK produces no client-visible behavior change and sends no message to any client - that remains a separate, not-yet-drafted play-half WORK (see Scope Correction below).

**Generalized invariant (added 2026-09-20, migration reconciliation - no scope change to this WORK's own domain-half outcome)**: the underlying broadcast-then-close mechanism a future play-half WORK builds on top of this one must be a single reusable mechanism on the rebuilt `play.Coordinator`, because every future terminal cause this Project's remaining WORK introduces (manual cancellation, WORK-0011; timer-driven termination, WORK-0012; inactivity/reaper, WORK-0016) is required to reuse it rather than reinventing "notify connected clients on termination" per cause. This WORK's own domain-half scope is unaffected by that eventual reuse; it is recorded here only so the play-half WORK is drafted against a mechanism designed for reuse from the start.

The eventual client-visible message is a generic, cause-agnostic "session ended" UI signal - the same fixed screen for every client regardless of which of the three reasons applied. A personalized results screen (winners/losers/score) is explicitly deferred - see `docs/product/IDEAS.md -> Personalized End-of-Game Screen` - and this WORK durably persists none of `RunOutcome`'s own payload (`Result`/`Error`/`Reason`) toward it; if a future WORK needs that payload, WORK-0019's replay-first model can re-derive it by replaying the durable signal log through the pinned Definition, the same reasoning that already let WORK-0006 skip persisting Presentations/Effects.

## Context

**Why detection is Session Runtime's own concern, not a play-half concern**: `Manager` already recognizes and durably reacts to Outputs produced during its own drain (Question/AskGroup capture, per WORK-0004; Effect/Presentation return, per WORK-0006) - a `RunCompletedOutput` is exactly the same kind of fact, just one this WORK is the first to give a durable consequence (`TERMINAL`) rather than a returned value alone. Nothing about detecting it or terminalizing the Session requires a live connection, a Coordinator, or any transport code to exist.

**The game's own outcome is a deterministic, forced signal, not a design choice** (confirmed against the engine's actual source): a compiled `Program` has exactly one workflow instance for the lifetime of a Session (GAME-ADR-0026 removed Child Workflows/Task Groups entirely). `WorkflowInstance.Outcome` is nil while running and, once set by an authored `CompleteControl`/`FailControl`/`CancelControl`, the instance can never receive another transition - so a `RunCompletedOutput` is not merely "probably done," it is structurally guaranteed to never do anything else. Treating any `RunCompletedOutput` as "the game is over, terminalize the Session" is therefore the only coherent interpretation. A game that wants a post-game continuation (rematch, lobby) simply does not apply a terminal control at what a player perceives as "the end" - it loops back to a non-terminal state instead; that is an authoring choice, not something Session Runtime needs to accommodate specially.

**Why three `TerminalReason` values, not one**: `RunOutcomeKind` already distinguishes `Completed` (an authored `CompleteControl` - the game finished as designed), `Failed` (an authored `FailControl` - the game's own logic determined a failure condition), and `Cancelled` (an authored `CancelControl` - the game's own logic abandoned the instance). All three are equally deterministic and equally terminal, but a consumer may reasonably want to act differently depending on which one applied (for example, future analytics, or a differently-worded generic screen) - collapsing them into one reason would discard information the engine already hands over for free. Naming them for the *game's* outcome ("Game...") rather than "Workflow" (an engine-internal term this domain no longer needs to disambiguate, per the Canonical Context correction above) or "Session" (already the aggregate itself, and reserved for session-lifecycle-level causes) keeps them unambiguous from both directions - in particular from a future host-initiated manual cancellation (WORK-0011), which is a session-lifecycle action on an otherwise-still-running game, not the game's own authored `CancelControl` reaching a terminal state.

Human-confirmed direction (2026-09-20, refined 2026-09-24): the eventual client message is a generic "ended" signal, not a full Presentation, the same fixed screen for every client regardless of which of the three reasons applied; this WORK's own domain half distinguishes the three reasons durably even though nothing client-facing reads that distinction yet.

## Scope

### In Scope

- **New Session Runtime business logic**: `sessionlifecycle.Manager`'s `Start`/`AnswerInteraction` recognize a `RunCompletedOutput` produced during their own drain and terminalize the Session on it - mirroring the existing fatal path's shape (`sessions.phase = TERMINAL`, closing any still-`ACTIVE` interactions via the existing `CloseAllActiveInteractionsForSession`), but recorded as an ordinary, expected outcome, not a failure.
- Three new `session.TerminalReason` values, one per `engine.RunOutcomeKind`: `TerminalReasonGameCompleted`, `TerminalReasonGameFailed`, `TerminalReasonGameCancelled` - alongside the existing `TerminalReasonLobbyExpired`/`RuntimeExecutionFailed`/`RuntimeStateInvalid`.
- Additive termination info on `StartResult`/`AnswerInteractionResult` (exact field shape Implementation Freedom - e.g. a nullable `TerminalReason`) so a future Coordinator can learn "this call's result means the Session is now `TERMINAL`, for this reason" without needing to know which outcome strings imply termination itself (resolves former Blocker 3).
- Covers both paths that can produce a `RunCompletedOutput` today: `Start`'s own first Turn (an authored game whose very first transition already completes/fails/cancels the workflow) and `AnswerInteraction`'s Turn.

### Out of Scope

- **Broadcasting any message to connected clients, and closing their connections** - the play-half: a dedicated `play.Coordinator` broadcast-to-everyone-bound mechanism (distinct from `Deliver`'s per-recipient `Event` contract, since a termination fact applies to every currently-bound connection of either role, not one recipient) and the transport-level send-then-close itself. Deferred to a new, not-yet-drafted WORK following `docs/projects/active/session-runtime-v1/works/WORK-0020-role-aware-live-connections.md`, exactly like WORK-0006's own client-delivery half - there is no `play`/transport code to extend today (former Blockers 2/4).
- Durably persisting `RunOutcome`'s own payload (`Result`/`Error`/`Reason`) - confirmed NOT NEEDED (human-confirmed 2026-09-24): nothing reads it today, and WORK-0019's replay-first model can re-derive it later if a concrete consumer ever needs it.
- A personalized/configurable end-of-game screen (winners/losers/score, derived from `RunOutcome.Result`) - `docs/product/IDEAS.md -> Personalized End-of-Game Screen`, explicitly deferred.
- Lobby-phase expiration (`LOBBY_EXPIRED`) notification - a Session can only expire from `LOBBY`, before `Start`, when whether any client is even connected yet is a separate question this WORK does not resolve; may be worth folding in later but is not assumed here. This becomes concretely reachable once WORK-0008 (Live Lobby / Session Bootstrap) lets clients hold a live connection before `Start` - worth resolving when this WORK's play half and WORK-0008 are jointly refined, not decided here.
- The general "other connected players learn about ordinary gameplay state changes" gap (WORK-0006) - a different mechanism (per-recipient `Event` translation vs. this WORK's session-wide broadcast); the two WORKs are related but not the same code path.
- Reconnect/resync - a client not connected at the moment of termination simply never received anything, consistent with this whole initiative's best-effort/no-replay stance (GAME-ADR-0020).
- Any change to how a non-fatal, non-terminal decline (`AnswerInteractionOutcomeRejected`/`Conflict`) is reported - unaffected, still only a direct reply to the caller.

## Scope Addition (Engine Output/Outcome Renaming, 2026-09-24)

While drafting this WORK's own domain-half scope, a human review of `engine.WorkflowCompletedOutput`/`WorkflowOutcome`/`WorkflowOutcomeKind` questioned why `game/session` - a consumer that only compiles and executes a Game Language `Program`, the way a caller runs a compiled binary without needing to know it was built from a compiler's own AST/IR - would need to reason in terms of "workflow" at all: that is the engine's own internal execution formalism, not something a caller consuming its reported facts needs to know about. Traced to confirm scope: `game/session` production code never touches `program.WorkflowDeclaration`, `engine.Workflow`, `engine.WorkflowInstance`, or `Program.Workflows`/`RootWorkflow` - those remain genuinely internal-to-`engine`/authoring-DSL concepts, unaffected by this change. The only symbols `game/session` (via this WORK) actually needs to reference directly are the three renamed here, human-approved 2026-09-24:

- `engine.WorkflowCompletedOutput` -> `engine.RunCompletedOutput` - also drops its `Workflow string` field (which compiled workflow template backed the instance): now that GAME-ADR-0026 guarantees exactly one instance per Session, this field only ever held an authoring-internal name a caller has no use for, the same category of vestigial field GAME-ADR-0026 itself already removed (`Path`).
- `engine.WorkflowOutcome` -> `engine.RunOutcome`
- `engine.WorkflowOutcomeKind` (and its `WorkflowOutcomeCompleted`/`Failed`/`Cancelled` constants) -> `engine.RunOutcomeKind` (`RunOutcomeCompleted`/`Failed`/`Cancelled`)

This is a pure rename plus one field removal - no behavior change, no new persistence, no wire-format impact (only `engine`'s internal advanced/tooling Snapshot codec serializes `RunOutcome`, and its JSON keys were already generic - `kind`/`result`/`error`/`reason` - never `workflow`-prefixed). Done and verified (full repository `go build`/`go vet`/`go test ./... -count=1`, real Postgres, all green) before implementing the rest of this WORK, so this WORK's own domain-half code is written directly against the renamed surface rather than needing a second pass. Updated: `engine/output.go`, `engine/instance.go`, `engine/commit.go`, `engine/internal/runtime/{step.go,execute.go}`, `engine/internal/codec/instance.go`, `engine/engineservice/*_test.go`, `engine/internal/runtime/*_test.go`, `game/language/v1/example.go`, `engine/README.md`, `engine/IMPLEMENTATION.md`. `game/docs/decisions/GAME-ADR-0026-...md` (historical, immutable) is unaffected and not updated - it predates and is not superseded by this pure naming cleanup.

## Scope Correction (Domain/Play Split, 2026-09-24)

This WORK was drafted 2026-09-20, mixing a Session-Runtime-domain concern (recognizing `RunCompletedOutput`, terminalizing the Session) with a `play`/transport concern (broadcasting a message, closing sockets) in one document - the same drift `PROJECT.md`'s Restructuring (2026-09-21) already flagged for WORK-0006/0007/0010/0011/0012, corrected for WORK-0006 on 2026-09-23. Per explicit human direction (2026-09-24), this WORK receives the same treatment: narrowed to its domain half only (former Blockers 1 and 3, both now resolved), with the broadcast/transport half (former Blockers 2 and 4) deferred to a new, not-yet-drafted WORK that lands once `WORK-0020` has rebuilt the `play` Coordinator from scratch - consistent with `PROJECT.md`'s inside-out Phase 1/Phase 2 sequencing. No production code was implemented before this correction.

## Blockers

Status: **RESOLVED, HUMAN-APPROVED (2026-09-24)**.

1. ~~Exact `TerminalReason`/business-logic shape for natural completion.~~ **Resolved**: three distinct values (`TerminalReasonGameCompleted`/`GameFailed`/`GameCancelled`), one per `engine.RunOutcomeKind`, named for the game's own outcome per the Context section's naming rationale above. No durable payload persistence. Detection happens directly in `step_start.go`/`step_answer_interaction.go` against the already-returned `outputs` slice (the same slice `interactions.Capture`/`clientoutputs.ClientFacing` already inspect) - a shared cross-step helper under `sessionlifecycle`'s own `internal/` per `docs/engineering/standards/domain-logic-placement.md`'s Shared Cross-Step Behavior standard, exact naming/placement Implementation Freedom.
2. ~~Where does "broadcast to everyone bound" live~~ - deferred to the play-half WORK (not this WORK's own scope, see Scope Correction above).
3. ~~How does the rebuilt `play` Coordinator learn "this call's result means the Session is now `TERMINAL`"~~ **Resolved**: an explicit additive field on `Manager`'s own `StartResult`/`AnswerInteractionResult`, this WORK's own domain-half scope (see Scope above); the rebuilt Coordinator reads it once WORK-0020 lands.
4. ~~Transport-level "send this message, then close the socket"~~ - deferred to the play-half WORK (not this WORK's own scope, see Scope Correction above).

Local implementation choices (exact `TerminalReason` string values beyond the three distinct concepts above, exact new field name/shape on `StartResult`/`AnswerInteractionResult`, exact detection helper's file/package name) remain Implementation Freedom.

## Acceptance Criteria

- A Definition authored so that answering a specific question causes the workflow to `Complete` results in `Manager.AnswerInteraction` terminalizing the Session (`phase = TERMINAL`, `terminal_reason = TerminalReasonGameCompleted`), closing any other still-`ACTIVE` interaction for that Session, and returning termination info on `AnswerInteractionResult` - verified by inspecting persisted state and the returned Go value directly, no live transport involved.
- The same, for a Definition whose answered transition applies an authored `FailControl` (`TerminalReasonGameFailed`) and one that applies an authored `CancelControl` (`TerminalReasonGameCancelled`) - all three reasons distinguishable, none conflated with each other or with the existing `RuntimeExecutionFailed`/`RuntimeStateInvalid` fatal-path reasons.
- The same proof for `Manager.Start`'s own first RuntimeTurn, for a Definition whose very first (`WorkflowStarted`) transition already completes/fails/cancels the workflow.
- No new durable column/table (verified by its own migration diff, if any, being empty) - `RunOutcome`'s own payload is not persisted.
- No change to any already-correct non-terminal outcome's behavior, and no change to the existing fatal-path (`RuntimeExecutionFailed`/`RuntimeStateInvalid`) behavior.
- `go build ./...`, `go vet ./...`, `go test ./... -count=1` pass, with no new failure beyond the already-recorded out-of-scope `getgame` JSONB-comparison defect.

The eventual client-visible "session ended" message/connection-close behavior remains the governing outcome for whichever play-half WORK eventually delivers this over the wire, but it is not this WORK's own Acceptance Criteria - this WORK produces no client-visible behavior change.

## Documentation Impact

- `game/CURRENT_STATE.md`, `game/docs/FLOWS.md` - describe `Manager`'s new game-completion detection/termination and the three new `TerminalReason` values, once implemented; explicitly note that client-visible delivery does not yet exist.
- `game/session/session.go` - three new `TerminalReason` constants, documented alongside the existing three.

## Scope Addition (Part H Reconciliation, 2026-09-20)

`game/docs/decisions/GAME-ADR-0025-role-aware-live-connections.md` (accepted by a broader reconciliation session, alongside `docs/projects/active/session-runtime-v1/works/WORK-0020-role-aware-live-connections.md`) establishes that a Session's live transport has two independent connection roles, `ADMIN` and `PARTICIPANT`, and that the same `UserUUID` may legitimately hold one of each simultaneously. The eventual play-half's "broadcast to every currently-bound connection for a Session" mechanism must reach every bound connection of *either* role, not only the participant registry this WORK's design was originally sketched against - including both of a host-as-Participant's two independent connections. This does not affect this WORK's own domain-half scope (unchanged by which/how many connections eventually receive the fact); it is recorded here for whoever drafts the play-half WORK.

## Completion Record

**DONE (2026-09-24).** Independent review by a fresh agent (no access to this session's own context) - APPROVED, three NON_BLOCKING findings, two fixed same-session. See "Implementation Report" and "Independent Review" below.

### Implementation Report (2026-09-24)

Work: `docs/projects/active/session-runtime-v1/works/WORK-0007-session-termination-live-notification.md`

Work status: IMPLEMENTING

Implemented:
- **Engine API rename (prerequisite, see "Scope Addition (Engine Output/Outcome Renaming, 2026-09-24)" above)**: `engine.WorkflowCompletedOutput` -> `RunCompletedOutput` (its `Workflow string` field dropped), `engine.WorkflowOutcome` -> `RunOutcome`, `engine.WorkflowOutcomeKind`/its three constants -> `RunOutcomeKind`/`RunOutcomeCompleted`/`RunOutcomeFailed`/`RunOutcomeCancelled`. Updated every call site (`engine/output.go`, `instance.go`, `commit.go`, `internal/runtime/{step.go,execute.go}`, `internal/codec/instance.go`, every `engineservice`/`internal/runtime` test that referenced the old names, `game/language/v1/example.go`) plus `engine/README.md`/`IMPLEMENTATION.md`.
- Three new `session.TerminalReason` values (`session.go`): `TerminalReasonGameCompleted`/`GameFailed`/`GameCancelled`, one per `engine.RunOutcomeKind`.
- New shared subpackage `sessionlifecycle/internal/completion` (`completion.go`): `Detect(outputs []engine.Output) (reason string, terminated bool)` scans a Turn's Outputs for a `RunCompletedOutput` and maps its `Outcome.Kind` to the matching `TerminalReasonGame*` value.
- `step_start.go`/`step_answer_interaction.go`: after a Turn commits, call `completion.Detect`; when it reports termination, call `SetSessionTerminal` (Start also calls `SetSessionRunning` first, since the Session genuinely ran before ending - `started_at` stays set) and `CloseAllActiveInteractionsForSession`, in the same transaction as the Turn. `startRepoAPI` gained `CloseAllActiveInteractionsForSession` (already satisfied by `internalrepo.Repo`).
- `StartResult`/`AnswerInteractionResult` gain an additive `TerminalReason string` field (persisted/replayed normally, unlike `Outputs` - a plain string has no `encoding/json` interface-decode problem), populated whenever the same call's Turn also terminalized the Session.
- New tests: `internal/completion/completion_test.go` (`TestDetect`, pure, no DB); new fixtures `answerTriggersTerminationDefinition`/`startTriggersTerminationDefinition`/`startedTerminationControlSession` (`testutil_test.go`); new integration subtests proving all three outcomes (Completed/Failed/Cancelled) terminalize the Session with the correct reason, close a bystander's still-`ACTIVE` interaction via terminal cleanup (not gameplay closure), and prove the same for `Start`'s own first Turn (`TestManagerStart_Integration/first_turn_completing_the_game_terminalizes_session_immediately`, `TestManagerAnswerInteraction_Integration/answer_{completing,failing,cancelling}_the_game_...`).
- Documentation synchronized: `game/CURRENT_STATE.md`, `game/docs/FLOWS.md` (both sequence diagrams, their bullets, and evidence lists).

Local implementation decisions:
- Exact `TerminalReason` string values (`GAME_COMPLETED`/`GAME_FAILED`/`GAME_CANCELLED`), the `completion` package name, and `StartResult`/`AnswerInteractionResult.TerminalReason`'s exact field name/shape - Implementation Freedom per the WORK's own text.
- `TerminalReason` is included in `StartResult`'s persisted idempotency JSON (no `json:"-"`), unlike `Outputs`: it is a plain string with no interface-decode hazard, and a replayed retry correctly reporting the original termination reason is strictly more useful than omitting it.

Deviations from the approved WORK:
- None.

Discoveries:
- None requiring escalation. The engine rename's scope (which exact symbols `game/session` actually touches) was investigated and confirmed narrow before implementing, per the "Scope Addition (Engine Output/Outcome Renaming, 2026-09-24)" section above - not a deviation, since the human directed and approved the rename itself in this same session before implementation began.

Verification performed:
- `go build ./...`, `go vet ./...` - clean, repository-wide.
- `gofmt -l`/`gofmt -d` on every changed/new `.go` file - flagged files are pure pre-existing CRLF-line-ending noise (confirmed via line-ending-normalized diff), consistent with the same finding WORK-0028/WORK-0006 already recorded.
- `go test . -run TestNoInternalDocCitationsInComments` - passes.
- `go test ./... -count=1` against real Postgres - full repository suite green except the already-known, pre-existing, out-of-scope `getgame` JSONB-whitespace defect (unrelated package, unrelated to this WORK).
- Mocks regenerated via `mockgen` after `startRepoAPI`'s interface change (byte-identical for the unaffected embedded interfaces, new mock methods generated for `CloseAllActiveInteractionsForSession`).

Documentation synchronized:
- `game/CURRENT_STATE.md`, `game/docs/FLOWS.md` - see Implemented above.
- `game/session/session.go` - three new `TerminalReason` constants, documented inline.
- `docs/projects/active/session-runtime-v1/PROJECT.md` - updated alongside this closure (Current Work, WORK table, Capability Coverage).

Known limitations:
- None beyond what this WORK's own Approved Design already scoped out (client-facing wire delivery, timers, each owned elsewhere).

Ready for independent review:
YES.

### Independent Review (2026-09-24)

A fresh agent, with no access to this session's own context, reviewed this WORK per `docs/ai/protocols/IMPLEMENTATION_REVIEW.md`. It determined the actual change-set via `git status`/`git diff` (confirmed exact match to scope, no ambiguity), read every changed production file in full (engine rename call sites, `completion.go`, both step files, `types.go`), confirmed via repository-wide search that zero `.go` references to the old engine names remain anywhere (only historical/immutable documents still name them, correctly), traced the transaction boundary and `started_at` semantics to prove Start's own immediate-completion case is genuinely distinguishable from the pre-existing pre-first-Turn fatal path, read the new tests in full to confirm they assert real Postgres-backed behavior (including `InteractionStateTerminated` vs. `Closed` for the bystander proof), checked every non-terminating code path for a correctly-empty `TerminalReason`, and ran its own fresh `go build`/`go vet`/`gofmt -l`+`gofmt -d`/`TestNoInternalDocCitationsInComments`/the full repository suite against real Postgres.

Verdict: APPROVED, three NON_BLOCKING findings, no REQUIRED_FIX or DECISION_REQUIRED findings.

Findings and fixes:
1. **(LOW, NON_BLOCKING)** `AnswerInteraction`'s pre-existing "replay by interaction state" branch (a retried, semantically-equivalent answer to an already-`CLOSED` interaction) always returned `TerminalReason: ""`, even when the original answer that closed the interaction also terminalized the Session - unlike Start's own idempotency-JSON replay, which correctly persists/replays it. **Fixed**: that branch now reads `lockedSession.TerminalReason` (already loaded under lock) and includes it. New test added: `TestManagerAnswerInteraction_Integration/retried_equivalent_answer_completing_the_game_still_reports_terminal_reason`.
2. **(LOW, NON_BLOCKING)** This WORK's own Implementation Report claimed `PROJECT.md` was updated as part of documentation synchronization, but it had not actually been edited yet. **Fixed**: `PROJECT.md`'s Current Work bullet, WORK table row, both Capability Coverage rows, and the WORK-count summary line are now updated to reflect this WORK's DONE status.
3. **(LOW, NON_BLOCKING)** `gofmt -l` flags `step_start.go`/`step_answer_interaction.go` for a real (non-CRLF) import-ordering issue (`internalrepo` listed before `internal/replay`, alphabetically reversed) - confirmed pre-existing in `HEAD` before this WORK's changes, untouched by this diff. Not fixed, as it is out of this WORK's own scope and was not introduced or worsened by it.

Re-verified after applying fixes 1-2: `go build ./...`, `go vet ./...` - clean; `go test ./game/session/... -count=1` (real Postgres) - all pass, including the new replay test. No unresolved REQUIRED_FIX or DECISION_REQUIRED finding remains. Closed to DONE.
