// Package api is the external HTTP/WebSocket edge of the system. Endpoints
// are declared by route groups (one per workflow) and registered by Server,
// which wraps every route with the observability its transport shape needs.
package api

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/diegobermudez03/playhoot/api/internal/routing"
	"github.com/diegobermudez03/playhoot/api/internal/wsx"
	apisession "github.com/diegobermudez03/playhoot/api/session"
	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/logging"
	sessionpkg "github.com/diegobermudez03/playhoot/play"
	"github.com/diegobermudez03/playhoot/play/runtime"
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

// ownerLocator is the Runtime's live Session ownership.
type ownerLocator interface {
	LocateOrClaimOwner(ctx context.Context, sessionUUID string) (runtime.OwnerPlacement, error)
}

// adminAttacher is the Runtime's admission of admin connections.
type adminAttacher interface {
	AttachAdmin(ctx context.Context, sessionUUID string, userUUID string, ownerEpoch int64) (runtime.AttachAdminResult, error)
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
		apisession.NewHandler(authorizer, sessionCreator, owners, admins, webSocketOriginPatterns),
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

			handshakeCtx := wsx.StartHandshake(ctx, pattern)
			defer wsx.FinishHandshake(handshakeCtx)

			next(w, r.WithContext(handshakeCtx))
		}
	}
}

// Routes returns Server's HTTP handler.
func (s *Server) Routes() http.Handler {
	return s.mux
}
