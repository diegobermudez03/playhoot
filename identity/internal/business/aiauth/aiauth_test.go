package aiauth

import (
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestVerifyPKCE(t *testing.T) {
	verifier := strings.Repeat("a", 43)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	require.True(t, ValidChallenge(challenge))
	require.True(t, VerifyPKCE(verifier, challenge))
	require.False(t, VerifyPKCE(strings.Repeat("b", 43), challenge), "another verifier")
	require.False(t, VerifyPKCE("short", challenge), "a verifier under 43 characters")
	require.False(t, ValidChallenge("plain-challenge"))
}

func TestValidRedirectURI(t *testing.T) {
	for uri, valid := range map[string]bool{
		"https://claude.ai/api/mcp/auth_callback": true,
		"http://127.0.0.1:33418/callback":         true,
		"http://localhost:8080/cb":                true,
		"http://[::1]:9000/cb":                    true,
		"http://example.com/cb":                   false,
		"https://example.com/cb#fragment":         false,
		"myapp://callback":                        false,
		"/relative":                               false,
		"":                                        false,
	} {
		require.Equal(t, valid, ValidRedirectURI(uri), uri)
	}
}

func TestCheckRegistration(t *testing.T) {
	registered, err := CheckRegistration("", "", []string{"https://claude.ai/cb"}, nil, nil, "", "")
	require.NoError(t, err)
	require.Equal(t, "AI application", registered.ClientName)
	require.Equal(t, []string{GrantAuthorizationCode, GrantRefreshToken}, registered.GrantTypes)

	for name, call := range map[string]func() error{
		"no redirect": func() error { _, err := CheckRegistration("x", "", nil, nil, nil, "", ""); return err },
		"a secret client": func() error {
			_, err := CheckRegistration("x", "", []string{"https://a/cb"}, nil, nil, "client_secret_basic", "")
			return err
		},
		"implicit grant": func() error {
			_, err := CheckRegistration("x", "", []string{"https://a/cb"}, []string{"implicit"}, nil, "", "")
			return err
		},
		"token response": func() error {
			_, err := CheckRegistration("x", "", []string{"https://a/cb"}, nil, []string{"token"}, "", "")
			return err
		},
		"unknown scope": func() error {
			_, err := CheckRegistration("x", "", []string{"https://a/cb"}, nil, nil, "", "admin")
			return err
		},
		"insecure http": func() error {
			_, err := CheckRegistration("x", "", []string{"http://evil.example/cb"}, nil, nil, "", "")
			return err
		},
		"name too long": func() error {
			_, err := CheckRegistration(strings.Repeat("n", 101), "", []string{"https://a/cb"}, nil, nil, "", "")
			return err
		},
	} {
		require.ErrorIs(t, call(), ErrInvalidMetadata, name)
	}
}

func TestCheckScope(t *testing.T) {
	scope, ok := CheckScope("")
	require.True(t, ok)
	require.Equal(t, ScopeGames, scope)
	_, ok = CheckScope("games publish")
	require.False(t, ok)
}

func TestSecrets(t *testing.T) {
	secret, err := NewSecret(CodePrefix)
	require.NoError(t, err)
	require.True(t, WellFormed(secret, CodePrefix))
	require.False(t, WellFormed(secret, RefreshPrefix))
	require.Len(t, Hash(secret), 64)
}

func TestSigner(t *testing.T) {
	key := []byte(strings.Repeat("k", 32))
	signer, err := NewSigner(key, "https://api.example", "https://api.example/mcp")
	require.NoError(t, err)
	now := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	token, expiresAt, err := signer.Issue("user-1", "conn-1", "client-1", ScopeGames, now)
	require.NoError(t, err)
	require.Equal(t, now.Add(AccessLifetime), expiresAt)

	verified, err := signer.Verify(token, now.Add(time.Minute))
	require.NoError(t, err)
	require.Equal(t, Verified{UserUUID: "user-1", ConnectionID: "conn-1", ClientID: "client-1", Scopes: []string{ScopeGames}, ExpiresAt: expiresAt}, verified)

	_, err = signer.Verify(token, expiresAt.Add(2*ClockSkew))
	require.ErrorIs(t, err, ErrInvalidToken, "expired")

	other, err := NewSigner(key, "https://api.example", "https://other.example/mcp")
	require.NoError(t, err)
	_, err = other.Verify(token, now)
	require.ErrorIs(t, err, ErrInvalidToken, "a token for another resource")

	otherKey, err := NewSigner([]byte(strings.Repeat("z", 32)), "https://api.example", "https://api.example/mcp")
	require.NoError(t, err)
	_, err = otherKey.Verify(token, now)
	require.ErrorIs(t, err, ErrInvalidToken, "signed with another key")
}

func TestWithQuery(t *testing.T) {
	require.Equal(t, "https://a.example/cb?code=c1&state=s1", WithQuery("https://a.example/cb", map[string]string{"code": "c1", "state": "s1"}))
	require.Equal(t, "https://a.example/cb?error=access_denied&x=1", WithQuery("https://a.example/cb?x=1", map[string]string{"error": "access_denied", "state": ""}))
}
