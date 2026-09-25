# WORK-0006: Broaden Live Fan-Out — Effects and Presentations (UI) — Domain Half

Status: DONE
Created: 2026-09-20
Last status change: 2026-09-24 (IMPLEMENTING -> DONE, independent review APPROVED with one NON_BLOCKING finding, fixed same-session; DRAFT -> READY -> IMPLEMENTING, resumed: the human explicitly instructed resuming this WORK, with no unresolved blocker to raise first - both conditions the prior pause was waiting on were already satisfied by that point (GAME-ADR-0026 is accepted and implemented, and the recommended prerequisite `docs/projects/completed/game-language-flat-execution-model/works/WORK-0028-engine-owned-turn-execution.md` is DONE, landing `engineservice.StartTurn`/`AdvanceTurn` returning `[]engine.Output` directly to `sessionlifecycle`'s two call sites - exactly the surface this WORK needed settled before implementing once), and this WORK's own Blockers 1/2 were already RESOLVED, HUMAN-APPROVED on 2026-09-20 and unaffected by the pause. Prior update, same day: READY -> DRAFT: paused, and a first implementation pass reverted, pending GAME-ADR-0026 (then PROPOSED) - see "Paused (2026-09-24): Game Language Engine Redesign" below. Prior update: 2026-09-23, DRAFT -> READY, human-approved, immediately after the Domain/Play Split narrowed this WORK to the domain half only - the client-delivery half this WORK originally also described is deferred to a new, not-yet-drafted WORK that follows WORK-0020's Coordinator rebuild - see "Scope Correction (Domain/Play Split, 2026-09-23)" below. Prior update: 2026-09-20 Part C reconciliation, same day: Start-`Seed` persistence removed from this WORK's scope - see "Scope Correction (Part C Reconciliation, 2026-09-20)" below)

Related decisions:
- GAME-ADR-0002 (Session Runtime durable boundary, Live Session Coordinator responsibility boundary)
- GAME-ADR-0007 (RuntimeTurn/Interaction persistence model - `OpenQuestionOutput`/`CloseQuestionOutput` durable capture is unaffected by this WORK)
- GAME-ADR-0020 (post-commit client delivery semantics, best-effort, no outbox)

Canonical context:
- `docs/projects/active/session-runtime-v1/works/WORK-0005-thin-live-coordinator.md` (Blocker 4's resolved scope: only `OpenQuestionOutput`/`CloseQuestionOutput` were ever translated, back when `play` existed; also the origin of the "`Manager`'s method signatures/internal behavior do not change" constraint this WORK's Blocker 1 explicitly revises; Blocker 11 records that `play`/`play/sessionruntime` were deleted in full on 2026-09-21 - no Coordinator/transport code exists to extend today)
- `docs/projects/active/session-runtime-v1/works/WORK-0020-role-aware-live-connections.md` (owns rebuilding the `play` Coordinator from scratch; this WORK's play-half, once drafted, lands on top of it - see "Scope Correction (Domain/Play Split, 2026-09-23)" below)
- `api/README.md` (current transport shape - a skeleton only, per WORK-0005's Blocker 11; no domain-facing code exists in it yet)
- `game/language/v1/engine/output.go` (`EmitEffectOutput`, `ActivatePresentationOutput`, `UpdatePresentationOutput`, `RemovePresentationOutput` field definitions)
- `game/language/v1/engine/internal/runtime/presentation.go` (`deriveActivePresentations`/`diffPresentations` - how the engine decides Activate vs. Update vs. Remove per commit; also why a future resync capability needs none of this WORK's own persistence - see Context)
- `game/language/v1/engine/README.md` (Determinism section - the basis for this WORK's Seed-persistence addition)
- `game/session/workflows/sessionlifecycle/internal/runtimeturn/runtimeturn.go` (`StepTrace` - already carries `Outputs` in memory during drain; today deliberately excluded from what gets persisted, `json:"-"`)
- `game/session/workflows/sessionlifecycle/step_start.go` (`drawSeed()` - draws the one-time Start `Seed` that is not currently persisted anywhere)

## Outcome

A game's authored UI (`ActivatePresentationOutput`/`UpdatePresentationOutput`/`RemovePresentationOutput` - a mounted view with its data model, per `(recipient, slot)`) and presentation-only effects (`EmitEffectOutput` - animation/sound/etc., addressed to one or more recipients) become available to whatever calls `sessionlifecycle.Manager`, with **zero new durable persistence** for either. This is the domain-side prerequisite for those Outputs eventually reaching connected clients over the live transport, the same way `OpenQuestionOutput`/`CloseQuestionOutput` already did when `play` existed - actual client delivery is a separate, deferred WORK (see Scope).

## Context

### The wire-protocol design principle this WORK establishes

Only three shapes of message ever cross the live-transport boundary to a client: **Question** (already implemented), **Presentation** (a mounted UI component with its data), and **Effect** (a presentation-only animation/sound). A client is never told that some other internal thing happened ("an interaction was answered", "a turn committed") as its own event - if that fact matters to a client, it is because the backend already decided it changes what that client's UI shows, which means it manifests as a Presentation update or an Effect addressed to that client, never as a raw domain/interaction notification. This principle governs every future Output the (not-yet-rebuilt) Coordinator ever fans out, not only the two kinds this WORK adds - it should be documented in `play/README.md` as an ongoing design rule once WORK-0020 recreates that file, not only in this WORK's own scope.

### Why zero persistence is correct here, not merely convenient

`OpenQuestionOutput`/`CloseQuestionOutput` are durably captured (`session_interactions`) for a reason independent of live delivery: `AnswerInteraction` needs that row later, when the player actually responds, possibly a long time after it opened. Presentations and Effects have no such independent need - nothing ever reads a past Presentation/Effect back to decide anything. The only reason WORK-0006 originally proposed capturing them at all was mechanical: `sessionlifecycle.Manager` does not return a Turn's Outputs to its caller, so a durable capture-then-read-back step was the only way `play/sessionruntime` could learn what happened. That constraint is what this WORK's Blocker 1 revises - once `Manager` returns the Outputs directly, there is nothing left to capture.

This also removes the concern raised in this WORK's original Blocker 1 about a future resync (Slice 7) needing "currently active Presentation state": `deriveActivePresentations` (`game/language/v1/engine/internal/runtime/presentation.go`) is a pure function of `(Program, the current WorkflowInstance)` - the *current* Session Snapshot already contains everything needed to recompute "what's currently mounted for this player" from scratch, on demand, exactly when a resync capability needs it. There is nothing to keep in sync with a separate table, because nothing needs to be kept - resync recomputes, it does not replay history.

### Correlation note (added 2026-09-20, migration reconciliation - no scope change)

Game Language's `AnswerQuestionAction` carries no question-slot identity of its own (it relies entirely on the mounted presentation/question context to identify which open interaction it answers). This is already correctly resolved today, by WORK-0005/WORK-0004, not by this WORK: Session Runtime persists each opened interaction as its own `session_interactions` row (keyed by the engine's own `(EnginePath, EngineSlot)` identity, GAME-ADR-0007), and the live wire protocol already carries that row's opaque ID as routing/correlation context for an answer (`api/session/wire.go`'s `inboundMessage.InteractionID`, part of WORK-0005). An `AnswerQuestion` action from a rendered QuestionPresentation therefore already resolves unambiguously to the intended open Interaction without exposing internal engine slot/path identities. This WORK adds no new correlation mechanism and does not need one - it is recorded here only so the requirement is visibly confirmed satisfied, not silently unaddressed.

### Replay is a separate, later concern, now owned by WORK-0019

A human question during this WORK's own design confirmed engine execution is fully deterministic given `(compiled Program, starting Snapshot, driving Signal sequence)` - no wall-clock, no OS randomness anywhere in `game/language/v1/engine`. That means Presentations/Effects genuinely do not need to be persisted for a future replay/rewatch capability - they are mechanically re-derivable by replaying durably-captured inputs through the same compiled Program. This WORK originally also persisted the one-time `Seed` drawn at Start (`step_start.go`'s `drawSeed()`, otherwise unrecoverable once folded into `Snapshot.Random`) as a small addition toward that goal. **That addition has moved (2026-09-20, Part C reconciliation) to `docs/projects/active/session-runtime-v1/works/WORK-0019-replay-first-session-runtime-persistence-migration.md`**, which now owns the complete durable replay-input model (Seed, `RootParameters`, and every other RuntimeTurn-driving cause), per `game/docs/decisions/GAME-ADR-0024-replay-first-session-runtime-persistence.md`. This WORK no longer persists anything toward that goal - see "Scope Correction (Part C Reconciliation, 2026-09-20)" below. Whether to eventually stop persisting full per-Turn Snapshots is no longer a deferred idea - GAME-ADR-0024 already accepts stopping that, and WORK-0019 owns the migration.

## Scope

### In Scope

- Extend `sessionlifecycle.Manager.Start` and `Manager.AnswerInteraction`'s return types to also carry the committed Turn's `EmitEffectOutput`/`ActivatePresentationOutput`/`UpdatePresentationOutput`/`RemovePresentationOutput` values directly - additive data only, already computed in memory during `runtimeturn.Drain` (`drainResult.Steps[*].Outputs`) and simply not currently returned. No change to either method's existing parameters, business logic, or persistence for anything already implemented (Open/CloseQuestion capture, idempotency, transaction scope all unchanged).

### Out of Scope

- **(2026-09-23, Domain/Play Split) Translating these in-memory Outputs into `play.Event`s/wire messages and actually delivering them to a connected client.** There is no `play`/transport code left to extend - `play`/`play/sessionruntime` were deleted in full on 2026-09-21 (WORK-0005 Blocker 11) and have not been rebuilt yet. That rebuild is WORK-0020's own scope (the Coordinator, both connection registries, command dispatch); a play-half WORK for this translation - not yet drafted or numbered - follows once WORK-0020 lands, consistent with `PROJECT.md`'s inside-out Phase 1/Phase 2 sequencing. This WORK's own completion does not require, and does not claim, any client-visible behavior change.
- New `play.EventKind` values or wire message types (`api/session/wire.go`) for Presentation/Effect delivery - same reason, part of the deferred play-half.
- `WorkflowCompletedOutput` - picked up by `docs/projects/active/session-runtime-v1/works/WORK-0007-session-termination-live-notification.md` instead (broadened to cover natural game completion, not only fatal failure), since a human-confirmed follow-up decided the root workflow completing is the deterministic "game over" signal and belongs with that WORK's termination-notification mechanism, not this one's Output-translation mechanism.
- `ScheduleTimerOutput`/`CancelTimerOutput` - WORK-0012's own scope, unaffected by this WORK.
- Any change to delivery reliability/guarantees - GAME-ADR-0020's best-effort, no-outbox, no-replay rule will govern these Outputs' eventual delivery exactly as it governs Open/CloseQuestion today, once the play-half exists.
- **(2026-09-20, Part C reconciliation) Persisting Start's `Seed`, or any other replay-input persistence.** Moved to WORK-0019, which owns the complete durable replay-input model per GAME-ADR-0024. This WORK adds no new durable column of any kind.
- Reducing existing full-Snapshot persistence - no longer a deferred idea; owned by WORK-0019 per GAME-ADR-0024, not this WORK's concern.

## Blockers

Status: **RESOLVED, HUMAN-APPROVED (2026-09-20)**.

1. **Persist Presentation/Effect state, or have `Manager` return Outputs directly?** Originally DRAFT pending this decision. **Resolution: HUMAN-APPROVED - return directly, zero persistence.** This revises WORK-0005's "`Manager`'s method signatures do not change" constraint, deliberately and narrowly: `Start`/`AnswerInteraction` gain additive return data only, nothing about their existing behavior, parameters, or the durable capture Open/CloseQuestion already relies on changes. Confirmed this does not create a future resync gap (`deriveActivePresentations` recomputes current Presentation state fresh from the current Snapshot on demand - see Context).
2. **Does replay remain theoretically possible without persisting Presentations/Effects?** **Resolution: yes, confirmed** - engine execution is fully deterministic given `(Program, Snapshot, Signal sequence)`, verified against `game/language/v1/engine`'s actual source (no wall-clock/OS-randomness dependency anywhere). Closing the actual replay-input gaps (Start's `Seed` and the rest of the durable replay-input model) is owned by WORK-0019 (GAME-ADR-0024), not this WORK - this WORK's own zero-persistence choice for Presentations/Effects does not make that goal any less achievable.

Local implementation choices (exact wire message names/shapes, exact new return-type shape on `StartResult`/`AnswerInteractionResult`) remain Implementation Freedom.

## Acceptance Criteria

- A Definition authored so that answering one player's question causes `EmitEffectOutput`/`ActivatePresentationOutput`/`UpdatePresentationOutput` addressed to every player in the roster (via `Recipients`/target-users referencing the `players` list) results in `Manager.AnswerInteraction`'s returned result value containing exactly those Outputs, in commit order - verified by a test inspecting the returned Go value directly, no live transport involved.
- The same, for `Manager.Start`'s first RuntimeTurn.
- No Output this WORK returns is fabricated or reordered relative to what `runtimeturn.Drain` actually produced during the same commit.
- This WORK adds no new durable column/table (verified by its own migration diff, if any, being empty).
- `go build ./...`, `go vet ./...`, `go test ./... -count=1` pass, with no new failure beyond the already-recorded out-of-scope `getgame` JSONB-comparison defect.

The wire-protocol design principle ("a client only ever receives Question-, Presentation-, or Effect-shaped messages, never a raw internal/domain fact") remains the governing rule for whichever WORK eventually delivers these Outputs over the wire, but it is not this WORK's own Acceptance Criteria - this WORK produces no wire message.

## Documentation Impact

- `game/CURRENT_STATE.md`, `game/docs/FLOWS.md` - describe `Manager`'s broadened return values once implemented; explicitly note that client-visible delivery does not yet exist.
- `docs/projects/active/session-runtime-v1/PROJECT.md` - WORK-0006's row update alongside this WORK's revision.

## Scope Correction (Part C Reconciliation, 2026-09-20)

This WORK originally included persisting Start's `Seed` (a single new durable column) as a small addition toward keeping replay theoretically possible. A broader reconciliation session accepted GAME-ADR-0024 (Replay-First Session Runtime Persistence), which gives the *complete* durable replay-input model - Seed, `RootParameters`, and every other RuntimeTurn-driving cause - a single dedicated owner, `docs/projects/active/session-runtime-v1/works/WORK-0019-replay-first-session-runtime-persistence-migration.md`. Persisting only the Seed here, in isolation from that complete model, would risk a mismatched or premature partial implementation of a model WORK-0019 owns designing as a whole. This WORK's scope, Blockers, Acceptance Criteria, and Documentation Impact above have been revised accordingly: this WORK now adds zero new durable persistence of any kind and is purely about live UI delivery (Presentations/Effects fan-out), exactly matching its own title. No production code was implemented or changed by this correction; this WORK's Status remains DRAFT, unaffected by this scope narrowing (still awaiting human READY authorization, as before).

## Scope Correction (Domain/Play Split, 2026-09-23)

This WORK's own Scope/Acceptance Criteria/Documentation Impact above still described translating the returned Outputs into `play.Event`s and delivering them over the wire, as if `play` were a live package to extend. It is not: `play`/`play/sessionruntime` were deleted in full on 2026-09-21 (WORK-0005 Blocker 11, `PROJECT.md`'s Restructuring), and the codebase currently has no Coordinator/transport code of any kind beyond WORK-0005's route/WebSocket-upgrade skeleton. This WORK was drafted the same day as that deletion and was never reconciled against it - a genuine documentation drift, not a new decision. Per the human's explicit direction (2026-09-23), this WORK is narrowed to its domain half only: `Manager` returning the Outputs in memory. The client-delivery half - translating those Outputs into `play.Event`s/wire messages and actually reaching a connected client - is deferred to a new WORK, not yet numbered or drafted, that is written once `docs/projects/active/session-runtime-v1/works/WORK-0020-role-aware-live-connections.md` has rebuilt the `play` Coordinator from scratch (WORK-0020 is now that reintroduction WORK - see its own "Scope Correction" section). This matches `PROJECT.md`'s already-accepted inside-out Phase 1/Phase 2 sequencing and its own note that WORK-0006's play half is "not yet split into documents." No production code was touched by this correction.

## Paused (2026-09-24): Game Language Engine Redesign

A human design discussion, prompted by revisiting why answering an interaction requires the caller to know engine-internal `Path`/`Slot` addressing, escalated into re-examining whether Game Language's nested-workflow execution model (Child Workflows, Task Groups) is needed at all. An audit against the product's target game range (Parqués, UNO, Poker, Monopoly, The Game of Life, Clue, both synchronous and asynchronous Kahoot/Quizizz-style quizzes, word search/crossword, and a deliberately hard two-team simultaneous-decision scenario) found none of them need a genuinely separate execution instance - every case is expressible on one flat workflow instance with keyed interaction slots (generalizing GAME-ADR-0012's Keyed Timer Slots to Questions/Ask Groups/Presentations) and engine-assigned interaction IDs replacing `Path`/`Slot` addressing entirely. This is now `game/docs/decisions/GAME-ADR-0026-flat-workflow-execution-model-and-keyed-interaction-slots.md` (PROPOSED).

This WORK's own already-completed implementation pass (`StartResult`/`AnswerInteractionResult` gaining an `Outputs` field, `collectTurnOutputs`, the `presentationEffectDefinition` test fixture) was reverted - not because it was incorrect, but because `sessionlifecycle` sits directly on the engine `Output`/`Signal` shapes GAME-ADR-0026 changes, and continuing to build or review against the current engine surface would mean near-certain rework the moment that record is accepted and implemented. This WORK returns to DRAFT rather than staying READY, since resuming it needs to be re-specified (if at all materially) against whatever the accepted engine contract ends up being, per the normal re-approval path for a material discovery during implementation.

This WORK, and the rest of `session-runtime-v1`'s Phase 1, stay paused until GAME-ADR-0026 is accepted and at least the engine/compiler/program side of it is implemented. See `PROJECT.md`'s own pause note.

## Completion Record

**DONE (2026-09-24).** Independent review by a fresh agent (no access to this session's own context) - APPROVED, one NON_BLOCKING finding (this WORK's own status-history wording), corrected in the same pass, no re-review required for a documentation-only correction.

### Implementation Report (2026-09-24)

Work: `docs/projects/active/session-runtime-v1/works/WORK-0006-broaden-live-fanout-effects-presentations.md`

Work status: IMPLEMENTING

Implemented:
- `sessionlifecycle.StartResult`/`AnswerInteractionResult` gain an additive `Outputs []engine.Output` field (`types.go`), tagged `json:"-"` so it never enters the JSON payload `Start` persists for idempotent replay (an interface-typed slice `encoding/json` cannot decode back into concrete types).
- New `clientFacingOutputs(outputs []engine.Output) []engine.Output` (`outputs.go`) selects exactly `EmitEffectOutput`/`ActivatePresentationOutput`/`UpdatePresentationOutput`/`RemovePresentationOutput`, in original order, out of a Turn's full Output list - excluding Question-kind Outputs (already durably captured by `captureInteractions`) and every Output kind this WORK does not own (timers, `WorkflowCompleted`).
- `step_start.go`: the fresh-execution success path (`StartOutcomeStarted` after `engineservice.StartTurn` actually commits) sets `Outputs: clientFacingOutputs(outputs)`. Every other return path (the already-RUNNING short-circuit, every decline, both fatal-terminalization paths, every idempotency-replay path) leaves `Outputs` at its nil zero value, since none of them executed the engine.
- `step_answer_interaction.go`: only the path that actually calls `engineservice.AdvanceTurn` and commits a new RuntimeTurn (`AnswerInteractionOutcomeAnswered`, non-replayed) sets `Outputs: clientFacingOutputs(outputs)`. Every declined/conflicting/replayed-equivalent-answer path leaves it nil.
- New test fixture `presentationEffectDefinition` (`testutil_test.go`): a workflow-level Presentation (slot "hud") targeting the full player roster, projecting global `score`, mounted at Start (`ActivatePresentationOutput`) and recomputed when a Question-answer transition sets `score` (`UpdatePresentationOutput`), which also emits a client-facing Effect addressed to every player (`EmitEffectOutput`).
- New integration test cases (real Postgres): `TestManagerStart_Integration/first_turn_returns_activated_presentations_in_memory` and `TestManagerAnswerInteraction_Integration/accepted_answer_returns_effect_and_updated_presentation_outputs`, asserting the returned `Outputs` against real engine-computed values (actual actor-ID-derived recipients, actual recomputed presentation models) rather than mocks.
- New unit test `outputs_test.go` (`TestClientFacingOutputs`) proving the filter selects the right kinds, preserves order, and returns nil when nothing qualifies - independent of any database.
- Documentation synchronized: `game/CURRENT_STATE.md` (Session Runtime row and Current Gaps), `game/docs/FLOWS.md` (both sequence diagrams and their "Implemented behavior" bullets) now describe `Start`/`AnswerInteraction` returning Effect/Presentation Outputs in memory, explicitly noting no live-transport Coordinator yet consumes them.

Local implementation decisions:
- Exact field/function naming (`Outputs`, `clientFacingOutputs`) - Implementation Freedom per the WORK's own text; no wire message or `play.Event` shape is introduced, since that remains explicitly out of this WORK's scope.
- The WORK's own Scope/Context sections still reference `runtimeturn.Drain`/`drainResult.Steps[*].Outputs` - a mechanism removed by the already-completed, independent WORK-0028 (engine-owned turn execution). The actual call sites correctly use the current post-WORK-0028 surface (`outputs []engine.Output` returned directly by `engineservice.StartTurn`/`AdvanceTurn`); this pre-existing textual drift in the WORK's historical Scope section was left uncorrected, per this repository's Completed Work Immutability convention for approved-design text, since it does not change this WORK's actual approved outcome.

Deviations from the approved WORK:
- None.

Discoveries:
- None requiring escalation. One local, non-blocking finding from independent review, corrected in the same pass: this WORK's own "Last status change" line initially narrated a "READY -> IMPLEMENTING" transition without ever recording the DRAFT -> READY step that the human's explicit resumption instruction actually authorized - corrected to "DRAFT -> READY -> IMPLEMENTING" with accurate wording.

Verification performed:
- `go build ./...`, `go vet ./...` - clean, repository-wide.
- `gofmt -l`/`gofmt -d` on every changed/new `.go` file - the flagged files are pure pre-existing CRLF-line-ending noise from this Windows checkout (confirmed via a line-ending-normalized diff), not a real formatting defect; consistent with the same finding already recorded by WORK-0028.
- `go test ./game/session/... -count=1` - all pass, including the new integration tests, against the real `playhoot-postgres-1` container.
- `go test . -run TestNoInternalDocCitationsInComments` - passes.
- `go test ./... -count=1` against real Postgres - full repository suite green, no failures.

Documentation synchronized:
- `game/CURRENT_STATE.md`, `game/docs/FLOWS.md` - see Implemented above.
- `docs/projects/active/session-runtime-v1/PROJECT.md` - updated alongside this closure (Current Work, WORK table, Capability Coverage).

Known limitations:
- None beyond what this WORK's own Approved Design already scoped out (client-facing wire delivery, timers, `WorkflowCompleted`, each owned elsewhere).

Ready for independent review:
YES.

### Independent Review (2026-09-24)

A fresh agent, with no access to this session's own context, reviewed this WORK per `docs/ai/protocols/IMPLEMENTATION_REVIEW.md`. It determined the actual change-set itself via `git status`/`git diff` (confirming it matched exactly the files this WORK's scope implies, no unrelated changes), read `game/language/v1/engine/output.go` directly to confirm `clientFacingOutputs` selects exactly the right closed set of Output kinds, traced every return path in `step_start.go`/`step_answer_interaction.go` to confirm `Outputs` is nil on every non-executing path, ran its own fresh `go build`/`go vet`/`gofmt -l`+`gofmt -d`/`TestNoInternalDocCitationsInComments`/the full `game/session` suite/the full repository suite against real Postgres, and read the new test fixture and integration assertions in full to confirm they are tied to real engine-computed values rather than tautological.

Verdict: APPROVED, one NON_BLOCKING finding, no REQUIRED_FIX or DECISION_REQUIRED findings.

Finding and fix:
1. **(LOW, NON_BLOCKING)** This WORK's own "Last status change" narration described a "READY -> IMPLEMENTING" transition without recording that the DRAFT -> READY step happened via the human's explicit instruction to resume, in this same session. **Fixed anyway**: reworded to "DRAFT -> READY -> IMPLEMENTING" naming that authorization directly.

Re-verified after applying the fix: no unresolved REQUIRED_FIX or DECISION_REQUIRED finding remains. Closed to DONE.
