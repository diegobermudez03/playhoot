package api

import (
	"context"

	"github.com/diegobermudez03/playhoot/identity"
	sessionpkg "github.com/diegobermudez03/playhoot/play"
	"github.com/diegobermudez03/playhoot/play/runtime"
)

// authorizer is Identity's coarse authorization.
type authorizer interface {
	Authorize(ctx context.Context, token string, operation identity.Operation) (string, identity.AccessDecision, error)
}

// sessionCreator coordinates Session creation.
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
