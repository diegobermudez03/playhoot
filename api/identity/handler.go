// Package identity is the route group of the identity domain: obtaining the
// credential a client calls every other endpoint with, and the level each
// operation requires. Each endpoint's handler has its own file.
package identity

import (
	"context"

	"github.com/diegobermudez03/playhoot/api/internal/routing"
	identitypkg "github.com/diegobermudez03/playhoot/identity"
)

//go:generate mockgen -package=identity -destination=handler_mock_test.go . guestCreator,operationLister

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

// Handler exposes the identity domain's endpoints.
type Handler struct {
	guests     guestCreator
	operations operationLister
}

// NewHandler constructs a Handler.
func NewHandler(guests guestCreator, operations operationLister) *Handler {
	return &Handler{guests: guests, operations: operations}
}

// RESTRoutes returns this group's request/response endpoints. Neither needs
// a token: creating a guest is how a client without an account gets one,
// and the operation catalog is read before choosing anything.
func (h *Handler) RESTRoutes() []routing.Route {
	return []routing.Route{
		{
			Pattern: "POST /v1/guests",
			Handler: h.handleCreateGuest,
		},
		{
			Pattern: "GET /v1/identity/operations",
			Handler: h.handleListOperations,
		},
	}
}

// WebSocketRoutes returns no endpoints: this group has no live ones.
func (h *Handler) WebSocketRoutes() []routing.Route {
	return nil
}
