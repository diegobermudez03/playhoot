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

A Playhoot business domain must not directly invoke another Playhoot business domain's capabilities.

Cross-domain read composition belongs to Composer. Cross-domain write/workflow coordination belongs to Orchestrator. The concrete communication mechanism remains workflow-specific; this rule does not require synchronous calls, events, queues, choreography, or any other universal mechanism.

Domains expose operations that protect and operate on their own business responsibilities. A domain must not directly reach into another domain's persistence.

Only explicitly accepted boundaries in canonical architecture or domain documentation are authoritative. Repository folder existence alone is not architectural authority.

## Accepted Business Boundaries

The accepted business bounded context currently normalized here is Game.

Game contains:

- Game Management: authored game lifecycle concerns.
- Session Runtime: execution/session lifecycle concerns.
- Game Language: supporting technical subsystem/library used to define, compile, and execute games.

Game Management and Session Runtime are capabilities inside the Game bounded context, not separate business domains. Game Language is not a separate business bounded context.

Sharing one bounded context does not imply Game Management and Session Runtime share one persistence or transaction boundary; their independent persistence/transaction ownership is recorded in `game/README.md` and `docs/decisions/architecture/ADR-0002-game-capability-persistence-transaction-boundary.md`.

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

## Intra-Domain Responsibility Boundary

Within a single Playhoot business domain, three responsibilities are distinct:

- Application (use cases/workflows) coordinates execution: loading required state, calling repositories, managing transactions, invoking domain behavior, persisting results, translating relevant errors, and handling monitoring/operational concerns.
- Domain code makes business decisions: rules, invariants, state transitions, calculations, and validations, independent from persistence, transport, monitoring, transactions, GORM, or other infrastructure concerns whenever practical.
- Persistence/repository code retrieves and stores data; it does not own business policy.

Use cases and workflows are allowed application control flow. The objective is not to remove every conditional or piece of logic from them; it is that reusable business decisions must not be owned by a use case/workflow merely because that is where they happen to be invoked.

A dedicated abstraction is not required merely because business logic exists. A mandatory generic business-service layer is rejected. Domain behavior should be placed as close as possible to the concept that owns it, in this preferred order:

1. A method on the domain type/value object that naturally owns the behavior (e.g. `Visibility.IsPlayable()`).
2. A pure, package-level domain function when no single type owns the behavior — explicit inputs, explicit outputs, no persistence or external I/O.
3. A capability-specific domain package (e.g. `playability`, `publishing`) only when a cohesive decision spans several domain concepts and does not naturally belong to one type/function.

Package names should describe the business capability, not a generic technical bucket (`businessservice`) or an unqualified abstraction-kind name used as a catch-all (`policy/`).

Types are owned by the concept they represent (domain, use case, workflow, repository, or transport); a capability package may consume an existing domain-owned type directly rather than duplicating it.

Repository interfaces remain narrow and consumer-driven per the Dependency Principles above; they must not become owners of business policy. Several narrow contracts may still be satisfied by shared infrastructure.

This is a responsibility/dependency boundary, not a requirement to introduce literal `/domain`, `/application`, `/infrastructure` top-level directories.

Rationale and alternatives considered are recorded in `docs/decisions/architecture/ADR-0001-intra-domain-responsibility-boundary.md`. Concrete package-organization, naming, type-ownership, and migration conventions implementing this boundary are recorded in `docs/engineering/standards/domain-logic-placement.md`.

## Current Boundary Questions

NON-CANONICAL / UNRESOLVED:

- Identity vs Profile responsibility.
- Session history ownership.
- Discovery responsibility/boundary.
- Community responsibility/boundary.
