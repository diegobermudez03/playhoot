# Logging Standard

Status: CANONICAL ENGINEERING STANDARD

This standard owns where the `logging` package's per-request log (`logging.Start`/`logging.Step`/`logging.LogFields`/`logging.LogError`/`logging.FinishRequestLog`) is started and flushed, and what every other layer may assume about the context it is given. It does not define a detailed global logging architecture (see `error-handling.md -> Error Logging Boundary` for error-specific logging).

## The Problem This Solves

`logging.Step`/`logging.LogFields`/`logging.LogError` silently do nothing when the `context.Context` they are given was never passed through `logging.Start` - there is no panic, no error, nothing to notice. A package that faithfully calls `logging.Step` at every exported method, called from a caller that never started a log, produces zero log output and gives no signal that anything is wrong. This is easy to miss: the code looks complete, builds, and passes tests, while actually logging nothing in production.

## The Rule

**Only an entry-point package calls `logging.Start`.** An entry-point package is the boundary where a unit of work enters the currently-deployed unit from outside it - today, that is `api` (the external transport/application edge): each HTTP handler, and each individual WebSocket message a connection receives, is its own entry point and must call `logging.Start` (or a shared helper that does) and `defer logging.FinishRequestLog(...)` before doing anything else.

**Every other package - domain workflows/managers, coordination layers, adapters - assumes the `context.Context` it receives already carries a started log**, and simply calls `logging.Step`/`logging.LogFields`/`logging.LogError` on it. It must not call `logging.Start` itself, and must not defensively check whether a log was already started.

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

A server-initiated push that was not caused by a message received on that specific connection (for example, a future timer-driven event) is a different case this standard does not yet resolve - there is no such push implemented today.

## Enforcement / Migration

FUTURE CODE ONLY. No retroactive migration was needed: existing domain code (`game/session/workflows/sessionlifecycle.Manager`) already followed the "assume the context carries a log" half of this rule correctly - it had simply never been called from behind a real entry point that actually called `logging.Start`, since no such entry point existed until `api`'s HTTP/WebSocket handlers were implemented.
