# WORK-0029: Session-Owned Output/Value Schema (Decouple `game/session` From `engine`)

Status: DONE
Created: 2026-09-24
Last status change: 2026-09-25 (IMPLEMENTING -> DONE: independent review APPROVED, two NON_BLOCKING findings both fixed same-session (a misleadingly named test, two assertions loosened to `gomock.Any()` where the exact actor-id set could be pinned) - see "Completion Record" below. Prior update, 2026-09-24: READY -> IMPLEMENTING, implemented per Approved Design - see "Implementation Report" below. Prior update, same day: DRAFT -> READY, human explicitly authorized this WORK's Approved Design for implementation, per the READY Review in `internal/HUMAN_REVIEW.md`. Prior update, same day: PLANNED -> DRAFT, human supplied direct design input on this WORK's own open Scope questions - see "Design Decisions (2026-09-24, Human-Directed)" below. Prior update, same day: human-directed resequencing, this WORK made the immediate next WORK to pick up in this Project. Prior update, same day: created, human-directed - see Outcome)

Related decisions:
- None yet. May need a lightweight ADR/standard once the design questions below are actually resolved, if the resulting rule is judged reusable beyond this one boundary.

Canonical context:
- `docs/projects/active/session-runtime-v1/works/WORK-0006-broaden-live-fanout-effects-presentations.md` (introduced `StartResult`/`AnswerInteractionResult.Outputs []engine.Output`, the surface this WORK must replace)
- `game/language/v1/engine/output.go` (`EmitEffectOutput`/`ActivatePresentationOutput`/`UpdatePresentationOutput`/`RemovePresentationOutput` - the four kinds `Outputs` currently carries)
- `game/language/v1/engine/value.go` (`Value`'s 11 variants - `Unit`/`Bool`/`Number`/`String`/`User`/`Enum`/`Record`/`Union`/`NewType`/`Optional`/`List`/`Map`, several recursive, each referencing `engine`'s own separate `Type` algebra) - the payload type this WORK must give `game/session` its own equivalent of
- `docs/projects/active/session-runtime-v1/works/WORK-0020-role-aware-live-connections.md` (the first real consumer of `Outputs` - the play Coordinator this WORK's schema must actually serve)

## Outcome

No exported symbol of `game/session` (directly, or through `sessionlifecycle`) requires a caller to import `game/language/v1/engine` or `game/language/v1/program`. Today it does: `StartResult`/`AnswerInteractionResult.Outputs []engine.Output` returns engine-owned interface types directly, each carrying `engine.Value`-typed Model/Arguments data and `engine.UserID`-typed (internal session-actor-derived) recipients. `game/session` must instead expose its own schema - mapped from the engine's own Output/Value/recipient data, at the boundary, into types designed for what a consumer (eventually a live Coordinator, eventually a UI) actually needs, not for what the engine's own execution model happens to produce.

This was raised as a live architecture concern (2026-09-24, human-directed) while implementing WORK-0006/0007: `AnswerInteraction`'s own input side was already fixed in that same session (`engine.Value` parameter replaced with the already-encoded wire payload, `json.RawMessage`, decoded internally) - this WORK is the output side, and is deliberately **not** implemented immediately, for a concrete reason: no consumer of `Outputs` exists yet (`play`/the Coordinator was deleted, WORK-0020 hasn't rebuilt it). Designing the full mapping now risks guessing the wrong shape and redoing it once a real consumer (WORK-0020 onward) reveals the actual wire/UI contract needed. This WORK exists so that requirement is tracked (per `docs/projects/README.md` Invariant 1 - a known-required future outcome must have a WORK, even PLANNED) rather than left only as a comment in this conversation.

## Context

**Why this is more than a type rename.** `engine.UserID` on `EmitEffectOutput.Recipients`/`ActivatePresentationOutput.Recipient`/etc. is the internal `session_actors.id`, not a real user identity - `Manager` has never exposed that identifier to any caller. Mapping it to something a consumer can actually use (`UserUUID`, the identity `Manager` already exposes everywhere else) needs a new repository lookup (actor id -> `session_actors.user_uuid`), not just a wrapper type.

**Why the Value payload cannot simply become `any`/raw bytes.** `engine.Value`'s type-kind information (this is a Number vs. a Record with named fields of specific types, etc.) is not an internal engine implementation detail - it is the actual schema a UI needs to know how to render/act on. Erasing it into an untyped blob would just push the same problem onto every consumer as undocumented convention (a worse, non-compile-checked coupling). `game/session` needs its own explicit mirror of this schema - not `engine`'s own Go types, but a session-owned equivalent covering the same logical shapes - so a consumer's compile-time contract comes from `session`, never from `engine` directly.

**Why this is not scoped down to "just the primitives used so far".** A quick audit (2026-09-24, during this same discussion) found the engine's `Value`/`Type` algebra has 11 variants with no depth/composition limit, but only one real Definition exists in the repository (Parqués), using a small subset (`enum`, flat `record`, `list<record>`) - no `map`/`optional`/`union`/`new_type` anywhere in any authored or example content today. That is weaker evidence than GAME-ADR-0026's own nested-workflow removal (which was backed by an exhaustive audit across the full committed target-game range finding zero use, not "the one simple example doesn't need it") - `Map` in particular is plausibly load-bearing for per-player/per-object state in other committed-range games (UNO's discard pile, Poker's hand/fold state) that just haven't been authored yet. This WORK does not decide to narrow the engine's Value/Type surface based on that weaker evidence; see the separate Engineering Radar item recording the question of whether the engine's Value/Type generality itself deserves its own GAME-ADR-0026-style audit, independent of this WORK.

## Design Decisions (2026-09-24, Human-Directed)

Resolved directly by the human, in the same session that moved this WORK PLANNED -> DRAFT, after a codebase investigation grounded the discussion in what `game/language/v1/engine/output.go`/`value.go` and `game/session/workflows/sessionlifecycle` actually do today (not assumed):

1. **Coverage: mirror all 11 `Value` variants, not just what Parqués exercises today.** The investigation confirmed the premise that prompted this question - "only interaction values need to cross the boundary" - is false: `ActivatePresentationOutput`/`UpdatePresentationOutput.Model` (the rendered presentation state, e.g. a board or a hand) and `EmitEffectOutput.Arguments` already cross the boundary today (added by WORK-0006), not only `OpenQuestionOutput.Arguments`. This is the exact question `docs/engineering/ENGINEERING_RADAR.md`'s "Game Language `Value`/`Type` Algebra May Be More General Than The Target Game Range Needs" item names as its own reevaluation trigger ("...or WORK-0029 is drafted for real and needs to decide its own schema's coverage"). Per that item's own recommendation, narrowing `engine`'s algebra itself would need the same kind of exhaustive target-game-range audit GAME-ADR-0026 performed for nested workflows, which does not exist yet for `Value`/`Type` - so this WORK does not narrow `engine`, and does not narrow its own mirror either, on the weaker evidence of one Definition (Parqués).
2. **Shape: a closed interface, one concrete exported type per variant** (`sessionlifecycle`'s own `Value` interface with an unexported marker method + `Kind()`, and `UnitValue`/`BoolValue`/`NumberValue`/`StringValue`/`UserValue`/`EnumValue`/`RecordValue`/`UnionValue`/`NewTypeValue`/`OptionalValue`/`ListValue`/`MapValue` structurally mirroring `engine.Value`'s own pattern) - not a single tagged/discriminated struct. This keeps compile-time exhaustiveness for consumers (the same property `engine.Value`'s own closed-interface design deliberately has), at the cost of being 99% structurally similar to `engine.Value` in practice. That similarity is accepted deliberately: the type family is still fully own by `sessionlifecycle` (own interface, own marker method, no `engine` import in the exported surface), so a future Game Language v2 that changes `engine`'s own internal representation only requires updating this WORK's translation function, not every consumer of `sessionlifecycle`'s public API - the goal is ownership of the exported contract, not artificial structural divergence from `engine.Value` for its own sake.
3. **Location: a new `outputs.go` file inside the `sessionlifecycle` package itself, not a new subpackage.** The session-owned `Value`/`Output` types and their mapping from `engine.Output`/`engine.Value` are declared together in `game/session/workflows/sessionlifecycle/outputs.go`, and the mapping is implemented as methods on `*Manager` (e.g. `(m *Manager) mapOutputs(...)`) so it can use `Manager`'s own existing repository access to resolve `Recipient`/`Recipients` (today `engine.UserID`, the internal `session_actors.id` as a decimal string - see `step_start.go`'s `engine.UserID(strconv.FormatUint(uint64(p.ActorID), 10))`) into real `UserUUID` values. This is a deliberate, human-approved exception to this codebase's usual small-internal-package-per-concern convention (`internal/clientoutputs`, `internal/completion`, `internal/expiration`, `internal/interactions`, `internal/replay`) - justified specifically because the mapping needs `Manager`'s repository access, and introducing a new collaborator/interface purely to preserve that convention would add ceremony without benefit for logic this cohesive. `internal/clientoutputs` is unchanged - it keeps filtering `[]engine.Output` down to the client-facing subset; `outputs.go` maps that already-filtered subset into the new exported schema.

Two lower-stakes Scope items from the original list remain deliberately deferred rather than decided now (non-blocking for DRAFT, not required before implementation starts):

- **Versioning/stability of the exported schema** - deferred until a real consumer (WORK-0020 onward) actually depends on it, per this WORK's own original timing rationale (Outcome section above).
- **Whether this implies a reusable engineering standard** ("domain packages must not export `game/language/v1/...` types in their own public API") - not promoted to a standard from a single instance, per `AGENTS.md`'s "do not infer new canonical standards from recurring implementation patterns"; left as a candidate to revisit if/when a second domain package needs the same rule.

## Scope

### In Scope

- A new `sessionlifecycle.Value` closed interface (own marker method, own `Kind` type) with one concrete exported type per engine `Value` variant (all 11), structurally mirroring `engine.Value` per Design Decision 2 above.
- A new `sessionlifecycle.Output` closed interface with one concrete exported type per the four kinds `Outputs` currently carries (`EmitEffectOutput`, `ActivatePresentationOutput`, `UpdatePresentationOutput`, `RemovePresentationOutput` - see `game/language/v1/engine/output.go`); naming of the session-owned types is Implementation Freedom.
- Mapping code (`game/session/workflows/sessionlifecycle/outputs.go`, methods on `*Manager`) translating `engine.Output`/`engine.Value` into the new types, including `Recipient`/`Recipients` resolution from the internal actor id to `UserUUID`.
- A new repository method resolving one or more internal actor ids to their `UserUUID`s within the same transaction a Turn committed in (batch, not N+1).
- Changing `StartResult`/`AnswerInteractionResult.Outputs`'s field type from `[]engine.Output` to `[]Output` (the new session-owned type).

### Out of Scope

- Any change to `engine.Value`/`engine.Output`/`engine.Type` themselves - this WORK does not narrow or otherwise redesign the engine's own algebra (see Design Decision 1).
- `OpenQuestionOutput`/`CloseQuestionOutput` (and their keyed variants) and `RunCompletedOutput` - not part of `Outputs`/`ClientFacing` today (questions are durably captured via `internal/interactions`; `RunCompletedOutput` is consumed internally by WORK-0007's termination logic), so they are not mapped by this WORK.
- Any client-delivery/wire/transport code - this WORK only changes what `sessionlifecycle.Manager` returns in memory to its own caller; fan-out to a live connection remains a not-yet-drafted play-half WORK following WORK-0020, per this Project's existing phasing.
- Versioning/stability guarantees on the new exported schema, and whether this becomes a formal engineering standard - both deliberately deferred (see Design Decisions above).

## Approved Design

`sessionlifecycle` (the package `Manager`/`StartResult`/`AnswerInteractionResult` already live in, and the actual exported surface consumers import) gains:

1. **`Value`** - a closed interface (unexported marker method, `Kind() Kind` where `Kind` is a new `sessionlifecycle`-owned enum, not `engine.Kind`) with one concrete struct per variant, covering all 11 of `engine.Value`'s variants (`Unit`/`Bool`/`Number`/`String`/`User`/`Enum`/`Record`/`Union`/`NewType`/`Optional`/`List`/`Map`). `UserValue` carries a `UserUUID` (real identity), not `engine.UserID`. Named-type variants (`Enum`/`Record`/`Union`/`NewType`) keep their `TypeName` the same way `engine`'s own do; element/key/value "type" information on `Optional`/`List`/`Map` is represented at whatever minimal fidelity a UI actually needs (at minimum, the element/key/value `Kind`) - the exact shape of that piece is Implementation Freedom, not a material decision this WORK needs to fix in advance.
2. **`Output`** - a closed interface with one concrete type per the four kinds `internal/clientoutputs.ClientFacing` already selects today (effect emitted; presentation activated/updated/removed), each carrying `Value`-typed model/argument data and `UserUUID`-typed recipient(s) instead of `engine.UserID`.
3. **`outputs.go`** - declares both of the above and the mapping from `[]engine.Output` (already filtered by `internal/clientoutputs.ClientFacing`, unchanged) into `[]Output`, as methods on `*Manager` so recipient resolution can use `Manager`'s own repository access. `Start`/`AnswerInteraction` call this mapping before returning `StartResult`/`AnswerInteractionResult`, in place of returning `engine.Output` directly.
4. A new narrow repository method (on the existing `internal/repo` package, alongside `FindActor`) resolving a batch of internal actor ids to their `Actor` rows (carrying `UserUUID`) in one query, within the same transaction the Turn committed in.

This is a translation/ownership change only: which Output kinds are returned, in what order, and under what conditions is unchanged from WORK-0006/WORK-0007's existing behavior - only the Go types crossing `sessionlifecycle`'s exported boundary change.

## Constraints and Invariants

- `game/session`'s own exported API (types, function signatures), directly or through `sessionlifecycle`, must not require a caller to import `game/language/v1/engine` or `game/language/v1/program`. `sessionlifecycle` may still import `engine`/`program` internally (it already does, throughout) - only the exported surface is constrained.
- No change to `AnswerInteraction`'s already-fixed input side (`json.RawMessage`, decoded internally via `engineservice.DecodeValue`).
- The new actor-id -> `UserUUID` repository lookup must batch-resolve every recipient a Turn's Outputs reference in one query, not one query per Output/recipient.
- `internal/clientoutputs.ClientFacing`'s existing filtering behavior (which Output kinds, in what relative order) is unchanged; this WORK only adds a translation step after it.

## Acceptance Criteria

- `go list -deps ./game/session/...`'s exported package surface contains no `game/language/v1/engine`/`game/language/v1/program` type in any exported function signature or exported struct field.
- `StartResult.Outputs`/`AnswerInteractionResult.Outputs` are typed `[]Output` (the new session-owned type), not `[]engine.Output`.
- Every one of `engine.Value`'s 11 variants has a corresponding `sessionlifecycle.Value` variant, and the mapping function is exhaustive (compiler-enforced via the closed-interface pattern, the same technique `engine` itself already uses).
- `Recipient`/`Recipients` on every mapped `Output` resolve to real `UserUUID` values, proven by a repository-integration test covering multiple recipients produced by one Turn.
- Existing WORK-0006/WORK-0007 behavior (which Output kinds are returned, in what order, under what conditions) is preserved exactly - existing tests are updated only for the new return-type shape, not for new/changed logic.

## Implementation Freedom

- Exact naming of the session-owned `Output`/`Value` concrete types and the `Kind` enum's values.
- Exact representation of `Optional`/`List`/`Map`'s element/key/value type information (full parallel `Type`-like structure vs. bare `Kind`), as long as it is enough for a consumer to know how to render/act on the value.
- Exact repository method name/signature for the batch actor-id -> `UserUUID` lookup, following this codebase's existing `internal/repo` naming conventions (see `docs/engineering/standards/repositories.md`).
- Whether the mapping is one method per Output kind or one dispatching method plus per-kind helpers, inside `outputs.go`.

## Verification

- `go build ./...`, `go vet ./...`, `gofmt -l` (changed files) clean.
- `go test ./... -count=1`: existing `sessionlifecycle` mocked-collaborator and integration tests updated and passing against the new `Outputs` shape.
- A new unit test exercising the `engine.Value` -> `sessionlifecycle.Value` mapping for at least one instance of each of the 11 variants, hand-assembled the same way `engineservice`'s own tests build fixtures (no real Postgres needed for this part).
- A new repository-integration test (real Postgres) proving the batch actor-id -> `UserUUID` resolution is correct for multiple recipients in one Turn.

## Documentation Impact

### Accepted / Canonical Knowledge

- None - this WORK does not change `engine`'s own algebra or any accepted architecture/domain decision. If the deferred "reusable engineering standard" question (Design Decisions above) is later promoted, that would be recorded separately, through the Engineering Standard process, not here.

### Current-State Documentation After Implementation

- `game/CURRENT_STATE.md`, `game/docs/FLOWS.md` - describe `sessionlifecycle`'s own `Output`/`Value` schema (`outputs.go`) in place of the current `engine.Output`/`engine.Value` description of what `StartResult`/`AnswerInteractionResult.Outputs` carries.

## Blockers

1. ~~Timing dependency: deliberately not drafted for real until a real consumer of `Outputs` exists (WORK-0020 onward).~~ **Overridden by explicit human direction (2026-09-24)** - resolved by drafting this WORK now.
2. ~~Exact session-owned `Value`/`Output` schema shape, coverage, and mapping location.~~ **Resolved by explicit human design input (2026-09-24)** - see "Design Decisions" above.
3. ~~READY authorization pending.~~ **Resolved (2026-09-24)** - human explicitly authorized DRAFT -> READY.

## Implementation Report

Not yet DONE - IMPLEMENTING (2026-09-24). Independent review has not run.

Implemented:
- `game/session/workflows/sessionlifecycle/outputs.go`: `Value`/`Type`/`Output` closed interfaces (one concrete type per variant/kind, per Design Decisions above) and `(*Manager).mapOutputs`, translating `engine.Output`/`engine.Value` into them, including recursive `UserValue` resolution (top-level Recipients/Recipient and any nested inside a Model/Arguments tree) via one batched repository lookup.
- `internal/repo.FindActorsByIDs` (`actor.go`): new batch actor-id -> `Actor` (carrying `UserUUID`) lookup, scoped to `(sessionID, ids)`.
- `Manager` gained an `outputsRepo outputsRepoAPI` field (own narrow interface, mirroring the existing per-step pattern), wired in `New`.
- `StartResult`/`AnswerInteractionResult.Outputs` changed from `[]engine.Output` to `[]Output`; `step_start.go`/`step_answer_interaction.go` now call `m.mapOutputs` after `clientoutputs.ClientFacing` instead of returning its result directly.

Local implementation decisions:
- `mapOutputs`/`mapValue`/`mapType` return an error for an unrecognized `engine.Output`/`Value`/`Type` variant rather than panicking, consistent with `internal/clientoutputs`'s own defensive style.
- `mapOutputs` builds its result with `var mapped []Output` (not `make([]Output, 0, ...)`) so it stays `nil` for no outputs, matching `internal/clientoutputs.ClientFacing`'s own nil-for-empty convention - `make` would have returned a non-nil empty slice, silently breaking the "Outputs shape preserved exactly" Acceptance Criterion for a same-token replay (an idempotency replay decodes stored JSON, where `Outputs` - `json:"-"` - always comes back `nil`; caught by the existing `retried_start_with_same_idempotency_key_replays_started` integration test).

Deviations from the approved WORK:
- None.

Discoveries:
- None.

Verification performed:
- `go build ./...`, `go vet ./...` - clean, repository-wide.
- `gofmt -l` - clean on every touched/new file (a large unrelated set of pre-existing files across the repository report as needing gofmt purely from Windows `core.autocrlf` CRLF checkout line endings, not a real formatting difference - confirmed via `gofmt -d` producing a whole-file diff on an untouched file; not this WORK's concern).
- `go test ./... -count=1` against real Postgres (this sandbox has a reachable `playhoot-postgres-1` container) - every test passes except `TestRepoGetGameCurrentVersion` in `game/management/usecases/getgame`, a pre-existing, already-documented, out-of-scope JSONB-canonicalization test defect untouched by this WORK (unrelated package, not `game/session`).
- New tests: `outputs_test.go`'s `TestManagerMapOutputs` (mocked-collaborator; covers all 11 `Value` variants including nested/top-level recipient resolution, effect recipients/arguments, nil-for-no-outputs, an unresolvable recipient, and a propagated repository error); `internal/repo/actor_test.go`'s `TestRepoFindActorsByIDs` (real-Postgres; batch resolution, session-scoping, empty-ids). Existing `step_start_integration_test.go`/`step_answer_interaction_integration_test.go` cases updated to assert against the new session-owned types and pass against real Postgres.

Documentation synchronized:
- `game/CURRENT_STATE.md` (Session Runtime row and Evidence bullet) and `game/docs/FLOWS.md` (Start/AnswerInteraction sequence diagrams, prose, and Evidence bullets) - both now describe `outputs.go`'s session-owned `Output`/`Value` schema and `mapOutputs`'s recipient resolution in place of returning `engine.Output` directly.

Known limitations:
- None beyond the above.

Ready for independent review:
YES

## Completion Record

DONE (2026-09-25).

Implementation summary: see "Implementation Report" above for the full detail - `outputs.go`'s session-owned `Value`/`Type`/`Output` schema and `(*Manager).mapOutputs`, `internal/repo.FindActorsByIDs`, and the `StartResult`/`AnswerInteractionResult.Outputs` type change.

Independent review: APPROVED (2026-09-25, fresh reviewer session, read-only). Findings: two NON_BLOCKING (a misleadingly named `outputs_test.go` subtest claiming to avoid a repository call it actually makes with an empty id list; two subtests asserting `gomock.Any()` for the resolved actor-id set instead of pinning it exactly). Both fixed same-session, no re-review required for NON_BLOCKING findings: the subtest was renamed to "returns nil for no outputs", and the "every Value variant"/"effect recipients" subtests now assert the exact expected id set via `gomock.InAnyOrder`. No REQUIRED_FIX or DECISION_REQUIRED finding was raised.

Verification actually performed: `go build ./...`/`go vet ./...` clean; `gofmt -l` clean on every touched/new file (a broad set of unrelated pre-existing files across the repository also report under `gofmt -l` purely from this Windows checkout's `core.autocrlf`-introduced CRLF line endings, confirmed via `gofmt -d` producing a whole-file diff on an untouched file - not a real formatting difference, not this WORK's concern); `go test ./... -count=1` against a real reachable Postgres instance - every test passes except the pre-existing, already-documented, out-of-scope `TestRepoGetGameCurrentVersion` in `game/management/usecases/getgame` (a JSONB-canonicalization whitespace mismatch, unrelated package, untouched by this WORK).

Required documentation synchronized: `game/CURRENT_STATE.md` (Session Runtime row and Evidence bullet) and `game/docs/FLOWS.md` (Start/AnswerInteraction sequence diagrams, prose, Evidence bullets) - independent review confirmed both accurately describe the implementation, no over/underclaim.

Approved deviations: none.

Follow-up explicitly out of scope, left for later: schema versioning/stability guarantees, and whether "domain packages must not export `game/language/v1/...` types" becomes a formal engineering standard - both deferred per this WORK's own "Design Decisions" section, not decided here. Client-delivery/wire/transport consumption of `Outputs` remains owned by the not-yet-drafted play-half WORK following WORK-0020, unchanged by this WORK.
