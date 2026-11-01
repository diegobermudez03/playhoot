package loginproviders

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

// GoogleIssuer is the issuer of every identity Google vouches for, as stored.
const GoogleIssuer = "https://accounts.google.com"

const (
	googleAuthorizeURL = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL     = "https://oauth2.googleapis.com/token"
	googleKeysURL      = "https://www.googleapis.com/oauth2/v3/certs"

	// googleTimeout bounds each call to Google.
	googleTimeout = 10 * time.Second
	// googleKeysLifetime is how long Google's signing keys are reused before
	// they are fetched again; an unknown key is fetched at once.
	googleKeysLifetime = time.Hour
	// googleKeysFallback is how old cached keys may still verify while
	// Google's keys cannot be fetched.
	googleKeysFallback = 24 * time.Hour
	// googleClockSkew is how far Google's clock may be from ours.
	googleClockSkew = time.Minute
)

// googleIssuers are the issuers Google's ID tokens carry.
var googleIssuers = map[string]bool{"https://accounts.google.com": true, "accounts.google.com": true}

// GoogleConfig is the OAuth client Playhoot is registered as in Google.
type GoogleConfig struct {
	ClientID     string
	ClientSecret string
	// RedirectURL is the login callback, exactly as registered in Google.
	RedirectURL string

	// Endpoints, Google's unless a test points them elsewhere.
	AuthorizeURL string
	TokenURL     string
	KeysURL      string
}

// Google signs people in with their Google account through OpenID Connect:
// Authorization Code with PKCE, the client secret only on the server, and the
// ID token verified against Google's keys.
type Google struct {
	config GoogleConfig
	client *http.Client
	now    func() time.Time

	keysMu        sync.Mutex
	keys          jose.JSONWebKeySet
	keysFetchedAt time.Time
}

// NewGoogle constructs the Google provider.
func NewGoogle(config GoogleConfig) *Google {
	if config.AuthorizeURL == "" {
		config.AuthorizeURL = googleAuthorizeURL
	}
	if config.TokenURL == "" {
		config.TokenURL = googleTokenURL
	}
	if config.KeysURL == "" {
		config.KeysURL = googleKeysURL
	}
	return &Google{config: config, client: &http.Client{Timeout: googleTimeout}, now: time.Now}
}

// AuthorizationURL implements Provider.
func (g *Google) AuthorizationURL(state string, nonce string, codeChallenge string) string {
	return g.config.AuthorizeURL + "?" + url.Values{
		"client_id":             {g.config.ClientID},
		"redirect_uri":          {g.config.RedirectURL},
		"response_type":         {"code"},
		"scope":                 {"openid email profile"},
		"state":                 {state},
		"nonce":                 {nonce},
		"code_challenge":        {codeChallenge},
		"code_challenge_method": {"S256"},
		// Lets a person with several Google accounts choose, every time.
		"prompt": {"select_account"},
	}.Encode()
}

// Exchange implements Provider: it trades the code for Google's ID token and
// verifies it.
func (g *Google) Exchange(ctx context.Context, code string, nonce string, codeVerifier string) (ExternalIdentity, error) {
	if code == "" {
		return ExternalIdentity{}, ErrInvalidAnswer
	}
	idToken, err := g.redeem(ctx, code, codeVerifier)
	if err != nil {
		return ExternalIdentity{}, err
	}
	return g.verify(ctx, idToken, nonce)
}

type googleTokenResponse struct {
	IDToken string `json:"id_token"`
	Error   string `json:"error"`
}

// redeem asks Google's token endpoint for the ID token of code.
func (g *Google) redeem(ctx context.Context, code string, codeVerifier string) (string, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"client_id":     {g.config.ClientID},
		"client_secret": {g.config.ClientSecret},
		"redirect_uri":  {g.config.RedirectURL},
		"code_verifier": {codeVerifier},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, g.config.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("building Google token request: %s", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := g.client.Do(request)
	if err != nil {
		return "", fmt.Errorf("calling Google's token endpoint: %s", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if err != nil {
		return "", fmt.Errorf("reading Google's token answer: %s", err)
	}
	var answer googleTokenResponse
	decodeErr := json.Unmarshal(body, &answer)
	switch {
	case response.StatusCode >= 500 || response.StatusCode == http.StatusTooManyRequests:
		return "", fmt.Errorf("google's token endpoint answered %d", response.StatusCode)
	case response.StatusCode >= 400 && answer.Error == "invalid_grant":
		// A code Google does not accept (used, expired, another verifier):
		// the answer proves nothing.
		return "", fmt.Errorf("%w: google refused the code (%d %s)", ErrInvalidAnswer, response.StatusCode, answer.Error)
	case response.StatusCode >= 400:
		// Google refuses this server (invalid_client, unauthorized_client,
		// redirect_uri_mismatch...), whatever the code.
		return "", fmt.Errorf("%w: google answered %d %s", ErrMisconfigured, response.StatusCode, answer.Error)
	case decodeErr != nil || answer.IDToken == "":
		return "", fmt.Errorf("%w: Google's token answer has no ID token", ErrInvalidAnswer)
	}
	return answer.IDToken, nil
}

type googleClaims struct {
	jwt.Claims
	Nonce         string `json:"nonce"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
}

// verify checks Google signed idToken for this client and this login, and
// returns who it vouches for.
func (g *Google) verify(ctx context.Context, idToken string, nonce string) (ExternalIdentity, error) {
	parsed, err := jwt.ParseSigned(idToken, []jose.SignatureAlgorithm{jose.RS256})
	if err != nil || len(parsed.Headers) == 0 {
		return ExternalIdentity{}, fmt.Errorf("%w: unreadable ID token", ErrInvalidAnswer)
	}
	key, err := g.signingKey(ctx, parsed.Headers[0].KeyID)
	if err != nil {
		return ExternalIdentity{}, err
	}
	var claims googleClaims
	if err := parsed.Claims(key, &claims); err != nil {
		return ExternalIdentity{}, fmt.Errorf("%w: ID token signature", ErrInvalidAnswer)
	}
	expected := jwt.Expected{AnyAudience: jwt.Audience{g.config.ClientID}, Time: g.now()}
	if err := claims.ValidateWithLeeway(expected, googleClockSkew); err != nil {
		return ExternalIdentity{}, fmt.Errorf("%w: ID token claims: %s", ErrInvalidAnswer, err)
	}
	switch {
	case !googleIssuers[claims.Issuer]:
		return ExternalIdentity{}, fmt.Errorf("%w: ID token issuer %q", ErrInvalidAnswer, claims.Issuer)
	case claims.Expiry == nil:
		return ExternalIdentity{}, fmt.Errorf("%w: ID token without expiry", ErrInvalidAnswer)
	case claims.Subject == "":
		return ExternalIdentity{}, fmt.Errorf("%w: ID token without subject", ErrInvalidAnswer)
	case nonce == "" || claims.Nonce != nonce:
		return ExternalIdentity{}, fmt.Errorf("%w: ID token for another login", ErrInvalidAnswer)
	}
	identity := ExternalIdentity{Issuer: GoogleIssuer, Subject: claims.Subject, Name: claims.Name}
	// Only an address Google verified is worth recording.
	if claims.EmailVerified {
		identity.Email = claims.Email
	}
	return identity, nil
}

// signingKey returns Google's key keyID, fetching Google's keys when they are
// old or do not include it.
func (g *Google) signingKey(ctx context.Context, keyID string) (*jose.JSONWebKey, error) {
	g.keysMu.Lock()
	defer g.keysMu.Unlock()
	cached := g.keys.Key(keyID)
	if len(cached) > 0 && g.now().Sub(g.keysFetchedAt) < googleKeysLifetime {
		return &cached[0], nil
	}
	if err := g.fetchKeys(ctx); err != nil {
		// Recent keys still verify while Google cannot be reached.
		if len(cached) > 0 && g.now().Sub(g.keysFetchedAt) < googleKeysFallback {
			return &cached[0], nil
		}
		return nil, err
	}
	keys := g.keys.Key(keyID)
	if len(keys) == 0 {
		return nil, fmt.Errorf("%w: ID token signed with an unknown key", ErrInvalidAnswer)
	}
	return &keys[0], nil
}

func (g *Google) fetchKeys(ctx context.Context) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, g.config.KeysURL, nil)
	if err != nil {
		return fmt.Errorf("building Google keys request: %s", err)
	}
	response, err := g.client.Do(request)
	if err != nil {
		return fmt.Errorf("fetching Google's keys: %s", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("fetching Google's keys: answered %d", response.StatusCode)
	}
	var keys jose.JSONWebKeySet
	if err := json.NewDecoder(io.LimitReader(response.Body, 256<<10)).Decode(&keys); err != nil {
		return fmt.Errorf("reading Google's keys: %s", err)
	}
	if len(keys.Keys) == 0 {
		return errors.New("google answered no keys")
	}
	g.keys = keys
	g.keysFetchedAt = g.now()
	return nil
}
