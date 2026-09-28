# WORK-0042: Effects Delivery (Best-Effort, Restated From GAME-ADR-0020)

Status: PLANNED
Created: 2026-09-27
Last status change: 2026-09-27

Related decisions:
- `game/docs/decisions/GAME-ADR-0020-session-runtime-post-commit-client-delivery-semantics.md`
- `docs/decisions/architecture/ADR-0015-javascript-rule-execution-and-iframe-frontend-contract.md`

Canonical context:
- `game/session/workflows/sessionlifecycle/manager.go` (WORK-0006/WORK-0029's already-implemented in-memory Effect/Presentation Output return, being adapted to the new command source)

## Outcome

Adapt cosmetic/presentation effect delivery to the new JS command source (`WORK-0039`), restating `GAME-ADR-0020`'s existing accepted stance unchanged: effects are best-effort, correctness never depends on a client receiving one, and correlation exists for repeated/late/absent effects. This is explicitly distinct from `WORK-0043` (durable delivery of confirmed interaction *results*, which `ADR-0015` extends beyond best-effort) — this WORK does not change delivery guarantees, only the source of what's being delivered.

## Context

Not yet designed. Depends on `WORK-0039`'s command vocabulary.

## Scope

Not yet designed.

## Approved Design

Not yet designed.

## Constraints and Invariants

- Must not introduce a durable outbox for effects — `GAME-ADR-0020`'s "no outbox for presentation" conclusion is unchanged by this WORK.

## Acceptance Criteria

Not yet designed.

## Implementation Freedom

Not yet designed.

## Verification

Not yet designed.

## Documentation Impact

### Current-State Documentation After Implementation

- `game/docs/FLOWS.md` — effect source updated from engine Output to JS command.

## Blockers

- Depends on `WORK-0039`.

## Completion Record

Not yet started.
