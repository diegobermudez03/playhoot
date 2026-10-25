# Database Constraints Standard

Status: CANONICAL ENGINEERING STANDARD

Accepted by the human on 2026-10-25.

This standard applies to every schema migration.

## Rules

1. The database enforces only:
   - primary keys and unique constraints;
   - indexes, for performance;
   - each column's data type, chosen for its use case, and whether it may be NULL.
2. Every other rule is enforced by the application, in the layer that owns it (`domain-logic-placement.md`): allowed values and catalogs, value ranges and sizes, formats and patterns, conditions between columns, and references between tables. No `CHECK` constraints and no foreign keys.
3. Data read back that breaks an application rule is a data integrity failure (`data-integrity.md`), not something the schema prevents.
4. A migration's comment does not repeat the rules the application enforces; it points to the code that owns them when that helps.

## Scope

Future migrations follow this standard. Existing migrations that predate it are migrated only when the human directs it; the ones affected on 2026-10-25 are recorded in `docs/projects/active/session-runtime-v1/PROJECT.md`.
