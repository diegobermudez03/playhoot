# WORK-0006: Broaden Live Fan-Out — Effects and Presentations (UI)

Status: DRAFT
Created: 2026-09-20
Last status change: 2026-09-20 (Part C reconciliation, same day: Start-`Seed` persistence removed from this WORK's scope - see "Scope Correction (Part C Reconciliation, 2026-09-20)" below)

Related decisions:
- GAME-ADR-0002 (Session Runtime durable boundary, Live Session Coordinator responsibility boundary)
- GAME-ADR-0007 (RuntimeTurn/Interaction persistence model - `OpenQuestionOutput`/`CloseQuestionOutput` durable capture is unaffected by this WORK)
- GAME-ADR-0020 (post-commit client delivery semantics, best-effort, no outbox)

Canonical context:
- `docs/projects/active/session-runtime-v1/works/WORK-0005-thin-live-coordinator.md` (Blocker 4's resolved scope: only `OpenQuestionOutput`/`CloseQuestionOutput` are translated today; also the origin of the "`Manager`'s method signatures/internal behavior do not change" constraint this WORK's Blocker 1 explicitly revises)
- `play/README.md`, `api/README.md` (current Coordinator/transport shape this WORK extends, not replaces)
- `game/language/v1/engine/output.go` (`EmitEffectOutput`, `ActivatePresentationOutput`, `UpdatePresentationOutput`, `RemovePresentationOutput` field definitions)
- `game/language/v1/engine/internal/runtime/presentation.go` (`deriveActivePresentations`/`diffPresentations` - how the engine decides Activate vs. Update vs. Remove per commit; also why a future resync capability needs none of this WORK's own persistence - see Context)
- `game/language/v1/engine/README.md` (Determinism section - the basis for this WORK's Seed-persistence addition)
- `game/session/workflows/sessionlifecycle/internal/runtimeturn/runtimeturn.go` (`StepTrace` - already carries `Outputs` in memory during drain; today deliberately excluded from what gets persisted, `json:"-"`)
- `game/session/workflows/sessionlifecycle/step_start.go` (`drawSeed()` - draws the one-time Start `Seed` that is not currently persisted anywhere)

## Outcome

A game's authored UI (`ActivatePresentationOutput`/`UpdatePresentationOutput`/`RemovePresentationOutput` - a mounted view with its data model, per `(recipient, slot)`) and presentation-only effects (`EmitEffectOutput` - animation/sound/etc., addressed to one or more recipients) actually reach connected clients over the live transport, the same way `OpenQuestionOutput`/`CloseQuestionOutput` already do - with **zero new durable persistence** for either. A game authored so that one player's answer changes shared state every player must see (for example, a shared board in a card game) can now make that visible to everyone connected, not just the answering player.

## Context

### The wire-protocol design principle this WORK establishes

Only three shapes of message ever cross the live-transport boundary to a client: **Question** (already implemented), **Presentation** (a mounted UI component with its data), and **Effect** (a presentation-only animation/sound). A client is never told that some other internal thing happened ("an interaction was answered", "a turn committed") as its own event - if that fact matters to a client, it is because the backend already decided it changes what that client's UI shows, which means it manifests as a Presentation update or an Effect addressed to that client, never as a raw domain/interaction notification. This principle governs every future Output this Coordinator ever fans out, not only the two kinds this WORK adds - it is documented in `play/README.md` as an ongoing design rule, not only in this WORK's own scope.

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
- `play/sessionruntime.Start`/`AnswerInteraction` translate these in-memory Outputs directly into `play.Event`s, addressed to each recipient's caller-facing `UserUUID` (the same non-leakage translation `eventsForTurn` already does for Open/CloseQuestion, applied to in-memory data instead of a read-after-commit query) - no new table, no new read.
- New `play.EventKind` values and wire message types (`api/session/wire.go`) for a Presentation activated/updated/removed and an Effect emitted - never exposing `Slot`/internal engine identities, only `Name`/`View`/`Model` (presentations) or `Effect`/`Arguments` (effects).

### Out of Scope

- `WorkflowCompletedOutput` - picked up by `docs/projects/active/session-runtime-v1/works/WORK-0007-session-termination-live-notification.md` instead (broadened to cover natural game completion, not only fatal failure), since a human-confirmed follow-up decided the root workflow completing is the deterministic "game over" signal and belongs with that WORK's termination-notification mechanism, not this one's Output-translation mechanism.
- `ScheduleTimerOutput`/`CancelTimerOutput` - Slice 5's own scope, unaffected by this WORK.
- Any change to delivery reliability/guarantees - GAME-ADR-0020's best-effort, no-outbox, no-replay rule already governs these Outputs exactly as it governs Open/CloseQuestion today.
- **(2026-09-20, Part C reconciliation) Persisting Start's `Seed`, or any other replay-input persistence.** Moved to WORK-0019, which owns the complete durable replay-input model per GAME-ADR-0024. This WORK is now purely about live UI delivery (Presentations/Effects fan-out), not historical replay persistence, and adds no new durable column of any kind.
- Reducing existing full-Snapshot persistence - no longer a deferred idea; owned by WORK-0019 per GAME-ADR-0024, not this WORK's concern.

## Blockers

Status: **RESOLVED, HUMAN-APPROVED (2026-09-20)**.

1. **Persist Presentation/Effect state, or have `Manager` return Outputs directly?** Originally DRAFT pending this decision. **Resolution: HUMAN-APPROVED - return directly, zero persistence.** This revises WORK-0005's "`Manager`'s method signatures do not change" constraint, deliberately and narrowly: `Start`/`AnswerInteraction` gain additive return data only, nothing about their existing behavior, parameters, or the durable capture Open/CloseQuestion already relies on changes. Confirmed this does not create a future resync gap (`deriveActivePresentations` recomputes current Presentation state fresh from the current Snapshot on demand - see Context).
2. **Does replay remain theoretically possible without persisting Presentations/Effects?** **Resolution: yes, confirmed** - engine execution is fully deterministic given `(Program, Snapshot, Signal sequence)`, verified against `game/language/v1/engine`'s actual source (no wall-clock/OS-randomness dependency anywhere). Closing the actual replay-input gaps (Start's `Seed` and the rest of the durable replay-input model) is owned by WORK-0019 (GAME-ADR-0024), not this WORK - this WORK's own zero-persistence choice for Presentations/Effects does not make that goal any less achievable.

Local implementation choices (exact wire message names/shapes, exact new return-type shape on `StartResult`/`AnswerInteractionResult`) remain Implementation Freedom.

## Acceptance Criteria

- A Definition authored so that answering one player's question causes `EmitEffectOutput`/`ActivatePresentationOutput`/`UpdatePresentationOutput` addressed to every player in the roster (via `Recipients`/target-users referencing the `players` list) results in every currently-connected player's client receiving the corresponding live message - not just the answering player.
- A client only ever receives Question-, Presentation-, or Effect-shaped messages - no wire message exposes a raw internal/domain fact directly.
- No Output this WORK translates is ever observed by a client before its causing transaction has committed (same rule WORK-0005 already established and proved for Open/CloseQuestion) - trivially true here since translation happens from `Manager`'s own successful return, which only ever happens after commit.
- This WORK adds no new durable column/table (verified by its own migration diff, if any, being empty).
- `go build ./...`, `go vet ./...`, `go test ./... -count=1` pass, with no new failure beyond the already-recorded out-of-scope `getgame` JSONB-comparison defect.

## Documentation Impact

- `game/CURRENT_STATE.md`, `game/docs/FLOWS.md` - describe the broadened fan-out and the wire-protocol design principle once implemented, alongside WORK-0005's existing Live Transport flow section.
- `play/README.md` - document the "Question/Presentation/Effect only" wire-protocol principle, and extend the `Event`/`EventKind` description.
- `docs/projects/active/session-runtime-v1/PROJECT.md` - WORK-0006's row update alongside this WORK's revision.

## Scope Correction (Part C Reconciliation, 2026-09-20)

This WORK originally included persisting Start's `Seed` (a single new durable column) as a small addition toward keeping replay theoretically possible. A broader reconciliation session accepted GAME-ADR-0024 (Replay-First Session Runtime Persistence), which gives the *complete* durable replay-input model - Seed, `RootParameters`, and every other RuntimeTurn-driving cause - a single dedicated owner, `docs/projects/active/session-runtime-v1/works/WORK-0019-replay-first-session-runtime-persistence-migration.md`. Persisting only the Seed here, in isolation from that complete model, would risk a mismatched or premature partial implementation of a model WORK-0019 owns designing as a whole. This WORK's scope, Blockers, Acceptance Criteria, and Documentation Impact above have been revised accordingly: this WORK now adds zero new durable persistence of any kind and is purely about live UI delivery (Presentations/Effects fan-out), exactly matching its own title. No production code was implemented or changed by this correction; this WORK's Status remains DRAFT, unaffected by this scope narrowing (still awaiting human READY authorization, as before).

## Completion Record

Not yet DONE. Status: **DRAFT**, revised 2026-09-20 after human review changed the core mechanism from durable capture to direct return from `Manager` (reopening, narrowly, WORK-0005's Manager-signature constraint) and added the "Question/Presentation/Effect only" wire-protocol principle; further revised the same day (Part C reconciliation) to remove Start-Seed persistence from scope, moved to WORK-0019. Both Blockers are resolved; ready to move to READY pending final confirmation.
