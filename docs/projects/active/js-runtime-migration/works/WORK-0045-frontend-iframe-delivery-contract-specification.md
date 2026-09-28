# WORK-0045: Frontend Iframe Delivery Contract Specification

Status: PLANNED
Created: 2026-09-27
Last status change: 2026-09-27

Related decisions:
- `docs/decisions/architecture/ADR-0015-javascript-rule-execution-and-iframe-frontend-contract.md`

Canonical context:
- None yet; this is the first record of its kind.

## Outcome

Produce the accepted, canonical specification of the frontend delivery contract `ADR-0015` establishes at the architecture-principle level: the trust boundary (iframe isolated in context/origin from the main application, no embedded credentials, restricted permissions), the small client-library API surface (conceptually: receive current view, receive effects, send an interaction — exact method names/shapes decided here), package loading (one version's frontend package loaded per Session, not re-sent per update), and the send/receive/apply/reject/fail distinction the mandate requires (a transport acknowledgement is never treated as an accepted play). This is a documentation deliverable per `ADR-0015`'s explicit scope boundary — implementing the frontend, its SDK, or its repository is out of this Project's scope — but the specification itself is approved design work, going through the same DRAFT/READY process as any other WORK, not freelanced outside it.

## Context

Not yet designed. Depends on `WORK-0039`'s command/view vocabulary existing in at least draft form, since the contract must be concrete about what a view/effect/interaction actually contains.

## Scope

Not yet designed.

## Approved Design

Not yet designed.

## Constraints and Invariants

- Must not describe a synchronous, infallible call shape — send/receive/apply/reject/fail must be explicitly distinguished.
- Must not grant the iframe direct network/credential access — all authenticated transport is owned by Playhoot's own (separately built) frontend, relayed to the iframe.

## Acceptance Criteria

- A published specification document exists covering the trust boundary, API surface, package loading, and delivery-state distinctions above, verifiable against `ADR-0015`'s own requirements.

## Implementation Freedom

Not yet designed.

## Verification

- Review against `ADR-0015`'s own text for completeness; no code verification, since this WORK produces no production code.

## Documentation Impact

### Accepted / Canonical Knowledge

- A new canonical specification document (location decided when this WORK is drafted, likely alongside `WORK-0046`'s implementation).

## Blockers

- Depends on `WORK-0039`.

## Completion Record

Not yet started.
