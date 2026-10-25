# Database Constraints Standard

Status: CANONICAL ENGINEERING STANDARD

Accepted by the human on 2026-10-25; the writer and reader rules (5 and 6) were added by the human the same day.

This standard applies to every schema migration and to every piece of code that writes or reads persisted rows.

## Rules

1. The database enforces only:
   - primary keys and unique constraints;
   - indexes, for performance;
   - each column's data type, chosen for its use case, and whether it may be NULL.
2. Every other rule is enforced by the application, in the layer that owns it (`domain-logic-placement.md`): allowed values and catalogs, value ranges and sizes, formats and patterns, conditions between columns, and references between tables. No `CHECK` constraints and no foreign keys.
3. Data read back that breaks an application rule is a data integrity failure (`data-integrity.md`), not something the schema prevents.
4. A migration's comment does not repeat the rules the application enforces; it points to the code that owns them when that helps.
5. **Writers validate.** The code that creates or updates a row is responsible for checking that every value it writes is valid by the business rules (typed constants, catalog checks, ranges, conditions between columns). A table no code writes yet (for example, only seeds) needs nothing now; its future writers validate.
6. **Readers never assume validity.**
   - A reader that only cares whether a value is X checks only for X and does not validate the other values.
   - A reader that handles every possible value (for example a `switch` or map over every status, outcome, kind or catalog entry) is implicitly validating, so it has an explicit path for a value outside the known ones: it emits an alert through the monitoring abstraction and returns an explicit error, as a data integrity failure (`data-integrity.md`). It never treats an unexpected value as impossible, and never lets a `default` branch stand for a known value.

## Scope

Future migrations follow this standard. The existing migrations that predated it and had `CHECK` constraints were migrated on 2026-10-25, at the human's direction, by `20261027000000_play_drop_check_constraints`; no migration had foreign keys. The record is in `docs/projects/active/session-runtime-v1/PROJECT.md`.
