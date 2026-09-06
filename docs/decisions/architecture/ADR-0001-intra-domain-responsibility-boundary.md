# ADR-0001: Intra-Domain Responsibility Boundary (Application Coordinates, Domain Decides, Persistence Stores)

Status: ACCEPTED
Created: 2026-09-06
Last status change: 2026-09-06
Supersedes: None
Superseded by: None

## Context

`ARCHITECTURE.md` already defines responsibility boundaries *between* Playhoot business domains (Component Model, Domain Boundaries, Dependency Principles), but had no accepted rule for how responsibility is organized *within* a single business domain.

The question surfaced concretely in the Game domain, where `game/game/internal/businessservice` was introduced as a single generic package intended to hold "business validation and business behavior" for any use case in that package. Discussion of whether this pattern should become a required Engineering Standard exposed a broader architectural question: what determines whether behavior belongs in a use case/workflow, in domain code, in a capability-specific domain package, or in a repository, and whether a generic business-service layer should be mandatory.

## Decision

Within a Playhoot business domain:

- Application (use cases/workflows) coordinates execution.
- Domain code makes business decisions.
- Persistence code retrieves and stores data.

Business rules, invariants, state transitions, calculations, validations, and reusable business decisions are independent from persistence, transport, monitoring, transactions, GORM, or other infrastructure concerns whenever practical.

Use cases and workflows are allowed application control flow (loading state, sequencing steps, branching on results). The goal is not to remove every conditional from them; it is that reusable business decisions must not be owned by a use case/workflow merely because that is where they are invoked.

A mandatory generic `businessservice` layer is rejected. Domain behavior should live as close as possible to the concept that owns it, in this preferred order:

1. A method on the domain type/value object that naturally owns the behavior (e.g. `Visibility.IsPlayable()`).
2. A pure, package-level domain function when no single type owns the behavior — explicit inputs, explicit outputs, no persistence/I/O.
3. A capability-specific domain package (e.g. `playability`, `publishing`) only when a cohesive decision spans several domain concepts and does not naturally belong to one type/function. A new struct/service abstraction is not required merely because business logic exists.

Package names should describe the business capability the code implements, not a generic technical bucket (`businessservice`) or an unqualified abstraction-kind name (a catch-all `policy/`).

Types are owned by the concept they represent (domain, use case, workflow, repository, or transport), not duplicated per policy/service. A capability package may consume an existing domain-owned type directly.

Repository interfaces remain narrow and consumer-driven, per the existing dependency principles in `ARCHITECTURE.md`; they must not own business policy. This does not require one repository implementation per use case — several narrow contracts may be satisfied by shared infrastructure.

This is a responsibility/dependency boundary, not a requirement to introduce literal `/domain`, `/application`, `/infrastructure` top-level directories.

## Rationale

Domain logic tied to persistence/transport/monitoring machinery is harder to test, reuse, and reason about independently of infrastructure. At the same time, forcing all business logic through one generic layer disconnects it from the domain language that should make it discoverable, and risks becoming an unstructured dumping ground disconnected from the concepts it operates on. Keeping behavior close to the concept it belongs to, and reserving a capability-specific abstraction for decisions that genuinely span multiple concepts, keeps the domain package readable without prescribing a service/abstraction for every function.

## Alternatives Considered

### Mandatory generic `businessservice` (or equivalent generic domain-service) layer

Rejected. Was already present in the Game domain (`game/game/internal/businessservice`) as an early, unvalidated experiment. A single generic bucket for "business behavior" does not scale with domain-language readability and tends to accumulate unrelated rules rather than expressing them near the concepts they belong to.

### Mandatory Clean-Architecture-style `/domain`, `/application`, `/infrastructure` top-level directories

Rejected as a universal requirement. The responsibility boundary is about ownership and dependency direction, not physical folder shape. Forcing the existing capability-oriented layout (e.g. `game/game/usecases/`) into a generic three-folder scheme would not add clarity and would conflict with the capability/domain-language package-naming direction adopted here.

### Status quo (no explicit intra-domain rule)

Rejected. Left the placement of business logic ad hoc and left `businessservice` in an unclear status (experiment vs. canonical pattern), which the discussion that produced this decision needed to resolve.

## Consequences

- Contributors have an explicit, ordered rule for where new business logic belongs, reducing case-by-case debate.
- `game/game/internal/businessservice` is explicitly not canonical; it is not removed by this decision and requires no immediate action.
- Some judgment remains in distinguishing "domain decision" from "application sequencing step," and in deciding when a capability package is warranted; this is expected to be resolved through code review rather than tooling.
- No repository-wide refactor is authorized by this decision.

## Canonical Knowledge Impact

- `ARCHITECTURE.md` — adds an "Intra-Domain Responsibility Boundary" section stating this boundary and the domain-logic placement preference order as accepted global architecture.
- `docs/engineering/standards/domain-logic-placement.md` — new engineering standard recording concrete package-organization, naming, type-ownership, and migration conventions implementing this boundary.

## Implementation Impact

None mandated. Existing code, including `game/game/internal/businessservice`, migrates opportunistically: when touched, or when a focused change makes migration worthwhile. See `docs/engineering/standards/domain-logic-placement.md` for the recorded migration strategy (OPPORTUNISTIC MIGRATION). No repository-wide migration work is routed by this record.
