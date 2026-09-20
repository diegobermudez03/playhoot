# API

API is the external transport/application edge for Playhoot.

- Exposes user-facing transport endpoints.
- Translates transport requests and responses.
- May handle transport-level concerns.
- May perform BFF response shaping for frontend needs.
- May call a single-domain capability.
- May call Composer for cross-domain reads.
- May initiate Orchestrator workflows for cross-domain writes.
- Owns no business state.

BFF response shaping is not the same as cross-domain read composition. If a request requires combining multiple domain reads, that composition belongs to Composer.

This README does not define Identity or authorization ownership.

## Organization: one edge, many route groups

`api` is *the* transport/application edge for the whole system - not one edge per domain. As more workflows/domains are exposed, their endpoints all land in this same package, so it is organized as a set of independent **route groups** rather than one flat list of handlers:

- `api/server.go` - the top-level `Server`/`NewServer(db *gorm.DB)`/`Routes()`. It composes every route group onto one `http.ServeMux`, and constructs each group's own dependency itself - a caller of `NewServer` supplies only infrastructure every group shares (today, a database handle), never a pre-built group or the routing mechanism.
- `api/<workflow>/` - one subpackage per route group (e.g. `api/session/`), each owning its own handlers, wire DTOs, and a `Handler.Register(mux)` method. A route group does not need a one-to-one relationship with a single business domain: it may call a single domain capability directly, or a cross-domain coordination layer (Composer/Orchestrator), depending on what its endpoints actually need - but it is still registered, tested, and reasoned about as one independent unit.
- `api/internal/httpx/` - small HTTP response mechanics (JSON encoding) shared across every route group's handlers; protocol plumbing, not business behavior.

Adding a new workflow's endpoints means adding a new `api/<workflow>/` package with its own `Handler`, and wiring it inside `api.NewServer` from whatever shared infrastructure it needs - not adding more branches to an existing file, and not handing `main.go` the job of assembling each workflow's own dependency chain. `api` is reasoned about as if it were its own deployed service: it owns knowing how to construct or reach every capability it talks to, the same way a real gateway service would own its own downstream-client bootstrap rather than receive that already assembled. Today that means `api/server.go` calls `play/sessionruntime.NewCoordinator(db)` to build the session route group's `play.Coordinator` - which does make `api`'s own build graph transitively include `game` packages (`go list -deps ./api` shows them), but `api`'s actual handler/business logic still never references a `game` type directly, only ever calling through `play.Coordinator`. `play`'s own dependency-inversion boundary is unaffected: `play`'s root package still imports no `game` package, and `game` still imports nothing from `play` or `api` (see `../play/README.md`).

## The session route group

`api/session` is the session workflow's route group, and today the only one: it hosts the WebSocket transport adapter for the Live Session Coordinator (`../play/`, see `../play/README.md`) - one HTTP upgrade handshake and one read-pump/write-pump goroutine pair per connection, decoding/encoding wire JSON and forwarding decoded client commands to `play`. `Create`/`Join` stay ordinary HTTP request/response (`POST /sessions`, `POST /sessions/join`); only `Start`/`AnswerInteraction` and their resulting fan-out ride the WebSocket (`GET /ws`). It holds no Session/connection registry and no business state of its own, and depends only on `play`'s exported API - never on any `game` package.

See `../ARCHITECTURE.md` for global architecture rules.
