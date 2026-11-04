// Package aiauthorization is Identity's OAuth authorization server for AI
// applications (MCP clients): they register themselves, a User approves one
// on a Playhoot page, and it gets tokens only the MCP resource accepts, for
// that User's games and never for publishing. Every connection can be
// revoked; secrets are kept only as hashes.
package aiauthorization

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/identity/internal/business/aiauth"
	internalrepo "github.com/diegobermudez03/playhoot/identity/usecases/aiauthorization/internal/repo"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/utils"
)

// store persists AI applications' access.
type store interface {
	CreateClient(ctx context.Context, client internalrepo.Client) error
	FindClient(ctx context.Context, tx *gorm.DB, clientID string) (*internalrepo.Client, error)
	CreateRequest(ctx context.Context, request internalrepo.Request) error
	FindRequest(ctx context.Context, tx *gorm.DB, requestID string, lock bool) (*internalrepo.Request, error)
	FindRequestByCode(ctx context.Context, tx *gorm.DB, codeSHA256 string) (*internalrepo.Request, error)
	DecideRequest(ctx context.Context, tx *gorm.DB, id int64, decidedAt time.Time, userID int64, approved bool, codeSHA256 *string, codeExpiresAt *time.Time) (bool, error)
	UseCode(ctx context.Context, tx *gorm.DB, id int64, usedAt time.Time, sessionRowID int64) (bool, error)
	FindUserID(ctx context.Context, tx *gorm.DB, userUUID string) (int64, bool, error)
	CreateSession(ctx context.Context, tx *gorm.DB, session internalrepo.Session) (int64, error)
	FindSession(ctx context.Context, tx *gorm.DB, connectionID string) (*internalrepo.Session, error)
	RevokeSession(ctx context.Context, tx *gorm.DB, id int64, revokedAt time.Time, reason string) (bool, error)
	ListLiveSessions(ctx context.Context, userUUID string) ([]internalrepo.Connection, error)
	CreateRefreshToken(ctx context.Context, tx *gorm.DB, sessionRowID int64, tokenSHA256 string, createdAt time.Time, expiresAt time.Time) error
	FindRefreshToken(ctx context.Context, tx *gorm.DB, tokenSHA256 string) (*internalrepo.RefreshToken, error)
	UseRefreshToken(ctx context.Context, tx *gorm.DB, id int64, usedAt time.Time) (bool, error)
}

// Why a connection was revoked.
const (
	revokedByUser          = "user"
	revokedByClient        = "client"
	revokedForCodeReuse    = "code_reuse"
	revokedForRefreshReuse = "refresh_reuse"
)

var (
	operationLabel = metrics.NewLabel("operation",
		"registered", "authorization_started", "authorization_refused", "approved", "denied",
		"code_exchanged", "refreshed", "token_refused", "reuse_detected", "revoked")

	aiAuthorizationTotal = metrics.NewCounter(
		"identity_ai_authorization_total",
		"AI application authorization operations: registrations, authorization requests and decisions, token grants and refusals, detected secret reuse and revocations.",
		operationLabel,
	)
)

// Config is where the authorization server and its one resource live.
type Config struct {
	// Issuer is the authorization server's identifier, the API's public base
	// URL; Resource the MCP endpoint the tokens are for.
	Issuer   string
	Resource string
	// ConsentURL is the web page a User approves a request on; the request
	// id is added to its query as "request".
	ConsentURL string
}

// Service is Identity's AI authorization use case.
type Service struct {
	db     utils.DBServicer
	store  store
	signer *aiauth.Signer
	config Config
	now    func() time.Time
	newID  func() string
}

// New constructs a Service on db, signing access tokens with a key derived
// from Identity's access token key.
func New(db *gorm.DB, accessTokenKey []byte, config Config) (*Service, error) {
	signer, err := aiauth.NewSigner(accessTokenKey, config.Issuer, config.Resource)
	if err != nil {
		return nil, err
	}
	repo := internalrepo.New(db)
	return &Service{db: repo, store: repo, signer: signer, config: config, now: time.Now, newID: uuid.NewString}, nil
}

func (s *Service) clock() time.Time {
	return s.now().UTC().Truncate(time.Microsecond)
}

func oauthError(code string, description string) *identity.AIAuthError {
	return &identity.AIAuthError{Code: code, Description: description}
}

// RegisterClient registers an AI application (RFC 7591). An invalid
// registration is an *identity.AIAuthError.
func (s *Service) RegisterClient(ctx context.Context, metadata identity.AIClientMetadata) (identity.RegisteredAIClient, error) {
	defer logging.Step(ctx, "aiauthorization.register_client").Close()
	checked, err := aiauth.CheckRegistration(metadata.ClientName, metadata.ClientURI, metadata.RedirectURIs, metadata.GrantTypes, metadata.ResponseTypes, metadata.TokenEndpointAuthMethod, metadata.Scope)
	if err != nil {
		code := "invalid_client_metadata"
		if strings.Contains(err.Error(), "redirect") {
			code = "invalid_redirect_uri"
		}
		return identity.RegisteredAIClient{}, oauthError(code, strings.TrimPrefix(err.Error(), aiauth.ErrInvalidMetadata.Error()+": "))
	}
	now := s.clock()
	client := internalrepo.Client{ClientID: s.newID(), ClientName: checked.ClientName, RedirectURIs: checked.RedirectURIs, GrantTypes: checked.GrantTypes, CreatedAt: now}
	if checked.ClientURI != "" {
		client.ClientURI = &checked.ClientURI
	}
	if err := s.store.CreateClient(ctx, client); err != nil {
		return identity.RegisteredAIClient{}, err
	}
	aiAuthorizationTotal.Inc(ctx, operationLabel.Value("registered"))
	logging.LogFields(ctx, logging.Field("ai_client_id", client.ClientID))
	return identity.RegisteredAIClient{
		ClientID: client.ClientID, ClientIDIssuedAt: now, ClientName: checked.ClientName, ClientURI: checked.ClientURI,
		RedirectURIs: checked.RedirectURIs, GrantTypes: checked.GrantTypes, ResponseTypes: []string{aiauth.ResponseTypeCode},
		TokenEndpointAuthMethod: aiauth.AuthMethodNone, Scope: aiauth.ScopeGames,
	}, nil
}

// StartAuthorization checks an authorization request (RFC 6749 §4.1.1 with
// PKCE and resource indicators) and keeps it waiting for its User's
// decision. A client or redirect URI that cannot be trusted is never
// redirected to; any other problem goes back to the application.
func (s *Service) StartAuthorization(ctx context.Context, params identity.AIAuthorizationParams) (identity.AIAuthorizationStart, error) {
	defer logging.Step(ctx, "aiauthorization.start").Close()
	refused := func(start identity.AIAuthorizationStart) (identity.AIAuthorizationStart, error) {
		aiAuthorizationTotal.Inc(ctx, operationLabel.Value("authorization_refused"))
		logging.LogFields(ctx, logging.Field("ai_authorization", string(start.Outcome)))
		return start, nil
	}
	client, err := s.store.FindClient(ctx, s.db.GetDB(), params.ClientID)
	if err != nil {
		return identity.AIAuthorizationStart{}, err
	}
	if client == nil {
		return refused(identity.AIAuthorizationStart{Outcome: identity.AIAuthorizationUntrusted, Error: oauthError("invalid_client", "this client_id is not registered")})
	}
	redirectURI := params.RedirectURI
	if redirectURI == "" && len(client.RedirectURIs) == 1 {
		redirectURI = client.RedirectURIs[0]
	}
	if !slices.Contains(client.RedirectURIs, redirectURI) {
		return refused(identity.AIAuthorizationStart{Outcome: identity.AIAuthorizationUntrusted, Error: oauthError("invalid_request", "redirect_uri is not one the client registered")})
	}
	back := func(code string, description string) (identity.AIAuthorizationStart, error) {
		return refused(identity.AIAuthorizationStart{
			Outcome:     identity.AIAuthorizationRedirectError,
			RedirectURL: aiauth.WithQuery(redirectURI, map[string]string{"error": code, "error_description": description, "state": params.State, "iss": s.config.Issuer}),
			Error:       oauthError(code, description),
		})
	}
	switch {
	case params.ResponseType != aiauth.ResponseTypeCode:
		return back("unsupported_response_type", "response_type must be code")
	case params.CodeChallengeMethod != aiauth.ChallengeMethodS256 || !aiauth.ValidChallenge(params.CodeChallenge):
		return back("invalid_request", "PKCE is required: code_challenge with code_challenge_method S256")
	case params.Resource != "" && strings.TrimRight(params.Resource, "/") != s.config.Resource:
		return back("invalid_target", "resource must be "+s.config.Resource)
	}
	scope, ok := aiauth.CheckScope(params.Scope)
	if !ok {
		return back("invalid_scope", "the only scope is "+aiauth.ScopeGames)
	}
	now := s.clock()
	request := internalrepo.Request{
		RequestID: s.newID(), ClientRowID: client.ID, RedirectURI: redirectURI, CodeChallenge: params.CodeChallenge,
		Scope: scope, Resource: s.config.Resource, CreatedAt: now, ExpiresAt: now.Add(aiauth.RequestLifetime),
	}
	if params.State != "" {
		request.State = &params.State
	}
	if err := s.store.CreateRequest(ctx, request); err != nil {
		return identity.AIAuthorizationStart{}, err
	}
	aiAuthorizationTotal.Inc(ctx, operationLabel.Value("authorization_started"))
	return identity.AIAuthorizationStart{
		Outcome: identity.AIAuthorizationStarted, RequestID: request.RequestID,
		RedirectURL: aiauth.WithQuery(s.config.ConsentURL, map[string]string{"request": request.RequestID}),
	}, nil
}

// pending returns requestID while it waits for a decision, or nil.
func (s *Service) pending(request *internalrepo.Request) bool {
	return request != nil && request.DecidedAt == nil && s.now().Before(request.ExpiresAt)
}

// ReadAuthorization returns what a User is asked to approve, or
// identity.ErrAIAuthorizationNotFound.
func (s *Service) ReadAuthorization(ctx context.Context, requestID string) (identity.PendingAIAuthorization, error) {
	request, err := s.store.FindRequest(ctx, s.db.GetDB(), requestID, false)
	if err != nil {
		return identity.PendingAIAuthorization{}, err
	}
	if !s.pending(request) {
		return identity.PendingAIAuthorization{}, identity.ErrAIAuthorizationNotFound
	}
	pending := identity.PendingAIAuthorization{
		ClientName: request.ClientName, RedirectHost: aiauth.RedirectHost(request.RedirectURI),
		Scopes: strings.Fields(request.Scope), ExpiresAt: request.ExpiresAt.UTC(),
	}
	if request.ClientURI != nil {
		pending.ClientURI = *request.ClientURI
	}
	return pending, nil
}

// DecideAuthorization records userUUID's decision on requestID and returns
// where the browser goes back to the application: with a code when
// approved, with access_denied otherwise.
func (s *Service) DecideAuthorization(ctx context.Context, userUUID string, requestID string, approve bool) (string, error) {
	defer logging.Step(ctx, "aiauthorization.decide").Close()
	var code string
	if approve {
		secret, err := aiauth.NewSecret(aiauth.CodePrefix)
		if err != nil {
			return "", err
		}
		code = secret
	}
	now := s.clock()
	redirectURL, err := utils.RunInDBTransaction(ctx, s.db, func(ctx context.Context, tx *gorm.DB) (string, error) {
		request, err := s.store.FindRequest(ctx, tx, requestID, true)
		if err != nil {
			return "", err
		}
		if !s.pending(request) {
			return "", identity.ErrAIAuthorizationNotFound
		}
		userID, found, err := s.store.FindUserID(ctx, tx, userUUID)
		if err != nil {
			return "", err
		}
		if !found {
			return "", identity.ErrUserNotFound
		}
		var codeHash *string
		var codeExpiresAt *time.Time
		if approve {
			hash := aiauth.Hash(code)
			expires := now.Add(aiauth.CodeLifetime)
			codeHash, codeExpiresAt = &hash, &expires
		}
		decided, err := s.store.DecideRequest(ctx, tx, request.ID, now, userID, approve, codeHash, codeExpiresAt)
		if err != nil {
			return "", err
		}
		if !decided {
			return "", identity.ErrAIAuthorizationNotFound
		}
		state := ""
		if request.State != nil {
			state = *request.State
		}
		params := map[string]string{"state": state, "iss": s.config.Issuer}
		if approve {
			params["code"] = code
		} else {
			params["error"] = "access_denied"
			params["error_description"] = "the user did not approve the access"
		}
		return aiauth.WithQuery(request.RedirectURI, params), nil
	})
	if err != nil {
		return "", err
	}
	if approve {
		aiAuthorizationTotal.Inc(ctx, operationLabel.Value("approved"))
	} else {
		aiAuthorizationTotal.Inc(ctx, operationLabel.Value("denied"))
	}
	logging.LogFields(ctx, logging.Field("ai_authorization_approved", approve))
	return redirectURL, nil
}

// ExchangeToken answers a token request: an authorization code (with its
// PKCE verifier) or a refresh token, which is rotated. A refused request is
// an *identity.AIAuthError. A code or refresh token used twice revokes the
// connection it belongs to.
func (s *Service) ExchangeToken(ctx context.Context, params identity.AITokenParams) (identity.AITokens, error) {
	defer logging.Step(ctx, "aiauthorization.token").Close()
	var tokens identity.AITokens
	var refusal *identity.AIAuthError
	switch params.GrantType {
	case aiauth.GrantAuthorizationCode:
		tokens, refusal = s.exchangeCode(ctx, params)
	case aiauth.GrantRefreshToken:
		tokens, refusal = s.refresh(ctx, params)
	default:
		refusal = oauthError("unsupported_grant_type", "grant_type must be authorization_code or refresh_token")
	}
	if refusal != nil {
		if refusal.Code == "server_error" {
			return identity.AITokens{}, errors.New(refusal.Description)
		}
		aiAuthorizationTotal.Inc(ctx, operationLabel.Value("token_refused"))
		logging.LogFields(ctx, logging.Field("ai_token_refused", refusal.Code))
		return identity.AITokens{}, refusal
	}
	return tokens, nil
}

func serverError(err error) *identity.AIAuthError {
	return &identity.AIAuthError{Code: "server_error", Description: err.Error()}
}

// issued is a connection granted new tokens inside a transaction.
type issued struct {
	userUUID     string
	connectionID string
	clientID     string
	scope        string
	refresh      string
	// reused means the secret had already been used: its connection was
	// revoked, and that revocation commits before the refusal is answered.
	reused bool
}

func (s *Service) exchangeCode(ctx context.Context, params identity.AITokenParams) (identity.AITokens, *identity.AIAuthError) {
	if !aiauth.WellFormed(params.Code, aiauth.CodePrefix) {
		return identity.AITokens{}, oauthError("invalid_grant", "the code is not valid")
	}
	refresh, err := aiauth.NewSecret(aiauth.RefreshPrefix)
	if err != nil {
		return identity.AITokens{}, serverError(err)
	}
	now := s.clock()
	granted, err := utils.RunInDBTransaction(ctx, s.db, func(ctx context.Context, tx *gorm.DB) (issued, error) {
		request, err := s.store.FindRequestByCode(ctx, tx, aiauth.Hash(params.Code))
		if err != nil {
			return issued{}, err
		}
		switch {
		case request == nil || request.Approved == nil || !*request.Approved || request.DecidedUserID == nil:
			return issued{}, oauthError("invalid_grant", "the code is not valid")
		case request.CodeUsedAt != nil:
			if request.SessionRowID != nil {
				if _, err := s.store.RevokeSession(ctx, tx, *request.SessionRowID, now, revokedForCodeReuse); err != nil {
					return issued{}, err
				}
			}
			return issued{reused: true}, nil
		case request.CodeExpiresAt == nil || !now.Before(*request.CodeExpiresAt):
			return issued{}, oauthError("invalid_grant", "the code expired")
		case request.ClientID != params.ClientID:
			return issued{}, oauthError("invalid_grant", "the code was issued to another client")
		case params.RedirectURI != "" && params.RedirectURI != request.RedirectURI:
			return issued{}, oauthError("invalid_grant", "redirect_uri does not match the authorization request")
		case !aiauth.VerifyPKCE(params.CodeVerifier, request.CodeChallenge):
			return issued{}, oauthError("invalid_grant", "code_verifier does not match the code challenge")
		case params.Resource != "" && strings.TrimRight(params.Resource, "/") != request.Resource:
			return issued{}, oauthError("invalid_target", "resource must be "+request.Resource)
		}
		connectionID := s.newID()
		sessionRowID, err := s.store.CreateSession(ctx, tx, internalrepo.Session{
			ConnectionID: connectionID, UserID: *request.DecidedUserID, ClientRowID: request.ClientRowID,
			Scope: request.Scope, Resource: request.Resource, CreatedAt: now,
		})
		if err != nil {
			return issued{}, err
		}
		used, err := s.store.UseCode(ctx, tx, request.ID, now, sessionRowID)
		if err != nil {
			return issued{}, err
		}
		if !used {
			return issued{}, oauthError("invalid_grant", "the code is not valid")
		}
		if err := s.store.CreateRefreshToken(ctx, tx, sessionRowID, aiauth.Hash(refresh), now, now.Add(aiauth.RefreshLifetime)); err != nil {
			return issued{}, err
		}
		session, err := s.store.FindSession(ctx, tx, connectionID)
		if err != nil || session == nil {
			return issued{}, fmt.Errorf("reading the new AI session back: %v", err)
		}
		return issued{userUUID: session.UserUUID, connectionID: connectionID, clientID: request.ClientID, scope: request.Scope, refresh: refresh}, nil
	})
	if refusal := s.refusalOf(ctx, granted, err); refusal != nil {
		return identity.AITokens{}, refusal
	}
	aiAuthorizationTotal.Inc(ctx, operationLabel.Value("code_exchanged"))
	return s.grant(granted, now)
}

func (s *Service) refresh(ctx context.Context, params identity.AITokenParams) (identity.AITokens, *identity.AIAuthError) {
	if !aiauth.WellFormed(params.RefreshToken, aiauth.RefreshPrefix) {
		return identity.AITokens{}, oauthError("invalid_grant", "the refresh token is not valid")
	}
	rotated, err := aiauth.NewSecret(aiauth.RefreshPrefix)
	if err != nil {
		return identity.AITokens{}, serverError(err)
	}
	now := s.clock()
	granted, err := utils.RunInDBTransaction(ctx, s.db, func(ctx context.Context, tx *gorm.DB) (issued, error) {
		token, err := s.store.FindRefreshToken(ctx, tx, aiauth.Hash(params.RefreshToken))
		if err != nil {
			return issued{}, err
		}
		switch {
		case token == nil || token.RevokedAt != nil:
			return issued{}, oauthError("invalid_grant", "the refresh token is not valid")
		case token.UsedAt != nil:
			if _, err := s.store.RevokeSession(ctx, tx, token.SessionID, now, revokedForRefreshReuse); err != nil {
				return issued{}, err
			}
			return issued{reused: true}, nil
		case !now.Before(token.ExpiresAt):
			return issued{}, oauthError("invalid_grant", "the refresh token expired")
		case token.ClientID != params.ClientID:
			return issued{}, oauthError("invalid_grant", "the refresh token was issued to another client")
		case params.Resource != "" && strings.TrimRight(params.Resource, "/") != token.Resource:
			return issued{}, oauthError("invalid_target", "resource must be "+token.Resource)
		}
		used, err := s.store.UseRefreshToken(ctx, tx, token.ID, now)
		if err != nil {
			return issued{}, err
		}
		if !used {
			return issued{}, oauthError("invalid_grant", "the refresh token is not valid")
		}
		if err := s.store.CreateRefreshToken(ctx, tx, token.SessionID, aiauth.Hash(rotated), now, now.Add(aiauth.RefreshLifetime)); err != nil {
			return issued{}, err
		}
		return issued{userUUID: token.UserUUID, connectionID: token.ConnectionID, clientID: token.ClientID, scope: token.Scope, refresh: rotated}, nil
	})
	if refusal := s.refusalOf(ctx, granted, err); refusal != nil {
		return identity.AITokens{}, refusal
	}
	aiAuthorizationTotal.Inc(ctx, operationLabel.Value("refreshed"))
	return s.grant(granted, now)
}

// refusalOf turns a token transaction's outcome into its refusal, if any: a
// refusal it returned (nothing written), a detected reuse (the revocation
// committed), or a failure.
func (s *Service) refusalOf(ctx context.Context, granted issued, err error) *identity.AIAuthError {
	var refusal *identity.AIAuthError
	switch {
	case errors.As(err, &refusal):
		return refusal
	case err != nil:
		return serverError(err)
	case granted.reused:
		aiAuthorizationTotal.Inc(ctx, operationLabel.Value("reuse_detected"))
		return oauthError("invalid_grant", "this secret was already used: the connection is revoked")
	}
	return nil
}

func (s *Service) grant(granted issued, now time.Time) (identity.AITokens, *identity.AIAuthError) {
	access, _, err := s.signer.Issue(granted.userUUID, granted.connectionID, granted.clientID, granted.scope, now)
	if err != nil {
		return identity.AITokens{}, serverError(err)
	}
	return identity.AITokens{AccessToken: access, ExpiresIn: int64(aiauth.AccessLifetime / time.Second), RefreshToken: granted.refresh, Scope: granted.scope}, nil
}

// Revoke revokes the connection token belongs to (RFC 7009): a refresh
// token of clientID, or an access token. An unknown token is not an error.
func (s *Service) Revoke(ctx context.Context, token string, clientID string) error {
	defer logging.Step(ctx, "aiauthorization.revoke").Close()
	now := s.clock()
	revoked, err := utils.RunInDBTransaction(ctx, s.db, func(ctx context.Context, tx *gorm.DB) (bool, error) {
		if aiauth.WellFormed(token, aiauth.RefreshPrefix) {
			found, err := s.store.FindRefreshToken(ctx, tx, aiauth.Hash(token))
			if err != nil || found == nil || found.ClientID != clientID {
				return false, err
			}
			return s.store.RevokeSession(ctx, tx, found.SessionID, now, revokedByClient)
		}
		verified, err := s.signer.Verify(token, s.now())
		if err != nil || verified.ClientID != clientID {
			return false, nil
		}
		session, err := s.store.FindSession(ctx, tx, verified.ConnectionID)
		if err != nil || session == nil {
			return false, err
		}
		return s.store.RevokeSession(ctx, tx, session.ID, now, revokedByClient)
	})
	if err != nil {
		return err
	}
	if revoked {
		aiAuthorizationTotal.Inc(ctx, operationLabel.Value("revoked"))
	}
	return nil
}

// VerifyAccessToken says whom an AI access token acts for, false when it is
// not one for the MCP resource or its connection was revoked.
func (s *Service) VerifyAccessToken(ctx context.Context, token string) (identity.AIAccess, bool, error) {
	verified, err := s.signer.Verify(token, s.now())
	if err != nil {
		return identity.AIAccess{}, false, nil
	}
	session, err := s.store.FindSession(ctx, s.db.GetDB(), verified.ConnectionID)
	if err != nil {
		return identity.AIAccess{}, false, err
	}
	if session == nil || session.RevokedAt != nil || session.UserUUID != verified.UserUUID {
		return identity.AIAccess{}, false, nil
	}
	return identity.AIAccess{UserUUID: verified.UserUUID, ConnectionID: verified.ConnectionID, Scopes: verified.Scopes, ExpiresAt: verified.ExpiresAt}, true, nil
}

// ListConnections returns userUUID's live AI connections.
func (s *Service) ListConnections(ctx context.Context, userUUID string) ([]identity.AIConnection, error) {
	found, err := s.store.ListLiveSessions(ctx, userUUID)
	if err != nil {
		return nil, err
	}
	connections := make([]identity.AIConnection, len(found))
	for i, connection := range found {
		connections[i] = identity.AIConnection{ConnectionID: connection.ConnectionID, ClientName: connection.ClientName, CreatedAt: connection.CreatedAt.UTC(), LastUsedAt: connection.LastUsedAt.UTC()}
	}
	return connections, nil
}

// RevokeConnection revokes userUUID's connection connectionID; false when
// they have no such live connection.
func (s *Service) RevokeConnection(ctx context.Context, userUUID string, connectionID string) (bool, error) {
	defer logging.Step(ctx, "aiauthorization.revoke_connection").Close()
	now := s.clock()
	revoked, err := utils.RunInDBTransaction(ctx, s.db, func(ctx context.Context, tx *gorm.DB) (bool, error) {
		session, err := s.store.FindSession(ctx, tx, connectionID)
		if err != nil || session == nil || session.UserUUID != userUUID || session.RevokedAt != nil {
			return false, err
		}
		return s.store.RevokeSession(ctx, tx, session.ID, now, revokedByUser)
	})
	if err != nil {
		return false, err
	}
	if revoked {
		aiAuthorizationTotal.Inc(ctx, operationLabel.Value("revoked"))
	}
	return revoked, nil
}
