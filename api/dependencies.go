package api

import (
	"context"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/play/runtime"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

// guestCreator is Identity's guest creation.
type guestCreator interface {
	CreateGuest(ctx context.Context, displayName string, idempotencyKey string) (identity.CreateGuestResult, error)
}

// operationLister is Identity's catalog of operations and the minimum
// authentication level each requires.
type operationLister interface {
	ListOperations(ctx context.Context) []identity.OperationLevel
}

// authorizer is Identity's coarse authorization.
type authorizer interface {
	Authorize(ctx context.Context, token string, operation identity.Operation) (identity.User, identity.AccessDecision, error)
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

// playerAttacher is the Runtime's admission of player connections.
type playerAttacher interface {
	AttachPlayer(ctx context.Context, sessionUUID string, player runtime.Player, ownerEpoch int64) (runtime.AttachPlayerResult, error)
}

// joinCodeResolver is the durable Session's join code lookup.
type joinCodeResolver interface {
	ResolveJoinCode(ctx context.Context, joinCode string) (sessionpkg.JoinCodeResolution, error)
}
