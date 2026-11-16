// Package identity is the route group of the identity domain: obtaining,
// renewing and ending the credentials a client calls every other endpoint
// with, who they stand for, and the level each operation requires. Each
// endpoint's handler has its own file.
package identity

import (
	"context"
	"slices"
	"time"

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/api/internal/routing"
	"github.com/diegobermudez03/playhoot/api/internal/throttle"
	identitypkg "github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/utils/ratelimit"
)

//go:generate mockgen -package=identity -destination=handler_mock_test.go . guestCreator,operationLister,credentialRefresher,sessionEnder,displayNameSetter,authorizer,loginStarter,loginCallbackHandler,loginCompleter,providerLister,localPages,providerLinker,emailLoginStarter,emailLoginCompleter,emailLinker,methodReader,methodRemover

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

// providerLinker adds a provider identity to an account already signed in,
// through the same round trip as a sign-in.
type providerLinker interface {
	StartLinkProvider(ctx context.Context, userUUID string, provider string, codeChallenge string) (identitypkg.StartLoginResult, error)
	CompleteLinkProvider(ctx context.Context, userUUID string, code string, codeVerifier string) (identitypkg.LinkMethodResult, error)
}

// emailLoginStarter asks for a one-time code to be sent to an address.
type emailLoginStarter interface {
	StartEmailLogin(ctx context.Context, address string, language string, guestUserUUID string) (identitypkg.StartEmailLoginResult, error)
}

// emailLoginCompleter exchanges an address and its code for credentials.
type emailLoginCompleter interface {
	CompleteEmailLogin(ctx context.Context, address string, code string, replacesRefreshToken string) (identitypkg.CompleteEmailLoginResult, error)
}

// emailLinker adds an address to an account already signed in.
type emailLinker interface {
	StartLinkEmail(ctx context.Context, userUUID string, address string, language string) (identitypkg.StartEmailLoginResult, error)
	CompleteLinkEmail(ctx context.Context, userUUID string, address string, code string) (identitypkg.LinkMethodResult, error)
}

// methodReader lists an account's ways in.
type methodReader interface {
	ListMethods(ctx context.Context, userUUID string) ([]identitypkg.SignInMethod, error)
}

// methodRemover unlinks one of them.
type methodRemover interface {
	RemoveMethod(ctx context.Context, userUUID string, methodID string) (identitypkg.RemoveMethodOutcome, error)
}

// providerLister lists the enabled login providers.
type providerLister interface {
	Enabled() []identitypkg.Provider
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
	Linker     providerLinker
	Providers  providerLister
	LocalPages localPages
	// CompleteURL is the client's sign-in completion route, which every
	// callback redirects to: chosen by configuration, never by a request.
	CompleteURL string
}

// guestCreationLimits add to the default per-address limits a cap on guest
// creation requests from one address in an hour: generous for a classroom
// behind one public address, since every guest stays. It counts requests
// (replays and refusals too), per instance.
var guestCreationLimits = routing.RequestLimits{
	PerUser:   throttle.DefaultLimits.PerUser,
	PerClient: append(slices.Clone(throttle.DefaultLimits.PerClient), ratelimit.Limit{Count: 200, Per: time.Hour}),
}

// EmailLogins is what the one-time email code needs.
type EmailLogins struct {
	Starter   emailLoginStarter
	Completer emailLoginCompleter
	Linker    emailLinker
}

// Methods is what reading and removing an account's ways in needs.
type Methods struct {
	Reader  methodReader
	Remover methodRemover
}

// emailCodeLimits bound how often one client address may ask for codes;
// how often one email address may be sent one is Identity's own rule.
var emailCodeLimits = routing.RequestLimits{
	PerUser:   throttle.DefaultLimits.PerUser,
	PerClient: append(slices.Clone(throttle.DefaultLimits.PerClient), ratelimit.Limit{Count: 30, Per: time.Hour}),
}

// emailCodeTryLimits bound how often one client address may try codes, so
// six digits cannot be guessed at from one place.
var emailCodeTryLimits = routing.RequestLimits{
	PerUser:   throttle.DefaultLimits.PerUser,
	PerClient: append(slices.Clone(throttle.DefaultLimits.PerClient), ratelimit.Limit{Count: 10, Per: time.Minute}),
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
	emailLogins  EmailLogins
	methods      Methods
}

// NewHandler constructs a Handler.
func NewHandler(guests guestCreator, operations operationLister, refreshes credentialRefresher, sessions sessionEnder, displayNames displayNameSetter, authorizer authorizer, logins Logins, emailLogins EmailLogins, methods Methods) *Handler {
	return &Handler{
		guests: guests, operations: operations, refreshes: refreshes, sessions: sessions,
		displayNames: displayNames, authorizer: authorizer, logins: logins, emailLogins: emailLogins, methods: methods,
	}
}

// RESTRoutes returns this group's request/response endpoints. Only `me` and
// its display name need an access token: the others are how a client gets, renews or ends one, and
// the operation catalog is read before choosing anything. The local
// provider's page answers only where that provider is enabled.
func (h *Handler) RESTRoutes() []routing.Route {
	return []routing.Route{
		{Pattern: "POST /v1/guests", Handler: h.handleCreateGuest, Limits: &guestCreationLimits},
		{Pattern: "GET /v1/identity/operations", Handler: h.handleListOperations},
		{Pattern: "POST /v1/identity/refresh", Handler: h.handleRefresh},
		{Pattern: "POST /v1/identity/logout", Handler: h.handleLogout},
		{
			Pattern: "POST /v1/identity/login/start",
			Handler: h.handleStartLogin,
			// A guest sends their access token, so a new account is theirs.
			Authorization: auth.Optional(h.authorizer, identitypkg.OperationViewOwnIdentity),
		},
		{
			Pattern: "POST /v1/identity/login/email/start",
			Handler: h.handleStartEmailLogin,
			// A guest sends their access token, so a new account is theirs.
			Authorization: auth.Optional(h.authorizer, identitypkg.OperationViewOwnIdentity),
			Limits:        &emailCodeLimits,
		},
		{Pattern: "POST /v1/identity/login/email/complete", Handler: h.handleCompleteEmailLogin, Limits: &emailCodeTryLimits},
		{Pattern: "GET /v1/identity/login/{provider}/callback", Handler: h.handleLoginCallback},
		{Pattern: "POST /v1/identity/login/complete", Handler: h.handleCompleteLogin},
		// this is for the local provider, only for local testing
		{Pattern: "GET /v1/identity/login/providers", Handler: h.handleListProviders},
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
		{
			Pattern:       "GET /v1/identity/me/methods",
			Handler:       h.handleListMethods,
			Authorization: auth.Require(h.authorizer, identitypkg.OperationManageSignInMethods),
		},
		{
			Pattern:       "POST /v1/identity/me/methods/email/start",
			Handler:       h.handleStartLinkEmail,
			Authorization: auth.Require(h.authorizer, identitypkg.OperationManageSignInMethods),
			Limits:        &emailCodeLimits,
		},
		{
			Pattern:       "POST /v1/identity/me/methods/email/complete",
			Handler:       h.handleCompleteLinkEmail,
			Authorization: auth.Require(h.authorizer, identitypkg.OperationManageSignInMethods),
		},
		{
			Pattern:       "POST /v1/identity/me/methods/provider/start",
			Handler:       h.handleStartLinkProvider,
			Authorization: auth.Require(h.authorizer, identitypkg.OperationManageSignInMethods),
		},
		{
			Pattern:       "POST /v1/identity/me/methods/provider/complete",
			Handler:       h.handleCompleteLinkProvider,
			Authorization: auth.Require(h.authorizer, identitypkg.OperationManageSignInMethods),
		},
		{
			Pattern:       "POST /v1/identity/me/methods/{methodID}/remove",
			Handler:       h.handleRemoveMethod,
			Authorization: auth.Require(h.authorizer, identitypkg.OperationManageSignInMethods),
		},
	}
}

// WebSocketRoutes returns no endpoints: this group has no live ones.
func (h *Handler) WebSocketRoutes() []routing.Route {
	return nil
}
