# Repository Standard

Status: CANONICAL ENGINEERING STANDARD

## Repository Queries

- For GORM raw-query reads into structs, prefer `Scan(&value)` and verify `RowsAffected` instead of scanning every selected column manually.
- SQL column aliases should match destination struct fields through GORM's normal mapping rules.
- For repository implementation and repository test setup, prefer raw SQL for reads, updates, and deletes.
- Inserts are the exception: prefer GORM `Create()` for inserts so field mapping is explicit and does not depend on fragile positional argument ordering.

This standard does not define transaction, locking, pagination, isolation-level, or broader ORM-vs-SQL policy.
