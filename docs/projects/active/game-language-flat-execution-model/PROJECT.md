# Project: Game Language Flat Execution Model

Status: ACTIVE
Created: 2026-09-24
Last updated: 2026-09-24

## Goal

Implement `game/docs/decisions/GAME-ADR-0026-flat-workflow-execution-model-and-keyed-interaction-slots.md` (ACCEPTED): remove Child Workflow/Task Group nested execution from Game Language, generalize keyed slots (already accepted for Timers, GAME-ADR-0012) to Questions/Ask Groups/Presentations, and replace `Path`/`Slot`/`Key`-based interaction addressing with an engine-owned incremental `InteractionID` and a single unified answer signal. Then update Session Runtime (`game/session/workflows/sessionlifecycle`) to consume the new contract, so `session-runtime-v1`'s paused Phase 1 (see that Project's "Paused (2026-09-24)" note) can resume.

## Explicitly Out Of Scope

- Any change to Ask Group's own collection semantics (`AskGroupCompletionPolicy`, multi-recipient collect-and-reevaluate behavior) beyond gaining a keyed variant - Ask Groups are not a nested-execution construct and are not being redesigned, only extended.
- Rebuilding `play`/the live Coordinator, or anything under `session-runtime-v1`'s own Phase 2/3 - unrelated to this Project.
- Deciding whether Child Workflow/Task Group should return later as a pure authoring-reuse (compile-time expansion) mechanism - GAME-ADR-0026's Alternatives Considered explicitly defers this until real authoring experience against the flat model demonstrates a concrete need.
- Declarative "keyed state machine" authoring sugar - same deferral as above.
- Redesigning `session_interactions`' persistence model end to end - WORK-0027 only needs the parts of GAME-ADR-0007/the persistence model that reference `engine_path`/`engine_slot` to change key to `InteractionID`; broader persistence-model concerns are unaffected.

## Current Work

- **WORK-0024** (Remove Child Workflow and Task Group) - IMPLEMENTING, implementation complete and self-reported ready for independent review (see its own Completion Record). Not yet DONE.
- WORK-0025/0026/0027 are PLANNED - each is drafted for real (design filled in, moved to DRAFT) once the WORK immediately before it is READY or DONE, matching this repository's established just-in-time drafting practice (see `session-runtime-v1`'s own PROJECT.md for precedent).

## Work

| Order | Work | Status |
|------:|------|--------|
| 1 | WORK-0024 — Remove Child Workflow and Task Group | IMPLEMENTING |
| 2 | WORK-0025 — Keyed Question/Ask Group/Presentation Slots | PLANNED |
| 3 | WORK-0026 — Engine-Owned Interaction Addressing (`InteractionID`, unified answer signal, `Kind`) | PLANNED |
| 4 | WORK-0027 — Session Runtime Interaction-Addressing Rework | PLANNED |

## Ordering / Dependencies

- **WORK-0024** removes `Signal.Path`/`PathStep`/`WorkflowCompletedOutput.Path` and the nested-instance tree entirely, since only one workflow instance exists once Child Workflows/Task Groups are gone. It depends on nothing and can start immediately.
- **WORK-0025** adds keyed families for Question/Ask Group/Presentation slots (Timer's own keyed family remains GAME-ADR-0012's separate, still-unimplemented scope, but the two are expected to share an implementation approach once either is built). It does not depend on WORK-0024 (Path/keyed-slots are independent axes) but is sequenced after it to keep each WORK's diff small and reviewable against a settled instance model.
- **WORK-0026** replaces `Slot`(+`Key`) as the caller-facing answer address with an engine-owned `InteractionID`, unifies the Question/Ask-Group answer signal, and adds an explicit `Kind` to the interaction-opened Output. It depends on WORK-0025, since a caller-facing ID scheme should account for keyed occurrences from the start rather than being redesigned again once keying exists.
- **WORK-0027** reworks `game/session/workflows/sessionlifecycle`'s `interaction_capture.go`/`replay.go`/answer-signal construction to consume `InteractionID` instead of encoding/decoding `engine_path`/`engine_slot`, and updates the `session_interactions` persistence shape accordingly (GAME-ADR-0007 follow-up). It depends on WORK-0026 and is what unblocks `session-runtime-v1`'s Phase 1 (WORK-0006 onward) to resume.

## Material Decisions Needing Human Input

- WORK-0027's exact `session_interactions` migration shape (replace `engine_path`/`engine_slot` outright, or add `interaction_id` alongside and deprecate the old columns) is not yet decided - deferred to when that WORK is drafted for real, per this repository's standard-compliance-migration precedent (see `session-runtime-v1`'s WORK-0001 history for the analogous prior case).

## Completion Criteria

This Project is complete when:

1. WORK-0024 through WORK-0027 are each DONE, or explicitly moved out of scope with human confirmation.
2. `go build ./...`/`go vet ./...`/the full test suite pass with Child Workflow/Task Group fully removed from `program`/`engine`/`internal/compiler`/`internal/runtime`, and no reference to either remains in canonical documentation as a current (non-historical) capability.
3. `session-runtime-v1`'s Phase 1 is confirmed unblocked (its own PROJECT.md's pause note is resolved).
