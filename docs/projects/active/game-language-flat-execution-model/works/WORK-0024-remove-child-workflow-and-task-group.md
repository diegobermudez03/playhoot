# WORK-0024: Remove Child Workflow and Task Group

Status: IMPLEMENTING
Created: 2026-09-24
Last status change: 2026-09-24 (READY -> IMPLEMENTING; DRAFT -> READY, human-approved, same day)

Related decisions:
- GAME-ADR-0026 (Flat Workflow Execution Model, Keyed Interaction Slots, and Engine-Owned Interaction Addressing - Decisions 1 and 6 specifically)

Canonical context:
- `game/docs/decisions/GAME-ADR-0026-flat-workflow-execution-model-and-keyed-interaction-slots.md`
- `game/language/v1/engine/LOGICAL_CONTRACT.md` ("the engine does not recursively execute multiple transitions inside one step" - becomes a direct structural consequence once only one instance can ever exist, not merely a constraint on a tree)
- `game/language/v1/program/child_workflow.go`, `game/language/v1/program/task_group.go` (the declarations/operations being removed)
- `game/language/v1/engine/signal.go`, `game/language/v1/engine/instance.go`, `game/language/v1/engine/output.go`, `game/language/v1/engine/limits.go` (the compiled/runtime types being removed)
- `game/language/v1/engine/internal/compiler/compile_workflows.go`/`compile_slots.go` and `game/language/v1/engine/internal/runtime/child_workflow_exec_test.go`/`task_group.go` for where these concepts are currently compiled/executed - the exact file boundary for each removal is discovered during implementation, not prescribed here

## Outcome

Game Language no longer supports spawning a nested workflow instance. A compiled `Program` has exactly one instantiated workflow for a Session's lifetime. `program.ChildWorkflowSlotDeclaration`/`SpawnChildWorkflowOperation`/`CancelChildWorkflowOperation`, `program.TaskGroupSlotDeclaration`/`BeginTaskGroupOperation`/`SpawnTaskGroupChildOperation`/`SealTaskGroupOperation`/`FinalizeTaskGroupOperation`/`CancelTaskGroupOperation`, their compiled `engine` counterparts, their signal sources (`ChildCompletedSignalSource`/`ChildFailedSignalSource`/`ChildCancelledSignalSource`/`TaskGroupCompletedSignalSource` and corresponding `SignalKind` values), and the instance-tree addressing they required (`engine.WorkflowInstance.ChildSlots`/`TaskGroupSlots`, `engine.PathStep`, `engine.Signal.Path`, `engine.Limits.MaxWorkflowDepth`, `engine.WorkflowCompletedOutput.Path`) are deleted.

This is required because GAME-ADR-0026 found, via an audit against the product's target game range, that no game needs a genuinely separate execution instance, and that this capability was never exercised by any authored example, test, or completed Session Runtime WORK, nor ever accepted through this repository's own decision process before now.

## Context

Ask Groups (`program.AskGroupSlotDeclaration`/`OpenAskGroupOperation`/`FinalizeAskGroupOperation`/`CancelAskGroupOperation`) are a distinct, unrelated concept and are explicitly unaffected by this WORK: an ask group collects answers from multiple recipients into one pending group within the same instance and never spawns a separate one. Removing that would be a scope error.

`AskGroupCompletedSignalSource`/`SignalKindAskGroupCompleted` also stay - they report an ask group (not a child/task-group) reaching completed-awaiting-join, an in-instance concept independent of nested execution.

Keyed slots (Question/AskGroup/Presentation - GAME-ADR-0026 Decision 2) and the `InteractionID`/unified-answer-signal/`Kind` redesign (Decision 3-5) are explicitly **not** this WORK's scope - they are WORK-0025/WORK-0026 in this Project, sequenced after this one so each change lands against a settled, reviewable diff. This WORK only removes; it adds nothing.

## Scope

### In Scope

- Remove from `program`: `ChildWorkflowSlotDeclaration`, `SpawnChildWorkflowOperation`, `CancelChildWorkflowOperation` (`child_workflow.go`); `TaskGroupSlotDeclaration`, `BeginTaskGroupOperation`, `SpawnTaskGroupChildOperation`, `SealTaskGroupOperation`, `FinalizeTaskGroupOperation`, `CancelTaskGroupOperation` (`task_group.go`); the corresponding `ChildWorkflowSlots`/`TaskGroupSlots` fields on `WorkflowDeclaration`/`WorkflowStateDeclaration` (`workflow.go`); `ChildCompletedSignalSource`/`ChildFailedSignalSource`/`ChildCancelledSignalSource`/`TaskGroupCompletedSignalSource` (`signal.go`); and every corresponding wire-codec type under `internal/codec`.
- Remove from `engine`: the compiled counterparts of everything above; `WorkflowInstance.ChildSlots`/`TaskGroupSlots`, `ChildWorkflowSlotInstance`, `TaskGroupSlotInstance`, `TaskGroupState`, `TaskGroupTask`, `TaskGroupPhase*`, `TaskGroupCompletionKind*` (`instance.go`); `PathStep` and `Signal.Path` (`signal.go`) - every signal now implicitly targets the one existing instance; `Limits.MaxWorkflowDepth` (`limits.go`); `WorkflowCompletedOutput.Path` (`output.go` - every occurrence is now unconditionally about the one instance, matching GAME-ADR-0026 Decision 6); `SignalKindChildCompleted`/`ChildFailed`/`ChildCancelled`/`TaskGroupCompleted`; the `ExecutionErrorCode` values that exist only for spawning/joining (`ExecutionErrorWorkflowDepthExceeded`, `ExecutionErrorChildOutcomeNotJoined`, `ExecutionErrorDuplicateTaskKey`, `ExecutionErrorTaskGroupNotJoined`, `ExecutionErrorTaskGroupLeftBuilding`).
- Remove from `internal/compiler`: the compilation passes validating/lowering child-workflow and task-group declarations/operations, and their references from workflow/slot compilation.
- Remove from `internal/runtime`: child-workflow and task-group execution (spawn, join, cancel, depth tracking) and the instance-tree walk it required (`applyInstancePath` or equivalent, if it exists only for this purpose - keep it if `AnswerInteraction`'s own signal-targeting still needs a form of it for the single remaining instance, otherwise remove).
- Update every test that exercises the removed constructs: delete tests that exist solely to prove child-workflow/task-group behavior; update any test that incidentally used a non-empty `Signal.Path`/`PathStep` to address the (now singular) instance without one.
- Update `game/language/v1/example.go` if it references any removed construct (an initial audit found it does not, but re-verify).

### Out of Scope

- Keyed slots for Question/AskGroup/Presentation (WORK-0025).
- `InteractionID`, the unified answer signal, and `Kind` on the interaction-opened Output (WORK-0026).
- Any Session Runtime (`game/session`) change - `sessionlifecycle` never used `Path`/child-workflow/task-group addressing beyond the single root instance already implied by its current implementation, so no Session Runtime code is expected to need a change for this WORK specifically; WORK-0027 covers the Session Runtime side of the overall redesign, once WORK-0026 exists to consume.
- `deriveActivePresentations`/`diffPresentations` becoming a public `engineservice` entry point for resync - unrelated, separately tracked concern, not part of GAME-ADR-0026.

## Approved Design

No design beyond GAME-ADR-0026 Decisions 1 and 6 themselves is required - this WORK is a removal of already-specified scope, not a new capability needing its own design choices beyond ordinary internal restructuring (private helper decomposition, which files hold what) that already falls inside normal Codebase Agent autonomy.

## Constraints and Invariants

- `Signal` must still let a caller identify *which kind* of thing happened (a named lifecycle signal, an intent, a question/ask-group answer, a timer expiration) - only the tree-addressing (`Path`) is removed by this WORK, not the signal-kind discrimination itself, which WORK-0026 will further revise for interaction-answering specifically.
- Every removal must be a genuine removal, not a soft-deprecation: no dead code path, no `// removed` comment, no unused-but-present type kept "just in case" - if it is later needed again, GAME-ADR-0026's own Alternatives Considered already records that as a deliberately deferred, separately-decided future possibility.
- No completed Session Runtime WORK (0001/0003/0004/0005/0019) may be reopened or have its historical scope rewritten by this change - GAME-ADR-0026 already confirmed none of them used the removed constructs; if implementation discovers otherwise, that is a DISCOVERY, not something to silently work around.

## Acceptance Criteria

- `go build ./...` and `go vet ./...` are clean repository-wide with every construct listed under In Scope actually removed (not merely unreferenced) - verified by confirming the removed identifiers no longer exist via search, not only that nothing calls them.
- `go list -deps` or an equivalent check confirms no remaining reference to a removed type/function anywhere in `game/language/v1` outside historical documentation/decision records (which are never rewritten, per Historical Immutability).
- The full existing engine/compiler/program test suite passes with only the expected removals/updates (tests that existed solely to prove removed behavior are deleted, not left failing or skipped).
- `game/language/v1/engine/LOGICAL_CONTRACT.md`'s "the engine does not recursively execute multiple transitions inside one step" statement remains accurate and is updated to note it is now a structural fact (only one instance exists) rather than a policy constraint on a tree.
- `go test ./game/session/... -count=1` (Session Runtime) continues to pass unmodified, confirming GAME-ADR-0026's finding that no completed Session Runtime WORK depended on the removed constructs.

## Implementation Freedom

Exact file/function boundaries inside `internal/compiler`/`internal/runtime` for where removed logic lived, and whether any now-single-purpose helper (e.g., an instance-tree-walk helper that only existed for child/task-group addressing) is deleted outright or generalized into whatever WORK-0025/0026 will need next, are ordinary local implementation choices within the normal Operating Model autonomy boundary.

## Verification

- `go build ./...`, `go vet ./...`, `gofmt -l` (changed files).
- `go test ./... -count=1` against real Postgres, per this repository's normal verification practice - Session Runtime's own suite is included specifically to prove no regression from this change (it should require zero Session Runtime code changes).
- A repository-wide search confirming zero remaining references to each removed identifier, outside decision-record/historical text.

## Documentation Impact

### Accepted / Canonical Knowledge

- `game/README.md` - remove the "nested workflows may later be coordinated... by a future language capability" sentence from the Authored Game Language Disconnect/Reconnect Contract section (moot: no nested instance exists to coordinate).
- `game/language/v1/engine/LOGICAL_CONTRACT.md` - update per Acceptance Criteria above.
- `game/language/v1/engine/README.md`, `game/language/v1/program/README.md` - remove Child Workflow/Task Group from their type/capability catalogs.

### Current-State Documentation After Implementation

- Any `CURRENT_STATE.md`/package-local doc describing the removed constructs as available capability.

### Intentionally Unchanged

- `game/docs/decisions/GAME-ADR-0026-...md` and every other existing GAME-ADR - decision records are historical and are not rewritten by their own implementation.
- Every completed Session Runtime WORK (`docs/projects/active/session-runtime-v1/works/WORK-0001` through `WORK-0005`/`WORK-0019`) - untouched, per Constraints above.

## Blockers

- None. GAME-ADR-0026 is ACCEPTED and this WORK's scope is fully specified by its Decisions 1 and 6.

## Completion Record

Not yet DONE. Independent review has not yet occurred.

### Implementation Report (2026-09-24)

Work: `docs/projects/active/game-language-flat-execution-model/works/WORK-0024-remove-child-workflow-and-task-group.md`

Work status: IMPLEMENTING

Implemented:
- `program`: removed `child_workflow.go`, `task_group.go`; removed `ChildSlots`/`TaskGroupSlots` from `WorkflowDeclaration`; removed `ChildCompletedSignalSource`/`TaskGroupCompletedSignalSource` and the "ParentCancelled" dead placeholder name from `signal.go`; removed the stale `ChildWorkflowSlotDeclaration`/`CancelChildWorkflowOperation` mentions from `ask_group.go`'s and `control.go`'s doc comments; `internal/codec` (`policy.go`, `slot.go`, `operation.go`, `signal.go`, `workflow.go`) has every Child/TaskGroup wire type and encode/decode path removed; `gameservice/validate.go` no longer validates the removed operations/slots/policy.
- `engine`: removed `task_group.go`; removed `SpawnChildWorkflowOperation`/`CancelChildWorkflowOperation` (`operation.go`); removed `ChildSlots`/`TaskGroupSlots`/`ChildWorkflowSlot`/`TaskGroupSlot` (`workflow.go`), `ChildWorkflowSlotInstance`/`TaskGroupSlotInstance`/`TaskGroupState`/`TaskGroupPhase*`/`TaskGroupCompletionKind*`/`TaskGroupTask` (`instance.go`), `ChildCompletedSignalSource`/`ChildFailedSignalSource`/`ChildCancelledSignalSource`/`TaskGroupCompletedSignalSource` (`signal_pattern.go` — found and removed during this session's final sweep, missed by the initial pass since these are engine-level types distinct from the already-removed `program`-level ones of the same name), `PathStep`/`Signal.Path`/`SignalKindChildCompleted`/`ChildFailed`/`TaskGroupCompleted` (`signal.go`), `WorkflowCompletedOutput.Path` (`output.go`), `MaxWorkflowDepth` (`limits.go`), `Trace.Path` (`commit.go`); rewrote `WorkflowInstance`/`WorkflowOutcome`/`Snapshot`/`Signal` doc comments to describe the one flat instance a Session runs.
- `engine/internal/compiler`: removed `compile_task_groups.go` (+ its test), `compile_child_workflows_test.go`; removed the two Child/TaskGroup signal-source compilation cases and the "ParentCancelled" entry from `compile_signals.go`; simplified `workflowContext`/removed `compileChildSlots`/`compileTaskGroupSlots` (`compile_slots.go`); removed `compileSpawnChildWorkflow`/`compileCancelChildWorkflow` and their dispatch cases (`compile_operations.go`); fixed stale doc comments in `compile_workflows.go` and `compile_symbols.go` that justified `workflowResultTypes`/`workflowParameterTypes` by a now-nonexistent cross-workflow child/task-group lookup (the fields themselves are still legitimately used for a workflow's own result/parameter types and were not touched).
- `engine/internal/runtime`: removed `task_group.go` (+ its exec test), `child_workflow_exec_test.go`; removed `path`/`childSlots`/`taskGroupSlots` from `execContext` and the slot-lookup/execution functions built on them (`execute.go`); major rewrite of `step.go` (removed 5 `ExecutionErrorCode` constants and their `String()` cases, `resolveInstance`/`applyInstancePath`/`findChildSlotIndex`/`clearedChildSlots`/`clearedTaskGroupSlots`/`validateChildOutcome`/`findInstanceChildSlot`, simplified `NewSnapshot`/`Step` to operate on `snapshot.Root` directly); fixed `ask_group.go`'s `stepAskGroupAnswer` to match.
- `engine/engineservice`: removed 5 `ExecutionErrorCode` re-exports (`runtime.go`); simplified `CheckSnapshotCompatibility` to a direct two-check version with no instance-tree recursion (`codec.go`); removed the Child/TaskGroup fixtures and integration tests from `helpers_test.go`, `codec_test.go`, `integration_features_test.go` (replacing one removed compatibility test with `TestCodec_CheckSnapshotCompatibilityDetectsUncompiledRootWorkflow`, which exercises the same still-live branch the deleted test's fixture happened to also cover).
- `engine/internal/codec/instance.go`: full rewrite removing every Child/TaskGroup wire type and codec function; Question/AskGroup/Timer slot codec unchanged.
- `game/session/workflows/sessionlifecycle`: `interaction_capture.go` replaces `encodeEnginePath(step.Path)` with a fixed `emptyEnginePath = []byte("[]")` constant (byte-identical to what was always persisted, since every real Session's Path was always empty); `internal/runtimeturn/runtimeturn.go` drops `StepTrace.Path`; `step_answer_interaction.go`/`replay.go` drop the now-nonexistent `Path` field from their `engine.Signal{}` literals. `session_interactions`' schema/migration is untouched — deferred to WORK-0027 per this WORK's own Constraints.
- Test-suite-wide mechanical fixes (no behavior change) across `program/internal/codec`, `program/gameservice`, `engine/internal/compiler`, `engine/internal/runtime`, `game/session/workflows/sessionlifecycle/internal/runtimeturn` to remove every reference to a deleted type/field/JSON key, including several raw-JSON test literals whose `"child_slots"`/`"task_group_slots"` keys would otherwise now trip `DisallowUnknownFields` with the wrong error.
- Documentation: `game/README.md` (removed the "nested workflows may later be coordinated..." sentence and the stale `ParentCancelled` mention from the Disconnect/Reconnect Contract), `game/language/v1/engine/README.md` (three stale `Path`/child-workflow/instance-tree passages rewritten), `game/language/v1/engine/LOGICAL_CONTRACT.md` (updated the "does not recursively execute" permanent constraint to state it is now a structural fact — exactly one instance exists — not a policy constraint on a tree; updated the `KeyedTimerSlot` note to drop the `Path` component of its key and cross-reference GAME-ADR-0026's generalization), `game/language/v1/program/README.md` (removed Child Workflow/Task Group from the type-catalog tables and the Slots section).

Local implementation decisions:
- Kept `workflowResultTypes`/`workflowParameterTypes`'s existing two-pass precomputation in `compile_symbols.go`/`compile_workflows.go` as-is rather than collapsing it into an inline per-workflow computation, even though its only reason for precomputing *all* workflows before compiling *any* of them (letting one workflow's child/task-group slot resolve another's result/parameter type) no longer applies — only the field's own doc comment was corrected. Collapsing the two-pass structure would be an unrequested simplification beyond this WORK's mechanical-removal scope; left for a future pass if it is ever judged worth doing on its own.
- Replaced `TestDuplicateOrdering_TaskSpawnArguments` and `TestExec_ActiveSlotLimitExceeded`'s fixtures (both built on now-deleted `SpawnTaskGroupChildOperation`/task-group-slot fixtures) with equivalent-purpose fixtures using still-live constructs (`OpenQuestionOperation`'s `Arguments`; three declared `TimerSlots` scheduled in one transition) rather than deleting the coverage outright, since the property each test verifies (argument-order preservation; the active-slot limit's atomicity) is unrelated to Child Workflow/Task Group itself.
- Deleted outright (no replacement fixture) tests whose entire subject was Child Workflow/Task Group semantics with no equivalent left to test: `TestExec_WorkflowDepthLimitExceeded` (`MaxWorkflowDepth` no longer exists), `TestExec_SignalToNonexistentPathIsSignalRejectedNotInputRejected` (Path addressing no longer exists; the `ErrSignalRejected`/`ErrInputRejected` distinction itself remains well covered elsewhere, e.g. `interaction_exec_test.go`/`ask_group_exec_test.go`), `TestWorkflowDeclaration_ComplexStructuredConcurrency`, `TestSemanticInvalidity_NilTaskGroupSpawnKey`, `TestSemanticInvalidity_InvalidTaskGroupLifecycleOrder`, `TestDecode_NestedPathFailure_WorkflowTaskGroupSlotKeyType`, plus the Child/TaskGroup integration tests and codec round-trip tests named in the file-level changes above.

Deviations from the approved WORK:
- None. This WORK's own "Implementation Freedom" section explicitly anticipated exactly the kind of test-fixture judgment calls made above.

Discoveries:
- `game/language/v1/engine/signal_pattern.go` still declared `ChildCompletedSignalSource`/`ChildFailedSignalSource`/`ChildCancelledSignalSource`/`TaskGroupCompletedSignalSource` as live, unused engine-level types after the rest of the mechanical removal was believed complete and all tests were passing (these compiled cleanly because nothing referenced them any more — a silent, not build-breaking, gap). Found via a final repository-wide identifier sweep (`grep -rln "ChildWorkflow\|TaskGroup\|..."`) run specifically because the compiler/test suite passing is not sufficient proof of complete removal per this WORK's own Acceptance Criteria ("verified by confirming the removed identifiers no longer exist via search, not only that nothing calls them"). Resolved as a LOCAL FIX: removed, with no behavior change (confirmed by full build/vet/test rerun).
- The previously known limitation from earlier in this session (Session Runtime needing minimal changes despite this WORK's own doc predicting otherwise; see `interaction_capture.go`/`runtimeturn.go`/`step_answer_interaction.go`/`replay.go` above) and the accepted `Drain` `MaxSteps`-cascade coverage gap (see `runtimeturn_test.go`'s comment at the removed subtests' former location) both remain as recorded — no new discovery beyond the `signal_pattern.go` one above.

Verification performed:
- `go build ./...`, `go vet ./...` — clean, repository-wide, after every edit in this pass (iterated to a clean state via the compiler/`go vet` error list, not merely spot-checked).
- `gofmt -l` — reports the entire pre-existing repository (every `.go` file, touched or not) due to CRLF line endings already present before this WORK; confirmed via `git diff`/`file` against an untouched file (`migrations.go`) that this is pre-existing environment noise, not a real formatting regression introduced here.
- `go test ./... -count=1` against real Postgres (`playhoot-postgres-1`, `TEST_DATABASE_*` env vars per this repository's standard practice) — every package passes except the already-known, pre-existing, out-of-scope `getgame` JSONB-whitespace comparison defect (`TestRepoGetGameCurrentVersion/returns_game_with_current_version`), run twice (before and after the `signal_pattern.go` discovery fix) with identical results both times. `go test ./game/session/... -count=1` passes unmodified, confirming this WORK's own predicted (mostly correct) claim that no completed Session Runtime WORK depended on the removed constructs — the four Session Runtime files listed above needed mechanical fixes only because they threaded the engine's own now-removed `Path`/`PathStep` types through, not because of any Session Runtime design dependency on Child Workflow/Task Group behavior itself.
- `go test . -run TestNoInternalDocCitationsInComments -v` — passes; no new comment cites an internal WORK/ADR/Blocker number as its justification.
- Repository-wide search (`grep -rln` across all `.go` files) for every removed identifier (`ChildWorkflow`, `TaskGroup`, `SpawnChild`, `CancelChildWorkflow`, `PathStep`, `SignalKindChild`, `SignalKindTaskGroup`) — zero remaining matches after the `signal_pattern.go` fix.

Documentation synchronized:
- `game/README.md`, `game/language/v1/engine/README.md`, `game/language/v1/engine/LOGICAL_CONTRACT.md`, `game/language/v1/program/README.md` — see Implemented above. `game/CURRENT_STATE.md` checked; contained no reference to the removed constructs, so left unchanged.

Known limitations:
- Session Runtime needed a minimal, mechanical change (four files) despite this WORK's own doc predicting zero Session Runtime change — recorded here rather than silently correcting that prediction after the fact.
- `Commit.InternalSignals`/`runtimeturn.Drain`'s `MaxSteps`-cascade path has no fixture left to exercise it (its only producer, spawning, is gone); documented in place as an accepted, intentional coverage gap rather than silently dropped.
- `gofmt -l`'s CRLF-driven whole-repository output (see Verification performed) was not otherwise investigated or fixed — normalizing repository-wide line endings is unrelated to this WORK's scope and was not attempted.

Ready for independent review:
YES.
