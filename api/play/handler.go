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

//go:generate mockgen -package=play -destination=session_creator_mock_test.go . sessionCreator
//go:generate mockgen -package=play -destination=owner_locator_mock_test.go . ownerLocator
//go:generate mockgen -package=play -destination=admin_attacher_mock_test.go . adminAttacher

// sessionCreator creates Sessions idempotently per key. A decline is reported
// through the result's Outcome; a reused key returns
// sessionpkg.ErrIdempotencyConflict.
type sessionCreator interface {
	CreateSession(ctx context.Context, userUUID string, gameUUID string, idempotencyKey string) (sessionpkg.CreateSessionResult, error)
}

// authorizer is Identity's coarse authorization.
type authorizer interface {
	Authorize(ctx context.Context, token string, operation identity.Operation) (string, identity.AccessDecision, error)
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

// liveRunner serves admitted live connections in the background until they
// end or the server shuts down; it refuses new ones once shutdown started.
type liveRunner interface {
	Serve(ctx context.Context, conn *websocket.Conn, disconnect func(), loops ...live.Loop) bool
}

// Handler exposes the play domain's endpoints.
type Handler struct {
	authorizer              authorizer
	creator                 sessionCreator
	owners                  ownerLocator
	admins                  adminAttacher
	live                    liveRunner
	webSocketOriginPatterns []string
}

// NewHandler constructs a Handler. webSocketOriginPatterns are the extra
// origin hosts browser pages may open live connections from.
func NewHandler(authorizer authorizer, creator sessionCreator, owners ownerLocator, admins adminAttacher, live liveRunner, webSocketOriginPatterns []string) *Handler {
	return &Handler{
		authorizer:              authorizer,
		creator:                 creator,
		owners:                  owners,
		admins:                  admins,
		live:                    live,
		webSocketOriginPatterns: webSocketOriginPatterns,
	}
}

// RESTRoutes returns this group's request/response endpoints.
func (h *Handler) RESTRoutes() []routing.Route {
	return []routing.Route{
		{
			Pattern: "POST /sessions",
			Handler: h.handleCreateSession,
			Middlewares: []routing.Middleware{
				auth.Require(h.authorizer, identity.OperationCreateSession),
			},
		},
	}
}

// WebSocketRoutes returns this group's live endpoints. Authorization runs
// before owner routing so an unauthorized caller never makes a Session owned.
func (h *Handler) WebSocketRoutes() []routing.Route {
	return []routing.Route{
		{
			Pattern: "GET /sessions/{sessionUUID}/admin",
			Handler: h.handleAdminJoin,
			Middlewares: []routing.Middleware{
				auth.RequireWebSocket(h.authorizer, identity.OperationJoinSessionAsAdmin),
				requireOwner(h.owners, "sessionUUID"),
			},
		},
	}
}
