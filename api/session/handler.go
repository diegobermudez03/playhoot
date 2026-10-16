// Package session is the session workflow's route group: creating a
// Session and the live connections to it.
package session

import (
	"context"

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/api/internal/ownerrouting"
	"github.com/diegobermudez03/playhoot/api/internal/routing"
	"github.com/diegobermudez03/playhoot/identity"
	sessionpkg "github.com/diegobermudez03/playhoot/play"
	"github.com/diegobermudez03/playhoot/play/runtime"
)

//go:generate mockgen -package=session -destination=session_creator_mock_test.go . sessionCreator
//go:generate mockgen -package=session -destination=admin_attacher_mock_test.go . adminAttacher

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

// ownerLocator is the Runtime's live Session ownership.
type ownerLocator interface {
	LocateOrClaimOwner(ctx context.Context, sessionUUID string) (runtime.OwnerPlacement, error)
}

// adminAttacher admits admin connections. Its Connection is served whether
// accepted or rejected; an error means no connection exists.
type adminAttacher interface {
	AttachAdmin(ctx context.Context, sessionUUID string, userUUID string, ownerEpoch int64) (runtime.AttachAdminResult, error)
}

// Handler exposes the session workflow's endpoints.
type Handler struct {
	authorizer              authorizer
	creator                 sessionCreator
	owners                  ownerLocator
	admins                  adminAttacher
	webSocketOriginPatterns []string
}

// NewHandler constructs a Handler. webSocketOriginPatterns are the extra
// origin hosts browser pages may open live connections from.
func NewHandler(authorizer authorizer, creator sessionCreator, owners ownerLocator, admins adminAttacher, webSocketOriginPatterns []string) *Handler {
	return &Handler{
		authorizer:              authorizer,
		creator:                 creator,
		owners:                  owners,
		admins:                  admins,
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
				ownerrouting.Require(h.owners, "sessionUUID"),
			},
		},
	}
}
