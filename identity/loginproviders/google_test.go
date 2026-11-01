package loginproviders

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/stretchr/testify/require"
)

// fakeGoogle is Google's token and keys endpoints for one client.
type fakeGoogle struct {
	t          *testing.T
	server     *httptest.Server
	key        *rsa.PrivateKey
	keyID      string
	idToken    func() string
	tokenCode  int
	tokenError string
	keysDown   atomic.Bool
	keyCalls   atomic.Int32
	lastForm   url.Values
}

func newFakeGoogle(t *testing.T) *fakeGoogle {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	fake := &fakeGoogle{t: t, key: key, keyID: "key-1", tokenCode: http.StatusOK, tokenError: "invalid_grant"}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		fake.lastForm = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(fake.tokenCode)
		if fake.tokenCode != http.StatusOK {
			_ = json.NewEncoder(w).Encode(map[string]string{"error": fake.tokenError})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"id_token": fake.idToken(), "access_token": "unused"})
	})
	mux.HandleFunc("GET /certs", func(w http.ResponseWriter, r *http.Request) {
		fake.keyCalls.Add(1)
		if fake.keysDown.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		keys := jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &fake.key.PublicKey, KeyID: fake.keyID, Algorithm: "RS256", Use: "sig"}}}
		_ = json.NewEncoder(w).Encode(keys)
	})
	fake.server = httptest.NewServer(mux)
	t.Cleanup(fake.server.Close)
	return fake
}

func (f *fakeGoogle) provider() *Google {
	return NewGoogle(GoogleConfig{
		ClientID:     "client-1",
		ClientSecret: "secret-1",
		RedirectURL:  "https://play.example/v1/identity/login/google/callback",
		AuthorizeURL: f.server.URL + "/authorize",
		TokenURL:     f.server.URL + "/token",
		KeysURL:      f.server.URL + "/certs",
	})
}

type googleTestClaims struct {
	jwt.Claims
	Nonce         string `json:"nonce,omitempty"`
	Email         string `json:"email,omitempty"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name,omitempty"`
}

func validClaims() googleTestClaims {
	now := time.Now()
	return googleTestClaims{
		Claims: jwt.Claims{
			Issuer:   "https://accounts.google.com",
			Subject:  "google-sub-1",
			Audience: jwt.Audience{"client-1"},
			IssuedAt: jwt.NewNumericDate(now),
			Expiry:   jwt.NewNumericDate(now.Add(time.Hour)),
		},
		Nonce:         "nonce-1",
		Email:         "ana@gmail.com",
		EmailVerified: true,
		Name:          "Ana Pérez",
	}
}

func (f *fakeGoogle) sign(claims googleTestClaims, key *rsa.PrivateKey, keyID string) string {
	f.t.Helper()
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", keyID))
	require.NoError(f.t, err)
	token, err := jwt.Signed(signer).Claims(claims).Serialize()
	require.NoError(f.t, err)
	return token
}

func TestGoogleAuthorizationURL(t *testing.T) {
	google := NewGoogle(GoogleConfig{ClientID: "client-1", RedirectURL: "https://play.example/cb"})

	authorization, err := url.Parse(google.AuthorizationURL("state-1", "nonce-1", "challenge-1"))
	require.NoError(t, err)

	require.Equal(t, "accounts.google.com", authorization.Host)
	query := authorization.Query()
	require.Equal(t, "client-1", query.Get("client_id"))
	require.Equal(t, "https://play.example/cb", query.Get("redirect_uri"))
	require.Equal(t, "code", query.Get("response_type"))
	require.Equal(t, "openid email profile", query.Get("scope"))
	require.Equal(t, "state-1", query.Get("state"))
	require.Equal(t, "nonce-1", query.Get("nonce"))
	require.Equal(t, "challenge-1", query.Get("code_challenge"))
	require.Equal(t, "S256", query.Get("code_challenge_method"))
}

func TestGoogleExchange(t *testing.T) {
	type test struct {
		claims        func() googleTestClaims
		otherKey      bool
		tokenCode     int
		tokenError    string
		misconfigured bool
		expected      ExternalIdentity
		invalid       bool
		unreachable   bool
	}

	tests := map[string]test{
		"verified_token_gives_the_identity": {
			claims:   validClaims,
			expected: ExternalIdentity{Issuer: GoogleIssuer, Subject: "google-sub-1", Email: "ana@gmail.com", Name: "Ana Pérez"},
		},
		"an_unverified_email_is_not_recorded": {
			claims: func() googleTestClaims {
				c := validClaims()
				c.EmailVerified = false
				return c
			},
			expected: ExternalIdentity{Issuer: GoogleIssuer, Subject: "google-sub-1", Name: "Ana Pérez"},
		},
		"token_for_another_login": {
			claims: func() googleTestClaims {
				c := validClaims()
				c.Nonce = "nonce-2"
				return c
			},
			invalid: true,
		},
		"token_for_another_client": {
			claims: func() googleTestClaims {
				c := validClaims()
				c.Audience = jwt.Audience{"client-2"}
				return c
			},
			invalid: true,
		},
		"token_from_another_issuer": {
			claims: func() googleTestClaims {
				c := validClaims()
				c.Issuer = "https://evil.example"
				return c
			},
			invalid: true,
		},
		"expired_token": {
			claims: func() googleTestClaims {
				c := validClaims()
				c.Expiry = jwt.NewNumericDate(time.Now().Add(-time.Hour))
				return c
			},
			invalid: true,
		},
		"token_without_subject": {
			claims: func() googleTestClaims {
				c := validClaims()
				c.Subject = ""
				return c
			},
			invalid: true,
		},
		"token_signed_with_another_key": {claims: validClaims, otherKey: true, invalid: true},
		"code_google_refuses":           {claims: validClaims, tokenCode: http.StatusBadRequest, invalid: true},
		"google_failing":                {claims: validClaims, tokenCode: http.StatusInternalServerError, unreachable: true},
		"google_limiting_us":            {claims: validClaims, tokenCode: http.StatusTooManyRequests, unreachable: true},
		"google_refusing_this_server":   {claims: validClaims, tokenCode: http.StatusUnauthorized, tokenError: "invalid_client", misconfigured: true},
		"redirect_not_registered":       {claims: validClaims, tokenCode: http.StatusBadRequest, tokenError: "redirect_uri_mismatch", misconfigured: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			fake := newFakeGoogle(t)
			if tc.tokenCode != 0 {
				fake.tokenCode = tc.tokenCode
			}
			if tc.tokenError != "" {
				fake.tokenError = tc.tokenError
			}
			signingKey := fake.key
			if tc.otherKey {
				other, err := rsa.GenerateKey(rand.Reader, 2048)
				require.NoError(t, err)
				signingKey = other
			}
			fake.idToken = func() string { return fake.sign(tc.claims(), signingKey, fake.keyID) }

			identity, err := fake.provider().Exchange(context.Background(), "code-1", "nonce-1", "verifier-1")

			switch {
			case tc.misconfigured:
				require.ErrorIs(t, err, ErrMisconfigured)
			case tc.invalid:
				require.ErrorIs(t, err, ErrInvalidAnswer)
			case tc.unreachable:
				require.Error(t, err)
				require.False(t, errors.Is(err, ErrInvalidAnswer), "a failing Google is not an invalid answer")
				require.False(t, errors.Is(err, ErrMisconfigured))
			default:
				require.NoError(t, err)
				require.Equal(t, tc.expected, identity)
				require.Equal(t, url.Values{
					"grant_type":    {"authorization_code"},
					"code":          {"code-1"},
					"client_id":     {"client-1"},
					"client_secret": {"secret-1"},
					"redirect_uri":  {"https://play.example/v1/identity/login/google/callback"},
					"code_verifier": {"verifier-1"},
				}, fake.lastForm)
			}
		})
	}
}

func TestGoogleReusesItsKeysUntilAnUnknownOneAppears(t *testing.T) {
	fake := newFakeGoogle(t)
	google := fake.provider()
	fake.idToken = func() string { return fake.sign(validClaims(), fake.key, fake.keyID) }

	_, err := google.Exchange(context.Background(), "code-1", "nonce-1", "v")
	require.NoError(t, err)
	_, err = google.Exchange(context.Background(), "code-2", "nonce-1", "v")
	require.NoError(t, err)
	require.EqualValues(t, 1, fake.keyCalls.Load(), "keys are reused")

	rotated, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	fake.key, fake.keyID = rotated, "key-2"
	_, err = google.Exchange(context.Background(), "code-3", "nonce-1", "v")
	require.NoError(t, err, "a key Google rotated in is fetched")
	require.EqualValues(t, 2, fake.keyCalls.Load())
}

func TestGoogleAcceptsOnlyRS256(t *testing.T) {
	fake := newFakeGoogle(t)
	hmac, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.HS256, Key: []byte("a-shared-secret-of-32-bytes-long")},
		(&jose.SignerOptions{}).WithHeader("kid", fake.keyID))
	require.NoError(t, err)
	hs256, err := jwt.Signed(hmac).Claims(validClaims()).Serialize()
	require.NoError(t, err)
	parts := strings.Split(fake.sign(validClaims(), fake.key, fake.keyID), ".")
	unsigned := "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0." + parts[1] + "."

	for name, token := range map[string]string{"hs256": hs256, "none": unsigned} {
		t.Run(name, func(t *testing.T) {
			fake.idToken = func() string { return token }
			_, err := fake.provider().Exchange(context.Background(), "code-1", "nonce-1", "v")
			require.ErrorIs(t, err, ErrInvalidAnswer)
		})
	}
}

func TestGoogleKeepsUsingOldKeysWhileItsKeysAreDown(t *testing.T) {
	fake := newFakeGoogle(t)
	google := fake.provider()
	fake.idToken = func() string { return fake.sign(validClaims(), fake.key, fake.keyID) }
	_, err := google.Exchange(context.Background(), "code-1", "nonce-1", "v")
	require.NoError(t, err)

	google.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	fake.keysDown.Store(true)
	later := validClaims()
	later.IssuedAt = jwt.NewNumericDate(time.Now().Add(2 * time.Hour))
	later.Expiry = jwt.NewNumericDate(time.Now().Add(3 * time.Hour))
	fake.idToken = func() string { return fake.sign(later, fake.key, fake.keyID) }

	_, err = google.Exchange(context.Background(), "code-2", "nonce-1", "v")
	require.NoError(t, err, "a known key still verifies")

	google.now = func() time.Time { return time.Now().Add(25 * time.Hour) }
	muchLater := validClaims()
	muchLater.IssuedAt = jwt.NewNumericDate(time.Now().Add(25 * time.Hour))
	muchLater.Expiry = jwt.NewNumericDate(time.Now().Add(26 * time.Hour))
	fake.idToken = func() string { return fake.sign(muchLater, fake.key, fake.keyID) }
	_, err = google.Exchange(context.Background(), "code-3", "nonce-1", "v")
	require.Error(t, err, "keys a day old are not trusted without Google")
	require.False(t, errors.Is(err, ErrInvalidAnswer))
}

func TestGoogleUnreachable(t *testing.T) {
	fake := newFakeGoogle(t)
	google := fake.provider()
	fake.server.Close()

	_, err := google.Exchange(context.Background(), "code-1", "nonce-1", "v")

	require.Error(t, err)
	require.False(t, errors.Is(err, ErrInvalidAnswer))
}

func TestGoogleRefusesAnEmptyCode(t *testing.T) {
	_, err := NewGoogle(GoogleConfig{}).Exchange(context.Background(), "", "nonce-1", "v")
	require.ErrorIs(t, err, ErrInvalidAnswer)
}

func TestRegistryListsEnabledProviders(t *testing.T) {
	require.Empty(t, New(Config{}).Enabled())
	both := New(Config{Google: &GoogleConfig{ClientID: "c"}, Local: &LocalConfig{}})
	require.Equal(t, []string{"google", "local"}, providerNames(both.Enabled()))
}

func providerNames[T ~string](providers []T) []string {
	names := make([]string, 0, len(providers))
	for _, p := range providers {
		names = append(names, string(p))
	}
	return names
}
