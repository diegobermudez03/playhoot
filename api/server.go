// Package api is the external transport/application edge for the whole
// system: every user-facing HTTP/WebSocket endpoint lives here, decoding/
// encoding wire payloads and forwarding decoded commands to whichever
// capability owns them - a single domain directly, or a cross-domain
// coordination layer when a request spans more than one. It owns no
// business state and no connection registry of its own.
//
// Endpoints are organized into route groups by workflow/feature (the
// session workflow's group lives in api/session) rather than registered
// in one place, since this package is expected to keep accumulating
// endpoints as more workflows are exposed. A route group does not need a
// one-to-one relationship with a single business domain - a group may
// call more than one capability, or a cross-domain coordination layer -
// but each one is still registered and testable on its own.
//
// A route group never touches an *http.ServeMux or starts its own
// request log: it only declares its endpoints (routing.Route: a
// pattern, a handler, and any route-specific middlewares), split by
// transport shape (RESTRoutes, WebSocketRoutes - an SSERoutes will join
// them once anything needs it). Server is what actually registers them
// and wraps every one with the observability behavior every endpoint of
// that transport shape needs, uniformly - a route group cannot forget to
// add it, because it never had the chance to.
//
package api

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/diegobermudez03/playhoot/api/internal/routing"
	apisession "github.com/diegobermudez03/playhoot/api/session"
	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/logging"
	sessionpkg "github.com/diegobermudez03/playhoot/play"
)

// routeGroup exposes one workflow/feature's endpoints, split by transport
// shape. Either method may return an empty slice.
type routeGroup interface {
	RESTRoutes() []routing.Route
	WebSocketRoutes() []routing.Route
}

// authorizer is Identity as this package needs it: token and operation in,
// the caller's UserUUID and whether the operation is allowed out.
type authorizer interface {
	Authorize(ctx context.Context, token string, operation identity.Operation) (string, identity.AccessDecision, error)
}

// sessionCreator is whatever coordinates Session creation (Orchestrator in
// production) as this package needs it.
type sessionCreator interface {
	CreateSession(ctx context.Context, userUUID string, gameUUID string, idempotencyKey string) (sessionpkg.CreateSessionResult, error)
}

// Server composes every route group into one HTTP handler.
type Server struct {
	mux *http.ServeMux
}

// NewServer constructs a Server exposing every endpoint every route group
// this package knows about declares, registering each one onto a shared
// mux and wrapping it with the observability behavior its transport shape
// requires (see restObservability/wsObservability). authorizer is Identity
// (token -> UserUUID and operation access) and sessionCreator whatever
// coordinates Session creation (Orchestrator in production); this package
// depends only on its own narrow ports for them, never on orchestrator or
// a domain's implementation directly.
func NewServer(authorizer authorizer, sessionCreator sessionCreator) *Server {
	mux := http.NewServeMux()
	groups := []routeGroup{
		apisession.NewHandler(authorizer, sessionCreator),
	}
	var restRoutes, wsRoutes []routing.Route
	for _, group := range groups {
		restRoutes = append(restRoutes, group.RESTRoutes()...)
		wsRoutes = append(wsRoutes, group.WebSocketRoutes()...)
	}

	var restRequestMetrics *restMetrics
	if len(restRoutes) > 0 {
		patterns := make([]string, len(restRoutes))
		for i, route := range restRoutes {
			patterns[i] = route.Pattern
		}
		restRequestMetrics = newRESTMetrics(patterns)
	}

	for _, route := range restRoutes {
		// Observability goes first (outermost), so it sees every
		// request/response regardless of what a route-specific
		// middleware does to it.
		mws := append([]routing.Middleware{restObservability(route.Pattern, restRequestMetrics)}, route.Middlewares...)
		mux.HandleFunc(route.Pattern, chain(mws, route.Handler))
	}
	for _, route := range wsRoutes {
		mws := append([]routing.Middleware{wsObservability(route.Pattern)}, route.Middlewares...)
		mux.HandleFunc(route.Pattern, chain(mws, route.Handler))
	}
	return &Server{mux: mux}
}

// restObservability is the one place an ordinary request/response
// endpoint's request log starts and flushes: it begins on the way in and
// flushes once the handler returns, named after pattern - the same
// http.ServeMux pattern the route is registered under. No handler needs
// to call logging.Start/FinishRequestLog itself; every handler can assume
// ctx already carries a started log, because this wrapper is the only
// place in the whole request path that ever starts one.
//
// Once the handler returns, and before the log flushes, it also measures the
// request under its route: how long it took and the status code the handler
// wrote. Every route therefore gets request count and duration without
// declaring anything, and the measurements are written into the same request
// log.
func restObservability(pattern string, requestMetrics *restMetrics) routing.Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			ctx := logging.Start(r.Context())
			defer logging.FinishRequestLog(ctx, slog.Default(), pattern)

			started := requestMetrics.now()
			recorder := newStatusRecorder(w)
			next(recorder, r.WithContext(ctx))
			requestMetrics.record(ctx, pattern, recorder.status, requestMetrics.now().Sub(started))
		}
	}
}

// wsObservability logs exactly two connection-lifecycle lines - one when
// the request arrives (flushed immediately, not deferred until the
// long-lived handler eventually returns, since that could be minutes or
// hours away), one when it does - both named after pattern and sharing
// one trace ID. A WebSocket connection carries many separate logical
// exchanges over its life (the join handshake, each inbound message), not
// one - so this wrapper deliberately logs only the two lines that bound
// the connection's own lifetime; every exchange in between is the
// handler's own responsibility to log, each as its own Start/
// FinishRequestLog pair, which will share this same trace ID since it
// runs against a context this wrapper already attached one to.
func wsObservability(pattern string) routing.Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			ctx := logging.Start(r.Context())
			logging.LogFields(ctx, logging.Field("event", "opened"))
			logging.FinishRequestLog(ctx, slog.Default(), pattern)

			defer func() {
				closeCtx := logging.Start(ctx)
				logging.LogFields(closeCtx, logging.Field("event", "closed"))
				logging.FinishRequestLog(closeCtx, slog.Default(), pattern)
			}()

			next(w, r.WithContext(ctx))
		}
	}
}

// Routes returns Server's HTTP handler.
func (s *Server) Routes() http.Handler {
	return s.mux
}
