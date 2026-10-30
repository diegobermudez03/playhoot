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

//go:generate mockgen -package=identity -destination=handler_mock_test.go . guestCreator,operationLister,credentialRefresher,sessionEnder,displayNameSetter,authorizer,loginStarter,loginCallbackHandler,loginCompleter,localPages

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

// displayNameSetter names a User.
type displayNameSetter interface {
	SetDisplayName(ctx context.Context, userUUID string, displayName string) (identitypkg.SetDisplayNameResult, error)
}

// authorizer resolves an access token to its caller.
type authorizer interface {
	Authorize(ctx context.Context, token string, operation identitypkg.Operation) (identitypkg.User, identitypkg.AccessDecision, error)
}

// loginStarter starts account sign-ins.
type loginStarter interface {
	StartLogin(ctx context.Context, provider string, codeChallenge string, guestUserUUID string) (identitypkg.StartLoginResult, error)
}

// loginCallbackHandler takes providers' answers to sign-ins.
type loginCallbackHandler interface {
	HandleCallback(ctx context.Context, provider string, state string, code string, providerError string) (identitypkg.LoginCallbackResult, error)
}

// loginCompleter completes sign-ins with a one-time code.
type loginCompleter interface {
	CompleteLogin(ctx context.Context, code string, codeVerifier string, replacesRefreshToken string) (identitypkg.CompleteLoginResult, error)
}

// localPages serves the local provider's sign-in page, when it is enabled.
type localPages interface {
	LocalAuthorizePage(state string) (string, bool)
}

// Logins is what account sign-in needs.
type Logins struct {
	Starter    loginStarter
	Callbacks  loginCallbackHandler
	Completer  loginCompleter
	LocalPages localPages
	// CompleteURL is the client's sign-in completion route, which every
	// callback redirects to: chosen by configuration, never by a request.
	CompleteURL string
}

// Handler exposes the identity domain's endpoints.
type Handler struct {
	guests       guestCreator
	operations   operationLister
	refreshes    credentialRefresher
	sessions     sessionEnder
	displayNames displayNameSetter
	authorizer   authorizer
	logins       Logins
}

// NewHandler constructs a Handler.
func NewHandler(guests guestCreator, operations operationLister, refreshes credentialRefresher, sessions sessionEnder, displayNames displayNameSetter, authorizer authorizer, logins Logins) *Handler {
	return &Handler{guests: guests, operations: operations, refreshes: refreshes, sessions: sessions, displayNames: displayNames, authorizer: authorizer, logins: logins}
}

// RESTRoutes returns this group's request/response endpoints. Only `me` and
// its display name need an access token: the others are how a client gets, renews or ends one, and
// the operation catalog is read before choosing anything. The local
// provider's page answers only where that provider is enabled.
func (h *Handler) RESTRoutes() []routing.Route {
	return []routing.Route{
		{Pattern: "POST /v1/guests", Handler: h.handleCreateGuest},
		{Pattern: "GET /v1/identity/operations", Handler: h.handleListOperations},
		{Pattern: "POST /v1/identity/refresh", Handler: h.handleRefresh},
		{Pattern: "POST /v1/identity/logout", Handler: h.handleLogout},
		{
			Pattern: "POST /v1/identity/login/start",
			Handler: h.handleStartLogin,
			// A guest sends their access token, so a new account is theirs.
			Authorization: auth.Optional(h.authorizer, identitypkg.OperationViewOwnIdentity),
		},
		{Pattern: "GET /v1/identity/login/{provider}/callback", Handler: h.handleLoginCallback},
		{Pattern: "POST /v1/identity/login/complete", Handler: h.handleCompleteLogin},
		// this is for the local provider, only for local testing
		{Pattern: "GET /v1/identity/login/local/authorize", Handler: h.handleLocalAuthorize},
		{
			Pattern:       "GET /v1/identity/me",
			Handler:       h.handleMe,
			Authorization: auth.Require(h.authorizer, identitypkg.OperationViewOwnIdentity),
		},
		{
			Pattern:       "PUT /v1/identity/me/display-name",
			Handler:       h.handleSetDisplayName,
			Authorization: auth.Require(h.authorizer, identitypkg.OperationSetOwnDisplayName),
		},
	}
}

// WebSocketRoutes returns no endpoints: this group has no live ones.
func (h *Handler) WebSocketRoutes() []routing.Route {
	return nil
}
