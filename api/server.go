// Package api is the external HTTP/WebSocket edge of the system. Endpoints
// are declared by route groups (one per workflow) and registered by Server,
// which wraps every route with the observability its transport shape needs.
package api

import (
	"log/slog"
	"net/http"

	"github.com/diegobermudez03/playhoot/api/internal/routing"
	apiplay "github.com/diegobermudez03/playhoot/api/play"
	"github.com/diegobermudez03/playhoot/logging"
)

// routeGroup exposes one workflow/feature's endpoints, split by transport
// shape. Either method may return an empty slice.
type routeGroup interface {
	RESTRoutes() []routing.Route
	WebSocketRoutes() []routing.Route
}

// Server composes every route group into one HTTP handler.
type Server struct {
	mux *http.ServeMux
}

// NewServer constructs a Server exposing every route group's endpoints.
// webSocketOriginPatterns are the extra origin hosts (path.Match patterns)
// browser pages may open live connections from.
func NewServer(authorizer authorizer, sessionCreator sessionCreator, owners ownerLocator, admins adminAttacher, webSocketOriginPatterns []string) *Server {
	mux := http.NewServeMux()
	groups := []routeGroup{
		apiplay.NewHandler(authorizer, sessionCreator, owners, admins, webSocketOriginPatterns),
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

// restObservability starts and flushes the request log of every REST route
// and records its request count and duration, so handlers never do either.
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

// wsObservability logs a WebSocket connection's opening and closing as two
// separate lines, since the connection may live for hours, and starts the
// handshake's own log because the route's middlewares log into it.
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

			handshakeCtx := routing.StartHandshakeLog(ctx, pattern)
			defer routing.FinishHandshakeLog(handshakeCtx)

			next(w, r.WithContext(handshakeCtx))
		}
	}
}

// chain wraps final in mws, mws[0] outermost, so the list reads in the order
// a request passes through it.
func chain(mws []routing.Middleware, final http.HandlerFunc) http.HandlerFunc {
	for i := len(mws) - 1; i >= 0; i-- {
		final = mws[i](final)
	}
	return final
}

// Routes returns Server's HTTP handler.
func (s *Server) Routes() http.Handler {
	return s.mux
}
