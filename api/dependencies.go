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

// credentialRefresher is Identity's credential renewal.
type credentialRefresher interface {
	Refresh(ctx context.Context, refreshToken string) (identity.RefreshResult, error)
}

// sessionEnder is Identity's sign-out.
type sessionEnder interface {
	EndSession(ctx context.Context, refreshToken string) error
}

// loginStarter is Identity's start of an account sign-in.
type loginStarter interface {
	StartLogin(ctx context.Context, provider string, codeChallenge string, guestUserUUID string) (identity.StartLoginResult, error)
}

// loginCallbackHandler is Identity's handling of a provider's answer.
type loginCallbackHandler interface {
	HandleCallback(ctx context.Context, provider string, state string, code string, providerError string) (identity.LoginCallbackResult, error)
}

// loginCompleter is Identity's completion of a sign-in.
type loginCompleter interface {
	CompleteLogin(ctx context.Context, code string, codeVerifier string, replacesRefreshToken string) (identity.CompleteLoginResult, error)
}

// localPages is Identity's local provider page, when enabled.
type localPages interface {
	LocalAuthorizePage(state string) (string, bool)
}

// Logins is what account sign-in needs.
type Logins struct {
	Starter    loginStarter
	Callbacks  loginCallbackHandler
	Completer  loginCompleter
	LocalPages localPages
	// CompleteURL is the client's sign-in completion route every provider
	// callback redirects to.
	CompleteURL string
}

// displayNameSetter is Identity's naming of a User.
type displayNameSetter interface {
	SetDisplayName(ctx context.Context, userUUID string, displayName string) (identity.SetDisplayNameResult, error)
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
