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
// request log: it only declares its endpoints (Route.go's Route: a
// pattern, a handler, and any route-specific middlewares), split by
// transport shape (RESTRoutes, WebSocketRoutes - an SSERoutes will join
// them once anything needs it). Server is what actually registers them
// and wraps every one with the observability behavior every endpoint of
// that transport shape needs, uniformly - a route group cannot forget to
// add it, because it never had the chance to.
//
// The session route group is currently a transport skeleton with no
// domain dependency at all (see api/session's doc comment), so NewServer
// takes no arguments today - that changes once a real dispatch layer
// exists behind it again.
package api

import (
	"log/slog"
	"net/http"

	apisession "github.com/diegobermudez03/playhoot/api/session"
	"github.com/diegobermudez03/playhoot/logging"
)

// routeGroup exposes one workflow/feature's endpoints, split by transport
// shape. Either method may return an empty slice.
type routeGroup interface {
	RESTRoutes() []Route
	WebSocketRoutes() []Route
}

// Server composes every route group into one HTTP handler.
type Server struct {
	mux *http.ServeMux
}

// NewServer constructs a Server exposing every endpoint every route group
// this package knows about declares, registering each one onto a shared
// mux and wrapping it with the observability behavior its transport shape
// requires (see restObservability/wsObservability).
func NewServer() *Server {
	mux := http.NewServeMux()
	groups := []routeGroup{
		apisession.NewHandler(),
	}
	for _, group := range groups {
		for _, route := range group.RESTRoutes() {
			// Observability goes first (outermost), so it sees every
			// request/response regardless of what a route-specific
			// middleware does to it.
			mws := append([]Middleware{restObservability(route.Pattern)}, route.Middlewares...)
			mux.HandleFunc(route.Pattern, chain(mws, route.Handler))
		}
		for _, route := range group.WebSocketRoutes() {
			mws := append([]Middleware{wsObservability(route.Pattern)}, route.Middlewares...)
			mux.HandleFunc(route.Pattern, chain(mws, route.Handler))
		}
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
func restObservability(pattern string) Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			ctx := logging.Start(r.Context())
			defer logging.FinishRequestLog(ctx, slog.Default(), pattern)
			next(w, r.WithContext(ctx))
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
func wsObservability(pattern string) Middleware {
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
