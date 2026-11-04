// Package aiauth is the business of AI applications' OAuth access to a
// User's games: how long each credential lives, the rules a registration and
// a redirect URI follow, PKCE, and the access tokens only the MCP resource
// accepts.
package aiauth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

const (
	// AccessLifetime is how long an access token works.
	AccessLifetime = time.Hour
	// RefreshLifetime is how long a refresh token works; using one gives a
	// new one, so a connection in use never expires.
	RefreshLifetime = 30 * 24 * time.Hour
	// RequestLifetime is how long an authorization request waits for its
	// User's decision.
	RequestLifetime = 10 * time.Minute
	// CodeLifetime is how long an approved request's code may be exchanged.
	CodeLifetime = 2 * time.Minute
	// ClockSkew is the tolerance applied to an access token's expiry.
	ClockSkew = time.Minute
)

// Limits of a registration.
const (
	MaxClientNameRunes = 100
	MaxRedirectURIs    = 8
	MaxURILength       = 2000
)

// Grant and response types, and the one authentication method: clients are
// public and prove themselves with PKCE.
const (
	GrantAuthorizationCode = "authorization_code"
	GrantRefreshToken      = "refresh_token"
	ResponseTypeCode       = "code"
	AuthMethodNone         = "none"
	ChallengeMethodS256    = "S256"
)

// Prefixes of the secrets, so a leaked one is recognizable.
const (
	CodePrefix    = "phc_"
	RefreshPrefix = "phr_"
)

const secretBytes = 32

// NewSecret returns a new random secret with prefix.
func NewSecret(prefix string) (string, error) {
	raw := make([]byte, secretBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generating a secret: %w", err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(raw), nil
}

// Hash is what Identity keeps of a secret: never the secret itself.
func Hash(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// WellFormed reports whether secret could be one with prefix, before any
// lookup.
func WellFormed(secret string, prefix string) bool {
	encoded, found := strings.CutPrefix(secret, prefix)
	if !found {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	return err == nil && len(raw) == secretBytes
}

// verifierPattern is RFC 7636 §4.1's code verifier.
var verifierPattern = regexp.MustCompile(`^[A-Za-z0-9._~-]{43,128}$`)

// challengePattern is an S256 challenge: 32 bytes in base64url.
var challengePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)

// ValidChallenge reports whether challenge is an S256 code challenge.
func ValidChallenge(challenge string) bool {
	return challengePattern.MatchString(challenge)
}

// VerifyPKCE reports whether verifier is the one challenge was made from
// (RFC 7636, S256 only).
func VerifyPKCE(verifier string, challenge string) bool {
	if !verifierPattern.MatchString(verifier) {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	return hmac.Equal([]byte(base64.RawURLEncoding.EncodeToString(sum[:])), []byte(challenge))
}

// ValidRedirectURI reports whether uri may receive authorization codes: an
// absolute https URI, or http only on a loopback host (a native
// application's local listener), never with a fragment.
func ValidRedirectURI(uri string) bool {
	if uri == "" || len(uri) > MaxURILength {
		return false
	}
	parsed, err := url.Parse(uri)
	if err != nil || parsed.Fragment != "" || parsed.Host == "" || parsed.User != nil {
		return false
	}
	switch parsed.Scheme {
	case "https":
		return true
	case "http":
		host := parsed.Hostname()
		if host == "localhost" {
			return true
		}
		ip := net.ParseIP(host)
		return ip != nil && ip.IsLoopback()
	default:
		return false
	}
}

// ErrInvalidMetadata is a registration that breaks a rule; its message says
// which.
var ErrInvalidMetadata = errors.New("invalid client metadata")

// Registration is a client's checked registration, with every default
// filled.
type Registration struct {
	ClientName   string
	ClientURI    string
	RedirectURIs []string
	GrantTypes   []string
}

// CheckRegistration checks what a client registers itself with.
func CheckRegistration(name string, clientURI string, redirectURIs []string, grantTypes []string, responseTypes []string, authMethod string, scope string) (Registration, error) {
	invalid := func(format string, args ...any) (Registration, error) {
		return Registration{}, fmt.Errorf("%w: %s", ErrInvalidMetadata, fmt.Sprintf(format, args...))
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "AI application"
	}
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) > MaxClientNameRunes {
		return invalid("client_name must hold at most %d characters", MaxClientNameRunes)
	}
	if clientURI != "" {
		parsed, err := url.Parse(clientURI)
		if err != nil || len(clientURI) > MaxURILength || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
			return invalid("client_uri must be an http(s) URL")
		}
	}
	if len(redirectURIs) == 0 || len(redirectURIs) > MaxRedirectURIs {
		return invalid("redirect_uris must list 1 to %d URIs", MaxRedirectURIs)
	}
	for _, uri := range redirectURIs {
		if !ValidRedirectURI(uri) {
			return invalid("redirect URI %q must be https, or http on a loopback host, without a fragment", uri)
		}
	}
	if len(grantTypes) == 0 {
		grantTypes = []string{GrantAuthorizationCode, GrantRefreshToken}
	}
	for _, grant := range grantTypes {
		if grant != GrantAuthorizationCode && grant != GrantRefreshToken {
			return invalid("grant type %q is not supported", grant)
		}
	}
	if !slices.Contains(grantTypes, GrantAuthorizationCode) {
		return invalid("grant_types must include authorization_code")
	}
	for _, responseType := range responseTypes {
		if responseType != ResponseTypeCode {
			return invalid("response type %q is not supported", responseType)
		}
	}
	if authMethod != "" && authMethod != AuthMethodNone {
		return invalid("token_endpoint_auth_method must be none: clients are public and use PKCE")
	}
	if scope != "" {
		if _, ok := CheckScope(scope); !ok {
			return invalid("scope %q is not supported", scope)
		}
	}
	return Registration{ClientName: name, ClientURI: clientURI, RedirectURIs: slices.Clone(redirectURIs), GrantTypes: slices.Clone(grantTypes)}, nil
}

// CheckScope returns the scope granted for requested: the one scope, also
// when nothing is asked, or false when it asks for anything else.
func CheckScope(requested string) (string, bool) {
	for _, scope := range strings.Fields(requested) {
		if scope != ScopeGames {
			return "", false
		}
	}
	return ScopeGames, true
}

// ScopeGames mirrors identity.AIScopeGames, the one scope granted.
const ScopeGames = "games"

// accessClaims are an AI access token's claims (RFC 9068 shape).
type accessClaims struct {
	jwt.Claims
	Scope string `json:"scope"`
	// ConnectionID is the AI connection the token was granted to.
	ConnectionID string `json:"sid"`
	ClientID     string `json:"client_id"`
}

// accessKeyLabel derives the AI tokens' key from Identity's access token
// key, so neither kind of token is ever accepted as the other.
const accessKeyLabel = "playhoot ai access token v1"

// Signer issues and verifies AI access tokens for one resource.
type Signer struct {
	key      []byte
	signer   jose.Signer
	issuer   string
	resource string
}

// NewSigner constructs a Signer from Identity's access token key, for
// tokens issued by issuer and accepted only by resource.
func NewSigner(key []byte, issuer string, resource string) (*Signer, error) {
	if len(key) < 32 {
		return nil, fmt.Errorf("the access token key must have at least 32 bytes")
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(accessKeyLabel))
	derived := mac.Sum(nil)
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.HS256, Key: derived}, (&jose.SignerOptions{}).WithType("at+jwt"))
	if err != nil {
		return nil, fmt.Errorf("creating the AI access token signer: %s", err)
	}
	return &Signer{key: derived, signer: signer, issuer: issuer, resource: resource}, nil
}

// Resource is the resource the tokens are for.
func (s *Signer) Resource() string {
	return s.resource
}

// Issue returns an access token for userUUID's connection.
func (s *Signer) Issue(userUUID string, connectionID string, clientID string, scope string, issuedAt time.Time) (string, time.Time, error) {
	expiresAt := issuedAt.Add(AccessLifetime).Truncate(time.Second)
	token, err := jwt.Signed(s.signer).Claims(accessClaims{
		Claims: jwt.Claims{
			Issuer: s.issuer, Subject: userUUID, Audience: jwt.Audience{s.resource},
			IssuedAt: jwt.NewNumericDate(issuedAt), Expiry: jwt.NewNumericDate(expiresAt),
		},
		Scope: scope, ConnectionID: connectionID, ClientID: clientID,
	}).Serialize()
	if err != nil {
		return "", time.Time{}, fmt.Errorf("signing an AI access token: %s", err)
	}
	return token, expiresAt, nil
}

// ErrInvalidToken is an access token this resource does not accept.
var ErrInvalidToken = errors.New("invalid AI access token")

// Verified is what a valid access token says.
type Verified struct {
	UserUUID     string
	ConnectionID string
	ClientID     string
	Scopes       []string
	ExpiresAt    time.Time
}

// Verify checks token's signature, issuer, audience and expiry; whether its
// connection is still approved is checked against storage by the caller.
func (s *Signer) Verify(token string, now time.Time) (Verified, error) {
	parsed, err := jwt.ParseSigned(token, []jose.SignatureAlgorithm{jose.HS256})
	if err != nil {
		return Verified{}, ErrInvalidToken
	}
	var c accessClaims
	if err := parsed.Claims(s.key, &c); err != nil {
		return Verified{}, ErrInvalidToken
	}
	if c.Subject == "" || c.ConnectionID == "" || c.Expiry == nil || !now.Before(c.Expiry.Time().Add(ClockSkew)) ||
		c.ValidateWithLeeway(jwt.Expected{Issuer: s.issuer, AnyAudience: jwt.Audience{s.resource}, Time: now}, ClockSkew) != nil {
		return Verified{}, ErrInvalidToken
	}
	return Verified{UserUUID: c.Subject, ConnectionID: c.ConnectionID, ClientID: c.ClientID, Scopes: strings.Fields(c.Scope), ExpiresAt: c.Expiry.Time().UTC()}, nil
}

// RedirectHost is how a redirect URI is shown to the User who approves it.
func RedirectHost(uri string) string {
	parsed, err := url.Parse(uri)
	if err != nil {
		return ""
	}
	return parsed.Host
}

// WithQuery returns uri with params added to its query.
func WithQuery(uri string, params map[string]string) string {
	parsed, err := url.Parse(uri)
	if err != nil {
		return uri
	}
	query := parsed.Query()
	for key, value := range params {
		if value != "" {
			query.Set(key, value)
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
