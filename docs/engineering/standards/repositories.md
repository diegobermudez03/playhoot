# Repository Standard

Status: CANONICAL ENGINEERING STANDARD

This standard owns concrete repository *implementation* conventions. For repository *contract ownership* (which consumer defines an interface, narrow consumer-driven contracts, workflow-step-owned persistence needs), see `domain-logic-placement.md -> Repository Contracts`.

## Repository Queries

- For GORM raw-query reads into structs, prefer `Scan(&value)` and verify `RowsAffected` instead of scanning every selected column manually.
- SQL column aliases should match destination struct fields through GORM's normal mapping rules.
- For repository implementation and repository test setup, prefer raw SQL for reads, updates, and deletes.
- Inserts are the exception: prefer GORM `Create()` for inserts so field mapping is explicit and does not depend on fragile positional argument ordering.

This standard does not define transaction, locking, pagination, isolation-level, or broader ORM-vs-SQL policy.

## Repositories May Know The Persistence Model

Repository abstraction exists so application behavior expresses persistent intent without orchestrating low-level storage representation unnecessarily. It is NOT required to make application code hypothetically independent of every possible future persistence technology or schema — do not design repository APIs around a hypothetical "we could replace Postgres with MongoDB without touching the application layer."

Repositories may know: the relational schema; tables/relationships; required ordering of persistence operations; IDs/FKs internal to the bounded context; and transactional persistence mechanics. Application code should express intent ("create this logical Session"); the repository decides how that accepted state/intent is represented atomically in storage (e.g. inserting a Session row, a host Actor row, and connecting their IDs). Application code should not need to manually orchestrate storage rows purely because repository methods were made CRUD-minimal.

## Multi-Table Persistence Encapsulation

A single behavior-oriented repository operation may perform several database statements when those statements are the persistence representation of one logical behavior. For example, `CreateSession(...)` may internally insert a Session, insert a host Actor, connect the host Actor, insert a JoinCode, and persist related request state — without exposing each statement as a separate repository capability to the application service. A repository operation can translate one logical persistence intent into multiple table operations.

However, repositories MUST NOT become owners of business policy. The application/domain layer decides WHAT must be true; persistence decides HOW that accepted state/intent is represented atomically in storage. Do not hide material business branching/policy inside a repository implementation merely to make an application service visually smaller.

Repository implementations use `repositories.md`'s conventions directly (raw SQL / GORM as described above). Do not introduce a mandatory `Service -> Repository -> Datastore -> GORM` layering, or any other ceremonial extra persistence layer, merely to claim storage independence. Introduce another persistence layer only when it has a concrete responsibility that independently justifies it.

## Sharing Rule: Protocols/Mechanics, Not Horizontal Entity APIs

Shared internal persistence infrastructure is appropriate when it represents a genuinely common technical/correctness mechanism where divergent implementations would be a bug or create significant risk — a *protocol*, not an entity's CRUD surface. Legitimate examples: per-Session locking (e.g. `internal/sessionlock`), idempotency-claim/replay mechanics (e.g. `internal/idempotency`), transaction mechanics, and database error classification (e.g. `internal/pgerrs`). These may live under shared internal packages, named for the invariant/mechanism they provide.

By contrast, do NOT create a horizontal entity-centric persistence package merely because several behaviors happen to perform similar CRUD/query operations against the same table(s) — for example a shared `internal/actors`, `internal/participants`, `internal/sessions`, or `internal/timers` package exposing Find/Create/Update-style methods consumed directly by multiple unrelated behaviors. That recreates entity-centric repository architecture beneath behavior-oriented application packages and defeats `domain-logic-placement.md`'s behavior-locality principle, even when introduced only to deduplicate code.

Heuristic: if divergence between two implementations would be legitimate (the behaviors may reasonably evolve independent query/locking/projection needs), sharing is optional and locality may be more valuable — keep the persistence code local to each behavior. If divergence would violate a shared protocol/invariant (for example, two different per-Session locking implementations), centralize the mechanism.

## Prefer Semantic Independence Over Premature DRY

Small persistence-code duplication is acceptable when the duplicated operations belong to behavior-local contracts whose semantics may evolve independently. For example, a Join step and a Leave step may each locally implement their own `findActor(...)`-equivalent query, even if the SQL is initially identical. This is acceptable when the code is small, the behavior-level semantics are independent, and future query/locking/projection requirements may diverge. Do not extract a shared abstraction solely because two pieces of SQL currently look alike — extract only when the underlying mechanism is genuinely shared (see Sharing Rule above).

## Private Mechanical Reuse Remains Allowed

This standard does not mean "duplication everywhere." A concrete persistence implementation may privately reuse low-level mechanics if that reuse does not create a horizontal entity API consumed directly by multiple behaviors. For example, a private helper used entirely inside one repository implementation is acceptable when it is truly mechanical, it does not become the semantic contract used by unrelated behaviors, and extracting it improves implementation clarity without hiding lifecycle/business intent. The relevant boundary is API/ownership coupling across behaviors, not merely whether two functions call the same private helper.

## Enforcement

Code review and this standard. No architecture linter or other automated enforcement tool is introduced by this standard.

During code review, flag in particular:

- a new entity-centric shared repository package (e.g. a horizontal `actors`/`sessions`/`participants`/`timers`-style package) created only to deduplicate CRUD/query code across otherwise-unrelated behaviors;
- a shared "common/general-purpose" persistence bucket introduced without a specific invariant/mechanism it exists to protect;
- a mandatory extra persistence layer (e.g. a ceremonial Datastore layer) added without a concrete responsibility that justifies it.
