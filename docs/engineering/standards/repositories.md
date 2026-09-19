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

A single repository operation may perform several database statements when those statements together represent one cohesive persistence/data-integrity operation — a structural invariant about what a validly persisted row set looks like, not an application-level orchestration decision. For example, `CreateSessionWithHost(...)` may internally insert the Session, insert its mandatory host SessionActor, and connect `sessions.host_actor_id` to it, because a successfully persisted Session without its mandatory host relationship is structurally invalid. A repository operation can translate one such logical persistence intent into multiple table operations without exposing each statement as a separate repository capability.

This does not extend to absorbing lifecycle/application orchestration merely because the writes happen to occur in the same transaction. Deciding that Create should also produce a JoinCode is workflow/lifecycle policy, owned by the Manager (see `domain-logic-placement.md -> Responsibility Categories`) — the Manager requests a separate `CreateJoinCode(...)`-style operation itself, inside the transaction it owns (see Transaction Ownership below), rather than `CreateSessionWithHost` creating the JoinCode on its behalf. Likewise, request-idempotency replay/conflict semantics belong to the Manager; a repository may expose persistence operations such as `ClaimSessionRequest(...)`/`CompleteSessionRequest(...)`, but the Manager explicitly orchestrates when each is called and what a claim/replay/conflict means (`idempotency.md -> Idempotency Policy Ownership`) — a repository operation must not itself own the entire idempotency-request lifecycle.

More generally, repositories MUST NOT become owners of business policy. The application/domain layer decides WHAT must be true; persistence decides HOW that accepted state/intent is represented atomically in storage. Do not hide material business branching/policy inside a repository implementation merely to make an application service visually smaller.

Repository implementations use `repositories.md`'s conventions directly (raw SQL / GORM as described above). Do not introduce a mandatory `Service -> Repository -> Datastore -> GORM` layering, or any other ceremonial extra persistence layer, merely to claim storage independence. Introduce another persistence layer only when it has a concrete responsibility that independently justifies it.

## Transaction Ownership

The workflow/service layer decides which operations constitute one logical atomic transaction — which repository calls, domain checks, and outcome recording must all commit or fail together. Persistence/infrastructure code performs the mechanical BEGIN/COMMIT/ROLLBACK; it does not decide transaction scope.

Concretely: the workflow should not manually call `db.Begin()`/`tx.Commit()`/`tx.Rollback()`, and should instead use a transaction helper conceptually equivalent to `WithinTransaction(ctx, func(txRepo Repository) error { ... })`, or an equivalent callback that supplies a transaction-scoped DB/repository handle. The exact API shape is implementation-local; the important split is that the workflow decides *what* belongs inside the transaction and infrastructure performs BEGIN/COMMIT/ROLLBACK.

This transaction helper is a function/mechanism the workflow calls, not necessarily an object the workflow controller must hold as an injected dependency. When a generic transaction helper already exists (for example a persistence-package `RunInDBTransaction[T](ctx, dbServicer, callback)`), a workflow controller should call it directly rather than introducing an additional interface/field (a `transactor`-style dependency) whose only purpose is to indirect into that already-generic helper. Introduce a dedicated transactor dependency only when it earns its place for an independent reason (for example, a test seam that a direct call cannot otherwise provide); do not add one merely as ceremony around an existing generic helper.

Repository methods consumed as part of a workflow transaction must operate using the caller-supplied transaction-scoped handle. They must not independently open and commit their own transaction — otherwise `operation A commits; operation B commits; operation C fails` could violate the workflow's intended logical atomicity. Read operations outside a transaction may use the ordinary DB handle. Do not introduce nested, autonomous repository-owned transactions as the normal mutation model.

### Callback Contract: Generic Result, No Outer-Variable Mutation

The transaction callback returns its result explicitly, the same way any other function should per `function-signatures.md -> Return Values`:

```go
result, err := RunInTransaction(
    ctx,
    repo,
    func(ctx context.Context, tx *gorm.DB) (T, error) {
        ...
        return value, nil
    },
)
```

Do not declare an outer-scoped result/error variable and mutate it from inside the callback (`var result T; RunInTransaction(func(...) error { result = ...; return nil })`) when the callback's own return value can express the same flow. The generic helper's `(T, error)` shape already carries the result out; closure mutation adds indirection without adding meaning.

The commit/rollback rule stays simple: the callback returning `(result, nil)` commits; returning `(zeroValue, err)` rolls back. If commit itself then fails, the transaction helper returns that commit error to the caller. There is no second, parallel error channel (a closure-captured `bizErr` variable, a `CommitWithError`-style method, or any other mechanism to let a business rejection escape separately from the callback's own return).

A business rejection inside a transaction is not automatically "the callback returned an error, so roll everything back." Some workflows require a durable outcome (an idempotency-request completion, a lazily-materialized expiration) to commit *together with* an otherwise-declining business decision. Per `error-handling.md -> Expected Business Outcome vs. Error`, an expected business decline is a normal result value, not necessarily an `error` — so the callback decides and persists the declined outcome, then returns it as an ordinary successful `(result, nil)` (where `result` carries the declined outcome, e.g. `JoinResult{Outcome: LobbyFull}`), letting the transaction commit normally. Reserve a non-nil callback error for cases the operation genuinely could not execute/evaluate (infrastructure failure, a violated invariant) — see `error-handling.md`.

## Repository Naming: Persistence Operations, Not Business Commands

Repository method names describe persistence/data operations; workflow/use-case method names describe business/lifecycle actions. Avoid repository methods named after the business command itself, such as `JoinSession`, `LeaveSession`, `ExpireLobby`, or `AdmitParticipant` — those names encode a business decision inside what should be a persistence-oriented name.

Prefer persistence-oriented names describing the data operation instead, for example (illustrative, not a fixed vocabulary): `FindSession...`, `LockSession...`, `CreateSessionWithHost`, `CreateJoinCode`, `FindActor`, `GetOrCreateActor`, `FindParticipant`, `CountActiveParticipants`, `CreateParticipant`, `ActivateParticipant`, `DeactivateParticipant`, `SetSessionTerminal`, `RevokeActiveJoinCode`, `ClaimSessionRequest`, `CompleteSessionRequest`. Exact names are implementation-local; the invariant is the naming *axis* — workflow methods name business/lifecycle actions, repository methods name persistence/data operations — not any specific vocabulary list.

This does not require CRUD-minimal repositories (see Multi-Table Persistence Encapsulation above): a persistence-oriented name may still encapsulate several statements, as long as the name describes what is persisted/mutated rather than the business decision that led to it.

## Sharing Rule: Protocols/Mechanics, Not Horizontal Entity APIs

Shared internal persistence infrastructure is appropriate when it represents a genuinely common technical/correctness mechanism where divergent implementations would be a bug or create significant risk — a *protocol*, not an entity's CRUD surface. Legitimate examples: per-Session locking (e.g. `internal/sessionlock`), idempotency-claim/replay mechanics (e.g. `internal/idempotency`), transaction mechanics, and database error classification (e.g. `internal/pgerrs`). These may live under shared internal packages, named for the invariant/mechanism they provide.

By contrast, do NOT create a horizontal entity-centric persistence package merely because several behaviors happen to perform similar CRUD/query operations against the same table(s) — for example a shared `internal/actors`, `internal/participants`, `internal/sessions`, or `internal/timers` package exposing Find/Create/Update-style methods consumed directly by multiple unrelated behaviors. That recreates entity-centric repository architecture beneath behavior-oriented application packages and defeats `domain-logic-placement.md`'s behavior-locality principle, even when introduced only to deduplicate code.

Heuristic: if divergence between two implementations would be legitimate (the behaviors may reasonably evolve independent query/locking/projection needs), sharing is optional and locality may be more valuable — keep the persistence code local to each behavior. If divergence would violate a shared protocol/invariant (for example, two different per-Session locking implementations), centralize the mechanism.

A shared protocol/mechanism package is meant to be called directly by the workflow that needs it — the same way a workflow calls a domain type's method — not indirected through a repository method that only forwards to it. Do not add a repository method whose entire body is a call to the shared package with no additional persistence responsibility of its own (for example, a `LockSessionByID` that only calls `sessionlock.LockByID`, or a `ClaimSessionRequest` that only calls `idempotency.Claim`); such a wrapper adds a layer of indirection without adding meaning. Call the shared mechanism package directly from the workflow step instead, and remove the forwarding method. A repository method remains justified when it does real persistence work beyond forwarding — for example, translating a shared package's result into a further mutation, or combining the shared call with other statements in the same operation.

## Naming Exposed To Workflow Code: Entities, Not Rows

A repository method's return type, as seen by workflow/application code, should be named for the concept the workflow reasons about, not for the storage mechanism that produced it. Prefer `session`, `lockedSession`, `actor`, `participant`, `request` over `row`-suffixed names such as `sessionRow`, `actorRow`, `participantRow`, `requestRow` in the workflow-facing contract and in the variables a workflow assigns from it:

```go
session, err := sessionlock.LockByID(ctx, tx, sessionID)
```

not

```go
row, err := sessionlock.LockByID(ctx, tx, sessionID)
```

This does not prohibit a private persistence struct named `sessionRow`/`participantRow`/`requestRow` *inside* a repository/storage package implementation — that naming is fine for a type that never crosses into workflow code. The rule concerns the contract exposed upward: workflow code should read as reasoning about entities/facts, not storage rows.

## Timestamp Ownership

Two different kinds of timestamp appear in persisted rows, and they have different owners.

**Audit/storage timestamps** — fields whose only purpose is persistence bookkeeping, such as `created_at`/`updated_at` — are populated by the repository, the ORM, or a DB default, not supplied by the workflow as an explicit `now` argument. A repository method whose only use for a caller-supplied `now` is to stamp `created_at`/`updated_at` should not take that parameter at all; the repository/ORM/DB decides that value. Workflow-facing repository return types should not expose audit timestamps upward by default, unless a real behavior genuinely needs to read one back.

**Semantic timestamps** remain explicit workflow input — do not generalize the rule above into "the repository owns all time." A timestamp that represents domain/lifecycle meaning (for example `lobby_expires_at`, `started_at`, `terminal_at`, `joined_at`, `left_at`, or `revoked_at` when it represents the logical revocation event) is workflow/domain policy and must be passed explicitly, with a parameter name that communicates its meaning (`joinedAt`, `leftAt`, `terminalAt`) rather than a generic `now` — even when the underlying value happens to come from the same clock reading as an audit timestamp elsewhere in the same call.

A single method may need to distinguish the two in the same call: a repository operation that both stamps `created_at` (audit — repository/DB-owned) and sets `terminal_at`/`revoked_at` (semantic — explicit workflow input) should accept the semantic timestamp as a named parameter and let the audit timestamp be handled internally, not accept one generic `now` that quietly serves both purposes.

## Prefer Semantic Independence Over Premature DRY

Small persistence-code duplication is acceptable when the duplicated operations belong to behavior-local contracts whose semantics may evolve independently. For example, a Join step and a Leave step may each locally implement their own `findActor(...)`-equivalent query, even if the SQL is initially identical. This is acceptable when the code is small, the behavior-level semantics are independent, and future query/locking/projection requirements may diverge. Do not extract a shared abstraction solely because two pieces of SQL currently look alike — extract only when the underlying mechanism is genuinely shared (see Sharing Rule above).

## Private Mechanical Reuse Remains Allowed

This standard does not mean "duplication everywhere." A concrete persistence implementation may privately reuse low-level mechanics if that reuse does not create a horizontal entity API consumed directly by multiple behaviors. For example, a private helper used entirely inside one repository implementation is acceptable when it is truly mechanical, it does not become the semantic contract used by unrelated behaviors, and extracting it improves implementation clarity without hiding lifecycle/business intent. The relevant boundary is API/ownership coupling across behaviors, not merely whether two functions call the same private helper.

## Enforcement

Code review and this standard. No architecture linter or other automated enforcement tool is introduced by this standard.

During code review, flag in particular:

- a new entity-centric shared repository package (e.g. a horizontal `actors`/`sessions`/`participants`/`timers`-style package) created only to deduplicate CRUD/query code across otherwise-unrelated behaviors;
- a shared "common/general-purpose" persistence bucket introduced without a specific invariant/mechanism it exists to protect;
- a mandatory extra persistence layer (e.g. a ceremonial Datastore layer) added without a concrete responsibility that justifies it;
- a workflow manually calling `Begin`/`Commit`/`Rollback`, or introducing an injected `transactor`-style dependency purely to indirect into an already-generic transaction helper, or a repository method opening/committing its own transaction while also being called as part of a workflow transaction (see Transaction Ownership above);
- a transaction callback that mutates an outer-scoped result/error variable instead of returning its result directly, or a separate `bizErr`/`CommitWithError`-style channel used to carry a business rejection out alongside the callback's own return (see Callback Contract above);
- a repository method named after a business command (`JoinSession`, `AdmitParticipant`, `ExpireLobby`) instead of the persistence operation it performs (see Repository Naming above);
- a repository method that only forwards to a shared protocol/mechanism package with no persistence responsibility of its own (see Sharing Rule above);
- a workflow-facing repository return type/variable named with a `Row` suffix instead of the entity/fact it represents (see Naming Exposed To Workflow Code above);
- a repository method parameter named `now` that is used only to populate an audit timestamp (`created_at`/`updated_at`) instead of being removed, or a semantic lifecycle timestamp passed as an unlabeled `now` instead of a meaningfully named parameter (see Timestamp Ownership above).
