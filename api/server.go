// Package api is the external HTTP/WebSocket edge of the system. Endpoints
// are declared by route groups (one per workflow) and registered by Server,
// which wraps every route with the observability its transport shape needs.
package api

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/diegobermudez03/playhoot/api/internal/live"
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
	mux  *http.ServeMux
	live *live.Controller
}

// NewServer constructs a Server exposing every route group's endpoints.
// webSocketOriginPatterns are the extra origin hosts (path.Match patterns)
// browser pages may open live connections from.
func NewServer(authorizer authorizer, sessionCreator sessionCreator, owners ownerLocator, admins adminAttacher, webSocketOriginPatterns []string) *Server {
	mux := http.NewServeMux()
	liveConnections := live.New()
	groups := []routeGroup{
		apiplay.NewHandler(authorizer, sessionCreator, owners, admins, liveConnections, webSocketOriginPatterns),
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
	return &Server{mux: mux, live: liveConnections}
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

// wsObservability starts and flushes the request log of a WebSocket route's
// handshake. Handlers return once the connection is admitted and running, so
// the line is written then; the connection logs its own end under the same
// trace.
func wsObservability(pattern string) routing.Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			ctx := logging.Start(r.Context())
			defer logging.FinishRequestLog(ctx, slog.Default(), pattern)
			next(w, r.WithContext(ctx))
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

// Shutdown closes every live connection and waits for them until ctx is done.
// Call it after the HTTP server stopped accepting requests.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.live.Shutdown(ctx)
}

// Routes returns Server's HTTP handler.
func (s *Server) Routes() http.Handler {
	return s.mux
}
