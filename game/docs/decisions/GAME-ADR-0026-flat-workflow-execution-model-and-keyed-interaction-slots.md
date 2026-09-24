# GAME-ADR-0026: Flat Workflow Execution Model, Keyed Interaction Slots, and Engine-Owned Interaction Addressing

Status: ACCEPTED
Created: 2026-09-24
Last status change: 2026-09-24 (PROPOSED -> ACCEPTED, human-approved)
Supersedes: None
Superseded by: None
Generalizes: GAME-ADR-0012 (Keyed Timer Slots) - the keyed-slot concept accepted there for timers is extended here to Questions, Ask Groups, and Presentations, and its underlying rationale ("a general primitive, not a narrow special case") is what motivates removing nested workflow execution in favor of it.

## Context

Game Language's engine (`game/language/v1/engine`) supports nested workflow execution: a running workflow instance may spawn a **Child Workflow** (`program.ChildWorkflowSlotDeclaration`/`SpawnChildWorkflowOperation`/`CancelChildWorkflowOperation`) or begin a **Task Group** (`program.TaskGroupSlotDeclaration`/`BeginTaskGroupOperation`/`SpawnTaskGroupChildOperation`/`SealTaskGroupOperation`/`FinalizeTaskGroupOperation`/`CancelTaskGroupOperation`) - a dynamically-sized set of same-workflow child instances. Each spawned instance is independently addressed by an `engine.Signal.Path` (a walk from the root instance down through named child/task-group slots), forms its own node in a recursive instance tree (`engine.WorkflowInstance.ChildSlots`/`TaskGroupSlots`), and requires the caller to: (1) feed back the `WorkflowStarted` `engine.Commit.InternalSignals` entry a spawn produces, in a separate `Step` call, before the new instance does anything; and (2) once a child/task/ask-group reaches a terminal outcome, separately detect that (there is no automatic notification - the caller must inspect `Snapshot` slot/group `Phase` fields directly) and construct a join signal (`SignalKindChildCompleted`/`ChildFailed`/`ChildCancelled`/`TaskGroupCompleted`) addressed by the same `Path`+`Slot`, to let the parent instance react.

This capability was never put through this repository's own decision-record process: no GAME-ADR accepted it, and its rationale/scope was never written down. It reached the current implementation directly. Three concrete gaps were found while reviewing the consumer-facing (`engineservice`/Session Runtime) surface built on top of it:

1. **No authored example, test fixture, or Session Runtime WORK exercises it.** `game/language/v1/example.go` and every fixture under `game/session/workflows/sessionlifecycle` use only flat, single-instance workflows.
2. **The already-accepted canonical documentation itself defers how the outside world interacts with a nested instance.** `game/README.md`'s Authored Game Language Disconnect/Reconnect Contract states plainly: "there is no implicit broadcast to nested child/task-group/ask-group instances, since the engine's `Step` contract addresses one instance path per call and does not currently support hidden multi-instance fan-out. Nested workflows may later be coordinated explicitly by authored root-level logic or a future language capability." The same gap applies to `program.EmitUserIntentAction` (see below) and to Session Runtime, which has no implemented mechanism at all for delivering a child/ask-group/task-group join signal - a Session using these constructs today would leave a completed child permanently unjoined.
3. **The addressing scheme it requires (`Path`+`Slot`) leaks into the consumer at every point of contact**: answering an interaction, classifying what an `OpenQuestionOutput` actually is, resolving a `WorkflowCompletedOutput`'s meaning (root vs. informational-only child), and routing a `program.EmitUserIntentAction`. On that last point specifically: `EmitUserIntentAction`'s own doc comment gives it no mechanism analogous to `AnswerQuestionAction`'s ("the mounted question context, not this action, identifies the concrete question instance") - there is no authored UI construct that could ever supply a non-root target for a user intent. The engine mechanically permits a `UserIntentSignalSource` inside a non-root workflow, but nothing in the authoring language can ever address it. This is unreachable capability, not a deliberately supported one.

Separately, GAME-ADR-0012 already established, for the narrower case of `TimerSlotDeclaration`, that "one independent occurrence per statically named slot" is too restrictive whenever a game needs several simultaneous, independently-addressed occurrences of the same authored mechanism (its own motivating example: one disconnect timeout per player). It solved this with a **keyed slot** family (`KeyedTimerSlot<Key>`) rather than by requiring a separate workflow instance per player.

This record asks the same question GAME-ADR-0012 already answered for timers, of the whole nested-execution model: does any concrete target game need a genuinely separate execution instance (its own address, its own spawn/join lifecycle), or does "keyed slot" generalize to cover the same need with substantially less machinery?

## Decision

### 1. Remove Child Workflows and Task Groups; a Session runs exactly one workflow instance

Game Language no longer supports spawning a nested workflow instance. `program.ChildWorkflowSlotDeclaration`, `SpawnChildWorkflowOperation`, `CancelChildWorkflowOperation`, `TaskGroupSlotDeclaration`, `BeginTaskGroupOperation`, `SpawnTaskGroupChildOperation`, `SealTaskGroupOperation`, `FinalizeTaskGroupOperation`, and `CancelTaskGroupOperation` are removed from the authoring language, along with their compiled `engine` counterparts, their signal sources (`ChildCompletedSignalSource`/`ChildFailedSignalSource`/`ChildCancelledSignalSource`/`TaskGroupCompletedSignalSource` and the corresponding `SignalKind` values), and the recursive instance-tree shape they required (`engine.WorkflowInstance.ChildSlots`/`TaskGroupSlots`, `engine.PathStep`, `engine.Signal.Path`, `engine.Limits.MaxWorkflowDepth`).

**Ask Groups are a distinct, unrelated concept and are not affected.** `program.AskGroupSlotDeclaration`/`OpenAskGroupOperation`/`FinalizeAskGroupOperation`/`CancelAskGroupOperation` never spawn a separate instance - an ask group collects answers from multiple recipients into one pending group within the same instance. It is kept, and gains the keyed generalization below.

A compiled `Program` therefore has exactly one instantiated workflow (the former "root") for the lifetime of a Session. `engine.Signal` no longer needs a `Path` field: every signal targets the one existing instance, unconditionally.

### 2. Generalize keyed slots (GAME-ADR-0012) to Questions, Ask Groups, and Presentations

Question, Ask Group, and Presentation slots each gain a keyed family (`KeyedQuestionSlot<Key>`, `KeyedAskGroupSlot<Key>`, `KeyedPresentationSlot<Key>` - naming/API/Go type names are not frozen by this record, matching GAME-ADR-0012's own restraint), following the exact semantics already accepted for `KeyedTimerSlot<Key>`:

- Conceptual identity is `(slot, key)` - at most one pending occurrence per exact tuple.
- Opening into an already-occupied `(slot, key)` is an execution error, atomic for the whole enclosing transition; there is no implicit reset/replace/coalesce.
- Different keys under the same slot are fully independent and may be simultaneously pending (`quiz_question[P1]` and `quiz_question[P2]` may both be open at once, each progressing at its own pace).
- The key is authored-language information and is exposed to whatever resolves against it (a response's binding, a projection's arguments); it is never an internal addressing detail.

This is what "many players/teams/objects each independently progressing through the same authored mechanism" resolves to, replacing what Task Groups previously existed for.

### 3. Engine-assigned incremental interaction identity replaces `Path`+`Slot`(+`Key`) as the caller-facing address

Every opened Question or Ask Group occurrence (keyed or not) receives a unique, monotonically increasing `InteractionID`, assigned as part of the engine's own deterministic state (the same category of durable, replay-safe counter as `Snapshot.Sequence` - never computed by a caller, since a caller-assigned id would not survive replay identically). This ID is carried on the corresponding `Output` when the interaction opens. A caller answering it submits only `{InteractionID, Respondent, Answer}` - never a `Slot`, `Key`, or `Path`. The engine resolves internally which pending occurrence (and its owning slot/key) the ID addresses.

### 4. One unified "answer this interaction" signal, resolved by ID

The caller no longer chooses between what were `SignalKindQuestionAnswered` and `SignalKindAskGroupAnswered`. A single signal shape addresses "answer `InteractionID` with `Answer`, submitted by `Respondent`"; the engine determines from the ID alone which underlying execution semantics apply (an ordinary question's transition-selecting behavior vs. an ask group's non-transition-selecting collect-and-reevaluate behavior). A caller never needs to already know, or separately track, which kind of interaction it is answering.

### 5. The interaction-opened Output self-describes its kind

The Output produced when a Question or Ask Group occurrence opens carries an explicit `Kind` (Question vs. AskGroup) as one of its own fields. A caller never needs the compiled `Program` to classify what it just received by looking up which slot collection a name belongs to.

### 6. `WorkflowCompletedOutput` is always about the one existing instance

The `Path`-based distinction between "the root completed" (actionable) and "a child/task/ask-group-owning instance completed" (informational only, per its prior doc comment) is removed along with Child Workflows/Task Groups. Every occurrence of this Output is about the one instance that exists, and is always actionable the way a root completion already was.

### 7. `UserIntentSignalSource` always targets the one existing workflow instance

The previously open, never-actually-authorable question of "which instance does a user intent target" is resolved by construction: there is only one. `program.EmitUserIntentAction` needs no addressing mechanism, matching what the authoring language could actually produce even before this record.

## Target Game Coverage

This record's Alternatives analysis worked through the following games/scenarios to confirm the flat, keyed-slot model covers the product's accepted expressivity target (`docs/product/PRODUCT_STATE.md`'s "quizzes through Parqués/UNO/poker-level complexity") without loss, using concrete examples rather than only the two abstract patterns GAME-ADR-0012 already covered for timers:

| Game / scenario | Shape needed | Covered by |
|---|---|---|
| Parqués, UNO, Snakes and Ladders, The Game of Life | Sequential turns over shared board state | Flat root; per-player state indexed by a map/list (`program.MapTypeReference`, `IndexTarget`) |
| Poker | Sequential betting rounds, folded/all-in players skipped | Flat root; per-player keyed state, no new construct |
| Monopoly | Turn sequence + ad hoc two-player trade negotiation + jail | Flat root; a trade is a few keyed fields (`pending_trade`) and a targeted Question, not a nested instance |
| Clue | Turn sequence + a private "show one matching card" exchange targeted at one specific player + private per-player notes | Already-supported targeted Question + per-recipient Presentation scoping; no nested instance needed |
| Kahoot/Quizizz, synchronous variant (everyone answers the same question, time runs out, reveal) | One question opened to all recipients at once, quorum/timeout completion | Existing `AskGroupSlot`, unaffected by this record |
| Kahoot/Quizizz, asynchronous variant (each player proceeds through their own question sequence at their own pace, compares only at the end) | Many independent, simultaneously-pending, per-player occurrences of the same mechanism | `KeyedQuestionSlot<PlayerID>` (this record); a shared "finished" flag per player gates the final reveal |
| Circular prefix/suffix word game (rotating single active player, 5-second timer, typed word) | One active player at a time, a Question racing a Timer | Flat root; already-supported Question+Timer race |
| Codenames-style two-team clue game | Alternating team-scoped phases; a clue-giver role with a private view; guesses validated against hidden per-card ownership | Flat root; team/role membership is data checked in guards, not separate instances; role-scoped Presentation already supported |
| Word search / crossword | Independent per-player puzzle progress | `KeyedQuestionSlot`/keyed state per player, same shape as the asynchronous quiz |
| Two-team "simultaneous decision" scenario (each team internally deliberates - propose, role-gated approval, revise, lock in - both teams progressing at the same time, actions resolved together only once both lock in) | Two independently-progressing, role-gated, multi-step negotiations with a shared synchronization barrier | `KeyedQuestionSlot`/`KeyedTimerSlot` per team + per-team/per-object (ship) map-indexed state (`ships: map<ShipID, ShipState>`) + a guard on the barrier transition (`team_state[A].locked_in AND team_state[B].locked_in`) reading both teams' state directly, since it is all in one shared instance |

No game in this set required a genuinely separate execution instance. The "two-team simultaneous decision" scenario was deliberately chosen as the hardest test (roles, approval chains, per-object dynamic state, and a real synchronization barrier between two concurrently-progressing sub-processes) and is fully expressible without nested execution: apparent "parallelism" between the two teams is achieved because neither team's guards reference the other's progress until the barrier transition, not because the engine executes anything concurrently - the engine already only ever processes one signal at a time, regardless of this record.

Real-time/spatial games (free movement of an avatar through a scene, as opposed to discrete state/position changes) remain explicitly out of scope for Game Language, unrelated to this record.

## Rationale

**No demonstrated need.** Across every game/scenario examined, including one deliberately engineered to be the hardest plausible case for "true" parallelism, keyed slots on a single flat instance fully covered the requirement. Removing an unused, never-formally-decided capability that the canonical documentation itself already treats as unfinished (`game/README.md`'s "nested workflows may later be coordinated... by a future language capability") is not a loss against any accepted product commitment.

**This closes several previously-open problems as a side effect, not as separate fixes.** With exactly one instance:
- `Commit.InternalSignals`'s only motivating case (a spawned child's own `WorkflowStarted`) no longer exists, so the caller-side draining loop this repository already had to build (`runtimeturn.Drain`) has nothing left to drain in practice.
- The unresolved child/ask-group/task-group join-signal gap (never implemented in Session Runtime) is moot - there is nothing to join.
- The cross-instance presentation-slot collision risk found while reviewing `deriveActivePresentations` (nothing today prevents two different instances from claiming the same `(Slot, Recipient)` pair, since that check is scoped per-instance) cannot occur - there is only one instance to claim anything.
- `EmitUserIntentAction`'s unreachable-non-root-targeting inconsistency is resolved by construction rather than by a restrictive rule that would need to be remembered and enforced.

**Authoring reliability.** `docs/product/PRODUCT_STATE.md` commits to AI-assisted game creation from a creator's natural-language description, with low iteration/error rates as an explicit goal. "Declare fields, index by key, guard by role/team/identity" is a pattern much closer to ordinary data-and-conditionals programming, which language models generate reliably. "Construct a correctly addressed spawn/join instance tree" is a rarer, more structurally error-prone pattern with more subtle failure modes (an orphaned child, a wrong `Path`, a forgotten join) - exactly the kind of mistake more likely to survive a first generation pass undetected.

**Keyed slots are not a new kind of primitive.** They generalize a pattern this repository already accepted for timers (GAME-ADR-0012) for the identical underlying reason ("a general primitive, not a narrow special case"). Applying it consistently to Questions/Ask Groups/Presentations, instead of leaving those three to rely on nested instances for the same need, is the same decision GAME-ADR-0012 already made, extended to where it already logically belonged.

## Alternatives Considered

### Keep Child Workflows/Task Groups; fix the consumer-facing leaks around them (auto-drain internal signals, auto-generate join signals)

Rejected. This was the direction under discussion before the target-game audit. It would have removed the *caller-visible* symptoms (manual signal draining, manual join construction) but kept the underlying complexity that produces them: a recursive instance tree, `Path` addressing, cross-instance state-collision risk, and a capability no concrete target game was found to need. Fixing the symptom without removing the unneeded root cause is a worse trade once the audit showed zero games require it.

### Keep Child Workflows/Task Groups solely as an authoring-reuse (not execution) mechanism - e.g., compile-time inlining/expansion of a named sub-block into the parent instance

Considered as a possible future direction if authored-definition duplication (the same block repeated for two teams, for example) becomes a demonstrated pain point once real games are built against the flat model. Rejected for now as speculative: no concrete game analyzed required this, and building it preemptively without a demonstrated need contradicts this project's own anti-overengineering standard. May be revisited later as a NOW/SOON/LATER call once real authoring experience exists.

### Add declarative "keyed state machine" sugar (named per-key states/transitions) instead of relying on authors to hand-roll an enum field plus guards

Considered, for the case where a per-key sub-process has many named states. Rejected for now for the same reason as above - no concrete game analyzed needed more than a few state values per key, so this is deferred until real authoring experience demonstrates the ergonomic cost is worth a new construct, rather than being added preemptively.

### Leave interaction addressing on `Path`+`Slot`, only adding the `Kind` field and unifying the answer signal

Rejected. `Path` becomes entirely vestigial the moment Child Workflows/Task Groups are removed (there is nothing left to address beyond the one instance), so keeping it as a caller-facing concept would be dead structure for no benefit; the engine-assigned `InteractionID` is strictly simpler once `Path` has no reason to exist.

## Consequences

- `program` package: `child_workflow.go` and `task_group.go`'s spawn/seal/finalize/cancel operations and their slot declarations are removed; `ask_group.go` is unaffected except for gaining a keyed slot variant; `QuestionSlotDeclaration`/`PresentationSlotDeclaration`/`TimerSlotDeclaration` each gain a keyed counterpart.
- `engine` package: `WorkflowInstance.ChildSlots`/`TaskGroupSlots`, `PathStep`, `Signal.Path`, `Limits.MaxWorkflowDepth`, `SignalKindChildCompleted`/`ChildFailed`/`ChildCancelled`/`TaskGroupCompleted`, and the corresponding `ExecutionErrorCode` values tied to spawning/joining (`ExecutionErrorWorkflowDepthExceeded`, `ExecutionErrorChildOutcomeNotJoined`, `ExecutionErrorTaskGroupNotJoined`, `ExecutionErrorTaskGroupLeftBuilding`, `ExecutionErrorDuplicateTaskKey`) are removed. `OpenQuestionOutput`/ask-group-opened Output gain `InteractionID` and `Kind`; `SignalKindQuestionAnswered`/`SignalKindAskGroupAnswered` collapse into one signal shape. `WorkflowCompletedOutput` drops its `Path` field (or the field becomes permanently unused).
- `internal/compiler`: `compile_child_workflows.go`/`compile_task_groups.go` passes are removed; `compile_ask_groups.go`/question/presentation/timer-slot compilation gain keyed-family handling.
- `internal/runtime`: `child_workflow` and `task_group` execution files are removed; interaction-opening/answering logic gains ID assignment/resolution; `deriveActivePresentations`/`diffPresentations` simplify (no instance-tree walk needed - see also the still-open, separate item about exposing a public resync entry point, unaffected by this record).
- **No completed or in-progress Session Runtime WORK is invalidated by this record.** WORK-0001 through WORK-0005/WORK-0019 (DONE) and WORK-0006 (IMPLEMENTING, paused) never used Child Workflows/Task Groups or `Path`-based addressing beyond the single root instance already implied by the current implementation; this confirms the audit's finding that these constructs were never actually exercised. Session Runtime's own `interaction_capture.go`/`replay.go` will need rework to consume `InteractionID` instead of encoding/decoding `engine_path`/`engine_slot`, but this is new work against the new engine contract, not a reopening of prior WORK's historical scope.
- `session_interactions`' identity (GAME-ADR-0007: `(engine_path, engine_slot)`) needs a follow-up persistence-model update to key on the engine's own `InteractionID` instead - tracked as implementation impact below, not decided in full by this record.

## Canonical Knowledge Impact

- `game/README.md` - the Authored Game Language Disconnect/Reconnect Contract's "nested workflows may later be coordinated... by a future language capability" sentence is removed (moot); no broadcast-to-nested-instances concern exists once there is only one instance.
- `game/language/v1/engine/LOGICAL_CONTRACT.md` - "the engine does not recursively execute multiple transitions inside one step" remains true and becomes a direct structural consequence (there is only one instance to step) rather than a constraint on a tree; the accepted-not-yet-implemented `KeyedTimerSlot<Key>` note is updated to reference the now-general keyed-slot family.
- `game/language/v1/engine/README.md` - `Signal.Path`/nested-instance documentation removed; `InteractionID`/unified answer-signal/`Kind` documented once implemented.
- `game/language/v1/program/README.md` - Child Workflow/Task Group declarations and operations removed from the type catalog; keyed slot variants documented.
- `game/docs/decisions/GAME-ADR-0012-game-language-keyed-timer-slots.md` - remains ACCEPTED and historically accurate as written (Historical Immutability); this record notes it generalizes GAME-ADR-0012's concept rather than rewriting it.
- `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` / GAME-ADR-0007 - `session_interactions`' identity model needs a follow-up decision/update once implementation specifics (the concrete `InteractionID` persistence shape) are designed; not resolved by this record.

## Implementation Impact

No compiler, engine, program, migration, or Session Runtime code, and no WORK, is authorized by this record alone, matching GAME-ADR-0012's own precedent. Future WORKs must design and implement: the concrete keyed-slot declarations/operations/signal sources across Question/AskGroup/Presentation; the `InteractionID` assignment/persistence mechanism and its replay-determinism proof; the unified answer-signal shape; removal of Child Workflow/Task Group from `program`/`engine`/compiler/runtime; and Session Runtime's own rework of interaction capture/replay/answer-signal construction against the new contract. WORK-0006 (currently paused, IMPLEMENTING reverted to DRAFT pending this record) and the rest of `session-runtime-v1`'s Phase 1 remain paused until at least the engine/compiler/program side of this record lands, since they depend directly on the `Output`/`Signal` shapes this record changes.

### Implemented by

`docs/projects/active/game-language-flat-execution-model/works/WORK-0024-remove-child-workflow-and-task-group.md` (DONE) implements Decisions 1 and 6 (Child Workflow/Task Group removal, `WorkflowCompletedOutput` always actionable). `docs/projects/active/game-language-flat-execution-model/works/WORK-0025-keyed-interaction-slots.md` implements Decision 2's Question/Ask Group/Timer portion - human-approved as non-materially narrowed from this record's own text to explicitly exclude Presentation's keyed family, deferred pending a concrete demonstrated need (see that WORK's own Human Resolution). Decisions 3-5 (`InteractionID`, the unified answer signal, `Kind`) are implemented by `docs/projects/active/game-language-flat-execution-model/works/WORK-0026-engine-owned-interaction-addressing.md`; the Session Runtime rework this record's own Implementation Impact flagged as a follow-up is implemented by `docs/projects/active/game-language-flat-execution-model/works/WORK-0027-session-runtime-interaction-addressing-rework.md` (implemented together with WORK-0026 in one combined pass - see WORK-0026's own Human Resolution).
