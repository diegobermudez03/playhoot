# WORK-0027: Session Runtime Interaction-Addressing Rework

Status: DONE
Created: 2026-09-24
Last status change: 2026-09-24 (IMPLEMENTING -> DONE, independent review APPROVED after one fix pass, applied to WORK-0026's own commit.go - see this WORK's own Completion Record; READY -> IMPLEMENTING; DRAFT -> READY, human-approved "Approved, start")

Related decisions:
- GAME-ADR-0026 (Flat Workflow Execution Model, Keyed Interaction Slots, and Engine-Owned Interaction Addressing)
- GAME-ADR-0007 (Session Runtime Turn Architecture and Persistence Model - `session_interactions`' `(engine_path, engine_slot)` identity is superseded in part by this WORK; a follow-up cross-reference note is owned here, not a rewrite of GAME-ADR-0007 itself)

Canonical context:
- `docs/projects/active/game-language-flat-execution-model/works/WORK-0026-engine-owned-interaction-addressing.md` (DRAFT, implemented together with this WORK in one combined pass - see its own Human Resolution)
- `game/session/workflows/sessionlifecycle/interaction_capture.go` (`captureInteractions`, `resolveInteractionKind` - removed by this WORK)
- `game/session/workflows/sessionlifecycle/step_answer_interaction.go` (`answerSignalKind`, `AnswerInteraction`'s signal construction - reworked)
- `game/session/workflows/sessionlifecycle/replay.go` (`buildAnswerSignal`, `loadReplaySignal` - reworked)
- `game/session/workflows/sessionlifecycle/internal/repo/interaction.go` (`Interaction`, `CreateInteraction`, `CloseActiveInteraction` - migrated)
- `docs/projects/active/session-runtime-v1/PROJECT.md` (Phase 1 is paused pending this Project - see its own "Paused (2026-09-24)" section)
- `docs/projects/active/session-runtime-v1/works/WORK-0001-session-lobby-foundation.md`'s "Standard-Compliance Migration Record" section - this repository's own established precedent for a pre-launch schema replacement (drop-then-recreate via new migration files, historical migration files untouched), reused by this WORK's own migration approach below

## Outcome

`game/session/workflows/sessionlifecycle` consumes WORK-0026's new engine contract directly: `session_interactions` is keyed by the engine's own `InteractionID` instead of encoding/decoding `(engine_path, engine_slot)`, `captureInteractions` no longer needs the compiled `Program` to classify an opened interaction's kind (it reads `Kind` straight off the Output), and answering constructs the new unified `SignalKindInteractionAnswered` signal. This is what unblocks `session-runtime-v1`'s Phase 1 (WORK-0006 onward) to resume.

This WORK is implemented together with WORK-0026, in one combined pass, per WORK-0026's own Human Resolution - `game/session/...` has no durable `InteractionID` to construct WORK-0026's new signal shape with until this WORK's own persistence rework lands, so the two cannot be landed independently without leaving `game/session/...` broken.

## Context

`game/session/workflows/sessionlifecycle` today persists `session_interactions.engine_path`/`engine_slot` (GAME-ADR-0007) and uses them three ways: `interaction_capture.go`'s `captureInteractions` writes them from each captured `OpenQuestionOutput`/`CloseQuestionOutput`, calling `resolveInteractionKind` (a compiled-`Program` slot-collection lookup) to classify Question vs. AskGroup since `OpenQuestionOutput` itself carries no such field; `step_answer_interaction.go`'s `AnswerInteraction` reads the persisted `EngineSlot` back to construct the outgoing `engine.Signal`, choosing its `Kind` via `answerSignalKind`'s `interaction.Kind` (persisted at capture time) -> `engine.SignalKind` mapping; `replay.go`'s `buildAnswerSignal` does the identical reconstruction for replay. `engine_path` itself has been dead weight since WORK-0024: `captureInteractions` already writes a constant `emptyEnginePath` (`"[]"`) for every row, since a Session runs exactly one workflow instance.

`session_timer_obligations` (GAME-ADR-0007) also has `engine_path`/`engine_slot` columns "with the same internal meaning as for interactions" - but Timer is explicitly out of WORK-0026's scope (GAME-ADR-0026 Decision 3 only assigns `InteractionID` to Question/Ask Group), so `session_timer_obligations` is unaffected by this WORK; see Out of Scope.

Ask Group's own completed-awaiting-join signal (what was `SignalKindAskGroupCompleted`/`SignalKindKeyedAskGroupCompleted`, unified by WORK-0026 into `SignalKindInteractionCompleted`) is never constructed anywhere in `game/session/workflows/sessionlifecycle` today - `answerSignalKind` only ever produces what were `SignalKindQuestionAnswered`/`SignalKindAskGroupAnswered`, one per individually-answered recipient. This is a pre-existing gap (an authored transition gated on an Ask Group's completion would never fire through Session Runtime today), unrelated to interaction *addressing* and not introduced or worsened by this WORK - see Out of Scope.

## Scope

### In Scope

- `session_interactions`' persisted identity: replace `engine_path`/`engine_slot` with the engine's own `InteractionID`.
- `interaction_capture.go`: remove `resolveInteractionKind` and `emptyEnginePath`; `captureInteractions` reads `InteractionID`/`Kind` directly off `OpenQuestionOutput`/`CloseQuestionOutput` (now populated by WORK-0026).
- `step_answer_interaction.go`: `AnswerInteraction` constructs `engine.Signal{Kind: engine.SignalKindInteractionAnswered, InteractionID: ..., Respondent: ..., Answer: ...}` directly from the persisted `InteractionID`; `answerSignalKind`'s `interaction.Kind` -> `engine.SignalKind` branch is removed (no longer needed - the engine resolves Kind internally).
- `replay.go`: `buildAnswerSignal`/`loadReplaySignal` reworked the same way, for replay reconstruction.
- `internal/repo/interaction.go`: `Interaction`, `CreateInteraction`, `CloseActiveInteraction`, and their SQL migrated from `(engine_path, engine_slot)` to `engine_interaction_id`.
- A new migration (or migrations) implementing the schema replacement - see Approved Design.
- `docs/projects/active/session-runtime-v1/PROJECT.md`'s pause note - resolved/updated to reflect Phase 1 is unblocked (the pause note itself, not resuming Phase 1's own work, which is `session-runtime-v1`'s own Project's job).

### Out of Scope

- `session_timer_obligations` and anything Timer-related - unaffected; keeps `engine_path`/`engine_slot` exactly as today, including `engine_path`'s already-dead-weight-since-WORK-0024 value. (Recorded as a Tracked Follow-Up in this Project's `PROJECT.md`, not fixed here - a materially different, unrelated cleanup.)
- Adding Session Runtime support for keyed slots (`OpenKeyedQuestionOutput`/keyed Ask Group) - `captureInteractions` still has no case for them, exactly as today; no Session Runtime WORK authors a keyed slot yet.
- Adding Session Runtime construction of the Ask Group completed-awaiting-join signal (`SignalKindInteractionCompleted`) - not constructed today, and this WORK does not add that capability. Only the already-existing per-response answering path is reworked.
- Resuming `session-runtime-v1`'s own Phase 1 (WORK-0006 onward) - this WORK only removes the blocker; resuming that work is `session-runtime-v1`'s own Project's decision.
- Any change to `sessions`, `session_actors`, `session_participants`, `join_codes`, `session_requests`, `session_runtime_turns`, `session_runtime_steps`, or `session_runtime_state` - unaffected.

## Approved Design

**Migration approach: replace outright, not deprecate-alongside.** `session_interactions.engine_path`/`engine_slot` are dropped and replaced with a single `engine_interaction_id BIGINT NOT NULL` column, via new migration files that drop the two old columns and add the new one (existing historical migration files untouched) - the same "drop-then-recreate as new migrations" approach `session-runtime-v1`'s WORK-0001 already established and executed for its own pre-launch schema replacement, under the identical assumption recorded there: this project is pre-launch, no production deployment/data-preservation requirement exists for this schema. If that assumption is wrong by the time this WORK is implemented, this WORK must return to DRAFT before implementation proceeds, exactly as WORK-0001's own precedent states. `kind` stays as a column (still a useful read-model/API-facing label for "what kind of interaction is this") but is now populated directly from `Output.Kind` at capture time - `resolveInteractionKind`'s `Program`-lookup classification is removed entirely, not merely bypassed.

**Capture.** `captureInteractions` reads `o.InteractionID`/`o.Kind` directly off `engine.OpenQuestionOutput`/`OpenKeyedQuestionOutput` (translating `engine.InteractionKind` to the existing `session.InteractionKindQuestion`/`InteractionKindAskGroup` string constants - unchanged) and passes `InteractionID` to `CreateInteraction` in place of `enginePath`/`engineSlot`. `CloseActiveInteraction`'s matching predicate becomes `engine_interaction_id = ?` (a unique key on its own - no longer needs `session_actor_id` as a co-predicate, though keeping it as a defense-in-depth check is a reasonable local choice).

**Answering and replay.** `AnswerInteraction` and `buildAnswerSignal` both construct `engine.Signal{Kind: engine.SignalKindInteractionAnswered, InteractionID: interaction.EngineInteractionID, Respondent: ..., Answer: ...}` directly from the persisted `engine_interaction_id` column - no `Kind`-branching is needed to pick a `SignalKind` any more, since WORK-0026's engine resolves that internally from the ID alone. `answerSignalKind` is removed.

**Replay-determinism.** `reconstructCurrentSnapshot`'s replay loop is otherwise unchanged - it still drains one durable signal per historical Turn against the running `Snapshot`; only how that signal is *built* changes. WORK-0026's own `InteractionID` replay-determinism guarantee (Constraints) is what keeps this sound: replaying the same `InteractionID`-addressed signals against the same starting `Snapshot` reproduces identical assignments, so a persisted `engine_interaction_id` read back during replay always addresses the same occurrence it did live.

## Constraints and Invariants

- Must preserve GAME-ADR-0024's replay-first guarantee: replaying a Session's durable causes must reconstruct the exact same `InteractionID`-addressed signal sequence live execution produced.
- Must not reopen or reinterpret any already-DONE Session Runtime WORK's historical scope (WORK-0001-0005/WORK-0019) - this is new work against a new engine contract, not a correction of prior work, matching GAME-ADR-0026's own Consequences text.
- Not implemented, reviewed, or closed independently of WORK-0026 - see WORK-0026's own Human Resolution.
- Per the migration precedent this WORK follows: if a production deployment/data-preservation requirement for `session_interactions` exists by the time this WORK is implemented, this WORK must return to DRAFT before implementation proceeds rather than silently dropping columns.

## Acceptance Criteria

- `session_interactions` is keyed by `engine_interaction_id`; `engine_path`/`engine_slot` no longer exist on that table.
- `resolveInteractionKind` no longer exists anywhere in the repository; `captureInteractions` never reaches into a compiled `Program` to classify an interaction.
- `AnswerInteraction` and replay both construct `engine.Signal{Kind: engine.SignalKindInteractionAnswered, ...}`; no reference to a removed `SignalKind` constant remains in `game/session/...`.
- A round-trip test proves live execution and replay reconstruction agree: answering an interaction live and reconstructing the same Session's `Snapshot` purely from durable state (the existing `reconstructCurrentSnapshot` path) produce identical results, including through at least one multi-turn sequence.
- `go build ./...`, `go vet ./...` pass repository-wide.
- `go test ./game/session/... -count=1` passes, including against real Postgres (`playhoot-postgres-1`).
- `docs/projects/active/session-runtime-v1/PROJECT.md`'s pause note is updated to reflect Phase 1 is unblocked.

## Implementation Freedom

- Exact migration file naming/count, exact SQL predicate shape (e.g., whether `CloseActiveInteraction` keeps `session_actor_id` as a defense-in-depth co-predicate), and other local persistence-code choices are ordinary Codebase Agent autonomy, matching this repository's `repositories.md` engineering standard.

## Verification

- `go build ./...`, `go vet ./...`.
- `go test ./game/session/... -count=1` against real Postgres.
- `go test ./game/language/v1/... -count=1` (confirming WORK-0026's own suite, implemented in the same pass, is unaffected by Session Runtime changes).
- `go test . -run TestNoInternalDocCitationsInComments`.

## Documentation Impact

### Accepted / Canonical Knowledge

- `game/docs/decisions/GAME-ADR-0007-session-runtime-turn-and-persistence-model.md` - gains a follow-up cross-reference note that `session_interactions`' `engine_path`/`engine_slot` identity is superseded by this WORK's `engine_interaction_id`, matching the "Implemented by" cross-reference pattern GAME-ADR-0012/GAME-ADR-0026 already use; the ADR's own historical Decision text is not rewritten (`session_timer_obligations` is explicitly noted as unaffected).
- `game/docs/decisions/GAME-ADR-0026-...md` - "Implemented by" section gains this WORK alongside WORK-0024/0025/0026.

### Current-State Documentation After Implementation

- `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` - `session_interactions`' column list updated to `engine_interaction_id`; the still-open note about the concrete `InteractionID` persistence shape (added by GAME-ADR-0026) is resolved.
- `game/docs/DATA_MODEL.md`, `game/CURRENT_STATE.md`, `game/docs/FLOWS.md` - updated wherever they describe `engine_path`/`engine_slot` as `session_interactions`' identity.
- `docs/projects/active/session-runtime-v1/PROJECT.md` - pause note resolved.

### Intentionally Unchanged

- `session_timer_obligations` and everything Timer-related.
- Every other Session Runtime table.

## Blockers

None. The migration-shape question this Project's own `PROJECT.md` flagged as a Material Decision is resolved above by following this repository's own established pre-launch-schema-replacement precedent (WORK-0001) - confirm at READY approval rather than treated as silently settled.

## Completion Record

DONE (2026-09-24). Implemented together with WORK-0026 as one combined change, reviewed together, one REQUIRED_FIX applied (in WORK-0026's own `engine/commit.go` - nothing in this WORK's own diff needed a fix), then closed - all the same day. See "Implementation Report" below and WORK-0026's own "Independent Review" section for the full review record (shared, since both were reviewed as one combined implementation).

### Implementation Report (2026-09-24)

Work status: IMPLEMENTING

Implemented:
- `interaction_capture.go`: `resolveInteractionKind`/`emptyEnginePath` removed entirely; `captureInteractions` dropped its `compiledProgram` parameter and now reads `Kind`/`InteractionID` directly off `engine.OpenQuestionOutput`/`CloseQuestionOutput`, translating `Kind` via a new `interactionKindLabel` helper.
- `step_answer_interaction.go`: `AnswerInteraction` builds `engine.Signal{Kind: engine.SignalKindInteractionAnswered, InteractionID: ..., Respondent: ..., Answer: ...}` directly from the persisted `engine_interaction_id`; `answerSignalKind` removed.
- `step_start.go`: updated `captureInteractions` call site (dropped the now-removed `compiledProgram` argument).
- `replay.go`: `buildAnswerSignal`/`loadReplaySignal` reworked the same way; no longer calls a `Kind`-to-`SignalKind` mapping function.
- `internal/repo/interaction.go`: `Interaction`/`interactionInsert` migrated from `EnginePath []byte`/`EngineSlot string` to `EngineInteractionID uint64`; `CreateInteraction`/`CloseActiveInteraction`/`FindInteractionByUUID`/`GetInteractionByID` SQL updated to the new column.
- New migration `game/session/internal/storage/migrations/20260924000000_session_interactions_engine_interaction_id.go`: drops `engine_path`/`engine_slot` and the old active-slot unique index, adds `engine_interaction_id BIGINT NOT NULL`, and a new unique index `session_interactions_active_interaction_key` on `(session_id, engine_interaction_id) WHERE state='ACTIVE'`, with a full rollback. Registered in `migration.go`. Follows the drop-then-recreate precedent at `20260908000000_drop_legacy_session_schema.go` exactly, per this WORK's own Approved Design. `session_timer_obligations` untouched.
- Tests: `interaction_capture_test.go` rewritten (dropped the Program-lookup fixture entirely, fakes now use `engineInteractionID uint64`); `replay_fixture_test.go` updated to use `SignalKindInteractionAnswered`/`InteractionID`; `replay_integration_test.go`/`step_answer_interaction_integration_test.go`/`step_start_integration_test.go` updated (creation-order queries replacing `engine_slot`-keyed lookups, since that column no longer exists); `mocks_test.go` regenerated.
- Documentation: `game/docs/decisions/GAME-ADR-0007-...md` gained an "Implemented by" cross-reference section (historical Decision text unedited); `GAME-ADR-0026-...md`'s "Implemented by" section extended; `SESSION_RUNTIME_PERSISTENCE_MODEL.md`/`DATA_MODEL.md`/`FLOWS.md` updated wherever they described `engine_path`/`engine_slot` as `session_interactions`' identity; `docs/projects/active/session-runtime-v1/PROJECT.md`'s pause note resolved (its original "Paused" reasoning preserved as historical record, followed by a new "Unblocked" section). `game/CURRENT_STATE.md` needed no change - it never described `engine_path`/`engine_slot` as this identity.

Local implementation decisions:
- One migration file, not several.
- `CloseActiveInteraction` keeps `session_actor_id` as a defense-in-depth co-predicate alongside `engine_interaction_id = ?`, per this WORK's own suggestion.
- The repo-layer `EngineInteractionID` is a plain `uint64`, not an `engine.InteractionID`-typed field - keeps `internal/repo` free of an `engine` package dependency, consistent with its existing style; call sites in `sessionlifecycle` do the `uint64(...)`/`engine.InteractionID(...)` conversions.

Deviations from the approved WORK:
- None.

Discoveries:
- None requiring escalation. `go test ./game/language/v1/...` was transiently failing to compile early in this implementation pass, since WORK-0026's own test-file fixes were still in progress concurrently in the same repository checkout - resolved on its own once that concurrent pass completed; no session-side action was needed.

Verification performed:
- `go build ./...`, `go vet ./...` - clean, repository-wide.
- `go test . -run TestNoInternalDocCitationsInComments` - passes (2 citation violations in the new migration file/its registration comment were found and fixed during this pass).
- `go test ./game/language/v1/... -count=1` - passes, unmodified by this WORK.
- `go test ./game/session/... -count=1` against real Postgres (`playhoot-postgres-1`) - all packages pass, including `TestReconstructCurrentSnapshot_Integration` (the existing 3-turn live-vs-replay round-trip test, adapted to the new signal/column shape rather than rewritten - it already satisfied the "prove live execution and replay reconstruction agree ... through at least one multi-turn sequence" Acceptance Criterion, so no new test was needed).
- `go test ./... -count=1` against real Postgres - only the already-known, pre-existing, out-of-scope `getgame` JSONB-whitespace defect fails.

Documentation synchronized: see "Implemented" above for the full list.

Known limitations:
- None beyond what Approved Design already scoped out (`session_timer_obligations`, keyed-slot Session Runtime support, constructing the Ask Group completed-awaiting-join signal).

Ready for independent review:
YES (together with WORK-0026, per that WORK's own Human Resolution - both close together).

### Independent Review (2026-09-24)

Reviewed together with WORK-0026 as one combined implementation - see WORK-0026's own "Independent Review" section for the full record (verdict, verification performed, and the one REQUIRED_FIX, which landed entirely in `engine/commit.go`, not in any file this WORK owns). Nothing in this WORK's own diff (`interaction_capture.go`, `step_answer_interaction.go`, `replay.go`, `internal/repo/interaction.go`, the new migration, or its documentation updates) required a fix - the reviewer explicitly confirmed the migration is a genuine drop-then-recreate, `resolveInteractionKind` is fully removed repo-wide, and `game/docs/decisions/GAME-ADR-0007-...md`/`SESSION_RUNTIME_PERSISTENCE_MODEL.md`/`DATA_MODEL.md`/`FLOWS.md`/`session-runtime-v1/PROJECT.md`'s pause-note resolution (including the concurrent-edit collision noted in this Project's `AI_CONTEXT.md`) are all synchronized and internally coherent.

No unresolved REQUIRED_FIX or DECISION_REQUIRED finding remains for either WORK. Closed to DONE.
