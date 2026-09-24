# WORK-0025: Keyed Question, Ask Group, and Timer Slots

Status: DONE
Created: 2026-09-24
Last status change: 2026-09-24 (IMPLEMENTING -> DONE, independent review APPROVED; READY -> IMPLEMENTING; DRAFT -> READY, both Blockers human-resolved, same day)

Related decisions:
- GAME-ADR-0026 (Flat Workflow Execution Model, Keyed Interaction Slots, and Engine-Owned Interaction Addressing - Decision 2)
- GAME-ADR-0012 (Game Language Keyed Timer Slots - accepted 2026-09-07, unimplemented until this WORK)

Canonical context:
- `game/docs/decisions/GAME-ADR-0026-flat-workflow-execution-model-and-keyed-interaction-slots.md`
- `game/docs/decisions/GAME-ADR-0012-game-language-keyed-timer-slots.md` (the accepted semantics this WORK extends and implements: identity `(slot, key)`, atomic-failure-on-occupied-tuple, explicit-cancel-before-reschedule, key exposed to authored logic, no implicit reset/replace/coalesce)
- `docs/projects/active/game-language-flat-execution-model/works/WORK-0024-remove-child-workflow-and-task-group.md` (DONE - landed first, per this Project's Ordering)
- `game/language/v1/program/{workflow.go,interaction_operation.go,timer.go,ask_group.go,presentation.go,signal.go}` - the existing ordinary (non-keyed) declarations/operations/signal sources this WORK generalizes, read in full while drafting this WORK
- `game/language/v1/engine/{workflow.go,instance.go,output.go,signal.go}` and `internal/compiler/{compile_slots.go,compile_signals.go}` and `internal/runtime/{execute.go,ask_group.go,presentation.go}` - the compiled/runtime counterparts this WORK's design was checked against

## Outcome

An author can declare a Question, Ask Group, or Timer slot that holds independent, simultaneously-pending occurrences addressed by an authored key (typically a player, team, or object identity), instead of needing a separate workflow instance to get per-entity independent progress - this is what Task Group previously existed for, replaced per GAME-ADR-0026 Decision 2. This is required for the product's target range of games that need "the same simple mechanism, repeated independently per player/team/object" (an asynchronous self-paced quiz, a per-team negotiation, a per-object timer) without the nested-execution machinery WORK-0024 removed - see GAME-ADR-0026's Target Game Coverage table for the concrete games this unblocks (the asynchronous quiz/word-search pattern needs `KeyedQuestionSlot`; the two-team negotiation scenario needs `KeyedQuestionSlot`+`KeyedTimerSlot` together).

This WORK is also what finally implements GAME-ADR-0012 (ACCEPTED 2026-09-07, unimplemented since): Timer's own keyed family was deliberately included here rather than tracked separately - see Human Resolution below.

## Context

GAME-ADR-0012 already accepted the semantic shape for the Timer case - `(slot, key)` identity, atomic occupied-tuple failure, explicit cancel-before-reschedule, key exposed to authored logic, no implicit reset - but was never implemented; `game/language/v1/program/timer.go` today has only the ordinary, non-keyed `TimerSlotDeclaration`. GAME-ADR-0026 generalized that same shape to Question, Ask Group, and Presentation slots, for the identical underlying reason GAME-ADR-0012 already gave: a general primitive, not a narrow special case per mechanism.

Reading the existing ordinary slot kinds' actual code (not just their accepted ADRs) in full: Question, Ask Group, and Timer slots share one structural shape - a named location with an imperative open/schedule-then-close/cancel lifecycle, holding at most one pending occurrence, matched by a signal source keyed on the same static `Slot` name - so their keyed generalization is a direct, mechanical extension of that existing shape (add a dynamic `Key` alongside the static `Slot`, exactly as GAME-ADR-0012 already specified for Timer). Presentation is structurally different: it has no open/close operation at all - `PresentationDeclaration` is fully declarative, recomputed from `Targets` on every transition (see `deriveActivePresentations`/`diffPresentations`, `game/language/v1/engine/internal/runtime/presentation.go`), and its current occupancy identity is already `(Slot, Recipient)` - a *targeted* presentation already mounts one independent occupant per user in `Targets`, just never more than one per user for the same `Slot`. A keyed presentation's actual gap would be letting *one same user* hold several simultaneously active presentations under one `Slot`, differentiated by key - a materially different mechanism from "add a dynamic Key to an imperative open operation," and one no game in GAME-ADR-0026's own Target Game Coverage audit ever actually needed. See Human Resolution below for why this WORK does not implement it.

## Human Resolution (2026-09-24, HUMAN-APPROVED)

Both Blockers from this WORK's DRAFT pass resolved the same day:

1. **Timer's keyed-slot implementation is included in this WORK**, alongside Question/AskGroup, per the recommendation below. GAME-ADR-0012's multi-week-old implementation gap is closed here, not tracked separately.
2. **Presentation's keyed capability is deferred, not implemented by this WORK.** The proposed `KeyedPresentationDeclaration` design (recorded below for future reference) is not approved for implementation now - no concrete target game has demonstrated the need (GAME-ADR-0026's own Target Game Coverage audit never required it), mirroring the same "no demonstrated need" reasoning GAME-ADR-0026 itself used to remove Child Workflow/Task Group. This narrows this WORK's scope to Question, Ask Group, and Timer only. Revisiting Presentation's keyed capability is a future WORK's decision, made when a concrete authored game demonstrates the need - not silently reopened here.

Full original recommendations (for context, not restated as open questions):

- Blocker 1 recommended implementing Timer here because GAME-ADR-0026 itself anticipated Question/AskGroup/Timer sharing one implementation approach, and because deferring it would leave a fourth family reconciled later against already-shipped code instead of landing all three together in one reviewable diff. Accepted as recommended.
- Blocker 2 recommended approving the proposed Presentation design as the minimal extension of the existing declarative recompute-and-diff mechanism. Not accepted - deferred instead, per the point above about it having no demonstrated concrete need, unlike Question/Timer's design which the coverage audit already validated.

## Scope

### In Scope

- `KeyedQuestionSlotDeclaration`, `KeyedAskGroupSlotDeclaration`, `KeyedTimerSlotDeclaration`: new `program` declarations generalizing `QuestionSlotDeclaration`/`AskGroupSlotDeclaration`/`TimerSlotDeclaration` with an authored `KeyType TypeReference`, per the Approved Design below.
- `OpenKeyedQuestionOperation`/`CloseKeyedQuestionOperation`, `OpenKeyedAskGroupOperation`/`FinalizeKeyedAskGroupOperation`/`CancelKeyedAskGroupOperation`, `ScheduleKeyedTimerOperation`/`CancelKeyedTimerOperation`: new `program` operations generalizing the existing ordinary ones with a dynamic `Key Expression`.
- `KeyedQuestionAnsweredSignalSource`, `KeyedAskGroupCompletedSignalSource`, `KeyedTimerExpiredSignalSource`: new `program` signal sources exposing `key` as a bound schema field alongside the existing `respondent`/`answer` (question) or `responses`/`respondents`/`missing` (ask group), or alone (timer).
- The compiled `engine` counterparts of all of the above, `internal/compiler` validation/compilation support, and `internal/runtime` execution support (opening/scheduling into an occupied `(slot, key)` as an atomic execution error; per-key independent pending state; signal matching/resolution by `(slot, key)`; per-key presentation mounting for Question, reusing the existing `QuestionPresentationDeclaration` shape with an added implicit `key` binding available only when compiling against a keyed slot - Ask Group's own `Presentation` field compiles and validates identically, but is never mounted at runtime, since the ordinary, non-keyed `AskGroupSlotDeclaration.Presentation` this WORK's Ask Group family mirrors has no runtime mounting path either; this is pre-existing, unrelated drift this WORK neither introduces nor is scoped to fix).
- Updating `game/language/v1/example.go` if it would benefit from illustrating a keyed slot (optional, only if it clarifies rather than pads the file).

### Out of Scope

- **Presentation's keyed capability, deferred per Human Resolution above.** No `KeyedPresentationDeclaration` or equivalent is implemented by this WORK. The proposed design is recorded below for whenever a future WORK picks this up, but nothing in `program.PresentationDeclaration`, `program.PresentationSlotDeclaration`, or their compiled/runtime counterparts changes.
- `InteractionID`, the unified answer signal, and `Kind` on the interaction-opened Output (WORK-0026) - this WORK's `Key`-bearing operations/signals/outputs still address by `(slot, key)` (or `(slot, key, respondent)` where relevant), exactly like every existing ordinary slot still addresses by `slot` alone; WORK-0026 replaces that whole addressing scheme uniformly, for keyed and non-keyed occurrences together, and is explicitly sequenced after this WORK so it can design the ID scheme with keyed occurrences already in mind.
- Any Session Runtime (`game/session`) change, and any `session_interactions`/`session_timer_obligations` persistence-model change (GAME-ADR-0007/GAME-ADR-0012's own deferred "persistence consequence") - WORK-0027 covers the Session Runtime side once WORK-0026 exists to consume, per this Project's Ordering.
- Redesigning `AskGroupCompletionPolicy`'s own collection semantics - a keyed ask group reuses the existing three policies unchanged, evaluated independently per `(slot, key)` occurrence, exactly as an ordinary ask group evaluates them per `slot` occurrence today. No new completion-policy variant is introduced.
- Declarative "keyed state machine" authoring sugar, or Child Workflow/Task Group returning as a compile-time authoring-reuse mechanism - both explicitly deferred by GAME-ADR-0026's own Alternatives Considered, unrelated to this WORK.
- Any change to the *ordinary* (non-keyed) `QuestionSlotDeclaration`/`AskGroupSlotDeclaration`/`TimerSlotDeclaration`/`PresentationDeclaration` shapes or semantics - every keyed family is purely additive, mirroring how GAME-ADR-0012 already treated `KeyedTimerSlot` as "distinct from the existing ordinary single-pending-timer `TimerSlotDeclaration`," never a modification of it.

## Approved Design

### Key type

A keyed slot's `KeyType` is an ordinary `TypeReference`, validated exactly like `MapTypeReference.Key` already is today (no additional restriction - `gameservice/validate.go` places none on map key types, and every compiled `engine.Value` variant already implements structural `Equal`, which is all `(slot, key)` occupancy matching needs). This is not a new type-system rule; it reuses an already-accepted one.

### Question: `KeyedQuestionSlotDeclaration`

```go
type KeyedQuestionSlotDeclaration struct {
    Name         string
    Question     string
    KeyType      TypeReference
    Presentation *QuestionPresentationDeclaration // reused unchanged; see "Presentation reuse" below
}

type OpenKeyedQuestionOperation struct {
    Slot      string
    Key       Expression // must eventually compile to KeyType
    Recipient Expression
    Arguments []CallArgument
}

type CloseKeyedQuestionOperation struct {
    Slot string
    Key  Expression
}

type KeyedQuestionAnsweredSignalSource struct {
    Slot string // schema: "key" (KeyType), "respondent" (User), "answer" (question's ResponseType)
}
```

Semantics mirror `QuestionSlotDeclaration`/`OpenQuestionOperation`/`CloseQuestionOperation`/`QuestionAnsweredSignalSource` exactly, with `(Slot, Key)` as the occupancy identity instead of `Slot` alone: opening an already-occupied `(slot, key)` is an atomic execution error; different keys under the same slot are fully independent and may be simultaneously pending; closing an empty `(slot, key)` is an idempotent no-op; only a validated answer to the current pending occupant of `(slot, key)` ever produces the signal.

### Ask Group: `KeyedAskGroupSlotDeclaration`

```go
type KeyedAskGroupSlotDeclaration struct {
    Name         string
    Question     string
    KeyType      TypeReference
    Presentation *QuestionPresentationDeclaration // reused unchanged
}

type OpenKeyedAskGroupOperation struct {
    Slot       string
    Key        Expression
    Recipients Expression
    Arguments  []CallArgument
    Completion AskGroupCompletionPolicy
}

type FinalizeKeyedAskGroupOperation struct {
    Slot string
    Key  Expression
}

type CancelKeyedAskGroupOperation struct {
    Slot string
    Key  Expression
}

type KeyedAskGroupCompletedSignalSource struct {
    Slot string // schema: "key" (KeyType), "responses", "respondents", "missing" (same shapes as the ordinary ask-group signal, scoped to this one (slot,key) group)
}
```

Semantics mirror the ordinary `AskGroupSlotDeclaration` family exactly, `(Slot, Key)` as occupancy identity; every existing `AskGroupCompletionPolicy` variant (`AllResponses`/`FirstResponse`/`Quorum`) is reused unchanged, evaluated independently per `(slot, key)` group.

### Timer: `KeyedTimerSlotDeclaration`

```go
type KeyedTimerSlotDeclaration struct {
    Name    string
    KeyType TypeReference
}

type ScheduleKeyedTimerOperation struct {
    Slot              string
    Key               Expression
    DelayMilliseconds Expression
}

type CancelKeyedTimerOperation struct {
    Slot string
    Key  Expression
}

type KeyedTimerExpiredSignalSource struct {
    Slot string // schema: "key" (KeyType) - the one field GAME-ADR-0012 itself already specified
}
```

This is exactly GAME-ADR-0012's own already-accepted shape, given concrete Go names for the first time. Semantics mirror `TimerSlotDeclaration`/`ScheduleTimerOperation`/`CancelTimerOperation`/`TimerExpiredSignalSource` exactly, `(Slot, Key)` as occupancy identity.

### Presentation reuse for keyed Question/AskGroup slots

`QuestionPresentationDeclaration` is reused unchanged (not duplicated into a `KeyedQuestionPresentationDeclaration`) as the presentation config type for keyed Question/AskGroup slots. Its already-documented `ProjectionArguments` scope (recipient, question's captured parameters, global, resources, functions, built-ins) gains one more implicit binding, `"key"` (typed `KeyType`), available only when the compiler resolves it against a keyed slot's declaration. This is a minimal, additive extension of an already-precisely-documented contract, not a new declaration type. This is unrelated to, and does not depend on, the deferred `PresentationSlotDeclaration`/`PresentationDeclaration` keyed capability - it only concerns the small `QuestionPresentationDeclaration` config type already owned by (and private to) a question/ask-group slot.

At runtime, only keyed Question actually mounts its configured `Presentation`. A keyed Ask Group's `Presentation` compiles and validates identically but is never derived into an `Activate`/`Update`/`RemovePresentationOutput` - this mirrors the ordinary, non-keyed `AskGroupSlotDeclaration.Presentation`, which has the identical gap in the existing engine (`deriveActivePresentations` never walks `AskGroupSlotInstance`). This is pre-existing drift this WORK found but does not fix, since fixing it would mean introducing genuinely new Ask Group runtime behavior this WORK's own Constraint ("no change to any ordinary slot... semantics") does not authorize.

### Compiled (`engine`) and runtime shape

- `engine.Workflow` gains `KeyedQuestionSlots []KeyedQuestionSlot`, `KeyedAskGroupSlots []KeyedAskGroupSlot`, `KeyedTimerSlots []KeyedTimerSlot` - each carrying `Name`, `Question` (where applicable), `KeyType engine.Type`, `Presentation *QuestionPresentation` (where applicable).
- `engine.WorkflowInstance` gains `KeyedQuestionSlots []KeyedQuestionSlotInstance` etc., each holding a `map[string]PendingQuestion`-shaped (or equivalent ordered/deterministic) collection keyed by the *encoded* key value (using the same key-encoding approach `engine.Value.Equal`-based map lookups already use elsewhere in this codebase, e.g. `MapValue.Entries`) - exact Go collection shape (map vs. sorted slice, for deterministic snapshot encoding/replay) is Implementation Freedom, not prescribed here, so long as replay determinism (per `LOGICAL_CONTRACT.md`) is preserved.
- New `engine` Outputs: `OpenKeyedQuestionOutput`/`CloseKeyedQuestionOutput`, an ask-group-opened equivalent (if ask-group opening currently produces per-recipient `OpenQuestionOutput`s, as it does today - see `AskGroupSlotDeclaration`'s doc comment - the keyed variant does the same, each carrying `Key`), and `ScheduleKeyedTimerOutput`/`CancelKeyedTimerOutput` - each a close mirror of its ordinary counterpart plus a `Key engine.Value` field. These are kept as distinct types from the ordinary Outputs, not the ordinary Output types with an added always-nil-for-non-keyed-slots field, consistent with this Project's "genuine removal/addition, no dead-but-present field" standard already applied by WORK-0024.
- `internal/compiler`: `compile_slots.go` gains `compileKeyedQuestionSlots`/`compileKeyedAskGroupSlots`/`compileKeyedTimerSlots`, each validating `KeyType` the same way an ordinary `MapTypeReference.Key` is validated; `compile_signals.go` gains the three keyed `SignalSource` cases, each resolving `KeyType` into the bound `"key"` schema field.
- `internal/runtime`: `execute.go` gains `execOpenKeyedQuestion`/`execCloseKeyedQuestion` etc., using `(slot, key)` lookup instead of `slot` alone, with the identical atomic-occupied-tuple-failure/no-implicit-replacement behavior as the ordinary operations (see `execOpenQuestion`'s existing occupied-slot check as the direct precedent); `step.go`'s signal-matching logic resolves a keyed signal source's `Slot` to the right `(slot, key)` occupant using the signal's own submitted `Key`/`Respondent`, mirroring how it already resolves an ordinary `Slot`-only signal today.

### Deferred: Presentation's keyed capability (not implemented by this WORK)

Recorded here only so the proposal is not lost, for whenever a future WORK revisits it with a demonstrated concrete need: a new `KeyedPresentationDeclaration`, structurally parallel to `PresentationDeclaration` but replacing `Targets: Expression` (`list<User>`) with `KeyedTargets: Expression` (`list<record{key: KeyType, user: user}>`), occupancy identity becoming `(Slot, Key, Recipient)` instead of the ordinary `(Slot, Recipient)`, `deriveActivePresentations` gaining a `presentationKey.Key engine.Value` field and a parallel `addKeyedTargeted` pass alongside the existing `addTargeted`, reusing `diffPresentations` unchanged, plus one new implicit `"key"` projection-argument binding. This is not approved design and is not implemented by this WORK - see Human Resolution above.

## Constraints and Invariants

- Must match GAME-ADR-0012's already-accepted per-tuple semantics for every keyed slot family implemented here: atomic failure on an occupied `(slot, key)`, explicit cancel/close before reschedule/reopen, no implicit reset/replace/coalesce.
- Must not reintroduce any form of nested execution/instance addressing - every keyed slot's multiple occurrences all belong to the one existing workflow instance WORK-0024 already established; `Key` is ordinary authored-language data, never an addressing mechanism reaching outside the one instance.
- The key is authored-language information and must be exposed to whatever resolves against it (a matched transition's bound `"key"`, a keyed question/ask-group presentation's projection arguments) - never an internal addressing detail, mirroring GAME-ADR-0012's explicit requirement.
- No change to any *ordinary* (non-keyed) slot declaration, operation, signal source, Output, or their compiled/runtime counterparts, and no `KeyedPresentationDeclaration` or equivalent - every implemented keyed family is additive only, and Presentation is untouched entirely (see Scope's Out of Scope).
- Genuine addition, not soft/partial scaffolding: no unused type, no `// not implemented` placeholder path, no dead field for the deferred Presentation capability anywhere in `program`/`engine`.

## Acceptance Criteria

- `program`, `engine`, `internal/compiler`, and `internal/runtime` compile, validate, and execute `KeyedQuestionSlotDeclaration`, `KeyedAskGroupSlotDeclaration`, and `KeyedTimerSlotDeclaration` with the semantics in Approved Design: independent simultaneous occurrences per key, atomic occupied-`(slot,key)` failure, no implicit replacement, key exposed to a matching transition's bound signal and (when a `Presentation` is configured, for Question only) to the mounted view's projection arguments. Ask Group's `Presentation` field compiles/validates but has no runtime mounting path, matching the ordinary Ask Group family's own identical, pre-existing gap.
- This WORK is GAME-ADR-0012's implementing WORK (still ACCEPTED, historically unedited - this WORK implements it, does not rewrite it).
- A repository-wide test proves the core occupied-tuple invariant for at least Question (the family every audited target game actually needs): opening `(slot, key=A)` while `(slot, key=B)` is already pending succeeds and both remain independently pending and independently answerable; opening `(slot, key=A)` while `(slot, key=A)` is already pending atomically fails the whole transition, leaving every other pending mutation/output in that transition uncommitted.
- Equivalent occupied-tuple coverage exists for Ask Group and Timer.
- `go build ./...`, `go vet ./...`, and the full existing `program`/`engine`/compiler/runtime test suite pass, with new tests covering every keyed family this WORK implements.
- `go test ./game/session/... -count=1` continues to pass unmodified - this WORK, like WORK-0024, is not expected to require any Session Runtime change (Session Runtime does not yet author or consume any keyed slot).
- Canonical documentation (`game/language/v1/program/README.md`, `game/language/v1/engine/README.md`, `game/docs/decisions/GAME-ADR-0012-...md`'s cross-reference note) reflects exactly Question/AskGroup/Timer's keyed families as implemented - no documentation claims Presentation's keyed capability is available.
- No repository-wide reference describes `KeyedPresentationSlot`/`KeyedPresentationDeclaration` as implemented or available (only, where useful, as an explicitly deferred future possibility).

## Implementation Freedom

- Exact Go field/type names beyond what Approved Design fixes conceptually (e.g., whether `engine.WorkflowInstance`'s keyed-slot occupancy is a `map[string]...` or a sorted `[]...` for deterministic encoding, exact wire/codec shape in `internal/codec` if keyed slot state needs snapshot persistence) is ordinary Codebase Agent autonomy.
- Internal compiler/runtime code sharing across the three keyed families (a shared occupied-tuple-check helper, for instance) is ordinary internal restructuring within this WORK's own scope, not a cross-WORK "shared package" decision requiring the `repositories.md` Sharing Rule escalation that applies to Session Runtime's Go persistence code - this is language/engine-internal implementation, a different concern.

## Verification

- `go build ./...`, `go vet ./...`, `gofmt -l` (changed files).
- `go test ./game/language/v1/... -count=1` - full suite, including new tests for Question/AskGroup/Timer's keyed families.
- `go test ./game/... -count=1` against real Postgres, confirming `game/session/...` passes unmodified (no Session Runtime change expected, matching WORK-0024's own confirmed pattern).
- A repository-wide search confirming no keyed-family type/operation/signal-source this WORK's Scope claims to implement is left partially wired (declared in `program` but not compiled, compiled but not executable, etc.), and confirming no live reference anywhere describes Presentation's keyed capability as implemented.

## Documentation Impact

### Accepted / Canonical Knowledge

- `game/docs/decisions/GAME-ADR-0012-game-language-keyed-timer-slots.md` - remains ACCEPTED and historically unedited (Historical Immutability); this WORK is recorded as GAME-ADR-0012's implementing WORK (a cross-reference addition to `GAME-ADR-0012`'s own "Implementation Impact" section is *not* a rewrite of its accepted content, matching how other GAME-ADRs already record their implementing WORK).
- `game/docs/decisions/GAME-ADR-0026-...md` - historically unedited; this WORK is one of its Implementation Impact's named future WORKs (partially - Presentation's keyed capability remains unimplemented and undecided-when).

### Current-State Documentation After Implementation

- `game/language/v1/program/README.md`, `game/language/v1/engine/README.md` - add Question/AskGroup/Timer's keyed families to their existing type/capability catalogs, exactly alongside the ordinary families they generalize.
- `game/language/v1/engine/LOGICAL_CONTRACT.md` - update the already-present "accepted-not-yet-implemented `KeyedTimerSlot<Key>`" note to describe it as implemented.
- `game/language/v1/program/DEFINITION.md` - add Question/AskGroup/Timer's keyed slot families to the AI-authoring reference (operation table, signal-source table, workflow JSON shape) - this document is authoring-facing and must never describe a capability this WORK did not actually implement, mirroring the lesson from WORK-0024's own multi-pass documentation-accuracy review.

### Intentionally Unchanged

- Every ordinary (non-keyed) slot declaration/operation/signal-source's own documentation - unaffected, per this WORK's purely additive scope.
- `program.PresentationDeclaration`/`program.PresentationSlotDeclaration` and all their documentation - unaffected; Presentation's keyed capability is deferred, not implemented.
- `docs/projects/active/session-runtime-v1/` - unaffected; Session Runtime does not consume any keyed slot until WORK-0027.

## Blockers

None. Both Blockers from this WORK's DRAFT pass were resolved by explicit human decision - see Human Resolution above.

## Completion Record

DONE (2026-09-24). Implemented, reviewed, fixed, and independently re-reviewed to a clean APPROVED verdict in a single day - see the Implementation Report, Independent Review, and Independent Re-Review sections below for the full history.

### Implementation Report (2026-09-24)

Work: `docs/projects/active/game-language-flat-execution-model/works/WORK-0025-keyed-interaction-slots.md`

Work status: IMPLEMENTING

Implemented:
- `program`: `KeyedQuestionSlotDeclaration` (`workflow.go`), `KeyedAskGroupSlotDeclaration` + `OpenKeyedAskGroupOperation`/`FinalizeKeyedAskGroupOperation`/`CancelKeyedAskGroupOperation` (`ask_group.go`), `KeyedTimerSlotDeclaration` + `ScheduleKeyedTimerOperation`/`CancelKeyedTimerOperation` (`timer.go`), `OpenKeyedQuestionOperation`/`CloseKeyedQuestionOperation` (`interaction_operation.go`), `KeyedQuestionAnsweredSignalSource`/`KeyedTimerExpiredSignalSource`/`KeyedAskGroupCompletedSignalSource` (`signal.go`); `WorkflowDeclaration` gained `KeyedQuestionSlots`/`KeyedAskGroupSlots`/`KeyedTimerSlots`.
- `program/internal/codec`: wire encode/decode for every new declaration (`slot.go`), operation (`operation.go`), and signal source (`signal.go`); `WorkflowDeclaration`'s wire struct extended with the three new fields, canonical key order `question_slots, ask_group_slots, timer_slots, keyed_question_slots, keyed_ask_group_slots, keyed_timer_slots, presentations, ...`.
- `program/gameservice`: `validate.go` validates each keyed slot's `KeyType` (reusing `validateTypeReference`, the same rule `MapTypeReference.Key` already gets - no new restriction), each keyed slot's `Presentation`, and each new operation's `Key`/other expressions.
- `engine`: compiled `KeyedQuestionSlot`/`KeyedAskGroupSlot`/`KeyedTimerSlot` (`workflow.go`), `KeyedQuestionSlotInstance`/`KeyedAskGroupSlotInstance`/`KeyedTimerSlotInstance` holding a `[]KeyedPending*` slice per slot (occupancy identity `(slot, key)`, searched linearly by `Value.Equal` - the same technique `MapValue.Entries` already uses, since an arbitrary authored key type has no cheaper canonical hash) (`instance.go`), `OpenKeyedQuestionOutput`/`CloseKeyedQuestionOutput`/`ScheduleKeyedTimerOutput`/`CancelKeyedTimerOutput` (`output.go`), the three compiled keyed signal sources (`signal_pattern.go`), four new `SignalKind` values and a `Signal.Key` field (`signal.go`), and the seven compiled keyed `Operation` variants (`operation.go`, `ask_group.go`).
- `internal/compiler`: `compileKeyedQuestionSlots`/`compileKeyedAskGroupSlots`/`compileKeyedTimerSlots` (`compile_slots.go`, registering each into new `workflowContext` maps carrying the slot's own compiled `KeyType`); `compileQuestionPresentation` gained a `keyType engine.Type` parameter adding an implicit `"key"` scope binding when non-nil (both ordinary call sites pass `nil`, unchanged behavior); three new signal-source compile cases (`compile_signals.go`); seven new operation compile functions plus a shared `compileSlotKey` helper (`compile_operations.go`, `compile_ask_groups.go`).
- `internal/runtime`: `execContext` gained keyed slot candidate-copy fields and find/declaration helpers (`execute.go`); four keyed question/timer exec functions plus `activeSlotCount` extended to count every occupied `(slot, key)` tuple (`execute.go`); three keyed ask-group exec functions plus `stepKeyedAskGroupAnswer` (mirroring `stepAskGroupAnswer`) (`ask_group.go`); `step.go` wired keyed signal routing/validation/accept-and-clear/newTarget-assembly/terminal-outcome-clearing (`clearedKeyedAskGroupSlots`), `NewSnapshot`'s keyed-slot instance initialization, `signalSchemaFields`/`signalMatchesSource` keyed cases; `presentation.go` wired keyed question slots into `deriveActivePresentations` (one occupant per `(slot, key)` entry, `"key"` bound in projection scope) - keyed ask-group presentations are deliberately not wired, mirroring ordinary `AskGroupSlotInstance`'s own presentation handling, which this function likewise never derives.
- Documentation: `game/language/v1/program/README.md`, `game/language/v1/engine/README.md` (new "Keyed Interaction Slots" sections + closed-variant-type table/Outputs table updates), `game/language/v1/engine/LOGICAL_CONTRACT.md` (moved the keyed-timer note out of "accepted, not yet implemented," now describing the implemented Question/AskGroup/Timer state and Presentation's deferral), `game/language/v1/program/DEFINITION.md` (Operation table, SignalSource table, `WorkflowDeclaration`/keyed-slot JSON shapes, workflow-shape ordering list; also fixed a stale "structured-concurrency ... validation" phrase left over from WORK-0024's own removal), `game/README.md` (Session Runtime's own "Keyed Timer Slots" section corrected from future-tense "will support" to present-tense "implements," clarifying only the Session Runtime persistence consequence remains open), `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` (two references to a "not-yet-designed keyed-timer-slot compiler/engine implementation" corrected - that part is now implemented; only `engine_key`'s own column representation remains open), `game/docs/decisions/GAME-ADR-0012-game-language-keyed-timer-slots.md` and `GAME-ADR-0026-flat-workflow-execution-model-and-keyed-interaction-slots.md` (new "Implemented by" cross-reference sections, historical content otherwise unedited).

Local implementation decisions:
- Used three concrete Go struct types (`keyedQuestionSlotEntry`/`keyedAskGroupSlotEntry`/`keyedTimerSlotEntry`) in the compiler's `workflowContext` instead of a generic type parameterized over the three declaration types. This codebase does use generics elsewhere (e.g. `nilIfEmpty[T any]` in `engine/internal/codec/wire.go`, similar helpers in `program/internal/codec/wire.go` and `utils/db_transactions.go`), so the choice here was not about introducing the first generic - it was that a generic parameterized over three otherwise-unrelated declaration types would need either an interface method set none of them naturally share or a type-switch inside the generic function anyway, while three concrete three-line structs are simpler to read at each of the (few, non-repeating) call sites.
- Kept `OpenKeyedQuestionOutput`/`CloseKeyedQuestionOutput` as distinct `Output` types from the ordinary `OpenQuestionOutput`/`CloseQuestionOutput` (each simply gaining a `Key` field) rather than adding an optional `Key` field to the existing ordinary types, so no ordinary (non-keyed) call site carries an always-nil field - consistent with this Project's established "no dead-but-present field" standard from WORK-0024.
- `KeyedQuestionSlotInstance`/`KeyedTimerSlotInstance`/`KeyedAskGroupSlotInstance` each hold occupancy as a `[]KeyedPending*` slice searched linearly by `Value.Equal`, not a Go map - an arbitrary authored key `Value` has no canonical hash to bucket by, and this mirrors the identical technique `engine.MapValue.Entries` already uses elsewhere in this codebase for the same reason.
- `KeyedPendingAskGroup` embeds the existing `PendingAskGroup` unchanged (adding only `Key`) rather than duplicating its five fields, since a keyed occurrence's own collection semantics are byte-for-byte identical to an ordinary ask group's.

Deviations from the approved WORK:
- None.

Discoveries:
- **Canonical-vs-implementation drift, unrelated to keyed slots, pre-existing (not introduced by this WORK):** `program/control.go`'s own doc comment on `WorkflowControl` states "Expressions inside a WorkflowControl observe the transition's working state, including any mutations made by earlier operations in the same transition" - but `engineservice.Step`'s actual implementation does not do this. `step.go` builds one `scope` from `snapshot.GlobalState`/the pre-transition instance once, before `Operations` runs; every `SetOperation` (and every other mutation) writes to `ctx.global`/`ctx.local` directly, never back into that `scope` value. The result: not just `Control`, but any *later operation in the same block* that reads `global`/`local` by reference also sees the pre-transition value, not an earlier operation's own just-made mutation within the identical transition - only a later, separate transition (once the mutation has actually committed to the new `Snapshot`) observes it. Confirmed by reading `step.go`'s `scope`/`ctx.global` threading directly; affects every operation/control combination identically regardless of keyed or ordinary slots; was not previously exercised by any existing test (the one prior integration test that could have exposed it, `TestIntegration_HeadlessGuessTheRollGame`, never takes the code path that would depend on it). Surfaced only because this WORK's own new integration test initially relied on the documented (but not actual) behavior and had to be restructured around the real one. Not fixed here - changing `engineservice.Step`'s core same-transition-mutation-visibility semantics is a material behavior change clearly outside this WORK's own scope (keyed-slot families) - but recorded here as drift rather than silently worked around or left undocumented, since a future author relying on `WorkflowControl`'s own documented contract would hit the identical surprise. Worth a dedicated follow-up (either fix `Step` to match the documented contract, or correct the contract to describe actual behavior) - not decided or authorized here.

Verification performed:
- `go build ./...`, `go vet ./...` - clean, repository-wide.
- `gofmt -l` - clean for every new/changed file, checked directly (not merely against the repository's pre-existing CRLF-driven whole-repo noise, per the lesson from WORK-0024's own verification history).
- `go test ./game/language/v1/... -count=1` - all pass, including 50 new test functions in dedicated new test files across `program/internal/codec` (`keyed_slot_test.go`, round-trip/decode-null), `program/gameservice` (`validate_keyed_slots_test.go`, KeyType/operation-expression validation), `engine/internal/compiler` (`compile_keyed_interactions_test.go`, compile success/undeclared-slot/key-type-mismatch/argument-mismatch/signal-binding/duplicate-name/presentation-key-binding), `engine/internal/runtime` (`keyed_interaction_exec_test.go`/`keyed_ask_group_exec_test.go`, the occupied-tuple atomicity/independent-simultaneous-keys/correct-key-routing/active-slot-limit-counting/presentation-mounting proofs for Question, Ask Group, and Timer), and `engine/engineservice` (`integration_keyed_question_test.go`, a full Compile->NewSnapshot->Step integration test for an asynchronous two-player quiz using `KeyedQuestionSlot`; `keyed_codec_test.go`, the Snapshot round-trip proof added in the fix pass below).
- `go test ./... -count=1` against real Postgres (`playhoot-postgres-1`) - every package passes except the already-known, pre-existing, out-of-scope `getgame` JSONB-whitespace defect (`TestRepoGetGameCurrentVersion`); `go test ./game/session/... -count=1` passes unmodified, confirming this WORK's own predicted "no Session Runtime change" (Session Runtime does not yet author or consume any keyed slot).
- `go test . -run TestNoInternalDocCitationsInComments` - passes; six comments that initially cited GAME-ADR-0012/0026/WORK-0025 as justification (in `program/{workflow,timer,signal}.go`, `internal/runtime/presentation.go`, and one test file) were rewritten to state their reasoning directly, per `docs/engineering/standards/code-comments.md`.
- A repository-wide search confirmed no keyed-family type/operation/signal-source is left partially wired (every one is declared, compiled, and executable), and no live reference anywhere describes Presentation's keyed capability as implemented.

Documentation synchronized: see "Implemented" above for the full list.

### Independent Review (2026-09-24)

A fresh independent review was performed per `docs/ai/protocols/IMPLEMENTATION_REVIEW.md` against the Implementation Report above, reasoning from primary evidence (the WORK spec, GAME-ADR-0012/0026, `LOGICAL_CONTRACT.md`, the actual diff, and by running `go build`/`go vet`/`go test` directly) rather than trusting the self-report.

Verdict: DECISION_REQUIRED, plus REQUIRED_FIX and NON_BLOCKING findings.

Findings and how each was resolved:
1. **(HIGH, REQUIRED_FIX)** `engine/internal/codec/instance.go`'s Snapshot codec never encoded/decoded the 3 new keyed-slot `WorkflowInstance` fields (`KeyedQuestionSlots`/`KeyedAskGroupSlots`/`KeyedTimerSlots`) - `EncodeSnapshot` followed by `DecodeSnapshot` silently dropped all keyed-slot state, meaning any pending keyed question/ask-group/timer would vanish across persistence and subsequent answer/expire/complete attempts would fail. **Fixed**: added `keyedQuestionSlotWire`/`keyedPendingQuestionWire`, `keyedAskGroupSlotWire`/`keyedPendingAskGroupWire`, `keyedTimerSlotWire`/`keyedPendingTimerWire` and their encode/decode functions, wired into `EncodeWorkflowInstance`/`DecodeWorkflowInstance`. Verified by a new test, `engine/engineservice/keyed_codec_test.go`'s `TestCodec_KeyedSlotsRoundTrip`, which opens occurrences in all 3 keyed families (including a partially-collected keyed ask group), round-trips the Snapshot, asserts equality, and then continues executing directly against the decoded snapshot (answering the remaining question, completing the ask group, expiring the timer) to prove the restored state is genuinely usable, not just byte-identical.
2. **(MEDIUM, DECISION_REQUIRED)** The Scope/AC text above overclaimed "per-key presentation mounting for Question/AskGroup," but the implementation (correctly, mirroring ordinary AskGroup) never mounts AskGroup presentations. Resolved without escalating to the human: this WORK's own Constraint ("no change to ordinary slot semantics") already settles it, since ordinary (non-keyed) AskGroup does not mount presentations either - correcting the overclaiming text to match actually-approved behavior is the documentation fix; implementing new AskGroup presentation-mounting behavior would itself be the unauthorized scope expansion. **Fixed**: corrected `program/README.md`, `LOGICAL_CONTRACT.md`, `program/ask_group.go`'s doc comment, and this WORK's own Scope/AC/Presentation-reuse text.
3. **(MEDIUM, REQUIRED_FIX)** No test exercised keyed Question presentation mounting/key-binding end to end. **Fixed**: added `TestExec_KeyedQuestionPresentation_MountsIndependentlyPerKeyWithKeyBinding` to `engine/internal/runtime/keyed_interaction_exec_test.go`.
4. **(LOW, REQUIRED_FIX)** The PresentationSlot-collision constraint (one recipient cannot hold 2 simultaneous keys with `Presentation` set on the same ordinary `PresentationSlot`) was not documented. **Fixed**: documented in `program/workflow.go`'s `Presentation` field doc comment and `DEFINITION.md`.
5. **(LOW, REQUIRED_FIX)** The occupied-tuple atomicity test only proved atomicity trivially (via by-value snapshot copy), not genuinely across multiple operations in one transition. **Fixed**: added `TestExec_OccupiedKeyFailsAtomically_DiscardsEarlierOperationsInSameTransition` with a dedicated fixture.
6. **(LOW, REQUIRED_FIX)** A test comment and this WORK's Discoveries text described the same-transition Control-visibility gap too narrowly (as if specific to Control or to later transitions). **Fixed**: corrected `integration_keyed_question_test.go`'s comment and the Discoveries entry above to describe the actual, broader gap (any later operation in the same block, not just Control).
7. **(LOW, REQUIRED_FIX)** Stale/overclaiming tracking text: `PROJECT.md`'s Ordering/Dependencies bullet still listed Presentation in scope and referenced the long-resolved Blocker 1; `internal/AI_CONTEXT.md`'s Resume Context body still described WORK-0024's review, not WORK-0025's implementation; `step.go`'s `ExecutionErrorSlotOccupied`/`ExecutionErrorInputRejected`/`ErrInputRejected` doc comments and `IMPLEMENTATION.md`'s `Step` description did not mention the keyed variants. **Fixed**: all corrected.
8. **(NON_BLOCKING)** The Local implementation decisions entry above claimed "this codebase has no existing generics usage anywhere" - false (generics exist at `engine/internal/codec/wire.go`, `program/internal/codec/wire.go`, `utils/db_transactions.go`). The underlying decision (3 concrete structs over a generic) remains sound for other reasons. **Fixed**: corrected the rationale above; also corrected the new-test count (was stated as 46, actually 50 new test functions in dedicated new test files - see Verification above).

Re-verified clean after applying every REQUIRED_FIX and the self-resolved DECISION_REQUIRED finding: `go build ./...`, `go vet ./...`, `go test ./game/language/v1/... -count=1`, `go test . -run TestNoInternalDocCitationsInComments`.

Re-verified again after this section was written, confirming the fix pass itself: `go build ./...`, `go vet ./...`, `go test . -run TestNoInternalDocCitationsInComments`, `go test ./game/language/v1/... -count=1`, and `go test ./game/... -count=1` against real Postgres (`playhoot-postgres-1`) - all clean except the already-known, pre-existing, out-of-scope `getgame` JSONB-whitespace defect (`TestRepoGetGameCurrentVersion`); `game/session/workflows/sessionlifecycle` passes unmodified.

### Independent Re-Review (2026-09-24)

A second, genuinely fresh independent review (no memory of the implementation or first review pass) was performed per `docs/ai/protocols/IMPLEMENTATION_REVIEW.md`, specifically to re-verify the fix pass above given Finding #1's severity - mirroring WORK-0024's own multi-pass precedent.

Verdict: **APPROVED**.

The reviewer independently verified all 8 findings' claimed fixes by reading the actual current code/docs (not the write-up), with particular scrutiny on Finding #1: read `engine/internal/codec/instance.go` in full against `engine.WorkflowInstance`'s actual struct fields, confirmed every field of all 3 keyed families round-trips (including `KeyedPendingAskGroup`'s embedded `PendingAskGroup` fields), and confirmed `keyed_codec_test.go` is a genuinely meaningful test (drives execution against the decoded snapshot, not just a byte-equality check). The reviewer also ran a fresh repository-wide sweep (grepping for ordinary, non-keyed slot field names to find any call site that might have an overlooked keyed counterpart) and found none. `go build`/`go vet`/the full `program`/`engine`/compiler/runtime suite/the doc-citation standards test were all re-run directly by the reviewer, not trusted from this record. Every Acceptance Criterion above was independently confirmed met, not merely claimed met.

The reviewer independently re-examined the self-resolved DECISION_REQUIRED finding (#2, AskGroup presentation mounting) and, exercising the protocol's own reviewer authority for non-material clarifications ("the reviewer determines whether the original finding is resolved or no longer blocking"), confirmed the resolution is correct and the finding is closed - reasoning that the fix changed no runtime behavior, only documentation, bringing it into compliance with this WORK's own already-approved Constraint.

One NON_BLOCKING finding, out of this WORK's scope:
- **[LOW] Pre-existing documentation drift, not introduced by this WORK.** `game/language/v1/program/ask_group.go`'s ordinary (non-keyed) `AskGroupSlotDeclaration` doc comment (lines 24-35) claims presentation mounting happens per recipient - this is false against the current runtime (`deriveActivePresentations` never walks `AskGroupSlots`/`KeyedAskGroupSlots`), confirmed predating this WORK via `git diff`. Not fixed here - out of this WORK's authorized scope (an unrelated pre-existing file) - but recorded here per this repository's drift-reporting rule (`AGENTS.md` point 7) so it is not lost. Worth a trivial standalone follow-up fix; flagged as conspicuous since it sits directly above this WORK's own, correctly-worded `KeyedAskGroupSlotDeclaration` doc comment in the same file.

No unresolved REQUIRED_FIX or DECISION_REQUIRED findings remain. Closure preconditions are satisfied: approved scope is fully implemented, tests exist and pass, required documentation is synchronized, no unapproved material deviation exists. WORK-0025 is closed to DONE (see Status header above).

Known limitations:
- Presentation's keyed capability remains deferred, per the Human Resolution - not a limitation of this implementation, an explicit scope boundary.
- The `Control`-sees-stale-global-state property recorded under Discoveries is a pre-existing `engineservice.Step` characteristic, not something this WORK changed or is positioned to fix.

Ready for independent review:
YES.
