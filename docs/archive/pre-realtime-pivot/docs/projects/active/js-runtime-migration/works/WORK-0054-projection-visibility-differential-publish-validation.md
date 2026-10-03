# WORK-0054: Projection Visibility Differential Publish-Validation

Status: PLANNED
Created: 2026-09-29
Last status change: 2026-09-29

Related decisions:
- `docs/decisions/architecture/ADR-0015-javascript-rule-execution-and-iframe-frontend-contract.md`

Canonical context:
- `docs/projects/active/js-runtime-migration/works/WORK-0041-per-player-view-computation-and-privacy-verification.md` (this WORK's own origin - see its "Human Decision" section for why the differential check was carved out here rather than built as part of it)
- `session/workflows/sessionlifecycle/internal/visibility/LOGICAL_CONTRACT.md` (the `Filter`/`LeakCheck` mechanism this WORK adds a third, offline layer on top of)

## Outcome

`WORK-0041` built Session Runtime's per-viewer privacy boundary as two layers: `Filter`, a capability-based construction-time guarantee (the untrusted script never receives data a viewer is not authorized to see), and `LeakCheck`, a live, cheap, defense-in-depth check for a literal/verbatim leak of excluded data into a computed `ClientState`.

Both already close the failure mode `ADR-0015` names explicitly - an author forgetting to filter a field. Given `Filter`'s own capability-based guarantee (verified exhaustively for `WORK-0041`, including nested structures, array-indexed collections, multi-participant ownership isolation, and every declassification primitive), a subtle *derived*-value leak (the untrusted script deriving something revealing from private data without copying it verbatim) is already structurally impossible against a correctly-implemented `Filter`: the script never receives the excluded data at all, so it has nothing to derive from.

This WORK is therefore explicit **defense-in-depth against a regression or a subtle bug in `Filter`/the declassification model itself** - not a runtime safeguard against arbitrary authored script behavior. Its value is catching a bug in Playhoot's own filtering/declassification implementation before a game ever reaches a real player, at game-version publish/validation time, not on the live request path.

**Human decision (2026-09-29, during `WORK-0041`'s independent review):** deferred out of `WORK-0041` into this standalone follow-up. `WORK-0041`'s own `Filter`+`LeakCheck` are independently sufficient and already delivered; this WORK does not block that one's completion, and does not block this Project's own completion criteria (its capability-coverage row is already satisfied by `WORK-0041`'s delivered mechanism).

## Context

Not yet designed beyond the conceptual shape already recorded in `WORK-0041`'s own now-superseded Approved Design (kept here for continuity, not binding): for representative fixture states, for each viewer V and each path declared private to some other owner, perturb that path and re-run `Filter`+`project` for V; if V's `ClientState` changes, confirm the change corresponds to a declared declassified derivative of that path - if not, flag it. A differential test must be evaluated against the declared visibility/declassification model, not a bare "did the output change" check, since a legitimate declassified derivative (for example, an opponent's card count) is expected to vary with its own private source.

Open questions genuinely undesigned:
- Fixture-state provisioning (author-supplied vs. Playhoot-generated) - at least one representative state per distinct visibility declaration is needed for meaningful coverage.
- Where in the publish/versioning workflow this check runs, and what a failing result blocks (rejecting publish outright vs. a warning) - this is a product/workflow question, not purely technical, and belongs to whichever WORK/process owns Game-version publishing (`docs/work/active/WORK-0033-cross-domain-game-publish-composition.md`'s own future scope, unaffected by this WORK's own narrower "does the check exist" concern).
- Whether this runs against the real, separately deployed JavaScript Executor (consistent with `ADR-0016`) or some other harness.

**Independent-review recommendation, carried forward from `WORK-0041`'s own Completion Record (2026-09-29):** `WORK-0041`'s `Schema.validate()` (the rejection layer this WORK's own differential check is additional defense-in-depth beyond) went through six independent-review rounds; five found and fixed a genuinely new, distinct bug shape each time, each narrowly targeted at the one reproduction its own reviewer happened to construct by hand. The sixth reviewer's own explicit assessment: that pattern is itself weak evidence that further confidence-building via one more hand-reasoning pass has a structurally poor hit rate against this specific combinatorial space (wildcard count × nesting × Source-coverage direction × container type) - and recommended that if this WORK (or any future hardening of `internal/visibility`) wants more confidence than manual review already bought, it should be property-based/fuzz testing generating random Schemas across that combinatorial space, checked against an oracle invariant ("no value reachable from a state path classified private-to-someone-else or server_only ever appears in a non-owning viewer's `Filter` output"), not another identical manual round. Whoever drafts this WORK's own Approved Design should weigh whether such a generator/property-test harness is itself part of this WORK's scope, or a distinct, even-more-standalone follow-up - not decided here.

## Scope

Not yet designed.

## Approved Design

Not yet designed.

## Constraints and Invariants

- Must not run on the live `GetClientState` request path - this is explicitly an offline/publish-time capability, per the human decision above and `ADR-0016`'s own cost/latency reasoning for the Executor.
- Must not flag a legitimate declassified derivative as a leak (the same correction already recorded in `WORK-0041`'s own Material Decision 2, restated here since it still applies to whatever concretely implements this WORK).

## Acceptance Criteria

Not yet designed.

## Implementation Freedom

Not yet designed.

## Verification

Not yet designed.

## Documentation Impact

### Accepted / Canonical Knowledge

- `session/workflows/sessionlifecycle/internal/visibility/LOGICAL_CONTRACT.md`'s own "Explicitly Not Implemented Here" section, once this WORK lands, moves this item from "not implemented" to a reference to this WORK's own implementation.

## Blockers

- None. `WORK-0041` (the `Filter`/`LeakCheck` mechanism this WORK adds a third layer on top of) is DONE.

## Completion Record

Not yet started.
