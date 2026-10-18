// Package identity is the route group of the identity domain: obtaining the
// credential a client calls every other endpoint with. Each endpoint's
// handler has its own file.
package identity

import (
	"context"

	"github.com/diegobermudez03/playhoot/api/internal/routing"
	identitypkg "github.com/diegobermudez03/playhoot/identity"
)

//go:generate mockgen -package=identity -destination=handler_mock_test.go . guestCreator

// guestCreator creates guest Users idempotently per key. A decline is
// reported through the result's Outcome; a reused key with another request
// returns identitypkg.ErrIdempotencyConflict.
type guestCreator interface {
	CreateGuest(ctx context.Context, displayName string, idempotencyKey string) (identitypkg.CreateGuestResult, error)
}

// Handler exposes the identity domain's endpoints.
type Handler struct {
	guests guestCreator
}

// NewHandler constructs a Handler.
func NewHandler(guests guestCreator) *Handler {
	return &Handler{guests: guests}
}

// RESTRoutes returns this group's request/response endpoints. Creating a
// guest needs no token: it is how a client without an account gets one.
func (h *Handler) RESTRoutes() []routing.Route {
	return []routing.Route{
		{
			Pattern: "POST /v1/guests",
			Handler: h.handleCreateGuest,
		},
	}
}

// WebSocketRoutes returns no endpoints: this group has no live ones.
func (h *Handler) WebSocketRoutes() []routing.Route {
	return nil
}
