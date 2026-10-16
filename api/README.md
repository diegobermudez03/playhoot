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
- `api/<workflow>/` - one subpackage per route group (e.g. `api/play/`), each owning its own handlers, wire DTOs and anything specific to its endpoints. Inside a route group, each endpoint's handler (with its DTOs) has its own file named after the endpoint (`create_session.go`, `admin_join.go`); `handler.go` only declares the `Handler`, its dependencies and its routes. A route group does not need a one-to-one relationship with a single business domain: it may call a single domain capability directly, or a cross-domain coordination layer (Composer/Orchestrator), depending on what its endpoints actually need - but it is still registered, tested, and reasoned about as one independent unit.
- `api/internal/routing/` - the shared `Route`/`Middleware` vocabulary (and the WebSocket handshake log) a route group and `Server` both need, without either importing the other (a route group lives *under* `api/`, and `api` composes every route group, so a direct import either way would cycle).
- `api/internal/httpx/` - small HTTP response mechanics (JSON encoding) shared across every route group's handlers; protocol plumbing, not business behavior.
- `api/internal/auth/` - the authorization middleware (see below).

Adding a new workflow's endpoints means adding a new `api/<workflow>/` package with its own `Handler`, and wiring it inside `api.NewServer` from whatever shared infrastructure it needs - not adding more branches to an existing file, and not handing `main.go` the job of assembling each workflow's own dependency chain. `api` is reasoned about as if it were its own deployed service: it owns knowing how to construct or reach every capability it talks to, the same way a real gateway service would own its own downstream-client bootstrap rather than receive that already assembled.

### A route group only declares endpoints; Server registers and wraps them

A route group never touches an `*http.ServeMux` and never starts its own request log. It exposes two methods (an `SSERoutes()` will join them once anything needs Server-Sent Events), each returning a slice of `routing.Route{Pattern, Handler, Middlewares}`:

- `RESTRoutes()` - ordinary request/response endpoints.
- `WebSocketRoutes()` - long-lived connection endpoints.

`Server.NewServer` registers every route from every group onto the shared mux, wrapping each one with the observability behavior its transport shape needs *before* any route-specific middleware the group itself declared (see `docs/engineering/standards/logging.md`'s "Centralized Observability" section for exactly what that behavior is and why REST and WebSocket need different versions of it). This is the whole reason a route group returns declarations rather than registering itself directly: a route group cannot forget to add observability, because it never had the chance to decide whether to - and the same mechanism is where a future cross-cutting concern (metrics, for example) gets added once, for every endpoint, rather than copied into every handler.

A route's own `Middlewares` (`[]routing.Middleware`, each `func(http.HandlerFunc) http.HandlerFunc`) are for concerns specific to that one endpoint - an authorization check some routes need and others don't, a rate limiter, and so on - layered *inside* the observability wrapper, in declared order (the first middleware in the list is the outermost, so it is the first to run on the way in). Today they are the authorization middleware and, on live routes, owner routing.

For a WebSocket route, the observability wrapper also starts the handshake's own request log (`routing.StartHandshakeLog`), since the handshake starts before the route's middlewares run; the handler flushes it once the Runtime decides admission (`routing.FinishHandshakeLog`), and the wrapper flushes it if nothing did.

## The play route group

`api/play` is the play domain's route group, and today the only one. It declares `POST /sessions` (create a Session) and `GET /sessions/{sessionUUID}/admin` (the creator's live admin connection); the other live endpoints are added as the real-time design is accepted.

`POST /sessions` takes `{game_uuid, idempotency_key}` and returns `201 {session_uuid, join_code, lobby_expires_at}`; `404` when the Game is not playable for the caller, `409` when the idempotency key was used for a different request, `400` for an invalid body. The creator is never part of the request: it is the caller Identity resolves from the bearer token.

`GET /sessions/{sessionUUID}/admin` is a WebSocket. Before the upgrade it authorizes the caller for `JOIN_SESSION_AS_ADMIN` and routes the Session to its owner; the client must offer the `playhoot.v1` subprotocol (`400` otherwise) from an allowed origin (`403` otherwise), and an invalid Session UUID is `400`, a Session that cannot be joined live `404`. After the upgrade the connection is attached to the Runtime, which decides admission: an open WebSocket is not yet an admitted one. The Runtime answers with a `runtime.Connection`, two channels of its own message types: `Inbound` (client to Runtime) and `Outbound` (Runtime to client). The Runtime never sees the socket. The route group runs one goroutine per direction (`live_connection.go`), mapping frames to and from the Runtime's message types (`live_messages.go`), until the Runtime sends a `Close` (the socket is closed with its reason) or the client leaves (`Inbound` is closed). A rejected connection is served the same way: its `Outbound` only carries the rejection and the `Close`. A frame that is not a client message never reaches the Runtime and ends the connection: binary `1003`, undecodable `1007`, unknown type `1008`, over 32 KiB `1009`. The message catalog grows as messages are needed; today no client message exists, so any message a client sends ends its connection.

### Live routes are routed to the Session's owner

Every live route declares the route group's owner routing middleware (`owner_routing.go`) after authorization (`play/docs/decisions/SESSION-ADR-0029-live-session-ownership-and-owner-routing.md`). It asks the Runtime for the Session's owner, claiming one when there is none, and lets the request through only when the owner is this instance; the handler reads the routed Session and its ownership epoch from the request context and passes the epoch to the Runtime. It lives in the play route group because it is specific to play Sessions and the Runtime's types. Forwarding to an owner elsewhere is not implemented (answered `503`): with a single instance it cannot happen.

Browser origins other than the server's own are allowed through `WEBSOCKET_ALLOWED_ORIGINS` (comma-separated host patterns, such as `app.example.com` or `*.example.com`; empty allows only the server's own origin).

### Authorization: a per-route middleware backed by Identity

A route that needs an authenticated caller declares `auth.Require(authorizer, operation)` (`api/internal/auth`) in its `Middlewares`. The middleware extracts the `Authorization: Bearer <jwt>` token and asks Identity (its own unexported `authorizer` port) for the caller's `UserUUID` and whether that caller may perform the route's named operation (`identity.Operation`); `401` for a missing/invalid token, `403` when Identity forbids the operation. The handler behind it reads the caller with `auth.UserUUID(ctx)`. The api never interprets the token.

A WebSocket route declares `auth.RequireWebSocket(authorizer, operation)` instead. A browser cannot set headers on a WebSocket handshake, so besides the `Authorization` header (read first), the token may be offered the way the Kubernetes API server accepts it: as the subprotocol `base64url.bearer.authorization.playhoot.<token as unpadded base64url>`, next to `playhoot.v1`. The server only ever selects `playhoot.v1`, so the token is never echoed back. A token subprotocol that cannot be decoded is `401` `malformed bearer token`. Rejections are answered before the upgrade.

See `../ARCHITECTURE.md` for global architecture rules.
