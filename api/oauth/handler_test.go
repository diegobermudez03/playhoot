package oauth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/identity"
)

const user = "6d0796d4-36b8-4817-b1cb-6522ce6b18ae"

func newHandler(t *testing.T) (*Handler, *MockauthorizationServer) {
	t.Helper()
	server := NewMockauthorizationServer(gomock.NewController(t))
	return NewHandler(nil, server, Config{Issuer: "https://api.test/", Resource: "https://api.test/mcp", Documentation: "https://web.test/docs/sdk/v1/index.md"}), server
}

func asUser(r *http.Request) *http.Request {
	return r.WithContext(auth.WithCaller(r.Context(), identity.User{UUID: user, Level: identity.LevelAccount}))
}

func decode(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body
}

func TestMetadata(t *testing.T) {
	h, _ := newHandler(t)
	w := httptest.NewRecorder()
	h.handleResourceMetadata(w, httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource/mcp", nil))
	require.Equal(t, http.StatusOK, w.Code)
	resource := decode(t, w)
	require.Equal(t, "https://api.test/mcp", resource["resource"])
	require.Equal(t, []any{"https://api.test"}, resource["authorization_servers"])
	require.Equal(t, []any{"games"}, resource["scopes_supported"])

	w = httptest.NewRecorder()
	h.handleServerMetadata(w, httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", nil))
	server := decode(t, w)
	require.Equal(t, "https://api.test", server["issuer"])
	require.Equal(t, "https://api.test/oauth/token", server["token_endpoint"])
	require.Equal(t, "https://api.test/oauth/register", server["registration_endpoint"])
	require.Equal(t, []any{"S256"}, server["code_challenge_methods_supported"])
	require.Equal(t, []any{"none"}, server["token_endpoint_auth_methods_supported"])
}

func TestRegister(t *testing.T) {
	ctx := gomock.Any()
	t.Run("a registration answers the client", func(t *testing.T) {
		h, server := newHandler(t)
		server.EXPECT().RegisterClient(ctx, identity.AIClientMetadata{ClientName: "Claude", RedirectURIs: []string{"https://claude.ai/cb"}}).
			Return(identity.RegisteredAIClient{ClientID: "c1", ClientIDIssuedAt: time.Unix(100, 0), ClientName: "Claude", RedirectURIs: []string{"https://claude.ai/cb"}, GrantTypes: []string{"authorization_code", "refresh_token"}, ResponseTypes: []string{"code"}, TokenEndpointAuthMethod: "none", Scope: "games"}, nil)
		w := httptest.NewRecorder()
		h.handleRegister(w, httptest.NewRequest(http.MethodPost, "/oauth/register", strings.NewReader(`{"client_name":"Claude","redirect_uris":["https://claude.ai/cb"]}`)))
		require.Equal(t, http.StatusCreated, w.Code)
		body := decode(t, w)
		require.Equal(t, "c1", body["client_id"])
		require.Equal(t, float64(100), body["client_id_issued_at"])
		require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	})
	t.Run("an invalid registration is an OAuth error", func(t *testing.T) {
		h, server := newHandler(t)
		server.EXPECT().RegisterClient(ctx, gomock.Any()).Return(identity.RegisteredAIClient{}, &identity.AIAuthError{Code: "invalid_redirect_uri", Description: "bad"})
		w := httptest.NewRecorder()
		h.handleRegister(w, httptest.NewRequest(http.MethodPost, "/oauth/register", strings.NewReader(`{"redirect_uris":["http://evil/cb"]}`)))
		require.Equal(t, http.StatusBadRequest, w.Code)
		require.Equal(t, "invalid_redirect_uri", decode(t, w)["error"])
	})
}

func TestAuthorize(t *testing.T) {
	query := "?response_type=code&client_id=c1&redirect_uri=https%3A%2F%2Fclaude.ai%2Fcb&state=s1&code_challenge=abc&code_challenge_method=S256&resource=https%3A%2F%2Fapi.test%2Fmcp"
	params := identity.AIAuthorizationParams{ResponseType: "code", ClientID: "c1", RedirectURI: "https://claude.ai/cb", State: "s1", CodeChallenge: "abc", CodeChallengeMethod: "S256", Resource: "https://api.test/mcp"}
	for name, tc := range map[string]struct {
		start    identity.AIAuthorizationStart
		status   int
		location string
	}{
		"a started request goes to the consent page": {start: identity.AIAuthorizationStart{Outcome: identity.AIAuthorizationStarted, RequestID: "r1", RedirectURL: "https://web.test/connect-ai?request=r1"}, status: http.StatusFound, location: "https://web.test/connect-ai?request=r1"},
		"a refused request goes back to the client":  {start: identity.AIAuthorizationStart{Outcome: identity.AIAuthorizationRedirectError, RedirectURL: "https://claude.ai/cb?error=invalid_request"}, status: http.StatusFound, location: "https://claude.ai/cb?error=invalid_request"},
		"an untrusted client is never redirected to": {start: identity.AIAuthorizationStart{Outcome: identity.AIAuthorizationUntrusted, Error: &identity.AIAuthError{Code: "invalid_client", Description: "unknown"}}, status: http.StatusBadRequest},
	} {
		t.Run(name, func(t *testing.T) {
			h, server := newHandler(t)
			server.EXPECT().StartAuthorization(gomock.Any(), params).Return(tc.start, nil)
			w := httptest.NewRecorder()
			h.handleAuthorize(w, httptest.NewRequest(http.MethodGet, "/oauth/authorize"+query, nil))
			require.Equal(t, tc.status, w.Code)
			require.Equal(t, tc.location, w.Header().Get("Location"))
		})
	}
}

func TestToken(t *testing.T) {
	t.Run("a code is exchanged for tokens, never cached", func(t *testing.T) {
		h, server := newHandler(t)
		server.EXPECT().ExchangeToken(gomock.Any(), identity.AITokenParams{GrantType: "authorization_code", ClientID: "c1", Code: "phc_x", RedirectURI: "https://claude.ai/cb", CodeVerifier: "v"}).
			Return(identity.AITokens{AccessToken: "at", ExpiresIn: 3600, RefreshToken: "phr_y", Scope: "games"}, nil)
		form := url.Values{"grant_type": {"authorization_code"}, "client_id": {"c1"}, "code": {"phc_x"}, "redirect_uri": {"https://claude.ai/cb"}, "code_verifier": {"v"}}
		r := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		h.handleToken(w, r)
		require.Equal(t, http.StatusOK, w.Code)
		body := decode(t, w)
		require.Equal(t, "Bearer", body["token_type"])
		require.Equal(t, "phr_y", body["refresh_token"])
		require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	})
	t.Run("a refused grant is an OAuth error", func(t *testing.T) {
		h, server := newHandler(t)
		server.EXPECT().ExchangeToken(gomock.Any(), gomock.Any()).Return(identity.AITokens{}, &identity.AIAuthError{Code: "invalid_grant", Description: "expired"})
		r := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader("grant_type=refresh_token&refresh_token=phr_z"))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.SetBasicAuth("c1", "")
		w := httptest.NewRecorder()
		h.handleToken(w, r)
		require.Equal(t, http.StatusBadRequest, w.Code)
		require.Equal(t, "invalid_grant", decode(t, w)["error"])
	})
}

func TestRevokeAlwaysAnswersOK(t *testing.T) {
	h, server := newHandler(t)
	server.EXPECT().Revoke(gomock.Any(), "phr_z", "c1").Return(nil)
	r := httptest.NewRequest(http.MethodPost, "/oauth/revoke", strings.NewReader("token=phr_z&client_id=c1"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.handleRevoke(w, r)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestConsent(t *testing.T) {
	t.Run("the pending request is read and approved", func(t *testing.T) {
		h, server := newHandler(t)
		expires := time.Date(2026, 11, 3, 10, 10, 0, 0, time.UTC)
		server.EXPECT().ReadAuthorization(gomock.Any(), "r1").Return(identity.PendingAIAuthorization{ClientName: "Claude", RedirectHost: "claude.ai", Scopes: []string{"games"}, ExpiresAt: expires}, nil)
		r := httptest.NewRequest(http.MethodGet, "/v1/ai-authorizations/r1", nil)
		r.SetPathValue("requestID", "r1")
		w := httptest.NewRecorder()
		h.handleReadAuthorization(w, asUser(r))
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, "claude.ai", decode(t, w)["redirect_host"])

		server.EXPECT().DecideAuthorization(gomock.Any(), user, "r1", true).Return("https://claude.ai/cb?code=phc_x&state=s1", nil)
		r = httptest.NewRequest(http.MethodPost, "/v1/ai-authorizations/r1/approve", nil)
		r.SetPathValue("requestID", "r1")
		w = httptest.NewRecorder()
		h.handleApprove(w, asUser(r))
		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, "https://claude.ai/cb?code=phc_x&state=s1", decode(t, w)["redirect_url"])
	})
	t.Run("an unknown, expired or decided request is 404", func(t *testing.T) {
		h, server := newHandler(t)
		server.EXPECT().DecideAuthorization(gomock.Any(), user, "r2", false).Return("", identity.ErrAIAuthorizationNotFound)
		r := httptest.NewRequest(http.MethodPost, "/v1/ai-authorizations/r2/deny", nil)
		r.SetPathValue("requestID", "r2")
		w := httptest.NewRecorder()
		h.handleDeny(w, asUser(r))
		require.Equal(t, http.StatusNotFound, w.Code)
		require.Equal(t, codeAuthorizationNotFound, decode(t, w)["code"])
	})
}

func TestConnections(t *testing.T) {
	h, server := newHandler(t)
	created := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	server.EXPECT().ListConnections(gomock.Any(), user).Return([]identity.AIConnection{{ConnectionID: "c1", ClientName: "Claude", CreatedAt: created, LastUsedAt: created}}, nil)
	w := httptest.NewRecorder()
	h.handleListConnections(w, asUser(httptest.NewRequest(http.MethodGet, "/v1/ai-connections", nil)))
	require.Equal(t, http.StatusOK, w.Code)
	var listed []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listed))
	require.Equal(t, "Claude", listed[0]["client_name"])

	server.EXPECT().RevokeConnection(gomock.Any(), user, "c1").Return(true, nil)
	server.EXPECT().RevokeConnection(gomock.Any(), user, "c2").Return(false, nil)
	for id, status := range map[string]int{"c1": http.StatusNoContent, "c2": http.StatusNotFound} {
		r := httptest.NewRequest(http.MethodPost, "/v1/ai-connections/"+id+"/revoke", nil)
		r.SetPathValue("connectionID", id)
		w := httptest.NewRecorder()
		h.handleRevokeConnection(w, asUser(r))
		require.Equal(t, status, w.Code, id)
	}
}
