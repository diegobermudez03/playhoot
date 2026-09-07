# Cross-Domain Reference Naming Standard

Status: CANONICAL ENGINEERING STANDARD

This standard records naming conventions for persisted references to explicitly published cross-domain public entities. The architecture rule is owned by `ARCHITECTURE.md -> Cross-Domain Public Entity References` and its rationale is recorded in `docs/decisions/architecture/ADR-0005-cross-domain-public-entity-references.md`.

## Rule

For a persisted cross-domain reference, name the field/column after the public entity being referenced, not after the consumer's local role for it.

Examples:

- `user_uuid`
- `game_uuid`
- `organization_uuid`

When multiple references to the same public entity exist for different semantics, qualify the role while retaining the public entity name.

Examples:

- `created_by_user_uuid`
- `approved_by_user_uuid`

Avoid ambiguous names such as:

- `external_id`
- `reference_id`
- `actor_uuid`

when the value actually references a published `User` entity.

## Public Entity Must Be Accepted First

Do not prescribe a specific public entity name before the producing domain has accepted and documented that public entity.

For example, do not prescribe `user_uuid` for Session Runtime until Domain Design has accepted the public Identity entity name and semantics. If the public Identity entity is accepted as `Principal`, then a persisted cross-domain reference to it should be named from that public entity instead.

## Identifier Direction

A cross-domain referenceable/public entity must have a stable public UUID.

Internal-only entities should normally use local IDs unless they have another explicit reason to require globally stable identity.

Do not infer the inverse rule. A UUID does not by itself mean the entity is public/exported; public/referenceable status is an explicit domain contract.

## Enforcement

Enforcement is by canonical documentation and code review initially. Use type, package, and API structure where practical in future implementations.

No new linter or CI mechanism is required now.

## Migration Strategy

FUTURE CODE ONLY.

This standard does not authorize repository-wide migration or renaming of existing code.
