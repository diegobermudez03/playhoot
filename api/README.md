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

- `api/server.go` - the top-level `Server`/`NewServer()`/`Routes()`. It composes every route group's declared endpoints onto one `http.ServeMux`. A route group that needs a dependency declares its own narrow port for it, and `NewServer` is where its construction is wired.
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

`api/session` is the session workflow's route group, and today the only one. It declares no endpoints yet: the previous session endpoints (create, content access, WebSocket) were removed with the previous Session Runtime implementation, and new ones are added as the real-time design is accepted.

See `../ARCHITECTURE.md` for global architecture rules.
