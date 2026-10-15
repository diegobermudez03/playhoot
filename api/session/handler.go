// Package session is the session workflow's route group. POST /sessions
// creates a Session through sessionCreator (Orchestrator's CreateSession in
// production), for a caller Identity has authorized. The live WebSocket
// endpoints will be added as the Session Runtime's real-time design is
// accepted.
package session

import (
	"context"

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/api/internal/routing"
	"github.com/diegobermudez03/playhoot/identity"
	sessionpkg "github.com/diegobermudez03/playhoot/play"
)

//go:generate mockgen -package=session -destination=session_creator_mock_test.go . sessionCreator

// sessionCreator is this route group's own narrow dependency on whatever
// coordinates Session creation - Orchestrator in production, satisfied
// structurally rather than imported directly so this package never needs
// to depend on orchestrator's own dependencies.
//
// userUUID is the already-authenticated creator; creation is idempotent per
// idempotencyKey. A deterministic decline (for example, the Game is not
// playable) is reported through the result's Outcome with a nil error;
// reusing a key for a different request returns
// sessionpkg.ErrIdempotencyConflict, and any other error is an unexpected
// failure.
type sessionCreator interface {
	CreateSession(ctx context.Context, userUUID string, gameUUID string, idempotencyKey string) (sessionpkg.CreateSessionResult, error)
}

// authorizer is this route group's own narrow dependency on Identity, used
// only to build the authorization middleware its routes declare.
type authorizer interface {
	Authorize(ctx context.Context, token string, operation identity.Operation) (string, identity.AccessDecision, error)
}

// Handler exposes the session workflow's endpoints.
type Handler struct {
	authorizer authorizer
	creator    sessionCreator
}

// NewHandler constructs a Handler. authorizer is Identity's side of the
// authorization middleware its routes declare.
func NewHandler(authorizer authorizer, creator sessionCreator) *Handler {
	return &Handler{authorizer: authorizer, creator: creator}
}

// RESTRoutes returns this group's ordinary request/response endpoints,
// satisfying api.routeGroup. It never touches an http.ServeMux and never
// starts its own request log - Server owns both (see api/server.go).
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

// WebSocketRoutes returns this group's WebSocket endpoints, satisfying
// api.routeGroup.
func (h *Handler) WebSocketRoutes() []routing.Route {
	return nil
}
