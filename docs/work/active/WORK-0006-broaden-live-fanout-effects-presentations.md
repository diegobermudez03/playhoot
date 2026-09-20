# WORK-0006: Broaden Live Fan-Out — Effects and Presentations (UI)

Status: DRAFT
Created: 2026-09-20
Last status change: 2026-09-20

Related decisions:
- GAME-ADR-0002 (Session Runtime durable boundary, Live Session Coordinator responsibility boundary)
- GAME-ADR-0007 (RuntimeTurn/Interaction persistence model - `OpenQuestionOutput`/`CloseQuestionOutput` durable capture is unaffected by this WORK)
- GAME-ADR-0020 (post-commit client delivery semantics, best-effort, no outbox)

Canonical context:
- `docs/work/active/WORK-0005-thin-live-coordinator.md` (Blocker 4's resolved scope: only `OpenQuestionOutput`/`CloseQuestionOutput` are translated today; also the origin of the "`Manager`'s method signatures/internal behavior do not change" constraint this WORK's Blocker 1 explicitly revises)
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

### Replay is a separate, later concern - but this WORK closes the one real gap in its way

A human question during this WORK's own design confirmed engine execution is fully deterministic given `(compiled Program, starting Snapshot, driving Signal sequence)` - no wall-clock, no OS randomness anywhere in `game/language/v1/engine`. That means a hypothetical future "replay a Session to watch how it played out" feature genuinely does not need Presentations/Effects persisted at all - they are mechanically re-derivable by replaying the durably-captured inputs through the same compiled Program. What such a feature *would* need, and what is not captured today: the one-time `Seed` drawn at Start (`step_start.go`'s `drawSeed()`) is never written to any column - it only survives folded into the post-Turn-1 `Snapshot.Random`, not recoverably. This WORK persists that one value (a single column, written once, at Start) as a small, low-risk addition - not because this WORK needs it for live delivery, but because "make replay theoretically possible" is a stated goal and this is the one concrete hole standing in its way. Whether to eventually stop persisting full per-Turn Snapshots (since they too are re-derivable from inputs alone) is a separate, larger, explicitly deferred idea - see `docs/product/IDEAS.md -> Minimize Persisted RuntimeTurn History`.

## Scope

### In Scope

- Extend `sessionlifecycle.Manager.Start` and `Manager.AnswerInteraction`'s return types to also carry the committed Turn's `EmitEffectOutput`/`ActivatePresentationOutput`/`UpdatePresentationOutput`/`RemovePresentationOutput` values directly - additive data only, already computed in memory during `runtimeturn.Drain` (`drainResult.Steps[*].Outputs`) and simply not currently returned. No change to either method's existing parameters, business logic, or persistence for anything already implemented (Open/CloseQuestion capture, idempotency, transaction scope all unchanged).
- `play/sessionruntime.Start`/`AnswerInteraction` translate these in-memory Outputs directly into `play.Event`s, addressed to each recipient's caller-facing `UserUUID` (the same non-leakage translation `eventsForTurn` already does for Open/CloseQuestion, applied to in-memory data instead of a read-after-commit query) - no new table, no new read.
- New `play.EventKind` values and wire message types (`api/session/wire.go`) for a Presentation activated/updated/removed and an Effect emitted - never exposing `Slot`/internal engine identities, only `Name`/`View`/`Model` (presentations) or `Effect`/`Arguments` (effects).
- A new durable column capturing the Start Turn's `Seed` (written once, at Start, alongside the existing Turn-1 row) - the one gap closed for future replay, unrelated to this WORK's own live-delivery mechanism.

### Out of Scope

- `WorkflowCompletedOutput` - picked up by `docs/work/active/WORK-0007-session-termination-live-notification.md` instead (broadened to cover natural game completion, not only fatal failure), since a human-confirmed follow-up decided the root workflow completing is the deterministic "game over" signal and belongs with that WORK's termination-notification mechanism, not this one's Output-translation mechanism.
- `ScheduleTimerOutput`/`CancelTimerOutput` - Slice 5's own scope, unaffected by this WORK.
- Any change to delivery reliability/guarantees - GAME-ADR-0020's best-effort, no-outbox, no-replay rule already governs these Outputs exactly as it governs Open/CloseQuestion today.
- Building an actual replay feature - only the one persistence gap (`Seed`) blocking a future one is closed here.
- Reducing existing full-Snapshot persistence - tracked as its own explicitly-deferred idea (`docs/product/IDEAS.md`), not this WORK's concern.

## Blockers

Status: **RESOLVED, HUMAN-APPROVED (2026-09-20)**.

1. **Persist Presentation/Effect state, or have `Manager` return Outputs directly?** Originally DRAFT pending this decision. **Resolution: HUMAN-APPROVED - return directly, zero persistence.** This revises WORK-0005's "`Manager`'s method signatures do not change" constraint, deliberately and narrowly: `Start`/`AnswerInteraction` gain additive return data only, nothing about their existing behavior, parameters, or the durable capture Open/CloseQuestion already relies on changes. Confirmed this does not create a future resync gap (`deriveActivePresentations` recomputes current Presentation state fresh from the current Snapshot on demand - see Context).
2. **Does replay remain theoretically possible without persisting Presentations/Effects?** **Resolution: yes, confirmed** - engine execution is fully deterministic given `(Program, Snapshot, Signal sequence)`, verified against `game/language/v1/engine`'s actual source (no wall-clock/OS-randomness dependency anywhere). The one real gap (Start's `Seed` not durably captured) is closed by this WORK's own new column, so nothing about choosing zero-persistence for Presentations/Effects makes replay any less possible than it already was.

Local implementation choices (exact wire message names/shapes, exact new return-type shape on `StartResult`/`AnswerInteractionResult`, exact column/migration for the `Seed`) remain Implementation Freedom.

## Acceptance Criteria

- A Definition authored so that answering one player's question causes `EmitEffectOutput`/`ActivatePresentationOutput`/`UpdatePresentationOutput` addressed to every player in the roster (via `Recipients`/target-users referencing the `players` list) results in every currently-connected player's client receiving the corresponding live message - not just the answering player.
- A client only ever receives Question-, Presentation-, or Effect-shaped messages - no wire message exposes a raw internal/domain fact directly.
- No Output this WORK translates is ever observed by a client before its causing transaction has committed (same rule WORK-0005 already established and proved for Open/CloseQuestion) - trivially true here since translation happens from `Manager`'s own successful return, which only ever happens after commit.
- The Start Turn's `Seed` is durably persisted and recoverable.
- `go build ./...`, `go vet ./...`, `go test ./... -count=1` pass, with no new failure beyond the already-recorded out-of-scope `getgame` JSONB-comparison defect.

## Documentation Impact

- `game/CURRENT_STATE.md`, `game/docs/FLOWS.md` - describe the broadened fan-out and the wire-protocol design principle once implemented, alongside WORK-0005's existing Live Transport flow section.
- `play/README.md` - document the "Question/Presentation/Effect only" wire-protocol principle, and extend the `Event`/`EventKind` description.
- `docs/ai/workspaces/active/session-runtime-v1/PLAN.md` - Slice 11 update alongside this WORK's revision.

## Completion Record

Not yet DONE. Status: **DRAFT**, revised 2026-09-20 after human review changed the core mechanism from durable capture to direct return from `Manager` (reopening, narrowly, WORK-0005's Manager-signature constraint), added the "Question/Presentation/Effect only" wire-protocol principle, and added Start-Seed persistence for future replay. Both Blockers are now resolved; ready to move to READY pending final confirmation.
