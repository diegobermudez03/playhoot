// Package identity is Identity's public contract. It imports only the standard
// library and grows as Identity's capabilities are accepted.
package identity

import (
	"errors"
	"time"
)

// Operation names a coarse operation Identity decides whether a caller may
// perform at all. Object-level permissions (for example, whether a caller
// created a specific Session) are not decided by Identity.
type Operation string

const (
	// OperationCreateSession is creating a new Session. It requires a real
	// account, not a guest-only identity.
	OperationCreateSession Operation = "CREATE_SESSION"

	// OperationJoinSessionAsAdmin is opening an admin connection to a live
	// Session. It requires a real account, not a guest-only identity.
	OperationJoinSessionAsAdmin Operation = "JOIN_SESSION_AS_ADMIN"

	// OperationJoinSessionAsPlayer is opening a player connection to a live
	// Session. Guests may perform it.
	OperationJoinSessionAsPlayer Operation = "JOIN_SESSION_AS_PLAYER"

	// OperationReportClientTelemetry is sending the client's operational
	// telemetry (load times, reconnections, classified errors). Guests may
	// perform it.
	OperationReportClientTelemetry Operation = "REPORT_CLIENT_TELEMETRY"

	// OperationViewOwnIdentity is reading who the caller's credential stands
	// for. Guests may perform it.
	OperationViewOwnIdentity Operation = "VIEW_OWN_IDENTITY"

	// OperationSetOwnDisplayName is choosing the name other people see for
	// the caller. An account has none until its person chooses one.
	OperationSetOwnDisplayName Operation = "SET_OWN_DISPLAY_NAME"

	// OperationCreateGame is creating a new game to author. It requires a
	// real account.
	OperationCreateGame Operation = "CREATE_GAME"

	// OperationEditGame is reading and changing a game's version being
	// built. It requires a real account; whether the caller may edit that
	// game is Game Management's decision.
	OperationEditGame Operation = "EDIT_GAME"

	// OperationConnectAI is approving an AI application's access to the
	// caller's games and managing the AI connections they approved. It
	// requires a real account.
	OperationConnectAI Operation = "CONNECT_AI"
)

// Level is how strongly a caller is authenticated, on the ordered scale
// anonymous < guest < account.
type Level string

const (
	// LevelAnonymous is a caller presenting no valid identity.
	LevelAnonymous Level = "anonymous"

	// LevelGuest is a User whose only proof is a credential the client keeps.
	LevelGuest Level = "guest"

	// LevelAccount is a User with a registered account.
	LevelAccount Level = "account"
)

// OperationLevel is the minimum Level an Operation requires. It states
// authentication only: role capabilities and each domain's object-level
// permissions are decided elsewhere.
type OperationLevel struct {
	Operation     Operation
	RequiredLevel Level
}

// User is who a credential stands for, as Identity resolves it.
type User struct {
	UUID string

	// DisplayName is how other people see the User; Identity decides it.
	DisplayName string

	// Level is how strongly the User is authenticated: LevelGuest or
	// LevelAccount.
	Level Level
}

// AccessDecision is Identity's answer to whether a caller may perform an
// Operation.
type AccessDecision string

const (
	// AccessAllowed means the token is valid and its User may perform the
	// Operation.
	AccessAllowed AccessDecision = "allowed"

	// AccessUnauthenticated means the token is missing, malformed, expired
	// or otherwise not a valid proof of identity.
	AccessUnauthenticated AccessDecision = "unauthenticated"

	// AccessForbidden means the token is valid but its User may not perform
	// the Operation.
	AccessForbidden AccessDecision = "forbidden"
)

// Credentials are what a client authenticates with: a short access token sent
// with every request, and the refresh token it renews the access token with.
type Credentials struct {
	User User

	AccessToken     string
	AccessIssuedAt  time.Time
	AccessExpiresAt time.Time

	// RefreshToken stays the same across renewals until it is rotated.
	RefreshToken string
}

// CreateGuestOutcome is the decision a CreateGuest call reached.
type CreateGuestOutcome string

const (
	// GuestCreated means a guest User exists for the request. A retry with
	// the same idempotency key and name soon after gets the same User.
	GuestCreated CreateGuestOutcome = "created"

	// GuestInvalidDisplayName means the display name breaks Identity's rules.
	GuestInvalidDisplayName CreateGuestOutcome = "invalid_display_name"

	// GuestIdempotencyKeySpent means the idempotency key created a guest too
	// long ago, or that guest's session ended: it creates nothing new.
	GuestIdempotencyKeySpent CreateGuestOutcome = "idempotency_key_spent"
)

// CreateGuestResult is what a CreateGuest call decided. Credentials are set
// only when Outcome is GuestCreated.
type CreateGuestResult struct {
	Outcome     CreateGuestOutcome
	Credentials Credentials
}

// RefreshOutcome is the decision a refresh reached.
type RefreshOutcome string

const (
	// RefreshRenewed means the refresh token was accepted and new credentials
	// were issued.
	RefreshRenewed RefreshOutcome = "renewed"

	// RefreshInvalid means the refresh token is unknown, expired, or no longer
	// active: the client must authenticate again.
	RefreshInvalid RefreshOutcome = "invalid"
)

// RefreshResult is what a refresh decided. Credentials are set only when
// Outcome is RefreshRenewed.
type RefreshResult struct {
	Outcome     RefreshOutcome
	Credentials Credentials
}

// Provider names an external identity provider an account signs in with.
type Provider string

const (
	// ProviderGoogle is Google, through OpenID Connect.
	ProviderGoogle Provider = "google"

	// ProviderLocal is a fictitious provider for local development and tests,
	// never enabled in production.
	ProviderLocal Provider = "local"
)

// StartLoginOutcome is the decision a StartLogin call reached.
type StartLoginOutcome string

const (
	// LoginStarted means the client may send the person to AuthorizationURL.
	LoginStarted StartLoginOutcome = "started"

	// LoginProviderUnavailable means the provider is unknown or not enabled.
	LoginProviderUnavailable StartLoginOutcome = "provider_unavailable"

	// LoginInvalidCodeChallenge means the code challenge is not an S256 hash.
	LoginInvalidCodeChallenge StartLoginOutcome = "invalid_code_challenge"
)

// StartLoginResult is what a StartLogin call decided. AuthorizationURL is set
// only when Outcome is LoginStarted.
type StartLoginResult struct {
	Outcome          StartLoginOutcome
	AuthorizationURL string
}

// LoginCallbackOutcome is how the provider's answer to a login ended.
type LoginCallbackOutcome string

const (
	// LoginCodeIssued means the person proved their identity: LoginCode
	// completes the login once, with the client's verifier, shortly after.
	LoginCodeIssued LoginCallbackOutcome = "code_issued"

	// LoginCancelled means the person declined at the provider.
	LoginCancelled LoginCallbackOutcome = "cancelled"

	// LoginFailed means the attempt is unknown, expired or already answered,
	// or the provider's answer was not valid.
	LoginFailed LoginCallbackOutcome = "failed"

	// LoginUnavailable means the provider could not be reached.
	LoginUnavailable LoginCallbackOutcome = "unavailable"
)

// LoginCallbackResult is what the provider's answer led to. LoginCode is set
// only when Outcome is LoginCodeIssued.
type LoginCallbackResult struct {
	Outcome   LoginCallbackOutcome
	LoginCode string
}

// CompleteLoginOutcome is the decision a CompleteLogin call reached.
type CompleteLoginOutcome string

const (
	// LoginCompleted means the account is signed in with new credentials.
	LoginCompleted CompleteLoginOutcome = "completed"

	// LoginCodeInvalid means the code is unknown, used, expired, or does not
	// match the verifier: the login must start again.
	LoginCodeInvalid CompleteLoginOutcome = "login_code_invalid"
)

// CompleteLoginResult is what a CompleteLogin call decided. Credentials are
// set only when Outcome is LoginCompleted.
type CompleteLoginResult struct {
	Outcome     CompleteLoginOutcome
	Credentials Credentials
}

// SetDisplayNameOutcome is the decision a SetDisplayName call reached.
type SetDisplayNameOutcome string

const (
	// DisplayNameSet means the User is now named as asked.
	DisplayNameSet SetDisplayNameOutcome = "set"

	// DisplayNameInvalid means the name breaks Identity's rules.
	DisplayNameInvalid SetDisplayNameOutcome = "invalid_display_name"
)

// SetDisplayNameResult is what a SetDisplayName call decided. User is set
// only when Outcome is DisplayNameSet.
type SetDisplayNameResult struct {
	Outcome SetDisplayNameOutcome
	User    User
}

// ErrUserNotFound is returned when a User an operation names does not exist.
var ErrUserNotFound = errors.New("user not found")

// ErrIdempotencyConflict is returned when an idempotency key is reused for a
// request that differs from the one it was first used with.
var ErrIdempotencyConflict = errors.New("idempotency key reused with a conflicting request")

// ResolveResourceLinkOutcome is what a resource link token turned out to be.
type ResolveResourceLinkOutcome string

const (
	// ResourceLinkValid means the token is a live link: its holder may act
	// for UserUUID on the one resource it names.
	ResourceLinkValid ResolveResourceLinkOutcome = "valid"

	// ResourceLinkUnknown means no link has this token.
	ResourceLinkUnknown ResolveResourceLinkOutcome = "unknown"

	// ResourceLinkExpired means the link outlived its lifetime.
	ResourceLinkExpired ResolveResourceLinkOutcome = "expired"

	// ResourceLinkRevoked means the link was revoked or replaced by a newer
	// one for the same resource.
	ResourceLinkRevoked ResolveResourceLinkOutcome = "revoked"
)

// CreatedResourceLink is a new secret link to one resource. Token is shown
// only here: Identity keeps its hash.
type CreatedResourceLink struct {
	Token     string
	CreatedAt time.Time
	ExpiresAt time.Time
}

// ResourceLinkStatus is whether a User has a live link to a resource.
// CreatedAt and ExpiresAt are set only when Active.
type ResourceLinkStatus struct {
	Active    bool
	CreatedAt time.Time
	ExpiresAt time.Time
}

// ResolvedResourceLink is who a link token acts for and on what. UserUUID,
// Kind, ResourceID and ExpiresAt are set only when Outcome is
// ResourceLinkValid. Identity never knows what the resource is.
type ResolvedResourceLink struct {
	Outcome    ResolveResourceLinkOutcome
	UserUUID   string
	Kind       string
	ResourceID string
	ExpiresAt  time.Time
}

// AIScopeGames is the one scope an AI application is granted: reading,
// creating and changing its User's games, never publishing them.
const AIScopeGames = "games"

// AIAuthError is an OAuth error an AI authorization operation answers, with
// its RFC 6749 code (invalid_request, invalid_client, invalid_grant,
// unauthorized_client, unsupported_grant_type, invalid_scope,
// invalid_target, invalid_redirect_uri, invalid_client_metadata,
// access_denied) and a description for the application's developer.
type AIAuthError struct {
	Code        string
	Description string
}

func (e *AIAuthError) Error() string {
	return e.Code + ": " + e.Description
}

// AIClientMetadata is what an AI application registers itself with (RFC
// 7591). Only public clients exist: they authenticate with PKCE, never a
// secret.
type AIClientMetadata struct {
	ClientName              string
	ClientURI               string
	RedirectURIs            []string
	GrantTypes              []string
	ResponseTypes           []string
	TokenEndpointAuthMethod string
	Scope                   string
}

// RegisteredAIClient is an AI application Identity knows, as registered.
type RegisteredAIClient struct {
	ClientID                string
	ClientIDIssuedAt        time.Time
	ClientName              string
	ClientURI               string
	RedirectURIs            []string
	GrantTypes              []string
	ResponseTypes           []string
	TokenEndpointAuthMethod string
	Scope                   string
}

// AIAuthorizationParams are an authorization request's parameters, as the
// application's browser sent them.
type AIAuthorizationParams struct {
	ResponseType        string
	ClientID            string
	RedirectURI         string
	State               string
	CodeChallenge       string
	CodeChallengeMethod string
	Scope               string
	Resource            string
}

// AIAuthorizationStartOutcome is what an authorization request turned out
// to be.
type AIAuthorizationStartOutcome string

const (
	// AIAuthorizationStarted means the request waits for its User's decision
	// under RequestID.
	AIAuthorizationStarted AIAuthorizationStartOutcome = "started"
	// AIAuthorizationRedirectError means the request is refused and the
	// browser goes back to the application's RedirectURL with the error.
	AIAuthorizationRedirectError AIAuthorizationStartOutcome = "redirect_error"
	// AIAuthorizationUntrusted means the client or its redirect URI are not
	// trusted: the error is shown, the browser is never redirected.
	AIAuthorizationUntrusted AIAuthorizationStartOutcome = "untrusted"
)

// AIAuthorizationStart is the answer to an authorization request.
type AIAuthorizationStart struct {
	Outcome     AIAuthorizationStartOutcome
	RequestID   string
	RedirectURL string
	Error       *AIAuthError
}

// PendingAIAuthorization is what a User is asked to approve.
type PendingAIAuthorization struct {
	ClientName   string
	ClientURI    string
	RedirectHost string
	Scopes       []string
	ExpiresAt    time.Time
}

// ErrAIAuthorizationNotFound is an authorization request that does not
// exist, expired or was already decided.
var ErrAIAuthorizationNotFound = errors.New("AI authorization request not found")

// AITokenParams are a token request's parameters (RFC 6749 §4.1.3, §6).
type AITokenParams struct {
	GrantType    string
	ClientID     string
	Code         string
	RedirectURI  string
	CodeVerifier string
	RefreshToken string
	Resource     string
}

// AITokens are what a token request is granted.
type AITokens struct {
	AccessToken  string
	ExpiresIn    int64
	RefreshToken string
	Scope        string
}

// AIAccess is whom an AI access token acts for, valid until ExpiresAt.
type AIAccess struct {
	UserUUID     string
	ConnectionID string
	Scopes       []string
	ExpiresAt    time.Time
}

// AIConnection is an AI application a User approved, until revoked.
// LastUsedAt is the last time it was granted tokens.
type AIConnection struct {
	ConnectionID string
	ClientName   string
	CreatedAt    time.Time
	LastUsedAt   time.Time
}
