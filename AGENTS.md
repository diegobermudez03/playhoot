# Playhoot backend — technical entry for agents

Method, plan, product spec and current task live in the workspace one level up:
`../AGENTS.md`, `../docs/agent/METHOD.md`, `../docs/agent/STATE.md`. Read those
first. This file only holds what is specific to this repository.

## What this repository is

Go monolith, one deployable unit. Every top-level package is a domain that must
be able to become its own service.

- `api` — HTTP, WebSocket and MCP entry; request authentication; rate limits.
- `orchestrator` — writes that cross domains. `composer` — reads that cross domains.
- `identity`, `game`, `play` (`session`, `runtime`) — traditional domains.
- `utils`, `logging`, `monitoring` — shared technical packages. `third_party/qjs` — patched QuickJS.

The target domain tree and everything each domain owns is in the system
documentation site (`../docs/system/index.html`; this repo's data in `docs/system/`).

## Architecture rules

- A top-level domain reads and writes only its own tables. No foreign keys across
  domains; cross-domain references are public identifiers.
- No direct calls between top-level domains: only through `orchestrator` or `composer`.
  `api` may call a single-domain operation directly.
- No database transaction stays open across a call that leaves the domain.
- Exported signatures of a domain use only that domain's own types
  (`orchestrator` and `composer` are the exception).
- A subdomain is used only by its parent.
- Domain layout: `usecases/<verb>` (one independent operation),
  `workflows/<name>` (multi-step lifecycle, one `step_*.go` per step),
  `internal/business/<behavior>` (pure rules), `internal/repo`, `internal/migrations`.
- Every write has logical business limits owned by its domain. Capacity limits that
  depend on the plan arrive as parameters from the caller; the domain applies the
  stricter of received and own.
- Every row change is traceable (write-once columns or append-only change tables).
- Every query is scoped and backed by a declared index.
- Anything asynchronous goes through the shared transactional queue with dead-letter
  handling (to be built in H1); do not add ad-hoc queues.
- External providers sit behind a port with a real local or simulated implementation.
  Never write code meant to be thrown away at integration time.
- The HTTP endpoint metric reports only the HTTP result. The domain operation
  declares its own logical-outcome metric.

## Engineering standards

`docs/engineering/standards/INDEX.md` lists the canonical standards (testing,
repositories and SQL, data history, idempotency, error handling, logging, metrics,
object storage, database constraints, code comments, naming). They stay in force.
Key points: raw SQL for reads/updates with Go constants bound as parameters, never
literals; inserts with GORM `Create()`; no `CHECK`/FK; table-driven tests with
`map[string]func`; repository tests against a disposable real database; signed
URLs only when the backend never touches the bytes.

Before the first deployment, schema changes edit the original migration in place;
tell the creator when the local database must be recreated.

## Commands

| Action | Command |
|---|---|
| Build / vet | `go build ./...` · `go vet ./...` |
| Tests | `go test ./...` (repository tests skip without a database) |
| Tests with database | `docker compose up -d postgres`, export `TEST_DATABASE_HOST/PORT/USERNAME/PASSWORD/NAME/SSL_MODE`, then `go test ./...` |
| Mocks | `go generate ./...` |
| Run | `docker compose up -d` and `go run .` with `.env` from `.env.example`; game builds need `GAME_TOOLCHAIN_DIR=../playhoot_frontend` and Node |

`comment_standard_test.go` fails the build if a comment cites internal documents,
exceeds five lines, or leaks lower-layer names. Run the three commands above before
every commit.

## Historical decisions

`docs/technical-decisions/` keeps the accepted ADRs as a record of why the
runtime is the way it is. They are history, not current truth; new decisions
go to `../docs/agent/DECISIONS.md`. `ARCHITECTURE.md` holds the invariants
still in force.
