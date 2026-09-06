# Composer

Composer is the coordination layer for cross-domain reads.

- Coordinates composite read operations across business domains.
- Is stateless with respect to business/domain state.
- Invokes domain read capabilities and composes the result.
- May contain composition/read-coordination logic.
- Owns no business entities.
- Owns no domain business rules.

Illustrative example only: a composite read may need to consider whether data read from different domains represents a consistent enough view. Some future composition might use version/snapshot information if participating domains support it. This is not a global requirement that every domain support historical or versioned reads.

See `../ARCHITECTURE.md` for global architecture rules.
