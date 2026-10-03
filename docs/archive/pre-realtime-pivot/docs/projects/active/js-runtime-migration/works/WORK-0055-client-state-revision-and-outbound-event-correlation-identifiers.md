# WORK-0055: ClientState Revision & Outbound Event Correlation Identifiers

Status: DONE
Created: 2026-09-29
Last status change: 2026-09-29 (independent review returned APPROVED with no findings, one documentation correction applied opportunistically; closed)

Related decisions:
- `docs/decisions/architecture/ADR-0015-javascript-rule-execution-and-iframe-frontend-contract.md`

Canonical context:
- `docs/projects/active/js-runtime-migration/works/WORK-0045-frontend-iframe-delivery-contract-specification.md` (this WORK's own origin - its `playhoot.onState`/`playhoot.onEvent` contract requires a `revision` on every delivered view and an `id`/`revision` on every delivered transient event, discovered missing during that WORK's own human review)
- `docs/projects/active/js-runtime-migration/works/WORK-0041-per-player-view-computation-and-privacy-verification.md` (owns `session.GetClientStateResult`, DONE - this WORK adds a field to it, does not reopen its own approved scope)
- `docs/projects/active/js-runtime-migration/works/WORK-0042-effects-delivery-best-effort.md` (owns `session.OutboundEvent`, DONE - same relationship)

## Outcome

`WORK-0045`'s human-confirmed frontend contract requires two pieces of metadata neither `session.GetClientStateResult` nor `session.OutboundEvent` currently carries: a revision/sequence number on a delivered view (so a frontend can safely discard a delivery no newer than the last one it already applied), and a stable correlation identifier plus its own revision on a delivered transient event (so a frontend can recognize an already-applied duplicate, given `SEND_EVENT`'s own best-effort, may-repeat delivery semantics per `SESSION-ADR-0019`). Without these two additions, `WORK-0045`'s own contract cannot actually be satisfied by the backend once a live-transport layer (`session-runtime-v1`'s `WORK-0020`, this Project's `WORK-0046`) starts delivering these values to a real client.

This WORK adds exactly those two additions to already-DONE, already-shipped capabilities - it does not reopen `WORK-0041`'s or `WORK-0042`'s own approved scope/design, both of which remain historically accurate and correct for what they set out to do; this is new, additive WORK building on top of them, per this repository's Completed Work Immutability convention.

## Context

The natural backing data for both additions already exists durably and is already read internally by the relevant call sites, just not exposed:

- **View revision**: `GetClientState` (`session/workflows/sessionlifecycle/client_state.go`) already loads `currentTurn` (`*internalrepo.RuntimeTurn`, which carries a `Sequence`) to read `currentTurn.NewState` - the same value's own `Sequence` is the natural revision number, simply not yet returned on `GetClientStateResult`.
- **Event correlation identifier/revision**: every `SEND_EVENT` command is collected (`collectOutboundEvents`, `WORK-0042`) from a specific RUNNING-phase call that itself committed (or replayed) one specific RuntimeTurn - that Turn's own `Sequence` is the natural `revision`, and a stable `id` is derivable deterministically from `(sequence, index-within-turn)` without needing a new global counter or random generator, since a given Turn's own requested-command list has a fixed order.

Neither of these is a binding design yet - this WORK still needs to decide the exact field names/types, whether `id` is a composite value or an opaque encoded string, and whether `collectOutboundEvents`'s own signature needs to grow a `sequence` parameter to compute it (it currently takes only `commands`).

## Scope

**Ground-truth check:** confirmed by reading `session/types.go`, `client_state.go`, and all four RUNNING-phase step files directly. `GetClientState` (`client_state.go`) already loads `currentTurn` (`*internalrepo.RuntimeTurn`, carrying `Sequence uint64`) before calling `project()` - the exact value `revision` needs is already in scope, just not returned. Every RUNNING-phase step already computes the exact `sequence` value its own new RuntimeTurn commits under, as a literal already passed to `CreateRuntimeTurn`: `1` (`step_start.go`), `currentTurn.Sequence+1` (`step_submit_player_event.go`, `step_cancel_session.go`, `step_expire_timer.go`). `collectOutboundEvents` (`outbound_events.go`, `WORK-0042`) currently takes only `commands` - it needs that same sequence value passed in to compute a per-event `id`/`revision`.

### In Scope

- Add `Revision uint64` to `session.GetClientStateResult`, populated from `currentTurn.Sequence` in `GetClientState`.
- Add `ID string` and `Revision uint64` to `session.OutboundEvent`. `ID` is a stable, deterministic identifier derived from `(sequence, index-within-the-Turn's-own-command-list)` - reproducible for the exact same command, never randomly regenerated - and `Revision` is that same `sequence`.
- Change `collectOutboundEvents`'s signature to `collectOutboundEvents(sequence uint64, commands []platform.Command) []session.OutboundEvent`, and update its four call sites to pass the same `sequence` value each site already computes for its own `CreateRuntimeTurn` call.

### Out of Scope

- Any change to when/whether a live-transport layer actually delivers `revision`/`id` to a real client - that remains `session-runtime-v1`'s `WORK-0020` and this Project's `WORK-0046`.
- Any change to `GetClientStateResult`/`OutboundEvent`'s other, already-existing fields.
- Any change to `SubmitPlayerEventOutcome`/replay/idempotency behavior - unaffected by this WORK.

## Approved Design

- `session.GetClientStateResult` gains `Revision uint64 \`json:"revision,omitempty"\`` (additive; `GetClientStateResult` is already a plain JSON-serializable read result, unlike the idempotency-replayed Result types `WORK-0042` touched - no `json:"-"` concern here).
- `session.OutboundEvent` gains `ID string \`json:"id"\`` and `Revision uint64 \`json:"revision"\``, both matching `session/docs/FRONTEND_IFRAME_CONTRACT.md`'s own field names exactly (that document is already human-approved; this WORK is its direct, mechanical backend implementation, not a fresh API-shape decision).
- `collectOutboundEvents(sequence uint64, commands []platform.Command) []session.OutboundEvent` builds `ID` as `fmt.Sprintf("%d:%d", sequence, indexOfThisSendEventWithinCommands)` - stable and reproducible for the same command across a delivery retry, since it depends only on data already fixed at Turn-commit time, never on wall-clock time or a random draw. The exact string encoding is Implementation Freedom (the frontend contract treats `id` as opaque); the determinism/stability property is not.
- All four call sites (`step_start.go`, `step_submit_player_event.go`, `step_cancel_session.go`, `step_expire_timer.go`) pass their own already-computed `sequence` literal (`1`, or `currentTurn.Sequence+1`) into `collectOutboundEvents`.

## Constraints and Invariants

- Must not change `session.GetClientStateResult`/`session.OutboundEvent`'s existing fields - additive only, mirroring how `WORK-0042` itself added `Events` additively to four other Result types.
- Must not introduce a new durable column/table merely to serve this - both `RuntimeTurn.Sequence` values this WORK needs already exist and are already read by the relevant call sites.
- `id` must be stable and reproducible for the exact same underlying command across a delivery retry (so a frontend-side duplicate check actually works) - it must not be regenerated randomly per delivery attempt.

## Acceptance Criteria

- `GetClientState`'s returned `GetClientStateResult.Revision` equals the `Sequence` of the RuntimeTurn `ClientState` was actually computed from.
- A backend script issuing a `SEND_EVENT` from any of the four RUNNING-phase steps results in that call's `Events` entries each carrying a non-empty `ID` and a `Revision` equal to the committed Turn's own `Sequence`.
- Two `SEND_EVENT` commands from the same `execute` call produce two different `ID`s (distinguishable by their own index), both sharing the same `Revision`.
- Calling `collectOutboundEvents` twice with the same `(sequence, commands)` input produces byte-identical `ID`s both times (determinism, not merely uniqueness).
- No new durable column/table (empty migration diff).
- `go build ./...`, `go vet ./...`, `go test ./... -count=1` pass, with no new failure beyond this Project's already-recorded pre-existing, unrelated failures.

## Implementation Freedom

- `ID`'s exact string encoding (this WORK's own proposal, `"{sequence}:{index}"`, is illustrative - any deterministic, stable-per-command encoding satisfies the contract).
- Exact placement/naming of any small helper introduced to build `ID`.

## Verification

- Unit tests for `collectOutboundEvents` (per-event `ID` uniqueness within one call, determinism across repeated calls with the same input, `Revision` propagation).
- Extend the existing `TestManagerStart_Integration`/`TestManagerSubmitPlayerEvent_Integration`/`TestManagerCancelSession_Integration`/`TestManagerExpireTimer_Integration` SEND_EVENT-related test cases (`WORK-0042`) to also assert `ID`/`Revision` are populated and correct, rather than adding a wholly new integration suite.
- Unit test for `GetClientState` asserting `Revision` matches the loaded Turn's `Sequence` (extending `client_state_test.go`).

## Documentation Impact

### Accepted / Canonical Knowledge

- `session/docs/FRONTEND_IFRAME_CONTRACT.md` (once `WORK-0045` creates it) - no rewrite expected, since that document already specifies `revision`/`id` as required fields; this WORK is what makes the backend side of that requirement true.

## Blockers

None.

## Completion Record

### Implementation Report (2026-09-29)

Work: `docs/projects/active/js-runtime-migration/works/WORK-0055-client-state-revision-and-outbound-event-correlation-identifiers.md`

Work status: IMPLEMENTING

Implemented:
- `session/types.go`: additive `Revision uint64` (`json:"revision,omitempty"`) on `GetClientStateResult`; additive `ID string` (`json:"id"`) and `Revision uint64` (`json:"revision"`) on `OutboundEvent`.
- `session/workflows/sessionlifecycle/client_state.go`: `GetClientState` now populates `Revision` from the already-loaded `currentTurn.Sequence`.
- `session/workflows/sessionlifecycle/outbound_events.go`: `collectOutboundEvents` signature changed to `(sequence uint64, commands []platform.Command) []session.OutboundEvent`; each produced `OutboundEvent` gets `Revision: sequence` and a deterministic `ID` (`"{sequence}:{index-in-commands}"`, via `fmt.Sprintf`).
- All four call sites (`step_start.go`, `step_submit_player_event.go`, `step_cancel_session.go`, `step_expire_timer.go`) updated to pass the same `sequence` literal each already computes for its own `CreateRuntimeTurn` call (`1` for Start's first Turn; `currentTurn.Sequence+1` for the other three).

Local implementation decisions:
- `ID` encoding (`"{sequence}:{index}"` via `fmt.Sprintf`) - Implementation Freedom per the WORK's own text; the frontend contract treats `id` as opaque.

Deviations from the approved WORK:
- None.

Discoveries:
- None. A self-caught engineering-standard violation, not a design discovery: the first pass of `OutboundEvent`'s own doc comment cited an internal doc path (`session/docs/decisions/SESSION-ADR-0019`) directly, tripping `TestNoInternalDocCitationsInComments`. Fixed by restating the best-effort reasoning inline instead of citing the path, before considering this WORK complete.

Verification performed:
- `go build ./...`, `go vet ./...` - clean, repository-wide.
- `gofmt` diff-checked (CRLF-normalized) on every changed file - no real formatting defect, consistent with this repository's already-documented Windows-checkout CRLF finding.
- New/extended unit tests: `TestCollectOutboundEvents` (three new assertions - per-event id uniqueness within one call, revision propagation, and byte-identical determinism across two calls with the same input) and `TestManagerGetClientState`'s `computed_from_filtered_projection_input` case (asserts `Revision` matches the loaded Turn's own `Sequence`).
- Extended existing real-Postgres integration tests (`WORK-0042`'s own SEND_EVENT cases) in all four RUNNING-phase test files to assert the correct `ID`/`Revision` values rather than adding a new suite.
- `go test ./... -count=1` against real Postgres, run twice: clean except the same two confirmed pre-existing, unrelated failures this Project's history already documents repeatedly (`TestNoInternalDocCitationsInComments` against the untouched migration file; `TestManagerJoin_Integration_ConcurrentOperationRacingLobbyExpiration`, the pre-existing `session-runtime-v1`-owned Join/Leave race - confirmed pre-existing/unrelated via `git diff` showing its own file untouched by this WORK. Its exact flake ratio is environment-dependent, not a fixed constant: this session saw 2 failures/1 pass across three isolated runs, while the independent reviewer saw 4/4 failures in its own environment - both consistent with "genuinely flaky/racy," neither is the authoritative ratio).

Documentation synchronized:
- None required beyond `session/types.go`'s own doc comments (part of the implementation itself). `session/docs/FRONTEND_IFRAME_CONTRACT.md` already specified these exact field names/shapes before this WORK existed - no rewrite needed, this WORK makes that document's own requirement true.

Known limitations:
- None beyond what this WORK's own Approved Design already scoped out (no live-transport delivery of these fields yet - that remains `WORK-0046`/`session-runtime-v1`'s `WORK-0020`).

Ready for independent review:
YES.

### Independent Review (2026-09-29)

A fresh agent, with no access to this session's own context, reviewed this WORK per `docs/ai/protocols/IMPLEMENTATION_REVIEW.md`. It traced `GetClientState`'s `Revision` assignment directly to the same loaded `currentTurn` used to compute `ClientState` (not merely accepted the claim), read `collectOutboundEvents`'s full body to confirm `ID`/`Revision` depend only on `(sequence, index-within-commands)` with no randomness or hidden state, hand-traced all four RUNNING-phase call sites against their own `CreateRuntimeTurn` call in the same function to rule out a stale-sequence bug, confirmed the new determinism test asserts full struct equality rather than mere non-emptiness, verified the extended integration tests' expected `ID` values are arithmetically consistent with their own fixtures (Start = Turn 1, the other three = Turn 2 since `startedSessionWithExecutor` already commits Turn 1 via `Start`), checked the doc-comment fix against this repository's own comment-standard document and re-ran the enforcing test itself, and ran its own fresh `go build`/`go vet`/`go test ./... -count=1` against real Postgres.

Verdict: **APPROVED**, no findings.

One documentation/drift note, applied as a correction rather than left as a finding: the Completion Record's claimed "2 failures/1 pass" ratio for the known-flaky, pre-existing Join/Leave race did not hold in the reviewer's own environment (4/4 failures there) - confirmed pre-existing and unrelated either way (via `git diff` showing that test's own file untouched by this WORK), but the specific ratio was an overclaim of precision for something genuinely environment-dependent. Reworded above to state both observations without asserting either as the authoritative constant.

No REQUIRED_FIX or DECISION_REQUIRED finding remains. Closed to DONE.
