# Session Runtime

Status: CANONICAL DOMAIN MODEL - UNDER REDESIGN

## Responsibility

Session Runtime owns the runtime execution of Playhoot game sessions, for both discrete and continuous games.

## Direction

Session Runtime is being rebuilt as a real-time runtime (`session/docs/decisions/SESSION-ADR-0028-real-time-session-runtime-supersedes-discrete-turn-architecture.md`): one real-time controller per live session, holding that session's state and messages in memory with an open connection to the authored JavaScript script, executed inside Session Runtime's own process. Its persistence is no longer the authoritative record of every interaction.

The detailed model (what it owns, its entities, persistence, protocol, recovery) is not yet defined and is intentionally not documented here. Until accepted decisions say otherwise, nothing in this directory is a commitment about it.

## Does Not Own

- Transport/network connections (API owns the transport edge).
- Identity/profile ownership.
- Public discovery/search experience.
- Authored games, their metadata, and visibility/publication state (Game Management, `game/README.md`).

## Internal Structure

Session Runtime is an independent top-level package/business bounded context, a sibling to Game Management (`game/`), `identity/`, `composer/`, and `orchestrator/` (`docs/decisions/architecture/ADR-0014-management-session-domain-split.md`).

The previous implementation was removed. What remains is structure and reusable helpers:

```text
session/
  session.go                  # public contract package (empty for now)
  migration/                  # exposes Migrate(db) to the application's migrations.go
  usecases/                   # single-capability use cases (empty)
  workflows/
    sessionlifecycle/         # workflow layout: Manager + internal/repo (repo.go keeps the db-handle/transaction pattern)
  internal/
    storage/                  # schema documentation and migrations/ (the registry, with no migrations yet)
    sandbox/                  # QuickJS-on-WebAssembly script execution in an isolated worker process; helper functions, not wired to anything
    objectstore/              # object storage port with GCS and in-memory implementations, and a verifying cache; not wired to anything
    pgerrs/                   # PostgreSQL error classification helpers
    testdb/                   # disposable test database for this package
```

`sandbox` and `objectstore` are kept as reusable code without a current caller; how scripts and game content are loaded and executed in the new design is not decided.

Package placement and layering rules are in `docs/engineering/standards/domain-logic-placement.md`.
