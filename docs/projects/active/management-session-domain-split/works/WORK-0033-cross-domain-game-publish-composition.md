# WORK-0033: Cross-Domain Game Publish/Authoring Composition

Status: PLANNED
Created: 2026-09-27
Last status change: 2026-09-27

Related decisions:
- `docs/decisions/architecture/ADR-0014-management-session-domain-split.md`

Canonical context:
- `ARCHITECTURE.md` (Cross-Domain Writes, Orchestrator)
- `orchestrator/README.md`

## Outcome

After ADR-0014, publishing/authoring a Game must keep two independently-owned representations consistent: Game Management's own entity (visibility, metadata, ownership, images, authored history) and Session Runtime's own persisted executable Game Language definition (WORK-0031). This WORK is required so that consistency has a real owner: Orchestrator (`orchestrator/`), per `ARCHITECTURE.md -> Cross-Domain Writes`, validating an authored definition against Session Runtime's own Game Language (now Session Runtime's internal implementation) before either domain's write commits.

This is recorded as a known-required future outcome per ADR-0014's own Decision (the human explicitly described this composition when deciding the domain split), per `docs/projects/README.md` Invariant 1 — not because its design has started.

## Context

Not yet designed, and not yet designable: Game Management currently exposes no game-creation/authoring/publish write path at all (only `getgame`/`getgamedefinition` reads exist). This WORK depends on that authoring capability being built first — by this Project or a separate future one — which is itself out of this Project's scope (see `PROJECT.md -> Explicitly Out Of Scope`).

## Scope

Not yet designed.

## Approved Design

Not yet designed.

## Constraints and Invariants

- Per `ARCHITECTURE.md -> State and Transaction Boundaries`, no shared database transaction may span Game Management and Session Runtime; this workflow must account for partial failure between the two writes rather than relying on one.
- Per `ARCHITECTURE.md -> Cross-Domain Writes`, Orchestrator may persist workflow/idempotency/retry state as needed but must not become the permanent business source of truth for the Game/definition mapping itself — that remains each domain's own responsibility.

## Acceptance Criteria

Not yet designed.

## Implementation Freedom

Not yet designed.

## Verification

Not yet designed.

## Documentation Impact

### Accepted / Canonical Knowledge

- `orchestrator/README.md` likely gains a concrete example once real orchestration code exists there for the first time.

### Current-State Documentation After Implementation

- Not yet designed.

## Blockers

- Blocked on a Game Management authoring/publish write path existing at all (out of this Project's scope) — this WORK cannot move to DRAFT until that capability is at least planned concretely enough to design against.

## Completion Record

Not yet started.
