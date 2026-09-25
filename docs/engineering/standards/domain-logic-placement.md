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

See Preferred Workflow Package Shape below for the required shape of a `workflows/<workflow-name>/...` package specifically (one package, one controller, steps split by file - not one sibling package per step). The invariant for application-layer organization generally is that a developer navigating the tree can recognize which operations belong to the same lifecycle without opening every implementation file. A flat, undifferentiated `usecases/` directory containing every unrelated and lifecycle-related operation in a bounded context defeats this invariant just as much as a large domain Service would; package hierarchy itself should help communicate which operations belong together.

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

Grouping operations under a workflow package (conceptually `<domain>/workflows/<workflow-name>/...`) is a navigation/ownership boundary. See Preferred Workflow Package Shape below for the preferred shape: one cohesive controller (`Manager`) exposing the workflow's steps as methods, with each step's implementation kept in its own file. This does not require:

- a `Manager` that accretes unrelated non-lifecycle capabilities (see Workflow Controller vs Domain-Wide God Service below);
- a shared repository contract across every step (see Workflow Grouping Does Not Imply A Shared Repository Contract below).

Each step's implementation, tests, and narrow persistence contract remain independently owned even though they are exposed through the same `Manager` and package.

### Use Case

A use case is a relatively self-contained application capability that does not primarily exist as a transition/step of a larger lifecycle. It may still require the aggregate/domain to be in a particular state, read or modify durable data, use transactions, invoke domain behavior, or have substantial validation — a state precondition alone does NOT make something a workflow step.

Illustrative examples: a read/query capability (e.g. listing current Sessions for a User); an isolated configuration change; an independent command/action whose meaning is not primarily lifecycle progression.

### Classifying Ambiguous Cases

The important question is conceptual ownership: does this operation exist as part of an identifiable lifecycle/process, or is it an independently meaningful capability applied to the domain? Do not attempt an exhaustive classification test. When classification is materially ambiguous, prefer the structure that makes the actual domain behavior easiest to discover, and escalate through the Engineering Standard process if the choice establishes a new reusable pattern.

## Workflow Controller vs Domain-Wide God Service

REFINED: an earlier version of this section discouraged any single Manager/Service exposing a workflow's operations. That wording is superseded by this section — see the Engineering Standard history if the prior wording is needed for context.

A single cohesive workflow controller (conceptually `Manager`) exposing a workflow's lifecycle steps as its methods is the preferred shape when every exposed method is a genuine step/action of that same workflow. For example, `SessionLifecycle`'s `Manager` exposing `Create`, `Join`, `Leave`, `Start`, and later lifecycle transitions on one struct is desirable, not merely tolerated — see Preferred Workflow Package Shape below for the recommended file layout underneath it.

The prohibited pattern is a broad domain service that mixes genuine lifecycle steps with independent use cases, queries, configuration, or other unrelated capabilities onto the same struct merely because they touch the same domain or aggregate. A state precondition on an otherwise-independent capability does not make it a lifecycle step (see Classifying Ambiguous Cases above) — such capabilities belong under `usecases/`, not bolted onto the workflow controller.

Likewise, do not replace a big service with an equally opaque flat `usecases/` directory containing every unrelated and lifecycle-related operation in the bounded context (see Package Naming above). Neither an unfocused domain-wide service nor an undifferentiated flat package directory satisfies behavior locality — but a workflow controller whose methods are all genuine steps of one identifiable lifecycle is not the god-service pattern this section prohibits.

## Preferred Workflow Package Shape

For a workflow package (`workflows/<workflow-name>/...`), prefer one package exposing one discoverable controller/manager, with implementation split by step into separate files rather than split into one subpackage per verb:

```text
workflows/sessionlifecycle/
    manager.go
    step_create.go
    step_join.go
    step_leave.go
    step_start.go
    internal/
        repo/
            ...
```

not a flat sibling package per operation (`workflows/sessionlifecycle/{createsession,joinsession,leavesession}/`) that hides the fact all three are steps of the same lifecycle behind separate, independently-discoverable packages. Exact filenames may vary; the invariant is: one workflow capability, one discoverable controller, steps separated by file, not by sibling package.

This does not mean every Session/aggregate-touching operation belongs on the Manager — see Workflow Controller vs Domain-Wide God Service above and Classifying Ambiguous Cases. Manager methods should follow `function-signatures.md` (explicit parameters over ceremonial `Input` structs).

## Shared Cross-Step Behavior Within A Workflow Package

A workflow package accumulates more than persistence contracts as it grows: pure/execution behavior genuinely shared by two or more of its steps (not one step's own private implementation detail) — for example, lazy-expiration materialization, an Output-capture mechanism, or a replay-reconstruction routine. Left as flat top-level files alongside `manager.go`/`step_*.go`, these accumulate into the same navigation problem Preferred Workflow Package Shape exists to prevent: a reader can no longer tell, from the file list alone, which files are a lifecycle step and which are shared internal machinery.

Behavior genuinely used by two or more steps of the same workflow belongs under that workflow's own `internal/<mechanism-name>/` subpackage, sibling to `internal/repo/` — named for the mechanism/invariant it provides, the same naming discipline `repositories.md -> Sharing Rule` already requires for domain-wide shared persistence packages, applied one level down at the single-workflow scope. It is never elevated to the domain-wide `internal/` merely because it is shared within one workflow, and never merged into `internal/repo/` itself unless it is genuinely a persistence contract:

```text
workflows/sessionlifecycle/
    manager.go
    step_create.go
    step_join.go
    step_leave.go
    step_start.go
    step_answer_interaction.go
    types.go
    internal/
        repo/
            ...
        expiration/     # lazy lobby-expiration materialization, shared by Join/Leave/Start
        interactions/   # Output -> session_interactions capture, shared by Start/AnswerInteraction
        replay/         # durable-signal-log replay reconstruction, shared by AnswerInteraction
```

A helper used by exactly one step stays in that step's own file — do not pre-emptively extract a subpackage for a single consumer. Extract only once genuine sharing across two or more steps exists, per Behavior Locality's own "optimize for comprehensibility, not maximizing DRY" principle above; do not extract merely because a helper *could* be reused someday.

This does not change Workflow Grouping Does Not Imply A Shared Repository Contract: an extracted subpackage exposes the narrow interface/function shape its own mechanism actually needs, never a shared repository contract absorbing every step's persistence method.

## Use Cases And Workflows Remain Responsible For

Use cases/workflows remain responsible for: loading required state, repository calls, transactions, coordinating several operations, calling domain behavior, persisting resulting state, translating relevant errors, monitoring/operational handling, and workflow progression/retries/compensation/persisted workflow state where applicable.

They must not become the canonical owner of reusable business policy merely because they invoke it. Reusable domain behavior should not live under a specific use-case/workflow-step package unless it genuinely exists only as an implementation detail of that one operation.

## Responsibility Categories: Business/Lifecycle Policy, Data Integrity, Persistence Mechanics

Three distinct questions must not collapse into one owner. This taxonomy is more precise than the ambiguous shorthand "business vs. data" and should be used instead of it.

**Business/lifecycle policy** — "What should happen?" Examples: may this participant be admitted; is a time-bound window expired; what result should a repeated command produce; is a transition currently allowed; what does capacity/admission require. Owner: the workflow/use-case/domain layer.

**Data integrity** — "What persisted shapes must never be invalid?" Examples: a child row must always belong to its parent; a uniqueness constraint over a tuple of columns; an idempotency-identity uniqueness constraint. Owner: repository/schema/database constraints, as appropriate.

**Persistence mechanics** — "How is an already-decided, valid intent/state represented in storage?" Example: creating an aggregate with a mandatory child relationship may require multiple inserts/updates in one transaction. Owner: repository, per `repositories.md`'s Multi-Table Persistence Encapsulation.

A workflow/use case owns business/lifecycle policy — including deciding whether a state precondition permits an action, whether a time-bound window has elapsed, and what a repeated/idempotent command should return. A repository must not decide these merely to make a workflow method shorter; it may only report the facts (current state, elapsed time, existing rows) the workflow needs to decide them, and perform the resulting mutations the workflow requests. See `repositories.md -> Multi-Table Persistence Encapsulation` for the corresponding repository-side statement of this same boundary, and `repositories.md -> Transaction Ownership` and `-> Repository Naming` for two concrete consequences of it.

## Repository Contracts

Repository interfaces stay narrow and driven by the needs of their consumer (use case/workflow step), per `ARCHITECTURE.md -> Dependency Principles`. This does not require a separate concrete repository implementation per use case — several narrow contracts may be satisfied by shared infrastructure. Repositories must not own business policy; see `repositories.md` for repository implementation conventions.

Each workflow step/use case defines or owns the persistence contract shaped by that specific behavior. Even when two steps need conceptually similar operations (for example, both Join and Leave needing to locate a SessionActor), they remain separate consumers with separate semantic contracts — identical method signatures today do not imply one shared contract is owed. Do NOT introduce a broad repository interface merely to avoid repeating interface methods; it is acceptable and often desirable for different behavior-local repository contracts to independently contain similar operations. See `repositories.md -> Sharing Rule` for when persistence code should nonetheless be centralized (genuinely cross-cutting protocols/mechanics) versus when local duplication is preferable.

### Workflow Grouping Does Not Imply A Shared Repository Contract

Even operations belonging to the same workflow may define separate narrow persistence contracts. Do not create a single `SessionLifecycleRepository`-style interface containing every persistence method needed by every lifecycle step merely because the steps share a workflow folder. Likewise, do not create a generic `GeneralPurposeRepository`/`CommonRepository`/`BaseRepository` as a dumping ground for shared operations — shared protocols/mechanics should be named for the invariant/mechanism they provide (see `repositories.md -> Sharing Rule`).

## Existing `businessservice`

`game/management/internal/businessservice` predates this standard and is an early implementation experiment, not the canonical pattern.

Migration strategy: OPPORTUNISTIC MIGRATION. Migrate its contents to the placements above when the relevant code is touched, or when a focused change makes migration worthwhile. This standard does not authorize a repository-wide refactor to remove it.

## Enforcement

Code review, this standard, `ARCHITECTURE.md`, and focused domain unit tests (see `testing.md`). No architecture linter or other automated enforcement tool is introduced by this standard.

During code review, flag in particular:

- lifecycle steps placed as unrelated/flat use cases when their lifecycle ownership is clear;
- a workflow controller accumulating an independent use case, query, or configuration capability merely because it touches the same aggregate — as opposed to a controller whose methods are all genuine lifecycle steps, which is the preferred shape (see Workflow Controller vs Domain-Wide God Service above);
- a workflow split into one sibling package per verb instead of one package with per-step files (see Preferred Workflow Package Shape above);
- a broad domain/workflow repository interface, or a `Common`/`General`/`Base` repository dumping ground, created to avoid repeating similar methods (see `repositories.md -> Sharing Rule` for the corresponding entity-CRUD-package version of this same concern);
- a repository method deciding business/lifecycle policy (admission, expiration, idempotency replay meaning) instead of reporting facts and performing requested mutations (see Responsibility Categories above);
- behavior genuinely shared by two or more steps of the same workflow left as a flat top-level file in the workflow package instead of its own `internal/<mechanism-name>/` subpackage (see Shared Cross-Step Behavior Within A Workflow Package above) — and, conversely, a subpackage extracted for a helper only one step actually uses.
