# WORK-0042: SEND_EVENT Delivery (Best-Effort, Restated From SESSION-ADR-0019)

Status: PLANNED
Created: 2026-09-27
Last status change: 2026-09-27

Related decisions:
- `session/docs/decisions/SESSION-ADR-0019-session-runtime-post-commit-client-delivery-semantics.md`
- `docs/decisions/architecture/ADR-0015-javascript-rule-execution-and-iframe-frontend-contract.md`

Canonical context:
- `game/session/workflows/sessionlifecycle/manager.go` (WORK-0006/WORK-0029's already-implemented in-memory Effect/Presentation Output return, being adapted to the new command source)

## Outcome

Adapt cosmetic/transient event delivery to the new JS command source (`WORK-0039`'s `SEND_EVENT`), restating `SESSION-ADR-0019`'s existing accepted stance unchanged: a `SEND_EVENT` is best-effort, correctness never depends on a client receiving one, and correlation exists for repeated/late/absent delivery. This is explicitly distinct from `WORK-0043` (durable delivery of a confirmed `PLAYER_EVENT`'s own *outcome*, which `ADR-0015` extends beyond best-effort) — this WORK does not change delivery guarantees, only the source of what's being delivered.

## Context

Not yet designed. Depends on `WORK-0039`'s command vocabulary.

**Vocabulary update (2026-09-28):** `WORK-0039` names this command `SEND_EVENT`, deliberately not `EMIT_EFFECT` — its `name`/`payload` are entirely game-defined and opaque to Playhoot (this WORK validates/delivers only the envelope: recipients, session, size/count limits, correlation); whether the generated frontend treats a given `SEND_EVENT` as an animation, a sound, a notification, or nothing at all is frontend logic this WORK has no visibility into and must not assume.

## Scope

Not yet designed.

## Approved Design

Not yet designed.

## Constraints and Invariants

- Must not introduce a durable outbox for effects — `SESSION-ADR-0019`'s "no outbox for presentation" conclusion is unchanged by this WORK.

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
