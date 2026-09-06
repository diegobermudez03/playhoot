# Architecture

Status: CANONICAL

## Scope

This document owns accepted global architecture rules and boundaries for Playhoot. It does not own implementation progress, package-local details, product roadmap, future proposals, or detailed coding conventions.

## Architectural Style

Playhoot is initially a modular monolith.

- Business/domain boundaries should be explicit enough to support independent evolution and possible future service separation.
- Current co-location in one process or physical database does not erase logical ownership.
- Do not simulate distributed-system complexity merely to make the monolith behave like microservices.
- Add network, service, queue, or deployment machinery only when an actual problem requires it.

## Component Model

- Domain / Bounded Context: owns a coherent business responsibility, state, invariants, and public capabilities.
- Internal Capability / Module: implements part of a domain or subsystem without itself being a separate business boundary.
- Technical / Supporting Library: provides reusable behavior without owning business state.
- Coordination Layer: coordinates work across business domains without owning participating domains' business rules.
- Transport / Application Edge: exposes user-facing transport entry points and translates transport requests/responses.

Domain != package. Domain != folder. Domain != deployment. Domain != service.

A top-level repository directory does not automatically establish an accepted business domain.

## Domain Boundaries

A business domain must not directly coordinate operations with another Playhoot business domain.

Domains expose operations that protect and operate on their own business responsibilities. A domain must not directly reach into another domain's persistence.

Only explicitly accepted boundaries in canonical architecture or domain documentation are authoritative. Repository folder existence alone is not architectural authority.

## Accepted Business Boundaries

The accepted business bounded context currently normalized here is Game.

Game contains:

- Game Management: authored game lifecycle concerns.
- Session Runtime: execution/session lifecycle concerns.
- Game Language: supporting technical subsystem/library used to define, compile, and execute games.

Game Management and Session Runtime are capabilities inside the Game bounded context, not separate business domains. Game Language is not a separate business bounded context.

The current physical package layout may remain:

```text
game/
  game/
  session/
  language/
```

## Cross-Domain Reads

Composer is the coordination layer for cross-domain read composition.

- It is stateless with respect to business/domain state.
- It invokes domain read capabilities and composes the result.
- It may contain composition/read-coordination logic.
- It does not own business entities.
- It does not own business rules that belong to individual domains.

Cross-domain data composition should not be independently recreated in API handlers.

## Cross-Domain Writes

Orchestrator is the coordination layer for cross-domain write workflows.

- It coordinates operations across business domains.
- It may persist workflow state when required.
- It may manage retries, compensations, idempotency, and progression when required by a workflow.
- It may contain workflow/orchestration logic.
- It does not own the underlying business rules of participating domains.

No universal communication mechanism is accepted globally. Synchronous calls, asynchronous messaging, events, choreography, queues, and other mechanisms are workflow-specific decisions.

## Application Edge

API is the external transport/application edge.

- It exposes user-facing transport endpoints.
- It translates transport requests/responses.
- It may perform transport-level concerns.
- It may call a single-domain capability.
- It may call Composer for cross-domain reads.
- It may initiate Orchestrator workflows for cross-domain writes.
- It owns no business state.

API may perform BFF response shaping. BFF response shaping is not the same as cross-domain business/data composition; cross-domain read composition belongs to Composer.

This document does not define where identity or authorization policy is owned.

## State and Transaction Boundaries

Each business domain owns its state.

Co-location in the same physical infrastructure must not be used to bypass domain contracts. No database transaction should span independent business-domain ownership.

Cross-domain workflows must account for partial failure rather than relying on a shared database transaction across domains. Exact retry and consistency mechanisms belong to specific workflow designs.

## Dependency Principles

A business domain's public operations should primarily expose that domain's own concepts/types.

Depending directly on another business domain's internal/entity types should be avoided. Supporting behavior-only libraries may expose types intentionally consumed by domains when that dependency does not reverse business-domain ownership.

## Current Boundary Questions

NON-CANONICAL / UNRESOLVED:

- Identity vs Profile responsibility.
- Session history ownership.
- Discovery responsibility/boundary.
- Community responsibility/boundary.
