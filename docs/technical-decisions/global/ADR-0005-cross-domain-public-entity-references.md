# ADR-0005: Cross-Domain Public Entity References

Status: ACCEPTED
Created: 2026-09-06
Last status change: 2026-09-06
Supersedes: None
Superseded by: None

## Context

Session Runtime needs a way to correlate session-local actors to a future Identity-domain concept without taking ownership of Identity/Profile and without coupling Game persistence to Identity persistence. This exposed a reusable architecture rule: domains sometimes need to refer to another domain's stable business concept, but that reference must not become a database/table contract.

The same question appears anywhere one domain stores a reference to a concept owned by another domain, such as games, users/principals, organizations, or future business entities.

## Decision

A domain may explicitly publish certain logical entities as cross-domain referenceable entities.

For such an entity:

- it has a stable public UUID;
- the UUID identifies the logical entity, not a physical database row/table contract;
- the UUID is immutable/non-reusable as an identity;
- internal persistence may change without changing the public identity;
- consumers may persist that public UUID as a logical reference;
- consumers must not create cross-domain database foreign keys or directly access the producer's persistence as part of their domain behavior;
- consumers must not depend on the producer's internal table layout;
- the producer remains responsible for resolving the public identity to whatever internal representation it uses.

Cross-domain coupling to a published business concept is intentional and acceptable. Coupling to another domain's storage representation is not.

Public logical entity identifier and storage/table primary key contract are distinct concepts. The fact that an implementation happens to store the public UUID in a table column does not make that table part of the public contract.

A cross-domain referenceable/public entity must have a stable public UUID. Internal-only entities should normally use local IDs unless they have another explicit reason to require globally stable identity.

Do not define the inverse rule "every UUID means the entity is public/exported." Public/referenceable status is an explicit domain contract, not something inferred solely from identifier type.

Orchestrator may coordinate Identity and Session operations and persist workflow/saga state, idempotency, retries, and intermediate results. It must not become the permanent business source of truth for `global identity <-> SessionActor` merely because it has a database. If Session persistently needs correlation to a global identity, that relation belongs with business/domain state rather than workflow-state ownership.

## Rationale

Domains need stable ways to refer to each other's public business concepts without punching holes through domain ownership. A public UUID gives consumers a durable logical reference while preserving the producer's right to change tables, primary keys, storage layout, or resolution strategy.

The rule also prevents an overcorrection where every UUID-shaped value is assumed to be public. Public referenceability is a domain contract, not a type inference.

Keeping Orchestrator out of permanent identity mapping prevents workflow infrastructure from becoming accidental business state ownership.

## Alternatives Considered

### Cross-domain database foreign keys

Rejected. They couple domains to each other's physical storage and table lifecycle, even when the logical business relationship is valid.

### Direct repository/table access across domains

Rejected. It bypasses domain contracts and makes consumers depend on producer persistence implementation details.

### Orchestrator owns durable identity mapping

Rejected. Orchestrator may own workflow progress and idempotency state, but permanent business correlation belongs to the relevant business/domain state.

### Infer public/exported status from UUID usage

Rejected. Internal entities may have UUIDs for other reasons, and public entities may be represented internally in ways that change over time. Public referenceability must be explicit.

## Consequences

- Canonical domain documentation must explicitly identify any cross-domain referenceable public entity before other domains treat it as a public reference target.
- Consumers may persist public UUID references without cross-domain foreign keys.
- Public UUID field/column naming is governed by `docs/engineering/standards/cross-domain-reference-naming.md`.
- The future Identity Domain Design must decide the public Identity entity name and semantics before Session canonical documentation freezes a field such as `user_uuid`.

## Canonical Knowledge Impact

- `ARCHITECTURE.md` - adds the global cross-domain public entity reference rule.
- `docs/engineering/standards/cross-domain-reference-naming.md` - records concrete naming guidance for persisted cross-domain reference fields/columns.

## Implementation Impact

Future code should use public entity UUID references only for explicitly published cross-domain entities and should name persisted references according to the engineering standard. Migration strategy is FUTURE CODE ONLY; this record does not authorize repository-wide renaming or migration of existing code.
