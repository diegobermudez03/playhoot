package aiauthorization

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/identity/internal/business/aiauth"
	"github.com/diegobermudez03/playhoot/identity/internal/testdb"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m))
}

const (
	issuer   = "https://api.playhoot.test"
	resource = "https://api.playhoot.test/mcp"
	consent  = "https://playhoot.test/connect-ai"
	redirect = "http://127.0.0.1:33418/callback"
)

func TestAIAuthorization(t *testing.T) {
	ctx := context.Background()
	db := testdb.OpenIdentityDB(t)
	service, err := New(db, []byte(strings.Repeat("k", 32)), Config{Issuer: issuer, Resource: resource, ConsentURL: consent})
	require.NoError(t, err)
	now := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }

	newUser := func(t *testing.T) string {
		t.Helper()
		userUUID := uuid.NewString()
		require.NoError(t, db.Exec(`INSERT INTO identity_users (user_uuid, display_name) VALUES (?, ?)`, userUUID, "Ana").Error)
		return userUUID
	}
	verifier := strings.Repeat("v", 50)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	register := func(t *testing.T) identity.RegisteredAIClient {
		t.Helper()
		client, err := service.RegisterClient(ctx, identity.AIClientMetadata{ClientName: "Claude", RedirectURIs: []string{redirect}})
		require.NoError(t, err)
		return client
	}
	start := func(t *testing.T, client identity.RegisteredAIClient) identity.AIAuthorizationStart {
		t.Helper()
		started, err := service.StartAuthorization(ctx, identity.AIAuthorizationParams{
			ResponseType: "code", ClientID: client.ClientID, RedirectURI: redirect, State: "s1",
			CodeChallenge: challenge, CodeChallengeMethod: "S256", Scope: "games", Resource: resource,
		})
		require.NoError(t, err)
		require.Equal(t, identity.AIAuthorizationStarted, started.Outcome)
		return started
	}
	approve := func(t *testing.T, user string, started identity.AIAuthorizationStart) string {
		t.Helper()
		back, err := service.DecideAuthorization(ctx, user, started.RequestID, true)
		require.NoError(t, err)
		parsed, err := url.Parse(back)
		require.NoError(t, err)
		require.Equal(t, "s1", parsed.Query().Get("state"))
		require.Equal(t, issuer, parsed.Query().Get("iss"))
		return parsed.Query().Get("code")
	}
	exchange := func(t *testing.T, client identity.RegisteredAIClient, code string) identity.AITokens {
		t.Helper()
		tokens, err := service.ExchangeToken(ctx, identity.AITokenParams{GrantType: "authorization_code", ClientID: client.ClientID, Code: code, RedirectURI: redirect, CodeVerifier: verifier, Resource: resource})
		require.NoError(t, err)
		return tokens
	}
	refusal := func(t *testing.T, err error) string {
		t.Helper()
		var oauth *identity.AIAuthError
		require.ErrorAs(t, err, &oauth)
		return oauth.Code
	}

	t.Run("a registered client is approved, exchanges its code with PKCE and reaches only the MCP resource", func(t *testing.T) {
		user := newUser(t)
		client := register(t)
		started := start(t, client)
		require.Equal(t, consent+"?request="+started.RequestID, started.RedirectURL)

		pending, err := service.ReadAuthorization(ctx, started.RequestID)
		require.NoError(t, err)
		require.Equal(t, identity.PendingAIAuthorization{ClientName: "Claude", RedirectHost: "127.0.0.1:33418", Scopes: []string{"games"}, ExpiresAt: now.Add(aiauth.RequestLifetime)}, pending)

		code := approve(t, user, started)
		_, err = service.ReadAuthorization(ctx, started.RequestID)
		require.ErrorIs(t, err, identity.ErrAIAuthorizationNotFound, "a decided request is gone")

		_, err = service.ExchangeToken(ctx, identity.AITokenParams{GrantType: "authorization_code", ClientID: client.ClientID, Code: code, RedirectURI: redirect, CodeVerifier: strings.Repeat("w", 50)})
		require.Equal(t, "invalid_grant", refusal(t, err), "a wrong verifier")

		tokens := exchange(t, client, code)
		require.Equal(t, int64(3600), tokens.ExpiresIn)
		require.Equal(t, "games", tokens.Scope)
		access, valid, err := service.VerifyAccessToken(ctx, tokens.AccessToken)
		require.NoError(t, err)
		require.True(t, valid)
		require.Equal(t, user, access.UserUUID)

		var stored []string
		require.NoError(t, db.Raw(`SELECT token_sha256 FROM identity_ai_refresh_tokens`).Scan(&stored).Error)
		require.Contains(t, stored, aiauth.Hash(tokens.RefreshToken))
		require.NotContains(t, stored, tokens.RefreshToken, "only hashes are kept")

		connections, err := service.ListConnections(ctx, user)
		require.NoError(t, err)
		require.Len(t, connections, 1)
		require.Equal(t, "Claude", connections[0].ClientName)
		require.Equal(t, access.ConnectionID, connections[0].ConnectionID)
	})

	t.Run("a code used twice revokes the connection it made", func(t *testing.T) {
		user := newUser(t)
		client := register(t)
		code := approve(t, user, start(t, client))
		tokens := exchange(t, client, code)
		_, err := service.ExchangeToken(ctx, identity.AITokenParams{GrantType: "authorization_code", ClientID: client.ClientID, Code: code, RedirectURI: redirect, CodeVerifier: verifier})
		require.Equal(t, "invalid_grant", refusal(t, err))
		_, valid, err := service.VerifyAccessToken(ctx, tokens.AccessToken)
		require.NoError(t, err)
		require.False(t, valid)
	})

	t.Run("refresh tokens rotate, and reusing an old one revokes the connection", func(t *testing.T) {
		user := newUser(t)
		client := register(t)
		first := exchange(t, client, approve(t, user, start(t, client)))
		second, err := service.ExchangeToken(ctx, identity.AITokenParams{GrantType: "refresh_token", ClientID: client.ClientID, RefreshToken: first.RefreshToken})
		require.NoError(t, err)
		require.NotEqual(t, first.RefreshToken, second.RefreshToken)
		_, valid, err := service.VerifyAccessToken(ctx, second.AccessToken)
		require.NoError(t, err)
		require.True(t, valid)

		_, err = service.ExchangeToken(ctx, identity.AITokenParams{GrantType: "refresh_token", ClientID: client.ClientID, RefreshToken: first.RefreshToken})
		require.Equal(t, "invalid_grant", refusal(t, err))
		_, valid, err = service.VerifyAccessToken(ctx, second.AccessToken)
		require.NoError(t, err)
		require.False(t, valid, "the whole connection is revoked")
		_, err = service.ExchangeToken(ctx, identity.AITokenParams{GrantType: "refresh_token", ClientID: client.ClientID, RefreshToken: second.RefreshToken})
		require.Equal(t, "invalid_grant", refusal(t, err))
	})

	t.Run("untrusted clients and redirects are never redirected to; other problems go back to the client", func(t *testing.T) {
		client := register(t)
		unknown, err := service.StartAuthorization(ctx, identity.AIAuthorizationParams{ResponseType: "code", ClientID: uuid.NewString(), RedirectURI: redirect, CodeChallenge: challenge, CodeChallengeMethod: "S256"})
		require.NoError(t, err)
		require.Equal(t, identity.AIAuthorizationUntrusted, unknown.Outcome)
		require.Empty(t, unknown.RedirectURL)

		elsewhere, err := service.StartAuthorization(ctx, identity.AIAuthorizationParams{ResponseType: "code", ClientID: client.ClientID, RedirectURI: "https://evil.example/cb", CodeChallenge: challenge, CodeChallengeMethod: "S256"})
		require.NoError(t, err)
		require.Equal(t, identity.AIAuthorizationUntrusted, elsewhere.Outcome)

		for name, params := range map[string]identity.AIAuthorizationParams{
			"no PKCE":          {ResponseType: "code", ClientID: client.ClientID, RedirectURI: redirect, State: "s"},
			"plain PKCE":       {ResponseType: "code", ClientID: client.ClientID, RedirectURI: redirect, CodeChallenge: challenge, CodeChallengeMethod: "plain"},
			"token response":   {ResponseType: "token", ClientID: client.ClientID, RedirectURI: redirect, CodeChallenge: challenge, CodeChallengeMethod: "S256"},
			"another resource": {ResponseType: "code", ClientID: client.ClientID, RedirectURI: redirect, CodeChallenge: challenge, CodeChallengeMethod: "S256", Resource: "https://other.example/mcp"},
			"unknown scope":    {ResponseType: "code", ClientID: client.ClientID, RedirectURI: redirect, CodeChallenge: challenge, CodeChallengeMethod: "S256", Scope: "publish"},
		} {
			back, err := service.StartAuthorization(ctx, params)
			require.NoError(t, err, name)
			require.Equal(t, identity.AIAuthorizationRedirectError, back.Outcome, name)
			require.True(t, strings.HasPrefix(back.RedirectURL, redirect+"?"), name)
			require.Contains(t, back.RedirectURL, "error=", name)
		}
	})

	t.Run("a denied request goes back with access_denied and grants nothing", func(t *testing.T) {
		user := newUser(t)
		client := register(t)
		back, err := service.DecideAuthorization(ctx, user, start(t, client).RequestID, false)
		require.NoError(t, err)
		require.Contains(t, back, "error=access_denied")
		require.NotContains(t, back, "code=")
	})

	t.Run("requests and codes expire", func(t *testing.T) {
		user := newUser(t)
		client := register(t)
		started := start(t, client)
		code := approve(t, user, started)
		later := service.now
		service.now = func() time.Time { return now.Add(aiauth.CodeLifetime) }
		_, err := service.ExchangeToken(ctx, identity.AITokenParams{GrantType: "authorization_code", ClientID: client.ClientID, Code: code, CodeVerifier: verifier})
		require.Equal(t, "invalid_grant", refusal(t, err))

		waiting := start(t, client)
		service.now = func() time.Time { return now.Add(aiauth.CodeLifetime + aiauth.RequestLifetime) }
		_, err = service.DecideAuthorization(ctx, user, waiting.RequestID, true)
		require.ErrorIs(t, err, identity.ErrAIAuthorizationNotFound)
		service.now = later
	})

	t.Run("a user revokes a connection, and a client revokes its own", func(t *testing.T) {
		user := newUser(t)
		client := register(t)
		tokens := exchange(t, client, approve(t, user, start(t, client)))
		access, _, err := service.VerifyAccessToken(ctx, tokens.AccessToken)
		require.NoError(t, err)

		revoked, err := service.RevokeConnection(ctx, newUser(t), access.ConnectionID)
		require.NoError(t, err)
		require.False(t, revoked, "another user cannot revoke it")
		revoked, err = service.RevokeConnection(ctx, user, access.ConnectionID)
		require.NoError(t, err)
		require.True(t, revoked)
		_, valid, err := service.VerifyAccessToken(ctx, tokens.AccessToken)
		require.NoError(t, err)
		require.False(t, valid)
		connections, err := service.ListConnections(ctx, user)
		require.NoError(t, err)
		require.Empty(t, connections)

		other := exchange(t, client, approve(t, user, start(t, client)))
		require.NoError(t, service.Revoke(ctx, other.RefreshToken, client.ClientID))
		_, valid, err = service.VerifyAccessToken(ctx, other.AccessToken)
		require.NoError(t, err)
		require.False(t, valid)
		require.NoError(t, service.Revoke(ctx, "not-a-token", client.ClientID), "an unknown token is not an error")
	})

	t.Run("an invalid registration is an OAuth error", func(t *testing.T) {
		_, err := service.RegisterClient(ctx, identity.AIClientMetadata{ClientName: "x", RedirectURIs: []string{"http://evil.example/cb"}})
		require.Equal(t, "invalid_redirect_uri", refusal(t, err))
		_, err = service.RegisterClient(ctx, identity.AIClientMetadata{ClientName: "x", RedirectURIs: []string{redirect}, TokenEndpointAuthMethod: "client_secret_basic"})
		require.Equal(t, "invalid_client_metadata", refusal(t, err))
	})
}
