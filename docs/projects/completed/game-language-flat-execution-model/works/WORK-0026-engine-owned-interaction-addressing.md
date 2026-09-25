# WORK-0026: Engine-Owned Interaction Addressing

Status: DONE
Created: 2026-09-24
Last status change: 2026-09-24 (IMPLEMENTING -> DONE, independent review APPROVED after one fix pass; READY -> IMPLEMENTING; DRAFT -> READY, human-approved "Approved, start")

Related decisions:
- GAME-ADR-0026 (Flat Workflow Execution Model, Keyed Interaction Slots, and Engine-Owned Interaction Addressing - Decisions 3, 4, and 5)

Canonical context:
- `game/docs/decisions/GAME-ADR-0026-flat-workflow-execution-model-and-keyed-interaction-slots.md`
- `docs/projects/completed/game-language-flat-execution-model/works/WORK-0025-keyed-interaction-slots.md` (DONE - landed first, so a caller-facing ID scheme accounts for keyed occurrences from the start)
- `game/language/v1/engine/{signal.go,output.go,snapshot.go,instance.go,commit.go}` - the compiled/runtime shapes this WORK changes
- `game/language/v1/engine/internal/runtime/{execute.go,ask_group.go,step.go}` - where opening/answering/closing a Question or Ask Group occurrence is implemented today
- `game/language/v1/engine/internal/codec/instance.go` - the Snapshot persistence wire format this WORK must extend (see Constraints - WORK-0025's own Completion Record records a HIGH-severity bug where new pending-occurrence state was implemented but never wired into this file)
- `game/session/workflows/sessionlifecycle/step_answer_interaction.go` (`answerSignalKind`), `game/session/workflows/sessionlifecycle/interaction_capture.go` (`resolveInteractionKind`) - the exact consumer-side leaks this WORK removes, reworked by WORK-0027 in the same implementation pass (see Human Resolution)

## Outcome

Every opened Question or Ask Group occurrence (keyed or not) receives a unique, monotonically increasing `InteractionID`, assigned as part of the engine's own deterministic state, carried on the Output that opens (and closes) it alongside an explicit `Kind` (Question vs. AskGroup). Answering it submits only `{InteractionID, Respondent, Answer}` through one unified signal shape - never a `Slot`, `Key`, or the caller's own classification of what kind of interaction it is. This removes the last piece of engine-internal addressing that leaks into a caller, and directly enables WORK-0027's Session Runtime rework.

Timer is explicitly not part of this WORK: GAME-ADR-0026 Decision 3 scopes the engine-assigned ID to Question/Ask Group occurrences only. `TimerExpiredSignalSource`/`KeyedTimerExpiredSignalSource` and their `Signal.Slot`(+`Key`)-based addressing are unaffected - a caller does not "answer" a timer the way it answers a Question, so the leak this WORK removes does not apply to it.

## Context

GAME-ADR-0026's own Alternatives Considered rejected keeping `Path`+`Slot` and only adding `Kind`/unifying the answer signal: once WORK-0024 removed Child Workflow/Task Group, `Path` has nothing left to address, so an engine-assigned `InteractionID` is strictly simpler than keeping `Slot` (or `Slot`+`Key`, after WORK-0025) as the caller-facing address.

Reading the current implementation directly (not just the ADR): `game/language/v1/engine/internal/runtime/execute.go`'s `execOpenQuestion`/`execOpenKeyedQuestion`/`execOpenAskGroup`/`execOpenKeyedAskGroup` each occupy a slot (optionally keyed) and produce an `OpenQuestionOutput`/`OpenKeyedQuestionOutput` carrying `Slot`(+`Key`)/`Recipient`/`Question`/`Arguments` - no occurrence-identity field exists today beyond that address. Answering (`SignalKindQuestionAnswered`/`SignalKindAskGroupAnswered` and their keyed counterparts) and the Ask Group completed-awaiting-join signal (`SignalKindAskGroupCompleted`/`SignalKindKeyedAskGroupCompleted`) are all addressed the same way, by `Signal.Slot`(+`Key`). `game/session/workflows/sessionlifecycle` already leaks this directly into its own code: `step_answer_interaction.go`'s `answerSignalKind` maps a persisted `session_interactions.kind` string to a `engine.SignalKind` so it knows *which* signal shape to construct, and `interaction_capture.go`'s `resolveInteractionKind` reaches into the compiled `Program` to classify an already-opened interaction by looking up which slot collection its name belongs to - exactly the two leaks GAME-ADR-0026 Decisions 4/5 describe.

`OpenQuestionOutput`/`OpenKeyedQuestionOutput` are already reused, unchanged, for an Ask Group's own per-recipient opened questions (an ask group collects from multiple recipients, each getting their own opened-question Output, all belonging to the same one group occurrence) - see WORK-0025's own doc comments on these types. This matters directly for this WORK's design: the `InteractionID` identifies the *occurrence* being answered, so every recipient's `OpenQuestionOutput` for the same Ask Group occurrence must carry the *same* `InteractionID` - it is not one ID per opened-question Output, it is one ID per addressable, independently-answerable occurrence (an ordinary/keyed Question has exactly one recipient per occurrence, so the distinction is invisible there; an Ask Group occurrence has several).

### Human Resolution (2026-09-24, HUMAN-APPROVED)

Drafting this WORK surfaced a sequencing problem not previously identified: `game/session/workflows/sessionlifecycle` is already-shipped, DONE Session Runtime code (WORK-0001-0005/WORK-0019), and it directly constructs `engine.Signal{Kind: engine.SignalKindQuestionAnswered, Slot: interaction.EngineSlot, ...}` (`step_answer_interaction.go`, `replay.go`). If this WORK alone removes `Signal.Slot`-addressed answering, `game/session/...` cannot compile, let alone pass its own test suite - and unlike WORK-0024's forced Session Runtime edits (purely mechanical, behavior-preserving, no persistence-shape implication), there is no small bridging edit available here: Session Runtime has no durable `InteractionID` anywhere to construct the new signal with, since capturing one into `session_interactions` is WORK-0027's own persistence-model rework.

Presented to the human as a choice between (a) implementing WORK-0026 and WORK-0027 together in one combined pass so `game/session/...` is never left broken, (b) scoping WORK-0026 to also add a throwaway interim persistence bridge ahead of WORK-0027's real migration, or (c) accepting a temporarily broken `game/session/...` build until WORK-0027 lands.

**Decision: (a).** WORK-0026 and WORK-0027 are drafted and reviewed as two separate WORK specifications (each independently reviewable against its own accepted scope), but are implemented and closed together, in one combined implementation pass, so `game/session/...` remains buildable and its own existing test suite continues to pass at every point either WORK is DONE. Neither WORK is implemented, reviewed, or closed independently of the other. This also means: `go test ./game/session/... -count=1` passing is an Acceptance Criterion of the *combined* pass, not of WORK-0026 read in isolation - see WORK-0026's own Acceptance Criteria and WORK-0027's Constraints.

## Scope

### In Scope

- `engine.InteractionID` - a new engine-assigned identity type.
- `engine.Snapshot`'s own deterministic `InteractionID` counter.
- `engine.PendingQuestion`, `engine.KeyedPendingQuestion`, `engine.PendingAskGroup` (and, by embedding, `engine.KeyedPendingAskGroup`) gaining an `InteractionID` field.
- `engine.OpenQuestionOutput`/`OpenKeyedQuestionOutput` gaining `InteractionID`/`Kind`; `engine.CloseQuestionOutput`/`CloseKeyedQuestionOutput` gaining `InteractionID`.
- `engine.InteractionKind` (Question vs. AskGroup).
- Collapsing `SignalKindQuestionAnswered`/`SignalKindAskGroupAnswered`/`SignalKindKeyedQuestionAnswered`/`SignalKindKeyedAskGroupAnswered` into one unified `SignalKindInteractionAnswered`, and `SignalKindAskGroupCompleted`/`SignalKindKeyedAskGroupCompleted` into one unified `SignalKindInteractionCompleted` - both resolved by `InteractionID` alone.
- `internal/runtime`'s ID assignment (on open) and ID-to-occurrence resolution (on answer/complete), replacing today's direct `Slot`(+`Key`)-addressed lookups for Question/Ask Group only.
- `internal/codec` persistence of the new `Snapshot` counter and every pending occurrence's `InteractionID`.
- Updating every in-package (`game/language/v1/engine/...`) test/example/fixture that constructs one of the removed `SignalKind` values or reads the old `Output` shape.
- The combined implementation pass also includes WORK-0027's Session Runtime rework - see Human Resolution above and WORK-0027's own Scope.

### Out of Scope

- Anything about Timer - `TimerExpiredSignalSource`/`KeyedTimerExpiredSignalSource`, `Signal.Slot`/`Signal.Key`, `ScheduleTimerOutput`/`CancelTimerOutput`(+keyed) are unaffected; GAME-ADR-0026 Decision 3 scopes engine-assigned identity to Question/Ask Group only.
- Any change to `program` (the authoring language) or `internal/compiler` - `program.SignalSource`'s `Slot`-named variants and their compiled `engine.SignalSource` counterparts are unaffected; an authored transition still reacts to "the named slot's question was answered" exactly as today. Only the *caller-facing* `engine.Signal`/`engine.Output` shapes change - see Approved Design.
- Any change to Ask Group's own collection semantics (`AskGroupCompletionPolicy`, quorum/first-response/all-responses behavior) - unaffected, purely additive addressing change.
- `session_interactions`' persistence shape, `interaction_capture.go`/`replay.go`'s own rework - specified and implemented as WORK-0027's own scope, even though landed in the same pass.
- Adding Session Runtime support for keyed slots or for constructing the Ask Group completed-awaiting-join signal - Session Runtime does not do either today, and this WORK does not add that capability (see WORK-0027's own Out of Scope).

## Approved Design

**`InteractionID` type and assignment.** `engine.InteractionID` is a new defined type (`type InteractionID uint64`, matching `UserID`'s own precedent of a defined string/int type rather than a raw primitive). `0` is reserved to mean "no interaction" - never assigned to a real occurrence - so a zero-valued `InteractionID` (an unset Go field, or an absent/default persisted value) is never mistaken for a live one. `engine.Snapshot` gains `NextInteractionID InteractionID`, the same category of durable, replay-safe counter as `Snapshot.Sequence`: `engineservice.NewSnapshot` starts it at `1`; every time `execOpenQuestion`/`execOpenKeyedQuestion`/`execOpenAskGroup`/`execOpenKeyedAskGroup` assigns a new occurrence's `InteractionID`, it takes the current counter value and increments it, exactly mirroring how `Sequence` is threaded through `execContext` and reassembled into the new `Snapshot` at the end of a `Step` call (`internal/runtime/step.go`). Because a single transition's operations can open more than one occurrence (already proven possible by WORK-0025's own same-transition-atomicity test), the counter must live on the mutable `execContext`, not merely be `snapshot.Sequence`-derived. `InteractionID` values are never reused for the lifetime of a `Snapshot`, even after their occurrence closes/is answered - this is what guarantees a stale or replayed answer can never accidentally match a different, later occurrence, the same guarantee `Slot`(+`Key`)-addressing relied on the occupied-tuple check for.

**One `InteractionID` per occurrence, not per opened-question Output.** When `execOpenAskGroup`/`execOpenKeyedAskGroup` opens a group for several recipients, every recipient's own `OpenQuestionOutput`/`OpenKeyedQuestionOutput` carries the *same* `InteractionID` - the group is the addressable occurrence, an individual recipient's answer is authorized against it by `Respondent`, exactly as today's `Slot`-addressed `AskGroupResponse` recording already authorizes by `Respondent` within one slot.

**Pending-occurrence state gains the ID.** `PendingQuestion`, `KeyedPendingQuestion`, and `PendingAskGroup` (so `KeyedPendingAskGroup` inherits it through its embedding) each gain an `InteractionID InteractionID` field, assigned once at open time and never changed afterward.

**Output shape (Decision 5).** `OpenQuestionOutput`/`OpenKeyedQuestionOutput` gain `InteractionID InteractionID` and `Kind InteractionKind`. `CloseQuestionOutput`/`CloseKeyedQuestionOutput` gain `InteractionID InteractionID` too - the ADR's own Consequences text says the opened Output "gains" `InteractionID`/`Kind`, but a caller that must still resolve `Slot`+`Recipient` to close/correlate an interaction it already tracks by ID would keep exactly the leak this WORK removes, just moved to the closing side. Every existing field (`Slot`, `Key`, `Recipient`, `Question`, `Arguments`) stays - this is additive, not a replacement of informational content, only of the caller-facing *answering* address. `engine.InteractionKind` is a new closed enum: `InteractionKindQuestion` (zero value) and `InteractionKindAskGroup`.

**Signal shape (Decision 4).** `engine.Signal` gains `InteractionID InteractionID`. Four existing kinds collapse into one `SignalKindInteractionAnswered` (uses `InteractionID`, `Respondent`, `Answer` - covers what were `SignalKindQuestionAnswered`/`SignalKindAskGroupAnswered`/`SignalKindKeyedQuestionAnswered`/`SignalKindKeyedAskGroupAnswered`); two existing kinds collapse into one `SignalKindInteractionCompleted` (uses `InteractionID` only - covers what were `SignalKindAskGroupCompleted`/`SignalKindKeyedAskGroupCompleted`). `Signal.Slot`/`Signal.Key` remain, now meaningful only for `SignalKindTimerExpired`/`SignalKindKeyedTimerExpired`.

**Resolution.** `internal/runtime` gains a lookup, given an `InteractionID`, over `ctx`'s four pending-occurrence collections (`questionSlots`, `askGroupSlots`, `keyedQuestionSlots`, `keyedAskGroupSlots`) to find the one matching entry and which slot/key/kind it belongs to - a linear scan, consistent with the existing keyed-slot linear-search precedent (`Value.Equal`-searched `Pending` slices), since a workflow instance's simultaneously-pending-interaction count is expected to stay small. Once resolved, the rest of today's accept/reject/apply logic (Respondent authorization, answer-type/`Validation` checking, `AskGroupCompletionPolicy` re-evaluation, `SignalPattern`/`SignalBinding` matching against the compiled, still-`Slot`-named `SignalSource`) is unchanged, just parameterized by the resolved slot/key/kind instead of receiving it directly from the caller. An `InteractionID` that resolves to nothing pending (unknown, stale, or already-answered) is rejected the same way today's stale/duplicate/unauthorized `Slot`(+`Key`) submission is - see `ErrInputRejected` - no new error code is needed; this is a direct generalization of the same rejection rule.

**`program`/`internal/compiler` are untouched.** `program.QuestionAnsweredSignalSource`/`AskGroupCompletedSignalSource`(+ keyed) and their compiled `engine.SignalSource` counterparts keep naming a `Slot` - an authored transition still reacts to "the named slot's question was answered," unaffected. Only the caller-facing input/output shapes change.

## Constraints and Invariants

- `InteractionID` values must be part of `Snapshot`'s own deterministic state (never caller- or persistence-assigned) so that replaying the same signal sequence against the same starting Snapshot reproduces identical IDs - the same replay-determinism guarantee `Sequence`/`RandomState` already have.
- `InteractionID` values are never reused for the lifetime of a `Snapshot`.
- A caller answering or completing an interaction must never need to already know, or separately track, whether it is a Question or an Ask Group, or which slot/key it lives at - the engine resolves all of this from `InteractionID` alone (GAME-ADR-0026 Decision 4).
- The `Snapshot`-persistence codec (`internal/codec`) must round-trip `NextInteractionID` and every pending occurrence's `InteractionID` - WORK-0025's own Completion Record records a HIGH-severity bug where new pending-occurrence state was added to `WorkflowInstance` but never wired into this exact file, silently dropping it across persistence. This WORK's own dedicated round-trip test (see Acceptance Criteria) exists specifically so that mistake is not repeated.
- Per Human Resolution above: this WORK is not implemented, reviewed, or closed independently of WORK-0027 - both land together.

## Acceptance Criteria

- Every opened Question or Ask Group occurrence (ordinary or keyed) receives a unique, monotonically increasing, never-reused `InteractionID` as part of `Snapshot`'s own deterministic state; an Ask Group occurrence's several per-recipient `OpenQuestionOutput`/`OpenKeyedQuestionOutput`s all carry the same `InteractionID`.
- `OpenQuestionOutput`/`OpenKeyedQuestionOutput` carry `InteractionID` and `Kind`; `CloseQuestionOutput`/`CloseKeyedQuestionOutput` carry `InteractionID`. No existing field is removed.
- A single `SignalKindInteractionAnswered` replaces `SignalKindQuestionAnswered`/`AskGroupAnswered`/`KeyedQuestionAnswered`/`KeyedAskGroupAnswered`; a single `SignalKindInteractionCompleted` replaces `SignalKindAskGroupCompleted`/`KeyedAskGroupCompleted`. Neither removed `SignalKind` constant, nor any reference to it, remains anywhere in the repository (including WORK-0027's own combined implementation).
- `SignalKindTimerExpired`/`SignalKindKeyedTimerExpired` and every Timer `Output` are unchanged.
- A test proves replay-determinism for `InteractionID`: replaying an identical signal sequence against an identical starting `Snapshot` reproduces identical `InteractionID` assignments (mirroring the existing `Sequence`/`RandomState` determinism proof, if one already exists as precedent to follow).
- A dedicated `Snapshot` round-trip (encode then decode) test proves `NextInteractionID` and every pending occurrence's `InteractionID` persist correctly, per the Constraint above - the same class of test WORK-0025's Completion Record records as the fix for its own HIGH-severity finding.
- A test proves an `InteractionID` answered/completed twice (or with a wrong Respondent) is rejected via `ErrInputRejected`, exactly as today's stale/duplicate/unauthorized `Slot`(+`Key`) submission is.
- `go build ./...`, `go vet ./...`, and the full existing `program`/`engine`/compiler/runtime test suite pass.
- Per Human Resolution: `go test ./game/session/... -count=1` passes against the *combined* WORK-0026+WORK-0027 implementation (this WORK alone is not expected to leave it passing).

## Implementation Freedom

- Exact Go field/type names beyond what Approved Design fixes conceptually (for example, the precise resolution-helper function signature, whether `InteractionKind` gets a `String()` method) is ordinary Codebase Agent autonomy, matching WORK-0025's own precedent.
- Internal code sharing between the ordinary and keyed resolution paths is ordinary internal restructuring within this WORK's own scope.

## Verification

- `go build ./...`, `go vet ./...`.
- `go test ./game/language/v1/... -count=1` - full suite, including new tests for `InteractionID` assignment/resolution/replay-determinism/persistence round-trip.
- `go test . -run TestNoInternalDocCitationsInComments`.
- `go test ./game/... -count=1` against real Postgres, run once the combined WORK-0026+WORK-0027 implementation is complete (see WORK-0027's own Verification) - not meaningfully separable from this WORK given Human Resolution.

## Documentation Impact

### Accepted / Canonical Knowledge

- `game/docs/decisions/GAME-ADR-0026-...md` - "Implemented by" section gains this WORK, alongside WORK-0024/WORK-0025 (historically unedited otherwise).

### Current-State Documentation After Implementation

- `game/language/v1/engine/README.md` - `Signal`/`Output` catalogs updated to the unified/ID-carrying shapes; any remaining `Slot`(+`Key`)-addressed-answering language removed.
- `game/language/v1/engine/LOGICAL_CONTRACT.md` - if it documents the removed `SignalKind` values or `Slot`-addressed answering, updated to the new shape.
- `game/language/v1/engine/IMPLEMENTATION.md` - if it names the removed `SignalKind`/`Output` shapes in its execution-flow description, updated.

### Intentionally Unchanged

- `program` package and its own documentation (`program/README.md`, `DEFINITION.md`) - authoring-facing `SignalSource`/slot declarations are unaffected, per Approved Design.
- Everything Timer-related.

## Blockers

None. The one material sequencing question found while drafting was resolved by explicit human decision - see Human Resolution above.

## Completion Record

DONE (2026-09-24). Implemented, reviewed together with WORK-0027 (both implemented as one combined change), one REQUIRED_FIX applied and re-verified, then closed - all the same day. See "Implementation Report" and "Independent Review" below.

### Implementation Report (2026-09-24)

Work status: IMPLEMENTING

Implemented:
- `engine`: new `engine/interaction.go` - `InteractionID` (a `uint64`-based defined type, zero reserved as "no interaction") and `InteractionKind` (`InteractionKindQuestion`/`InteractionKindAskGroup`). `Snapshot` gained `NextInteractionID` (`snapshot.go`). `PendingQuestion`, `KeyedPendingQuestion`, and `PendingAskGroup` (so `KeyedPendingAskGroup` inherits it through embedding) each gained an `InteractionID` field (`instance.go`). `OpenQuestionOutput`/`OpenKeyedQuestionOutput` gained `InteractionID`/`Kind`; `CloseQuestionOutput`/`CloseKeyedQuestionOutput` gained `InteractionID` (`output.go`). `Signal` gained `InteractionID`; `SignalKindQuestionAnswered`/`SignalKindAskGroupAnswered`/`SignalKindKeyedQuestionAnswered`/`SignalKindKeyedAskGroupAnswered` collapsed into one `SignalKindInteractionAnswered`; `SignalKindAskGroupCompleted`/`SignalKindKeyedAskGroupCompleted` collapsed into one `SignalKindInteractionCompleted` (`signal.go`).
- `internal/runtime`: `execContext` gained a `nextInteractionID` counter and `assignInteractionID()` (`execute.go`), consumed by `execOpenQuestion`/`execOpenKeyedQuestion`/`execOpenAskGroup`/`execOpenKeyedAskGroup`, which now also propagate `InteractionID`/`Kind` onto their `Output`s (`execute.go`, `ask_group.go`); every `Close*Output`-producing path (`execCloseQuestion`, `execFinalizeAskGroup`, `execCancelAskGroup`, and their keyed counterparts, plus `stepAskGroupAnswer`/`stepKeyedAskGroupAnswer`'s own missing-recipient closes) now propagates the occurrence's already-assigned `InteractionID`. New `internal/runtime/interaction.go` - `resolvedInteraction`/`resolveInteractionID`: a linear scan over all four pending-occurrence collections (mirroring the existing keyed-slot linear-search precedent) resolving a caller's `InteractionID` to its underlying slot/key/family. `step.go`'s `Step` resolves `SignalKindInteractionAnswered`/`SignalKindInteractionCompleted` once, up front, into a local `resolved` copy of the signal with `Slot`/`Key` populated from that resolution - every existing Slot(+Key)-parameterized helper (`validateQuestionAnswer`/`validateKeyedQuestionAnswer`/`validateAskGroupCompletion`/`validateKeyedAskGroupCompletion`, the accept-and-clear switch, `signalSchemaFields`) is reused unchanged, just called with `resolved` instead of receiving `Slot`/`Key` directly from the caller. `signalMatchesSource`/`selectTransition` additionally take the resolved `occ.keyed` flag, since slot names are only checked for duplicates within their own declaration kind (an ordinary and a keyed slot may share a name) - needed once both families' answer signals collapse onto one `SignalKind`. `NewSnapshot` now seeds `NextInteractionID` at 1.
- `internal/codec`: `snapshot.go`'s wire struct gained `next_interaction_id`; `instance.go`'s `pendingQuestionWire`/`pendingAskGroupWire`/`keyedPendingQuestionWire`/`keyedPendingAskGroupWire` each gained `interaction_id`, wired through every encode/decode function that touches them (including the `keyedPendingAskGroupWire`/`pendingAskGroupWire` JSON round-trip reuse).
- `game/language/v1/example.go`: `gameSession.pendingQuestions` rekeyed from `map[string]OpenQuestionOutput` (by Slot) to `map[engine.InteractionID]OpenQuestionOutput`; `QuestionAnswered` now takes an `InteractionID` parameter instead of a slot name.
- Every in-package test referencing a removed `SignalKind` or the old `Output` shape updated across `engine/internal/runtime/{interaction_exec_test.go,ask_group_exec_test.go,keyed_interaction_exec_test.go,keyed_ask_group_exec_test.go}` and `engine/engineservice/{helpers_test.go,integration_features_test.go,integration_keyed_question_test.go,keyed_codec_test.go,codec_test.go}`.
- Documentation: `engine/README.md` (new "Answering an Interaction" section; "Keyed Interaction Slots" corrected to stop describing `Signal.Key`/removed `SignalKind`s as the caller-facing answer address; Outputs table updated), `engine/IMPLEMENTATION.md` (`interaction.go` added to file organization; `Step`'s commit-sequence description gained the resolution step).

Local implementation decisions:
- `InteractionID` zero is reserved to mean "no interaction," never assigned by `assignInteractionID` (which starts counting from `NewSnapshot`'s seeded `1`) - matches this WORK's own Approved Design.
- `signalMatchesSource`/`selectTransition` take the resolved `occ resolvedInteraction` as an explicit parameter rather than re-deriving keyed-ness from a second slot lookup inside `signalMatchesSource` itself - avoids a redundant lookup and keeps the resolution single-sourced from `Step`'s own one-time call to `resolveInteractionID`.
- `CloseQuestionOutput`/`CloseKeyedQuestionOutput` gained `InteractionID` even though GAME-ADR-0026's own Consequences text only explicitly mentions the *opened* Output gaining it - see this WORK's own Approved Design for why leaving `Close*Output` un-augmented would keep exactly the leak this WORK removes, just moved to the closing side.

Deviations from the approved WORK:
- None.

Discoveries:
- None new. (WORK-0026's own drafting-time discovery - the Session Runtime combined-implementation necessity - is recorded above under "Human Resolution", not here, since it was resolved before implementation began.)

Verification performed:
- `go build ./...`, `go vet ./...` - clean, repository-wide.
- `go test ./game/language/v1/... -count=1` - all pass, including new tests: `TestExec_InteractionIDAssignmentIsDeterministicAndNeverReused` (replay-determinism and never-reused proof, `internal/runtime/interaction_exec_test.go`) and `TestCodec_InteractionIDRoundTrips` (Snapshot round-trip proof through the real `NewSnapshot`/`Step` pipeline with genuinely non-zero assigned IDs, `engineservice/codec_test.go`) - the latter written specifically because of this Project's own WORK-0025 precedent (a HIGH-severity bug where new pending-occurrence state was never wired into the Snapshot codec).
- `go test . -run TestNoInternalDocCitationsInComments` - passes.
- `go test ./game/... -count=1` against real Postgres (`playhoot-postgres-1`) - passes except the already-known, pre-existing, out-of-scope `getgame` JSONB-whitespace defect; `game/session/...` passes, confirming the combined WORK-0026+WORK-0027 implementation (see WORK-0027's own Verification for its half).

Documentation synchronized: see "Implemented" above for the full list.

Known limitations:
- None beyond what Approved Design already scoped out (Timer, Presentation).

Ready for independent review:
YES (pending WORK-0027's own completion, per Human Resolution - both close together).

### Independent Review (2026-09-24)

A fresh independent review was performed per `docs/ai/protocols/IMPLEMENTATION_REVIEW.md`, covering WORK-0026 and WORK-0027 together as one combined implementation (per this WORK's own Human Resolution). The reviewer read the actual current code directly (engine, `internal/runtime`, `internal/codec`, and the Session Runtime side covered under WORK-0027), ran its own fresh `go build`/`go vet`/test verification (including real Postgres) rather than trusting either WORK's self-report, and specifically scrutinized the Snapshot codec against this Project's own WORK-0025 precedent (a HIGH-severity bug where new persisted state was never wired into `internal/codec`) - confirming `NextInteractionID` and every pending occurrence's `InteractionID` (including the `KeyedPendingAskGroup` path, which round-trips through a JSON re-marshal/unmarshal reuse) are correctly wired into both encode and decode, and that this is proven behaviorally (via `TestCodec_KeyedSlotsRoundTrip` and `TestCodec_InteractionIDRoundTrips` continuing execution against decoded state), not merely structurally.

Verdict: CHANGES_REQUIRED, one REQUIRED_FIX, no DECISION_REQUIRED findings.

Finding and fix:
1. **(LOW, REQUIRED_FIX)** `engine/commit.go`'s `Trace.TransitionName` doc comment still named the removed `SignalKindAskGroupAnswered` constant, violating this WORK's own Acceptance Criterion that no removed `SignalKind` constant or reference to it remains anywhere in the repository - the one place a repo-wide grep for all six removed constants found a live (non-historical-document) hit. **Fixed**: reworded to name `SignalKindInteractionAnswered` addressing an Ask Group occurrence instead.

The reviewer also noted one NON_BLOCKING documentation-drift observation, not required for this WORK's own closure since the document wasn't named in either WORK's Documentation Impact section: `game/docs/decisions/GAME-ADR-0024-replay-first-session-runtime-persistence.md`'s replay-input audit still named `SignalKindQuestionAnswered`/`session_interactions.engine_path`/`engine_slot` (its `SignalKindTimerExpired`/`session_timer_obligations` entry remains accurate, Timer being unaffected). Resolved anyway, mirroring the same "Implemented by" cross-reference pattern already used for GAME-ADR-0007's identical staleness, since it was a trivial, directly-analogous fix.

Re-verified after applying the fix: `go build ./...`, `go vet ./...`, `go test . -run TestNoInternalDocCitationsInComments`, a repo-wide grep for all six removed `SignalKind` constants (zero remaining Go-code hits), and `go test ./... -count=1` against real Postgres (`playhoot-postgres-1`) - clean except the already-known, pre-existing, out-of-scope `getgame` JSONB-whitespace defect. No unresolved REQUIRED_FIX or DECISION_REQUIRED finding remains for either WORK. Closed to DONE.
