# Session Runtime

Status: CANONICAL DOMAIN MODEL - UNDER REDESIGN

## Responsibility

Session Runtime owns the runtime execution of Playhoot game sessions, for both discrete and continuous games.

## Direction

Session Runtime is being rebuilt as a real-time runtime (`session/docs/decisions/SESSION-ADR-0028-real-time-session-runtime-supersedes-discrete-turn-architecture.md`): one real-time controller per live session, holding that session's state and messages in memory with an open connection to the authored JavaScript script, executed inside Session Runtime's own process. Its persistence is no longer the authoritative record of every interaction.

The detailed model (what it owns, its entities, persistence, protocol, recovery) is not yet defined and is intentionally not documented here. Until accepted decisions say otherwise, nothing in this directory is a commitment about it.

## Live Session Ownership

An active Session has exactly one owning Runtime instance. Ownership (locate-or-claim, epoch, release, and later lease and heartbeat) is the Runtime's; getting a request to the owner is the API edge's (`play/docs/decisions/SESSION-ADR-0029-live-session-ownership-and-owner-routing.md`, which also lists what must exist before more than one instance may own Sessions).

## Does Not Own

- Transport/network connections (API owns the transport edge).
- Identity/profile ownership.
- Public discovery/search experience.
- Authored games, their metadata, and visibility/publication state (Game Management, `game/README.md`).

## Internal Structure

Session Runtime is an independent top-level package/business bounded context, a sibling to Game Management (`game/`), `identity/`, `composer/`, and `orchestrator/` (`docs/decisions/architecture/ADR-0014-management-session-domain-split.md`).

The previous implementation was removed. Current layout:

```text
play/
  session.go                  # Session public contract (CreateSessionOutcome, CreateSessionResult, ErrIdempotencyConflict)
  docs/                       # DATA_MODEL.md, FLOWS.md, decisions/
  runtime/
    runtime.go                # live Runtime public contract (OwnerPlacement, Connection channels, message catalog, AttachAdminResult)
    sandbox/                  # QuickJS-on-WebAssembly script execution in an isolated worker process; not wired to anything yet
    usecases/
      ownership/              # LocateOrClaimOwner (placeholder: single instance, no registry)
    workflows/
      liveconnection/         # Manager.AttachAdmin (placeholder: admits everyone)
  session/
    migration/                # exposes Migrate(db) and Seed(db) (test data) to the application's migrations.go
    usecases/                 # single-capability use cases (empty)
    workflows/
      sessionlifecycle/       # Manager + one file per step (step_create_session.go) + internal/repo
    internal/
      business/<behavior>/    # business definitions and pure logic shared by steps (sessionstatus, gameversion)
      migrations/             # schema migrations (play_* tables) and seed_* test-data migrations
      objectstore/            # object storage port (GCS, in-memory) and verifying cache; not wired, storetest does not build
      pgerrs/                 # PostgreSQL error classification helpers
      testdb/                 # disposable test database for this domain (packages call testdb.Run from TestMain)
```

`sandbox` and `objectstore` are kept as reusable code without a current caller. Tables are prefixed `play_` and have no foreign keys (references are indexed and kept correct by the application).

Package placement and layering rules are in `docs/engineering/standards/domain-logic-placement.md`.
