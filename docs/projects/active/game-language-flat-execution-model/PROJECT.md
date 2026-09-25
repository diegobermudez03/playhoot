# Project: Game Language Flat Execution Model

Status: ACTIVE (WORK-0024–0027 DONE; reopened same day - WORK-0028 added, see "Current Work" below)
Created: 2026-09-24
Last updated: 2026-09-24 (WORK-0028 drafted: a post-closure audit found `sessionlifecycle` still implements its own step-chaining/replay loops on top of `engineservice`, the same class of caller-facing leak GAME-ADR-0026 removed for addressing but never applied to execution - see GAME-ADR-0027 (PROPOSED) and WORK-0028 (DRAFT). Prior update, same day: WORK-0026/0027 independent review APPROVED after one fix pass, both closed DONE - all four original WORK items were DONE and this Project was pending human confirmation to move to `docs/projects/completed/` before this reopening)

## Goal

Implement `game/docs/decisions/GAME-ADR-0026-flat-workflow-execution-model-and-keyed-interaction-slots.md` (ACCEPTED): remove Child Workflow/Task Group nested execution from Game Language, generalize keyed slots (already accepted for Timers, GAME-ADR-0012) to Questions/Ask Groups/Presentations, and replace `Path`/`Slot`/`Key`-based interaction addressing with an engine-owned incremental `InteractionID` and a single unified answer signal. Then update Session Runtime (`game/session/workflows/sessionlifecycle`) to consume the new contract, so `session-runtime-v1`'s paused Phase 1 (see that Project's "Paused (2026-09-24)" note) can resume.

Extended (2026-09-24, WORK-0028): also implement `game/docs/decisions/GAME-ADR-0027-engine-owned-turn-execution-and-replay.md` (PROPOSED) - move the RuntimeTurn step-chain-draining loop and the GAME-ADR-0024 replay loop, both currently implemented by `sessionlifecycle` itself on top of `engineservice`'s single-signal `Step` primitive, inside `engineservice`, so Session Runtime never holds, constructs, or reads an `engine.Snapshot` and never reimplements any part of the engine's own execution model.

## Explicitly Out Of Scope

- Any change to Ask Group's own collection semantics (`AskGroupCompletionPolicy`, multi-recipient collect-and-reevaluate behavior) beyond gaining a keyed variant - Ask Groups are not a nested-execution construct and are not being redesigned, only extended.
- Rebuilding `play`/the live Coordinator, or anything under `session-runtime-v1`'s own Phase 2/3 - unrelated to this Project.
- Deciding whether Child Workflow/Task Group should return later as a pure authoring-reuse (compile-time expansion) mechanism - GAME-ADR-0026's Alternatives Considered explicitly defers this until real authoring experience against the flat model demonstrates a concrete need.
- Declarative "keyed state machine" authoring sugar - same deferral as above.
- Redesigning `session_interactions`' persistence model end to end - WORK-0027 only needs the parts of GAME-ADR-0007/the persistence model that reference `engine_path`/`engine_slot` to change key to `InteractionID`; broader persistence-model concerns are unaffected.

## Current Work

- **WORK-0024** (Remove Child Workflow and Task Group) - DONE (2026-09-24). Three independent review passes were needed before a clean APPROVED state (each found a shrinking set of stale documentation/doc-comment references the previous pass's narrower sweep missed; no code-behavior defect beyond the original implementation was ever found). The one DECISION_REQUIRED finding (a forced, verified-behavior-preserving Session Runtime edit, plus a resulting untested `MaxSteps` bound) was resolved by explicit human decision, non-materially - see WORK-0024's own Completion Record.
- **WORK-0025** (Keyed Question, Ask Group, and Timer Slots) - DONE (2026-09-24). Two independent review passes were needed: the first found one HIGH-severity bug (the Snapshot codec never persisted keyed-slot state - fixed, with a comprehensive round-trip test) plus several lower-severity findings, all fixed; the second, fresh pass re-verified every fix directly and reached a clean APPROVED verdict. Timer's keyed slot is included, finally implementing the long-unimplemented GAME-ADR-0012; Presentation's keyed capability is deferred, per Human Resolution. See its own Completion Record for full history.
- **WORK-0026** (Engine-Owned Interaction Addressing) and **WORK-0027** (Session Runtime Interaction-Addressing Rework) - DONE (2026-09-24). Drafted, approved READY, and implemented together in one combined pass - see WORK-0026's own Human Resolution for why they could not be implemented independently. Reviewed together as one combined change; one REQUIRED_FIX (a stale doc-comment reference to a removed `SignalKind` constant in `engine/commit.go`) applied and re-verified. See WORK-0026's own Completion Record for the full review history.
- **WORK-0028** (Engine-Owned Turn Execution) - DRAFT. Found during a post-closure audit explicitly requested to verify this Project actually achieved its own "consumer/caller should know nothing about engine internals" goal: `sessionlifecycle`'s own `internal/runtimeturn` package reimplements the engine's step-chaining loop (GAME-ADR-0019's `MaxSteps` bound, enforced caller-side) and `replay.go`'s `reconstructCurrentSnapshot`/`replayTurn` reimplement GAME-ADR-0024's replay mechanism - both on top of `engineservice.Step`, both requiring `sessionlifecycle` to hold and thread an `engine.Snapshot` through hand-rolled loops. Blocked on GAME-ADR-0027 (PROPOSED) reaching ACCEPTED before this WORK can move DRAFT -> READY.

## Work

| Order | Work | Status |
|------:|------|--------|
| 1 | WORK-0024 — Remove Child Workflow and Task Group | DONE |
| 2 | WORK-0025 — Keyed Question, Ask Group, and Timer Slots | DONE |
| 3 | WORK-0026 — Engine-Owned Interaction Addressing (`InteractionID`, unified answer signal, `Kind`) | DONE |
| 4 | WORK-0027 — Session Runtime Interaction-Addressing Rework | DONE |
| 5 | WORK-0028 — Engine-Owned Turn Execution (Replay and Step-Chain Draining) | DRAFT |

## Ordering / Dependencies

- **WORK-0024** removes `Signal.Path`/`PathStep`/`WorkflowCompletedOutput.Path` and the nested-instance tree entirely, since only one workflow instance exists once Child Workflows/Task Groups are gone. It depends on nothing and can start immediately.
- **WORK-0025** adds keyed families for Question, Ask Group, and Timer slots (Presentation's keyed capability deferred, per Human Resolution) - DONE. It does not depend on WORK-0024 (Path/keyed-slots are independent axes) but is sequenced after it to keep each WORK's diff small and reviewable against a settled instance model.
- **WORK-0026** replaces `Slot`(+`Key`) as the caller-facing answer address with an engine-owned `InteractionID`, unifies the Question/Ask-Group answer signal, and adds an explicit `Kind` to the interaction-opened Output. It depends on WORK-0025 (DONE), since a caller-facing ID scheme should account for keyed occurrences from the start rather than being redesigned again once keying exists.
- **WORK-0027** reworks `game/session/workflows/sessionlifecycle`'s `interaction_capture.go`/`replay.go`/answer-signal construction to consume `InteractionID` instead of encoding/decoding `engine_path`/`engine_slot`, and updates the `session_interactions` persistence shape accordingly (GAME-ADR-0007 follow-up). It depends on WORK-0026 and is what unblocks `session-runtime-v1`'s Phase 1 (WORK-0006 onward) to resume. Per WORK-0026's own Human Resolution, WORK-0026 and WORK-0027 are implemented together in one combined pass rather than sequentially - see both WORKs' own files.
- **WORK-0028** moves `sessionlifecycle`'s own step-chaining/replay loops inside `engineservice` (GAME-ADR-0027). It does not depend on WORK-0024–0027's own addressing changes (a different axis - execution/replay ownership, not addressing), but touches the exact same `interaction_capture.go`/`step_answer_interaction.go`/`step_start.go`/`replay.go` files WORK-0027 just finished rewriting, and the exact same files `session-runtime-v1`'s WORK-0006 (currently DRAFT, unblocked, not yet resumed) is about to build new Effect/Presentation-Output-handling code against. **Recommendation: land WORK-0028 before WORK-0006 resumes implementation**, for the same reason WORK-0006 was previously paused for GAME-ADR-0026 - to avoid building/reviewing new code against a `captureInteractions`/Output-handling surface about to change shape again (`[]runtimeturn.StepTrace` -> `[]engine.Output`). This is a recommendation for `session-runtime-v1` to weigh, not a decision made here.

## Material Decisions Needing Human Input

- WORK-0027's `session_interactions` migration shape is confirmed (2026-09-24, approved alongside READY): replace `engine_path`/`engine_slot` outright, following `session-runtime-v1`'s WORK-0001 pre-launch-schema-replacement precedent - see WORK-0027's own Approved Design.
- GAME-ADR-0027 needs explicit human ACCEPT/reject before WORK-0028 can move DRAFT -> READY.
- Whether `session-runtime-v1`'s WORK-0006 should wait for WORK-0028 to land first (see this WORK's own recommendation in "Ordering / Dependencies" above) is a sequencing call for whoever resumes WORK-0006, not decided here.

## Tracked Follow-Ups (Non-Blocking)

- `game/language/v1/program/ask_group.go`'s ordinary (non-keyed) `AskGroupSlotDeclaration` doc comment (~lines 24-35) claims presentation mounting happens per recipient; this is false against the current runtime (`deriveActivePresentations` never walks `AskGroupSlots`/`KeyedAskGroupSlots`) and predates this Project entirely. Found as a NON_BLOCKING finding during WORK-0025's independent re-review; not fixed there since it is an unrelated pre-existing file outside that WORK's scope. Worth a trivial standalone fix whenever this file is next touched.
- `session_timer_obligations.engine_path` (GAME-ADR-0007) has been dead weight (always the constant `emptyEnginePath`) since WORK-0024 removed nested instances, mirroring `session_interactions.engine_path`'s identical pre-existing dead weight, which WORK-0027 does remove (bundled with its own `InteractionID` migration). Found while drafting WORK-0027; left alone there since Timer is outside WORK-0026/0027's own scope entirely. Worth a standalone cleanup WORK/migration later, not urgent.
- Session Runtime never constructs the Ask Group completed-awaiting-join signal (what is now `SignalKindInteractionCompleted`, formerly `SignalKindAskGroupCompleted`/`KeyedAskGroupCompleted`) - an authored transition gated on an Ask Group's completion would never fire through `game/session/workflows/sessionlifecycle` today. Found while drafting WORK-0027; pre-existing, unrelated to interaction *addressing*, and explicitly out of that WORK's scope. Worth a dedicated WORK once a real game needs Ask Group completion to drive a transition through Session Runtime.

## Completion Criteria

This Project is complete when:

1. ~~WORK-0024 through WORK-0027 are each DONE, or explicitly moved out of scope with human confirmation.~~ **Met (2026-09-24)** - all four DONE.
2. ~~`go build ./...`/`go vet ./...`/the full test suite pass with Child Workflow/Task Group fully removed from `program`/`engine`/`internal/compiler`/`internal/runtime`, and no reference to either remains in canonical documentation as a current (non-historical) capability.~~ **Met** - confirmed by WORK-0024's own three-pass review; unaffected by WORK-0026/0027.
3. ~~`session-runtime-v1`'s Phase 1 is confirmed unblocked (its own PROJECT.md's pause note is resolved).~~ **Met (2026-09-24)** - see that Project's own "Unblocked (2026-09-24)" section.
4. **(Added 2026-09-24, reopens this Project)** WORK-0028 is DONE, or explicitly moved out of scope with human confirmation - `sessionlifecycle` never holds/constructs/reads an `engine.Snapshot` and never reimplements the engine's own step-chaining/replay mechanics, per GAME-ADR-0027.

Criteria 1-3 were met and this Project was pending human confirmation to move to `docs/projects/completed/` when a follow-up audit (explicitly requested to verify the "consumer should know nothing about engine internals" goal was actually achieved, not merely the addressing half of it) found the gap Criterion 4 now tracks. This Project is not moved to `docs/projects/completed/` until Criterion 4 is also met (or explicitly descoped).
