# Session Runtime

Status: CANONICAL DOMAIN MODEL - UNDER REDESIGN

## Responsibility

Session Runtime owns the runtime execution of Playhoot game sessions, for both discrete and continuous games.

## Direction

Session Runtime is being rebuilt as a real-time runtime (`session/docs/decisions/SESSION-ADR-0028-real-time-session-runtime-supersedes-discrete-turn-architecture.md`): one real-time controller per live session, holding that session's state and messages in memory with an open connection to the authored JavaScript script, executed inside Session Runtime's own process. Its persistence is no longer the authoritative record of every interaction.

The detailed model (what it owns, its entities, persistence, protocol, recovery) is not yet defined and is intentionally not documented here. Until accepted decisions say otherwise, nothing in this directory is a commitment about it.

## Live Session Ownership

An active Session has exactly one owning Runtime instance. Ownership (locate-or-claim, epoch, release, and later lease and heartbeat) is the Runtime's; getting a request to the owner is the API edge's (`play/docs/decisions/SESSION-ADR-0029-live-session-ownership-and-owner-routing.md`, which also lists what must exist before more than one instance may own Sessions).

## Live Session Controller

Each Session this instance owns has one controller (`play/docs/decisions/SESSION-ADR-0030-the-session-controller-owns-its-live-execution.md`). It owns the Session's in-memory state, its Sandbox execution context and every live operation: lobby admission, SAMPLE ingress, COMMANDs, Start (including adopting a Session the database already has RUNNING), the running game's ticks, retries, restores and aborts. It calls the durable Session, the Sandbox and the replay recorder itself, and is the only component that talks to the Sandbox for its Session. Its state is locked only by itself.

From its creation until it ends, it has one main goroutine. Everything that touches the Sandbox or makes a durable write runs on it, so that goroutine is the Session's single mutation lane. Quick operations (admission, a SAMPLE, a disconnect, accepting a COMMAND) run on the caller's goroutine under a short lock. A SAMPLE or COMMAND arriving while a batch executes goes to the next batch; a COMMAND asks for that batch at once and its caller waits for the batch to commit.

The Runtime's entry point (the `liveconnection` ControllerResolver) only finds or creates a Session's controller when a connection arrives, without asking the durable Session anything, and stops every controller on shutdown. The controller decides the admission itself. After that, a connection's operations are bound directly to its controller.

A Session ends durably first, then live, always on its controller's main goroutine and without calling JavaScript, except a game its script finished: the game finishing, its creator cancelling, its deadlines (lobby expiry, maximum lifetime, inactivity of a running game) or the platform terminating it. Every connection then ends told the terminal status and cause, and the controller, its Sandbox context and its place in the resolver go away.

## Does Not Own

- Transport/network connections (API owns the transport edge).
- Identity/profile ownership.
- Public discovery/search experience.
- Authored games, their metadata, and visibility/publication state (Game Management, `game/README.md`).

## Internal Structure

Session Runtime is an independent top-level package/business bounded context, a sibling to Game Management (`game/`), `identity/`, `composer/`, and `orchestrator/` (`docs/decisions/architecture/ADR-0014-management-session-domain-split.md`).

The previous implementation was removed. Current layout:

```text
play/                         # a folder, not a Go package: each subpackage exposes its own types at its root
  docs/                       # DATA_MODEL.md, FLOWS.md, decisions/
  runtime/
    runtime.go                # live Runtime public contract (owner placement, attach results, admin and player connection operations and types)
    replay/                   # what a running game offers to its replay history
    sandbox/                  # QuickJS-on-WebAssembly execution contexts, one isolated worker process each; scriptcheck/ validates scripts for publication
    usecases/
      ownership/              # LocateOrClaimOwner (placeholder: single instance, no registry)
    workflows/
      liveconnection/         # the live Session: the ControllerResolver finds or creates each Session's controller; the controller runs everything else, one file per operation
    internal/
      business/               # pure logic shared by the controller's operations (checkpoint encoding, view patches)
  session/
    session.go                # durable Session public contract (CreateSession types, ErrIdempotencyConflict, AdminJoinEligibility)
    migration/                # exposes Migrate(db) and Seed(db) (test data) to the application's migrations.go
    usecases/
      checkadminjoin/         # EligibleToJoinAsAdmin + internal/repo
      checklivesession/       # IsSessionLive (asked by the Runtime's ownership before claiming) + internal/repo
    workflows/
      sessionlifecycle/       # Manager + one file per step (step_create_session.go) + internal/repo
    internal/
      business/<behavior>/    # business definitions and pure logic shared by steps (sessionstatus, gameversion)
      migrations/             # schema migrations (play_* tables) and seed_* test-data migrations
      objectstore/            # object storage port (GCS, in-memory) and verifying cache; not wired, storetest does not build
      pgerrs/                 # PostgreSQL error classification helpers
      testdb/                 # disposable test database for this domain (packages call testdb.Run from TestMain)
```

`sandbox/scriptcheck` has no caller yet. Tables are prefixed `play_` and have no foreign keys (references are indexed and kept correct by the application).

Package placement and layering rules are in `docs/engineering/standards/domain-logic-placement.md`.
