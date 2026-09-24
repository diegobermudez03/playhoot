Process: Project (`docs/projects/active/game-language-flat-execution-model/`)
Topic: Game Language flat execution model (removal of Child Workflow/Task Group; keyed interaction slots; engine-owned interaction addressing)
Current stage: WORK-0024/WORK-0025 DONE (2026-09-24, full history in their own Completion Records). WORK-0026 (Engine-Owned Interaction Addressing) and WORK-0027 (Session Runtime Interaction-Addressing Rework) are both drafted for real (DRAFT, 2026-09-24) - drafting WORK-0026 surfaced they cannot be implemented independently (see WORK-0026's own Human Resolution), so both are drafted together and will be implemented/closed together. Neither is READY yet.
Current execution surface: CODEBASE AGENT (design/drafting, per `docs/ai/protocols/FEATURE_DEVELOPMENT.md`; human explicitly requested "proceed with 0026")
Parent process: this Project itself.
Related durable artifacts: `../PROJECT.md` (Work table/Current Work/Ordering/Material Decisions/Tracked Follow-Ups all updated to reflect WORK-0026/0027 DRAFT), `../works/WORK-0026-engine-owned-interaction-addressing.md` and `../works/WORK-0027-session-runtime-interaction-addressing-rework.md` (both full DRAFT specs, `../works/WORK-0025-keyed-interaction-slots.md`/`../works/WORK-0024-remove-child-workflow-and-task-group.md` (both DONE, full history in their own files).
Blocked by: nothing material. Awaiting human review/approval of both DRAFT specs (DRAFT -> READY requires explicit human authorization per `docs/ai/protocols/FEATURE_DEVELOPMENT.md` - a Codebase Agent must not self-approve).
Next action: human reviews WORK-0026/WORK-0027's DRAFT specs (particularly WORK-0026's Human Resolution and WORK-0027's proposed migration-shape resolution) and, if acceptable, approves both DRAFT -> READY; a Codebase Agent then implements both together in one combined pass per `docs/ai/protocols/IMPLEMENTATION_REVIEW.md`.
Last durable checkpoint: this pass (2026-09-24) - WORK-0026/WORK-0027 drafted for real. See "What happened this pass" below for the material design content and the sequencing problem found and resolved.
Last updated: 2026-09-24

# Resume Context

## Earlier history (condensed - full detail in each WORK's own Completion Record)

WORK-0024 (Remove Child Workflow and Task Group) and WORK-0025 (Keyed Question, Ask Group, and Timer Slots) each reached DONE the same day (2026-09-24), each after multiple independent review cycles that found and fixed real defects (WORK-0024: three passes, stale-documentation findings only; WORK-0025: two passes, including one HIGH-severity Snapshot-codec bug that silently dropped keyed-slot state across persistence). One NON_BLOCKING follow-up from WORK-0025's own review remains tracked, not fixed: `program/ask_group.go`'s doc comment overclaiming AskGroup presentation mounting (see `../PROJECT.md`'s "Tracked Follow-Ups").

## What happened this pass

The human said "proceed with 0026." Following `docs/ai/protocols/FEATURE_DEVELOPMENT.md`, WORK-0026 was drafted for real: read GAME-ADR-0026 Decisions 3-5 in full, then the actual current `engine.Signal`/`engine.Output`/`engine.Snapshot`/`internal/runtime` code (not just the ADR) to ground the design in what exists today.

Drafting surfaced a real sequencing problem, not previously identified anywhere in this Project's tracking: `game/session/workflows/sessionlifecycle/step_answer_interaction.go` and `replay.go` are already-shipped, DONE Session Runtime code that directly constructs `engine.Signal{Kind: engine.SignalKindQuestionAnswered, Slot: interaction.EngineSlot, ...}`. WORK-0026 alone (removing `Slot`-addressed answering) would leave this code unable to compile, with no small bridging edit available - unlike WORK-0024's forced Session Runtime edits, there is no durable `InteractionID` anywhere for Session Runtime to construct the new signal with, since capturing one into `session_interactions` is WORK-0027's own persistence rework.

This was escalated to the human before drafting further (a material sequencing/persistence-shape question, not a Codebase Agent's to invent per AGENTS.md #10). Presented three options; the human chose: implement WORK-0026 and WORK-0027 together in one combined pass, so `game/session/...` is never left in a broken intermediate state - see WORK-0026's own "Human Resolution" section for the full record.

Both WORK-0026 and WORK-0027 were then drafted for real (PLANNED -> DRAFT), each reviewable against its own separate accepted scope but explicitly noted as implemented/closed together. Researched the actual current `internal/runtime/execute.go`/`ask_group.go` (how Question/AskGroup opening assigns state today), `internal/repo/interaction.go`/`interaction_capture.go`/`replay.go` (Session Runtime's current `(engine_path, engine_slot)` persistence and construction), and `session-runtime-v1`'s WORK-0001 "Standard-Compliance Migration Record" (the established pre-launch drop-then-recreate migration precedent, reused for WORK-0027's own proposed `session_interactions` migration instead of leaving that Material Decision unresolved again).

`../PROJECT.md` was updated throughout: Work table, Current Work, Ordering/Dependencies, Material Decisions (WORK-0027's migration shape now has a proposed resolution, pending READY-time confirmation), and two new Tracked Follow-Ups found while drafting (`session_timer_obligations.engine_path`'s identical pre-existing dead weight, unaffected by this WORK; Session Runtime never constructing the Ask Group completed-awaiting-join signal, a pre-existing unrelated gap).

## What is NOT done

- Neither WORK-0026 nor WORK-0027 is READY - only a human may authorize DRAFT -> READY (`docs/work/README.md`). Both need human review, in particular: WORK-0026's Human Resolution (combined-implementation decision) and WORK-0027's proposed migration-shape resolution.
- No implementation has started on either WORK.
- The `program/ask_group.go` NON_BLOCKING doc drift and the two new Tracked Follow-Ups found this pass (all in `../PROJECT.md`) remain unfixed, deliberately - all outside these WORKs' own scope.
