# Project: Game Language Flat Execution Model

Status: ACTIVE
Created: 2026-09-24
Last updated: 2026-09-24 (WORK-0026/0027 drafted for real, DRAFT, to be implemented together)

## Goal

Implement `game/docs/decisions/GAME-ADR-0026-flat-workflow-execution-model-and-keyed-interaction-slots.md` (ACCEPTED): remove Child Workflow/Task Group nested execution from Game Language, generalize keyed slots (already accepted for Timers, GAME-ADR-0012) to Questions/Ask Groups/Presentations, and replace `Path`/`Slot`/`Key`-based interaction addressing with an engine-owned incremental `InteractionID` and a single unified answer signal. Then update Session Runtime (`game/session/workflows/sessionlifecycle`) to consume the new contract, so `session-runtime-v1`'s paused Phase 1 (see that Project's "Paused (2026-09-24)" note) can resume.

## Explicitly Out Of Scope

- Any change to Ask Group's own collection semantics (`AskGroupCompletionPolicy`, multi-recipient collect-and-reevaluate behavior) beyond gaining a keyed variant - Ask Groups are not a nested-execution construct and are not being redesigned, only extended.
- Rebuilding `play`/the live Coordinator, or anything under `session-runtime-v1`'s own Phase 2/3 - unrelated to this Project.
- Deciding whether Child Workflow/Task Group should return later as a pure authoring-reuse (compile-time expansion) mechanism - GAME-ADR-0026's Alternatives Considered explicitly defers this until real authoring experience against the flat model demonstrates a concrete need.
- Declarative "keyed state machine" authoring sugar - same deferral as above.
- Redesigning `session_interactions`' persistence model end to end - WORK-0027 only needs the parts of GAME-ADR-0007/the persistence model that reference `engine_path`/`engine_slot` to change key to `InteractionID`; broader persistence-model concerns are unaffected.

## Current Work

- **WORK-0024** (Remove Child Workflow and Task Group) - DONE (2026-09-24). Three independent review passes were needed before a clean APPROVED state (each found a shrinking set of stale documentation/doc-comment references the previous pass's narrower sweep missed; no code-behavior defect beyond the original implementation was ever found). The one DECISION_REQUIRED finding (a forced, verified-behavior-preserving Session Runtime edit, plus a resulting untested `MaxSteps` bound) was resolved by explicit human decision, non-materially - see WORK-0024's own Completion Record.
- **WORK-0025** (Keyed Question, Ask Group, and Timer Slots) - DONE (2026-09-24). Two independent review passes were needed: the first found one HIGH-severity bug (the Snapshot codec never persisted keyed-slot state - fixed, with a comprehensive round-trip test) plus several lower-severity findings, all fixed; the second, fresh pass re-verified every fix directly and reached a clean APPROVED verdict. Timer's keyed slot is included, finally implementing the long-unimplemented GAME-ADR-0012; Presentation's keyed capability is deferred, per Human Resolution. See its own Completion Record for full history.
- **WORK-0026** (Engine-Owned Interaction Addressing) and **WORK-0027** (Session Runtime Interaction-Addressing Rework) are both DRAFT (2026-09-24), drafted together rather than just-in-time one-at-a-time: drafting WORK-0026 surfaced that it cannot be implemented independently of WORK-0027 without leaving `game/session/...` unable to compile (no durable `InteractionID` exists for it to answer with until WORK-0027's own persistence rework lands) - see WORK-0026's own Human Resolution. Both WORKs are reviewed against their own separate accepted scope but implemented and closed together, in one combined pass. Neither is READY yet - pending human approval.

## Work

| Order | Work | Status |
|------:|------|--------|
| 1 | WORK-0024 — Remove Child Workflow and Task Group | DONE |
| 2 | WORK-0025 — Keyed Question, Ask Group, and Timer Slots | DONE |
| 3 | WORK-0026 — Engine-Owned Interaction Addressing (`InteractionID`, unified answer signal, `Kind`) | DRAFT |
| 4 | WORK-0027 — Session Runtime Interaction-Addressing Rework | DRAFT |

## Ordering / Dependencies

- **WORK-0024** removes `Signal.Path`/`PathStep`/`WorkflowCompletedOutput.Path` and the nested-instance tree entirely, since only one workflow instance exists once Child Workflows/Task Groups are gone. It depends on nothing and can start immediately.
- **WORK-0025** adds keyed families for Question, Ask Group, and Timer slots (Presentation's keyed capability deferred, per Human Resolution) - DONE. It does not depend on WORK-0024 (Path/keyed-slots are independent axes) but is sequenced after it to keep each WORK's diff small and reviewable against a settled instance model.
- **WORK-0026** replaces `Slot`(+`Key`) as the caller-facing answer address with an engine-owned `InteractionID`, unifies the Question/Ask-Group answer signal, and adds an explicit `Kind` to the interaction-opened Output. It depends on WORK-0025 (DONE), since a caller-facing ID scheme should account for keyed occurrences from the start rather than being redesigned again once keying exists.
- **WORK-0027** reworks `game/session/workflows/sessionlifecycle`'s `interaction_capture.go`/`replay.go`/answer-signal construction to consume `InteractionID` instead of encoding/decoding `engine_path`/`engine_slot`, and updates the `session_interactions` persistence shape accordingly (GAME-ADR-0007 follow-up). It depends on WORK-0026 and is what unblocks `session-runtime-v1`'s Phase 1 (WORK-0006 onward) to resume. Per WORK-0026's own Human Resolution, WORK-0026 and WORK-0027 are implemented together in one combined pass rather than sequentially - see both WORKs' own files.

## Material Decisions Needing Human Input

- WORK-0027's `session_interactions` migration shape is now proposed (replace `engine_path`/`engine_slot` outright, following `session-runtime-v1`'s WORK-0001 pre-launch-schema-replacement precedent) - see WORK-0027's own Approved Design. Recorded here as still awaiting explicit human confirmation at DRAFT -> READY, not yet a closed question.

## Tracked Follow-Ups (Non-Blocking)

- `game/language/v1/program/ask_group.go`'s ordinary (non-keyed) `AskGroupSlotDeclaration` doc comment (~lines 24-35) claims presentation mounting happens per recipient; this is false against the current runtime (`deriveActivePresentations` never walks `AskGroupSlots`/`KeyedAskGroupSlots`) and predates this Project entirely. Found as a NON_BLOCKING finding during WORK-0025's independent re-review; not fixed there since it is an unrelated pre-existing file outside that WORK's scope. Worth a trivial standalone fix whenever this file is next touched.
- `session_timer_obligations.engine_path` (GAME-ADR-0007) has been dead weight (always the constant `emptyEnginePath`) since WORK-0024 removed nested instances, mirroring `session_interactions.engine_path`'s identical pre-existing dead weight, which WORK-0027 does remove (bundled with its own `InteractionID` migration). Found while drafting WORK-0027; left alone there since Timer is outside WORK-0026/0027's own scope entirely. Worth a standalone cleanup WORK/migration later, not urgent.
- Session Runtime never constructs the Ask Group completed-awaiting-join signal (what is now `SignalKindInteractionCompleted`, formerly `SignalKindAskGroupCompleted`/`KeyedAskGroupCompleted`) - an authored transition gated on an Ask Group's completion would never fire through `game/session/workflows/sessionlifecycle` today. Found while drafting WORK-0027; pre-existing, unrelated to interaction *addressing*, and explicitly out of that WORK's scope. Worth a dedicated WORK once a real game needs Ask Group completion to drive a transition through Session Runtime.

## Completion Criteria

This Project is complete when:

1. WORK-0024 through WORK-0027 are each DONE, or explicitly moved out of scope with human confirmation.
2. `go build ./...`/`go vet ./...`/the full test suite pass with Child Workflow/Task Group fully removed from `program`/`engine`/`internal/compiler`/`internal/runtime`, and no reference to either remains in canonical documentation as a current (non-historical) capability.
3. `session-runtime-v1`'s Phase 1 is confirmed unblocked (its own PROJECT.md's pause note is resolved).
