// Package play is the route group of the play domain: creating Sessions and
// the live connections to them. Each endpoint's handler has its own file.
package play

import (
	"context"

	"github.com/coder/websocket"

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/api/internal/live"
	"github.com/diegobermudez03/playhoot/api/internal/routing"
	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/play/runtime"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

//go:generate mockgen -package=play -destination=handler_mock_test.go . sessionCreator,ownerLocator,adminAttacher,playerAttacher,joinCodeResolver

// sessionCreator creates Sessions idempotently per key. A decline is reported
// through the result's Outcome; a reused key returns
// sessionpkg.ErrIdempotencyConflict.
type sessionCreator interface {
	CreateSession(ctx context.Context, userUUID string, gameUUID string, idempotencyKey string) (sessionpkg.CreateSessionResult, error)
}

// authorizer is Identity's coarse authorization.
type authorizer interface {
	Authorize(ctx context.Context, token string, operation identity.Operation) (identity.User, identity.AccessDecision, error)
}

// ownerLocator returns a Session's owner, claiming one when there is none.
type ownerLocator interface {
	LocateOrClaimOwner(ctx context.Context, sessionUUID string) (runtime.OwnerPlacement, error)
}

// adminAttacher admits admin connections. Its Connection is served whether
// accepted or rejected; an error means no connection exists.
type adminAttacher interface {
	AttachAdmin(ctx context.Context, sessionUUID string, userUUID string, ownerEpoch int64) (runtime.AttachAdminResult, error)
}

// playerAttacher admits player connections into a Session's lobby. Its
// Connection is served only when accepted; an error means no connection exists.
type playerAttacher interface {
	AttachPlayer(ctx context.Context, sessionUUID string, player runtime.Player, ownerEpoch int64) (runtime.AttachPlayerResult, error)
}

// joinCodeResolver finds the Session a join code names.
type joinCodeResolver interface {
	ResolveJoinCode(ctx context.Context, joinCode string) (sessionpkg.JoinCodeResolution, error)
}

// liveRunner bounds the live connections of this instance and serves
// admitted ones in the background until they end or the server shuts down; it
// refuses new ones once shutdown started.
type liveRunner interface {
	Reserve(ctx context.Context, caller string) (release func(), err error)
	Serve(ctx context.Context, conn *websocket.Conn, disconnect func(), loops ...live.Loop) bool
}

// Handler exposes the play domain's endpoints.
type Handler struct {
	authorizer              authorizer
	creator                 sessionCreator
	owners                  ownerLocator
	admins                  adminAttacher
	players                 playerAttacher
	joinCodes               joinCodeResolver
	live                    liveRunner
	webSocketOriginPatterns []string
}

// NewHandler constructs a Handler. webSocketOriginPatterns are the extra
// origin hosts browser pages may open live connections from.
func NewHandler(authorizer authorizer, creator sessionCreator, owners ownerLocator, admins adminAttacher, players playerAttacher, joinCodes joinCodeResolver, live liveRunner, webSocketOriginPatterns []string) *Handler {
	return &Handler{
		authorizer:              authorizer,
		creator:                 creator,
		owners:                  owners,
		admins:                  admins,
		players:                 players,
		joinCodes:               joinCodes,
		live:                    live,
		webSocketOriginPatterns: webSocketOriginPatterns,
	}
}

// RESTRoutes returns this group's request/response endpoints.
func (h *Handler) RESTRoutes() []routing.Route {
	return []routing.Route{
		{
			Pattern:       "POST /v1/sessions",
			Handler:       h.handleCreateSession,
			Authorization: auth.Require(h.authorizer, identity.OperationCreateSession),
		},
	}
}

// WebSocketRoutes returns this group's live endpoints. Authorization and rate
// limiting run before join code and owner routing, so an unauthorized or
// limited caller never learns a Session nor makes one owned.
func (h *Handler) WebSocketRoutes() []routing.Route {
	return []routing.Route{
		{
			Pattern:       "GET /v1/play/join/sessions/{sessionUUID}/admin",
			Handler:       h.handleAdminJoin,
			Authorization: auth.RequireWebSocket(h.authorizer, identity.OperationJoinSessionAsAdmin),
			Middlewares: []routing.Middleware{
				requireOwner(h.owners, "sessionUUID"),
			},
		},
		{
			Pattern:       "GET /v1/play/join/sessions/{sessionUUID}/player",
			Handler:       h.handlePlayerJoin,
			Authorization: auth.RequireWebSocket(h.authorizer, identity.OperationJoinSessionAsPlayer),
			Middlewares: []routing.Middleware{
				requireOwner(h.owners, "sessionUUID"),
			},
		},
		{
			Pattern:       "GET /v1/play/join/{joinCode}",
			Handler:       h.handlePlayerJoin,
			Authorization: auth.RequireWebSocket(h.authorizer, identity.OperationJoinSessionAsPlayer),
			Middlewares: []routing.Middleware{
				resolveJoinCode(h.joinCodes, "joinCode", "sessionUUID"),
				requireOwner(h.owners, "sessionUUID"),
			},
		},
	}
}
