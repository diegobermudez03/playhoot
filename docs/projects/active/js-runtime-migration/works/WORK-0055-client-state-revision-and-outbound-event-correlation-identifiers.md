# WORK-0055: ClientState Revision & Outbound Event Correlation Identifiers

Status: PLANNED
Created: 2026-09-29
Last status change: 2026-09-29

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

Not yet designed.

## Approved Design

Not yet designed.

## Constraints and Invariants

- Must not change `session.GetClientStateResult`/`session.OutboundEvent`'s existing fields - additive only, mirroring how `WORK-0042` itself added `Events` additively to four other Result types.
- Must not introduce a new durable column/table merely to serve this - both `RuntimeTurn.Sequence` values this WORK needs already exist and are already read by the relevant call sites.
- `id` must be stable and reproducible for the exact same underlying command across a delivery retry (so a frontend-side duplicate check actually works) - it must not be regenerated randomly per delivery attempt.

## Acceptance Criteria

Not yet designed.

## Implementation Freedom

Not yet designed.

## Verification

Not yet designed.

## Documentation Impact

### Accepted / Canonical Knowledge

- `session/docs/FRONTEND_IFRAME_CONTRACT.md` (once `WORK-0045` creates it) - no rewrite expected, since that document already specifies `revision`/`id` as required fields; this WORK is what makes the backend side of that requirement true.

## Blockers

- None known yet - both `WORK-0041` and `WORK-0042` (the capabilities this WORK extends) are DONE. Exact design (field names/types, `collectOutboundEvents` signature change) is undesigned; drafting this WORK is expected to be small and mechanical, not blocked on a material decision.

## Completion Record

Not yet started.
