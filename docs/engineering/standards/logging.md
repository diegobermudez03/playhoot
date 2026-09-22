# Logging Standard

Status: CANONICAL ENGINEERING STANDARD

This standard owns where the `logging` package's per-request log (`logging.Start`/`logging.Step`/`logging.LogFields`/`logging.LogError`/`logging.FinishRequestLog`) is started and flushed, and what every other layer may assume about the context it is given. It does not define a detailed global logging architecture (see `error-handling.md -> Error Logging Boundary` for error-specific logging).

## The Problem This Solves

`logging.Step`/`logging.LogFields`/`logging.LogError` silently do nothing when the `context.Context` they are given was never passed through `logging.Start` - there is no panic, no error, nothing to notice. A package that faithfully calls `logging.Step` at every exported method, called from a caller that never started a log, produces zero log output and gives no signal that anything is wrong. This is easy to miss: the code looks complete, builds, and passes tests, while actually logging nothing in production.

## The Rule

**Only an entry-point package calls `logging.Start`.** An entry-point package is the boundary where a unit of work enters the currently-deployed unit from outside it - today, that is `api` (the external transport/application edge): each HTTP handler, and each individual WebSocket message a connection receives, is its own entry point and needs `logging.Start`/`defer logging.FinishRequestLog(...)` called before doing anything else. In `api` specifically, this is not each handler's own job to remember - `Server` (`api/server.go`) applies it uniformly to every registered route (see "Centralized Observability" below), so a handler is never the one deciding whether to call `Start`.

**Every other package - domain workflows/managers, coordination layers, adapters, and route-group handlers themselves - assumes the `context.Context` it receives already carries a started log**, and simply calls `logging.Step`/`logging.LogFields`/`logging.LogError` on it. It must not call `logging.Start` itself, and must not defensively check whether a log was already started.

## Centralized Observability (REST vs. WebSocket)

A route group (e.g. `api/session`) never calls `logging.Start` and never touches an `http.ServeMux` directly - it only declares its endpoints (`api/internal/routing.Route`: a pattern, a handler, any route-specific middlewares), split by transport shape (`RESTRoutes()`, `WebSocketRoutes()`). `Server` is what actually registers each one and wraps it with the observability behavior its transport shape needs, so a route group cannot forget to add it - it never had the chance to, and a future concern (metrics, for example) that needs to apply to every endpoint uniformly is added the same way, in `Server`, once.

The two transport shapes need different observability behavior, because a WebSocket connection is not a single request/response the way an ordinary endpoint is:

- **REST** (`api/server.go`'s `restObservability`): the familiar shape - `logging.Start` on the way in, `defer logging.FinishRequestLog` flushing once the handler returns, named after the route's own pattern (e.g. `"POST /sessions"`). A handler behind this never calls `Start`/`FinishRequestLog` itself.
- **WebSocket** (`wsObservability`): logs exactly two connection-lifecycle lines, both named after the route's pattern - one flushed immediately when the connection is accepted (not deferred until the handler eventually returns, since that could be minutes or hours away), one flushed once the handler does return. **Everything that happens on the connection in between - the join handshake's own outcome, each individual inbound message - remains the handler's own responsibility to log**, exactly as "A Long-Lived Connection Is Many Entry Points, Not One" below already describes; this wrapper deliberately does not try to capture any of it. This is a considered choice, not an oversight: centralizing the connection's own open/close bookend removes boilerplate every WebSocket route group would otherwise repeat, without forcing a generic wrapper to understand any one route group's own message semantics.

## Trace IDs and Spans

This package follows the same trace/span model as OpenTelemetry, Jaeger, Zipkin, and the W3C Trace Context standard - not a bespoke scheme - so it stays recognizable to anyone who already knows distributed tracing, and so nothing here needs reinventing if this system ever integrates a real tracing backend.

- **`trace_id`** identifies the whole causal chain - the entire lifetime of a WebSocket connection, for example - and is shared, unchanged, by every request log produced anywhere within it.
- **`span_id`** identifies one specific unit of work - one `Start` call, one flushed request log line - and is minted fresh every single time, never reused.
- **`parent_span_id`** links a span back to whichever span enclosed it, forming a tree under one shared trace. A root span (nothing enclosing it) has no `parent_span_id` at all.

`logging.Start` resolves all three every time it is called: it reuses the trace ID already on `ctx` if there is one (attached by an earlier `Start` call further up this same causal chain, or by `logging.WithTraceID`) or mints a fresh one; it always mints a fresh span ID for this call; and if `ctx` already carries a span ID from an enclosing `Start` call, that ID becomes this new span's `parent_span_id`. Before returning, it attaches its own span ID to the context it hands back, so anything `Start`-ed later against a context derived from it becomes that span's child.

This is what lets a long-lived WebSocket connection's many separate per-message request logs - each its own independent `Start`/`FinishRequestLog` pair, per the rule above - be correlated as one connection *and* stay individually identifiable: `wsObservability`'s own connection-level `Start` call is the root span (mints the trace ID, has no parent); every `Start` call the handler makes afterward (the join handshake's own log, each message's own log, the deferred close log) is that root span's direct child - same `trace_id`, each its own `span_id`, all sharing one `parent_span_id`. None of that code needs to know trace IDs or spans exist at all; it inherits both automatically from the context `wsObservability` already threaded through.

Today, in a single deployed process, none of this provides correlation value beyond what a single flushed log line already gives on its own - there is only one service, so there is nothing to correlate a request's log lines across yet. The design is deliberately forward-looking on two fronts: once one operation within a trace calls another separately-deployed service, that call would carry this trace's `trace_id` plus its own current span ID as the callee's `parent_span_id` (the same W3C Trace Context propagation shape, just not yet needed over the wire); and `logging.WithTraceID` is the mechanism an inbound request (already carrying a trace ID a caller generated) would use to make this process's own logs join that same correlation. Neither needs a `logging` redesign when the time comes - the mechanism already exists.

### Why domains must not guess

A domain package cannot know, from inside itself, whether it is being called from within the same compiled deployable unit (in which case the real entry point already started a log) or is itself about to become its own separately-deployed unit with its own endpoints (in which case it would own starting its own log, at its own new entry-point boundary, once it has one). Domain code that defensively starts its own log "just in case" would either fight the real entry point's log (creating two disconnected logs for one request) or mask the actual bug this standard exists to prevent - a missing `logging.Start` at the true entry point going unnoticed because the domain quietly compensated for it.

## What This Looks Like

An entry-point handler:

```go
func (h *Handler) handleCreateSession(w http.ResponseWriter, r *http.Request) {
    ctx := logging.Start(r.Context())
    defer logging.FinishRequestLog(ctx, slog.Default(), "api.session.CreateSession")

    // ... decode, log relevant received fields, call downstream code with ctx ...
}
```

Downstream code (a domain workflow, a coordination layer, an adapter) at any depth:

```go
func (m *Manager) Create(ctx context.Context, ...) (CreatedSession, error) {
    defer logging.Step(ctx, "SessionLifecycle.Create").Close()
    logging.LogFields(ctx, logging.Field("game_uuid", string(gameUUID)))
    // ...
}
```

Every `logging.Step` call anywhere in the call graph nests under the entry point's single flushed log line, as long as the same `ctx` is threaded through unchanged the whole way down - passing a fresh/derived context that drops the original value breaks this.

## A Long-Lived Connection Is Many Entry Points, Not One

A single WebSocket connection is not one unit of work for logging purposes, even though it stays open for a long time. Each message it receives is its own entry point: start a log, log the received message and its outcome, flush, before processing the next one. Do not start one log for the whole connection's lifetime - that would either never flush (the connection never "finishes") or bury an unbounded number of unrelated exchanges inside one log entry. A connection's own lifecycle events (opened/closed) are themselves logged the same way, each as their own single-line entry, separate from the per-message entries.

A server-initiated push that was not caused by a message received on that specific connection (for example, a fan-out to multiple recipient connections, or a write-pump goroutine with no owning request) has no single request context to log under - see "Standalone Logs" below for how this case is resolved.

## Standalone Logs (No Request Context)

Some code genuinely has no request to log under: a WebSocket write-pump goroutine has no request context at all, since it drains sends that can originate from another connection's own delivery mechanism as easily as from its own read pump; a fan-out mechanism that delivers to potentially many recipient connections at once (not only the one whose request produced what it is delivering) has the same problem, since no single one of those connections' own request logs can correctly own a per-recipient delivery failure.

`logging.LogStandalone(logger *slog.Logger, tags []string, message string, fields ...logField)` is the one place in the `logging` package a caller may log without a `ctx` carrying a started request log. It requires at least one tag - a fixed, low-cardinality value identifying where in the system the log came from (e.g. `"websocket"`, `"session"`, `"delivery"`) - because a standalone log already sits outside this package's normal per-request correlation, and a tag is what keeps it filterable/findable in aggregate instead of disappearing into an unstructured stream. A call with no tags still logs (under `"untagged"`), so a missing tag is visible in the log stream itself rather than a log silently going untagged.

`LogStandalone` exists specifically so no code in this repository ever reaches for the standard library's `log`/`fmt.Println` to report a failure - if a piece of code has something worth logging and no request context to log it under, `LogStandalone` is the answer, not a parallel unstructured logging path. `api/session/ws.go`'s write-pump is this rule's original caller; a stateful live-session coordinator this repository briefly had (since removed - see WORK-0005's Blocker 11) was its other original caller, and whatever eventually rebuilds that coordinator's own fan-out mechanism will need this same capability again.

## Enforcement / Migration

FUTURE CODE ONLY. No retroactive migration was needed: existing domain code (`game/session/workflows/sessionlifecycle.Manager`) already followed the "assume the context carries a log" half of this rule correctly - it had simply never been called from behind a real entry point that actually called `logging.Start`, since no such entry point existed until `api`'s HTTP/WebSocket handlers were implemented.

**2026-09-21 addendum**: an independent-review pass over WORK-0005 found `api/session/ws.go` and a stateful live-session coordinator this repository briefly had both logging a real failure (a WS write failure; a best-effort delivery failure) via the standard library's `log.Printf` instead of this package, because at the time no non-request-scoped logging path existed here - the exact gap this section's opening paragraph used to describe as unresolved. `logging.LogStandalone` was added to close it, and both call sites were migrated to use it. `main.go`'s process-bootstrap `log.Fatalf`/`log.Printf` calls (reading env vars, opening Postgres, starting the HTTP server - all before any request or `logging` machinery exists) were deliberately left unchanged, a different category (process lifecycle, not application/request logging) rather than an oversight.

**2026-09-21 addendum (centralization)**: per direct human feedback, moving `logging.Start`/`FinishRequestLog` out of individual handlers and into `Server`'s own per-route wrapping (see "Centralized Observability" above) closes the exact risk this standard's own "The Problem This Solves" section describes: before this change, a future new handler could still forget to call `Start`, exactly as `game/session/workflows/sessionlifecycle.Manager` once was called from behind no entry point at all before `api` existed. After this change, there is exactly one place in the whole `api` package that ever calls `Start` for an ordinary request, and exactly one place that does for a WebSocket connection's own open/close bookend - a new route group's handler cannot get this wrong, because it never decides whether to call `Start` at all. Trace IDs (see "Trace IDs" above) were added in the same pass, for the same reason a request log itself exists: to make a request's - or a long-lived connection's many separate requests' - path through the system reconstructable later, now and once this system is more than one deployed process.
