# WORK-0043: Durable Confirmed-Turn Result Delivery (Outbox)

Status: PLANNED
Created: 2026-09-27
Last status change: 2026-09-27

Related decisions:
- `docs/decisions/architecture/ADR-0015-javascript-rule-execution-and-iframe-frontend-contract.md`
- `session/docs/decisions/SESSION-ADR-0019-session-runtime-post-commit-client-delivery-semantics.md` (extended, not reopened, by this WORK)

Canonical context:
- `docs/projects/active/session-runtime-v1/works/WORK-0020-role-aware-live-connections.md` (the live-connection layer this WORK's delivery mechanism must attach to)

## Outcome

Build the durable delivery mechanism `ADR-0015` requires for a confirmed interaction's *result* (accepted/rejected/failed), as distinct from a mere transport acknowledgement and as distinct from best-effort cosmetic effects (`WORK-0042`). `SESSION-ADR-0019` already anticipated that a future capability needing durable/irreversible delivery "must explicitly design its own delivery/idempotency/retry semantics as a separate decision" rather than inherit its own "no outbox" conclusion — this WORK is that decision made concrete. Without it, the mandate's requirement to distinguish "a transport acknowledgement" from "an accepted play" has no real mechanism.

## Context

Not yet designed. Depends on `WORK-0038`'s persistence model (what "confirmed" means to read back) and must coordinate with `session-runtime-v1`'s `WORK-0020` for where delivery actually attaches to a live connection.

## Scope

Not yet designed.

## Approved Design

Not yet designed.

## Constraints and Invariants

- Must not be conflated with or weaken `SESSION-ADR-0019`'s existing best-effort stance for cosmetic effects — this WORK adds a durable mechanism for a different, narrower class of output (confirmed interaction results), it does not generalize durability to everything.
- Must handle retries/duplicates and a failure between commit and delivery without double-applying or silently dropping a result.

## Acceptance Criteria

Not yet designed.

## Implementation Freedom

Not yet designed.

## Verification

Not yet designed. Must include a fault-injection test (failure between commit and delivery) proving the result is eventually delivered exactly once from the client's observable perspective.

## Documentation Impact

### Accepted / Canonical Knowledge

- A new domain-scoped decision record (when this WORK is drafted) recording the exact durable-delivery mechanism, since it is a material extension of `SESSION-ADR-0019`'s accepted scope, not an implementation detail.

## Blockers

- Durable outbox mechanism choice (`PROJECT.md` Material Decisions #4) — same-database table vs. external queue/broker.
- Depends on `WORK-0038` and coordination with `session-runtime-v1`'s `WORK-0020`.

## Completion Record

Not yet started.
