// Package api is the external HTTP/WebSocket edge of the system. Endpoints
// are declared by route groups (one per workflow) and registered by Server,
// which wraps every route with the observability its transport shape needs.
package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	apiidentity "github.com/diegobermudez03/playhoot/api/identity"
	"github.com/diegobermudez03/playhoot/api/internal/live"
	"github.com/diegobermudez03/playhoot/api/internal/routing"
	"github.com/diegobermudez03/playhoot/api/internal/throttle"
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

// Config tunes this instance's API edge.
type Config struct {
	// WebSocketOriginPatterns are the extra origin hosts (path.Match patterns)
	// browser pages may open live connections from.
	WebSocketOriginPatterns []string
	// TrustedProxyHops is how many proxies in front of the server append the
	// client address to X-Forwarded-For; 0 trusts none.
	TrustedProxyHops int
	// MaxLiveConnections bounds this instance's live connections; 0 takes the
	// default.
	MaxLiveConnections int
}

// NewServer constructs a Server exposing every route group's endpoints.
func NewServer(authorizer authorizer, guests guestCreator, sessionCreator sessionCreator, owners ownerLocator, admins adminAttacher, players playerAttacher, joinCodes joinCodeResolver, config Config) *Server {
	mux := http.NewServeMux()
	liveConnections := live.New(config.MaxLiveConnections)
	groups := []routeGroup{
		apiidentity.NewHandler(guests),
		apiplay.NewHandler(authorizer, sessionCreator, owners, admins, players, joinCodes, liveConnections, config.WebSocketOriginPatterns),
	}
	clients := throttle.ClientAddress{TrustedProxies: config.TrustedProxyHops}
	var restRoutes, wsRoutes []routing.Route
	for _, group := range groups {
		restRoutes = append(restRoutes, group.RESTRoutes()...)
		wsRoutes = append(wsRoutes, group.WebSocketRoutes()...)
	}

	var restRequestMetrics, handshakeMetrics *requestMetrics
	if len(restRoutes) > 0 {
		patterns := make([]string, len(restRoutes))
		for i, route := range restRoutes {
			patterns[i] = route.Pattern
		}
		restRequestMetrics = newRESTMetrics(patterns)
	}
	if len(wsRoutes) > 0 {
		patterns := make([]string, len(wsRoutes))
		for i, route := range wsRoutes {
			patterns[i] = route.Pattern
		}
		handshakeMetrics = newHandshakeMetrics(patterns)
	}

	for _, route := range restRoutes {
		// Observability goes first (outermost), so it sees every
		// request/response regardless of what a route-specific
		// middleware does to it.
		mux.HandleFunc(route.Pattern, chain(routeMiddlewares(route, restObservability(route.Pattern, restRequestMetrics), clients), route.Handler))
	}
	for _, route := range wsRoutes {
		mux.HandleFunc(route.Pattern, chain(routeMiddlewares(route, wsObservability(route.Pattern, handshakeMetrics), clients), route.Handler))
	}
	return &Server{mux: mux, live: liveConnections}
}

// routeMiddlewares are what a request to route passes through, in order:
// observability, the route's authorization, rate limiting, then the route's
// own middlewares. Limiting after authorization limits a caller as a user.
func routeMiddlewares(route routing.Route, observability routing.Middleware, clients throttle.ClientAddress) []routing.Middleware {
	limits := throttle.DefaultLimits
	if route.Limits != nil {
		limits = *route.Limits
	}
	mws := []routing.Middleware{observability}
	if route.Authorization != nil {
		mws = append(mws, route.Authorization)
	}
	mws = append(mws, throttle.Requests(limits, clients, time.Now))
	return append(mws, route.Middlewares...)
}

// restObservability starts and flushes the request log of every REST route
// and records its request count and duration, so handlers never do either.
func restObservability(pattern string, requestMetrics *requestMetrics) routing.Middleware {
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
// handshake and records how it ended (101 when upgraded) and how long it took.
// Handlers return once the connection is admitted and running; the connection
// logs and measures its own life.
func wsObservability(pattern string, handshakeMetrics *requestMetrics) routing.Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			ctx := logging.Start(r.Context())
			defer logging.FinishRequestLog(ctx, slog.Default(), pattern)

			started := handshakeMetrics.now()
			// The recorder unwraps to the real writer, so the upgrade can
			// still hijack the connection.
			recorder := newStatusRecorder(w)
			next(recorder, r.WithContext(ctx))
			handshakeMetrics.record(ctx, pattern, recorder.status, handshakeMetrics.now().Sub(started))
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
