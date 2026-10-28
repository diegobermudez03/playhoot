// Package identity is the route group of the identity domain: obtaining,
// renewing and ending the credentials a client calls every other endpoint
// with, who they stand for, and the level each operation requires. Each
// endpoint's handler has its own file.
package identity

import (
	"context"

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/api/internal/routing"
	identitypkg "github.com/diegobermudez03/playhoot/identity"
)

//go:generate mockgen -package=identity -destination=handler_mock_test.go . guestCreator,operationLister,credentialRefresher,sessionEnder,authorizer

// guestCreator creates guest Users idempotently per key. A decline is
// reported through the result's Outcome; a reused key with another request
// returns identitypkg.ErrIdempotencyConflict.
type guestCreator interface {
	CreateGuest(ctx context.Context, displayName string, idempotencyKey string) (identitypkg.CreateGuestResult, error)
}

// operationLister lists every operation with the minimum authentication
// level it requires.
type operationLister interface {
	ListOperations(ctx context.Context) []identitypkg.OperationLevel
}

// credentialRefresher renews credentials with a refresh token.
type credentialRefresher interface {
	Refresh(ctx context.Context, refreshToken string) (identitypkg.RefreshResult, error)
}

// sessionEnder ends the session of a refresh token.
type sessionEnder interface {
	EndSession(ctx context.Context, refreshToken string) error
}

// authorizer resolves an access token to its caller.
type authorizer interface {
	Authorize(ctx context.Context, token string, operation identitypkg.Operation) (identitypkg.User, identitypkg.AccessDecision, error)
}

// Handler exposes the identity domain's endpoints.
type Handler struct {
	guests     guestCreator
	operations operationLister
	refreshes  credentialRefresher
	sessions   sessionEnder
	authorizer authorizer
}

// NewHandler constructs a Handler.
func NewHandler(guests guestCreator, operations operationLister, refreshes credentialRefresher, sessions sessionEnder, authorizer authorizer) *Handler {
	return &Handler{guests: guests, operations: operations, refreshes: refreshes, sessions: sessions, authorizer: authorizer}
}

// RESTRoutes returns this group's request/response endpoints. Only `me` needs
// an access token: the others are how a client gets, renews or ends one, and
// the operation catalog is read before choosing anything.
func (h *Handler) RESTRoutes() []routing.Route {
	return []routing.Route{
		{Pattern: "POST /v1/guests", Handler: h.handleCreateGuest},
		{Pattern: "GET /v1/identity/operations", Handler: h.handleListOperations},
		{Pattern: "POST /v1/identity/refresh", Handler: h.handleRefresh},
		{Pattern: "POST /v1/identity/logout", Handler: h.handleLogout},
		{
			Pattern:       "GET /v1/identity/me",
			Handler:       h.handleMe,
			Authorization: auth.Require(h.authorizer, identitypkg.OperationViewOwnIdentity),
		},
	}
}

// WebSocketRoutes returns no endpoints: this group has no live ones.
func (h *Handler) WebSocketRoutes() []routing.Route {
	return nil
}
