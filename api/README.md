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

- `api/server.go` - the top-level `Server`/`NewServer()`/`Routes()`. It composes every route group's declared endpoints onto one `http.ServeMux`, and constructs each group's own dependency itself. `NewServer` currently takes no arguments: the session route group is a transport skeleton with no dependency of its own to build (see below) - that changes once a real dispatch layer exists behind it again.
- `api/<workflow>/` - one subpackage per route group (e.g. `api/session/`), each owning its own handlers and wire DTOs. A route group does not need a one-to-one relationship with a single business domain: it may call a single domain capability directly, or a cross-domain coordination layer (Composer/Orchestrator), depending on what its endpoints actually need - but it is still registered, tested, and reasoned about as one independent unit.
- `api/internal/routing/` - the shared `Route`/`Middleware` vocabulary a route group and `Server` both need, without either importing the other (a route group lives *under* `api/`, and `api` composes every route group, so a direct import either way would cycle).
- `api/internal/httpx/` - small HTTP response mechanics (JSON encoding) shared across every route group's handlers; protocol plumbing, not business behavior.

Adding a new workflow's endpoints means adding a new `api/<workflow>/` package with its own `Handler`, and wiring it inside `api.NewServer` from whatever shared infrastructure it needs - not adding more branches to an existing file, and not handing `main.go` the job of assembling each workflow's own dependency chain. `api` is reasoned about as if it were its own deployed service: it owns knowing how to construct or reach every capability it talks to, the same way a real gateway service would own its own downstream-client bootstrap rather than receive that already assembled.

### A route group only declares endpoints; Server registers and wraps them

A route group never touches an `*http.ServeMux` and never starts its own request log. It exposes two methods (an `SSERoutes()` will join them once anything needs Server-Sent Events), each returning a slice of `routing.Route{Pattern, Handler, Middlewares}`:

- `RESTRoutes()` - ordinary request/response endpoints.
- `WebSocketRoutes()` - long-lived connection endpoints.

`Server.NewServer` registers every route from every group onto the shared mux, wrapping each one with the observability behavior its transport shape needs *before* any route-specific middleware the group itself declared (see `docs/engineering/standards/logging.md`'s "Centralized Observability" section for exactly what that behavior is and why REST and WebSocket need different versions of it). This is the whole reason a route group returns declarations rather than registering itself directly: a route group cannot forget to add observability, because it never had the chance to decide whether to - and the same mechanism is where a future cross-cutting concern (metrics, for example) gets added once, for every endpoint, rather than copied into every handler.

A route's own `Middlewares` (`[]routing.Middleware`, each `func(http.HandlerFunc) http.HandlerFunc`) are for concerns specific to that one endpoint - an authorization check some routes need and others don't, a rate limiter, and so on - layered *inside* the observability wrapper, in declared order (the first middleware in the list is the outermost, so it is the first to run on the way in). No route declares any today; the mechanism exists so one can be added to a single endpoint without inventing a new registration path when the need arises.

## The session route group

`api/session` is the session workflow's route group, and today the only one. It is currently a **transport skeleton with no domain coupling**: it registers the same endpoints and keeps the same connection-upgrade mechanics already proven out, but calls no domain package - there is nothing to call yet. This is deliberate, not an oversight: a prior pass built a stateful live-session coordinator here on top of Session Runtime output-handling that was both incomplete and already known to change; rather than keep building on that foundation, this route group was reduced back to its transport plumbing until Session Runtime's own engine-Output handling is actually complete, at which point a real dispatch layer is rebuilt on top of it once, correctly (`docs/projects/active/session-runtime-v1/works/WORK-0005-thin-live-coordinator.md`, `docs/projects/active/session-runtime-v1/PROJECT.md`).

Concretely today: `POST /sessions` (declared via `RESTRoutes()`) decodes and validates the wire request but always answers `501`. `GET /ws` (declared via `WebSocketRoutes()`) performs a real WebSocket upgrade - a client really gets a live socket, and the single write-pump-per-connection design is real, working code - but the "was this a valid Join" decision is a stub that always proceeds, and no message sent over the connection has a real handler; every message answers `ERROR`/"unknown message type". The connection's own open/close is logged generically by `Server`'s `wsObservability` wrapper; the join handshake's own outcome and each inbound message are still logged by `api/session` itself, exactly as `docs/engineering/standards/logging.md` describes.

See `../ARCHITECTURE.md` for global architecture rules.
