# Domain Logic Placement Standard

Status: CANONICAL ENGINEERING STANDARD

This standard records concrete conventions implementing the intra-domain responsibility boundary owned by `ARCHITECTURE.md -> Intra-Domain Responsibility Boundary` (see `docs/decisions/architecture/ADR-0001-intra-domain-responsibility-boundary.md` for rationale). It does not redefine that boundary.

## Preferred Placement Order

1. Behavior naturally owned by a domain type/value object — a method on that type (e.g. `Visibility.IsPlayable()`).
2. Pure domain behavior with no single owning type — a package-level function in the domain package (e.g. `ValidateName(...)`, `CanTransition(...)`, `CalculateSomething(...)`), taking explicit inputs, returning explicit outputs, performing no persistence or external I/O.
3. A cohesive decision spanning several domain concepts, not naturally owned by one type or function — a capability-specific domain package (e.g. `playability`, `publishing`), exposing something like `Evaluate(input)` or a type such as `Policy`/`Decision`/`Validator`/`Calculator`/`Transition`/`Rules` when that name accurately describes the abstraction.

A new struct/service abstraction is not required merely because business logic exists. Introduce a capability package only when the behavior is cohesive and significant enough to justify one — not one package per function.

## Package Naming

Prefer capability/domain-language package names (`playability`, `publishing`) over generic technical buckets (`businessservice`). Avoid turning a `policy/` (or similarly generic) package into a dumping ground for unrelated business rules; the package name should describe the business capability, not the kind of abstraction it contains.

## Type Ownership

Types belong to the concept they represent, not to whichever package happens to consume them:

- persistence/query projections — repository/application-private;
- use-case command/result DTOs — use-case-owned;
- workflow state — workflow-owned;
- domain concepts — domain-owned;
- transport DTOs — transport-owned.

A capability package may consume an existing domain-owned type directly (e.g. `Visibility`) rather than duplicating it, and may define its own `Input`/`Decision`/similar types only when those genuinely represent that capability rather than duplicating an existing domain concept.

## Use Cases And Workflows

Use cases/workflows remain responsible for: loading required state, repository calls, transactions, coordinating several operations, calling domain behavior, persisting resulting state, translating relevant errors, monitoring/operational handling, and workflow progression/retries/compensation/persisted workflow state where applicable.

They must not become the canonical owner of reusable business policy merely because they invoke it. Reusable domain behavior should not live under a specific use-case package unless it genuinely exists only as an implementation detail of that one use case.

## Repository Contracts

Repository interfaces stay narrow and driven by the needs of their consumer (use case/workflow), per `ARCHITECTURE.md -> Dependency Principles`. This does not require a separate concrete repository implementation per use case — several narrow contracts may be satisfied by shared infrastructure. Repositories must not own business policy; see `repositories.md` for repository implementation conventions.

## Existing `businessservice`

`game/game/internal/businessservice` predates this standard and is an early implementation experiment, not the canonical pattern.

Migration strategy: OPPORTUNISTIC MIGRATION. Migrate its contents to the placements above when the relevant code is touched, or when a focused change makes migration worthwhile. This standard does not authorize a repository-wide refactor to remove it.

## Enforcement

Code review, this standard, `ARCHITECTURE.md`, and focused domain unit tests (see `testing.md`). No architecture linter or other automated enforcement tool is introduced by this standard.
