# Domain Logic Placement Standard

Status: CANONICAL ENGINEERING STANDARD

This standard records concrete conventions implementing the intra-domain responsibility boundary owned by `ARCHITECTURE.md -> Intra-Domain Responsibility Boundary` (see `docs/decisions/architecture/ADR-0001-intra-domain-responsibility-boundary.md` for rationale). It does not redefine that boundary.

## Core Organizing Principle: Behavior Locality

Application code is organized primarily around behavior and lifecycle responsibility, not around entities or generic domain-wide services.

A developer inspecting one behavior should be able to determine locally:

- what the behavior does;
- what state it needs;
- what persistence capabilities it requires;
- what invariants/preconditions it enforces;
- what collaborators it invokes.

Avoid forcing the reader to reconstruct one behavior by navigating through large domain-wide Service/Repository APIs or horizontal entity-CRUD packages. Optimize for behavioral locality and comprehensibility rather than maximizing DRY. See `repositories.md -> Sharing Rule` for how this principle constrains persistence-code sharing specifically.

## Preferred Placement Order

1. Behavior naturally owned by a domain type/value object — a method on that type (e.g. `Visibility.IsPlayable()`).
2. Pure domain behavior with no single owning type — a package-level function in the domain package (e.g. `ValidateName(...)`, `CanTransition(...)`, `CalculateSomething(...)`), taking explicit inputs, returning explicit outputs, performing no persistence or external I/O.
3. A cohesive decision spanning several domain concepts, not naturally owned by one type or function — a capability-specific domain package (e.g. `playability`, `publishing`), exposing something like `Evaluate(input)` or a type such as `Policy`/`Decision`/`Validator`/`Calculator`/`Transition`/`Rules` when that name accurately describes the abstraction.

A new struct/service abstraction is not required merely because business logic exists. Introduce a capability package only when the behavior is cohesive and significant enough to justify one — not one package per function.

## Package Naming

Package hierarchy should communicate business/process ownership. Prefer capability/domain-language package names (`playability`, `publishing`) over generic technical buckets (`businessservice`). Avoid turning a `policy/` (or similarly generic) package into a dumping ground for unrelated business rules; the package name should describe the business capability, not the kind of abstraction it contains.

For application-layer organization specifically:

- Prefer `workflows/<workflow-name>/...` (for example `workflows/sessionlifecycle/...`) for operations belonging to an identifiable lifecycle/process.
- Prefer `usecases/<independent-capability>/...` for independent application capabilities that are not primarily a lifecycle transition.

No single exact nesting depth is required — either `workflows/sessionlifecycle/joinsession/...` or another repository-consistent focused arrangement may be valid. The invariant is that a developer navigating the tree can recognize which operations belong to the same lifecycle without opening every implementation file. A flat, undifferentiated `usecases/` directory containing every unrelated and lifecycle-related operation in a bounded context defeats this invariant just as much as a large domain Service would; package hierarchy itself should help communicate which operations belong together.

## Type Ownership

Types belong to the concept they represent, not to whichever package happens to consume them:

- persistence/query projections — repository/application-private;
- use-case command/result DTOs — use-case-owned;
- workflow state — workflow-owned;
- domain concepts — domain-owned;
- transport DTOs — transport-owned.

A capability package may consume an existing domain-owned type directly (e.g. `Visibility`) rather than duplicating it, and may define its own `Input`/`Decision`/similar types only when those genuinely represent that capability rather than duplicating an existing domain concept.

## Workflow vs Use Case

Application behavior is either a **workflow step** or a **use case**. This distinction governs package placement; it is not a mathematically exhaustive classification, and it is not required to be resolved with certainty in every case — see Classifying Ambiguous Cases below.

### Workflow

A workflow groups operations that are steps/transitions/actions within the same durable lifecycle or cohesive multi-step process. Characteristics may include:

- operations are conceptually related through lifecycle progression;
- prior/current state constrains later operations;
- operations may be expected in some meaningful lifecycle relationship;
- the sequence may branch, repeat, skip, or be dynamic — a workflow is NOT required to be a rigid linear state machine;
- operations share a lifecycle/business process even if each operation has its own entry point and implementation.

Illustrative examples (not new product requirements): a Session lifecycle's Create/Join/Leave/Start/later transitions; an Order's authorization/capture/release-refund sequence.

Grouping operations under a workflow package (conceptually `<domain>/workflows/<workflow-name>/...`) is a navigation/ownership boundary. It does NOT require:

- one giant Manager/Service struct exposing every operation (see No Domain-Wide God Service below);
- a shared repository contract across every step (see Workflow Grouping Does Not Imply A Shared Repository Contract below).

Individual operations/steps may still own their own focused service/handler, their own input/output types, their own narrow persistence contract, and their own tests.

### Use Case

A use case is a relatively self-contained application capability that does not primarily exist as a transition/step of a larger lifecycle. It may still require the aggregate/domain to be in a particular state, read or modify durable data, use transactions, invoke domain behavior, or have substantial validation — a state precondition alone does NOT make something a workflow step.

Illustrative examples: a read/query capability (e.g. listing current Sessions for a User); an isolated configuration change; an independent command/action whose meaning is not primarily lifecycle progression.

### Classifying Ambiguous Cases

The important question is conceptual ownership: does this operation exist as part of an identifiable lifecycle/process, or is it an independently meaningful capability applied to the domain? Do not attempt an exhaustive classification test. When classification is materially ambiguous, prefer the structure that makes the actual domain behavior easiest to discover, and escalate through the Engineering Standard process if the choice establishes a new reusable pattern.

## No Domain-Wide God Service

Do not create a single large domain Service/Manager merely because operations belong to the same domain or workflow. For example, grouping operations under `workflows/sessionlifecycle/` does not imply that `SessionLifecycleService.Create`, `.Join`, `.Leave`, `.Start`, etc. must all live on one ever-growing struct. Workflow directories are primarily an ownership/navigation boundary; individual workflow steps may remain independent focused components.

Likewise, do not replace a big service with an equally opaque flat `usecases/` directory containing every unrelated and lifecycle-related operation in the bounded context (see Package Naming above). Neither a god service nor an undifferentiated flat package directory satisfies behavior locality.

## Use Cases And Workflows Remain Responsible For

Use cases/workflows remain responsible for: loading required state, repository calls, transactions, coordinating several operations, calling domain behavior, persisting resulting state, translating relevant errors, monitoring/operational handling, and workflow progression/retries/compensation/persisted workflow state where applicable.

They must not become the canonical owner of reusable business policy merely because they invoke it. Reusable domain behavior should not live under a specific use-case/workflow-step package unless it genuinely exists only as an implementation detail of that one operation.

## Repository Contracts

Repository interfaces stay narrow and driven by the needs of their consumer (use case/workflow step), per `ARCHITECTURE.md -> Dependency Principles`. This does not require a separate concrete repository implementation per use case — several narrow contracts may be satisfied by shared infrastructure. Repositories must not own business policy; see `repositories.md` for repository implementation conventions.

Each workflow step/use case defines or owns the persistence contract shaped by that specific behavior. Even when two steps need conceptually similar operations (for example, both Join and Leave needing to locate a SessionActor), they remain separate consumers with separate semantic contracts — identical method signatures today do not imply one shared contract is owed. Do NOT introduce a broad repository interface merely to avoid repeating interface methods; it is acceptable and often desirable for different behavior-local repository contracts to independently contain similar operations. See `repositories.md -> Sharing Rule` for when persistence code should nonetheless be centralized (genuinely cross-cutting protocols/mechanics) versus when local duplication is preferable.

### Workflow Grouping Does Not Imply A Shared Repository Contract

Even operations belonging to the same workflow may define separate narrow persistence contracts. Do not create a single `SessionLifecycleRepository`-style interface containing every persistence method needed by every lifecycle step merely because the steps share a workflow folder. Likewise, do not create a generic `GeneralPurposeRepository`/`CommonRepository`/`BaseRepository` as a dumping ground for shared operations — shared protocols/mechanics should be named for the invariant/mechanism they provide (see `repositories.md -> Sharing Rule`).

## Existing `businessservice`

`game/game/internal/businessservice` predates this standard and is an early implementation experiment, not the canonical pattern.

Migration strategy: OPPORTUNISTIC MIGRATION. Migrate its contents to the placements above when the relevant code is touched, or when a focused change makes migration worthwhile. This standard does not authorize a repository-wide refactor to remove it.

## Enforcement

Code review, this standard, `ARCHITECTURE.md`, and focused domain unit tests (see `testing.md`). No architecture linter or other automated enforcement tool is introduced by this standard.

During code review, flag in particular:

- lifecycle steps placed as unrelated/flat use cases when their lifecycle ownership is clear;
- a giant Service/Manager structure accumulating unrelated operations merely because they share a domain or workflow;
- a broad domain/workflow repository interface, or a `Common`/`General`/`Base` repository dumping ground, created to avoid repeating similar methods (see `repositories.md -> Sharing Rule` for the corresponding entity-CRUD-package version of this same concern).
