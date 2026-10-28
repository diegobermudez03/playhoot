// Package accesstoken issues and verifies access tokens: JWTs signed with the
// server's key carrying who the caller is and until when. They are never
// stored, so renewing writes nothing and authorizing reads nothing.
package accesstoken

import (
	"errors"
	"fmt"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"

	"github.com/diegobermudez03/playhoot/identity"
)

// Lifetime is how long an access token is valid.
const Lifetime = 15 * time.Minute

// ClockSkew is how far ahead of the verifying instance an issuing instance's
// clock may be.
const ClockSkew = time.Minute

// MinKeyBytes is the shortest signing key accepted: HS256 needs 256 bits.
const MinKeyBytes = 32

// claims is the token's payload: the user, their level and display name, and
// the standard expiry and issue time.
type claims struct {
	jwt.Claims
	Level       string `json:"lvl"`
	DisplayName string `json:"name"`
}

// Signer issues and verifies access tokens with one key.
type Signer struct {
	key    []byte
	signer jose.Signer
}

// NewSigner returns a Signer for key, which must have at least MinKeyBytes.
func NewSigner(key []byte) (*Signer, error) {
	if len(key) < MinKeyBytes {
		return nil, fmt.Errorf("access token key must have at least %d bytes", MinKeyBytes)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.HS256, Key: key}, (&jose.SignerOptions{}).WithType("JWT"))
	if err != nil {
		return nil, fmt.Errorf("creating access token signer: %s", err)
	}
	return &Signer{key: key, signer: signer}, nil
}

// Issue returns an access token for user, valid from issuedAt for Lifetime,
// and its expiry.
func (s *Signer) Issue(user identity.User, issuedAt time.Time) (string, time.Time, error) {
	expiresAt := issuedAt.Add(Lifetime).Truncate(time.Second)
	token, err := jwt.Signed(s.signer).Claims(claims{
		Claims: jwt.Claims{
			Subject:  user.UUID,
			IssuedAt: jwt.NewNumericDate(issuedAt),
			Expiry:   jwt.NewNumericDate(expiresAt),
		},
		Level:       string(user.Level),
		DisplayName: user.DisplayName,
	}).Serialize()
	if err != nil {
		return "", time.Time{}, fmt.Errorf("signing access token: %s", err)
	}
	return token, expiresAt, nil
}

// ErrInvalid means the token is malformed, not signed with this key, or
// expired.
var ErrInvalid = errors.New("invalid access token")

// Verify returns the user an access token stands for at now, or ErrInvalid.
func (s *Signer) Verify(token string, now time.Time) (identity.User, error) {
	parsed, err := jwt.ParseSigned(token, []jose.SignatureAlgorithm{jose.HS256})
	if err != nil {
		return identity.User{}, ErrInvalid
	}
	var c claims
	if err := parsed.Claims(s.key, &c); err != nil {
		return identity.User{}, ErrInvalid
	}
	// The library accepts the expiry instant itself; at that instant the token is over.
	// Expiry is strict; the issue time only tolerates another instance's clock
	// being slightly ahead.
	if c.Subject == "" || c.Expiry == nil || !now.Before(c.Expiry.Time()) ||
		c.ValidateWithLeeway(jwt.Expected{Time: now}, ClockSkew) != nil {
		return identity.User{}, ErrInvalid
	}
	return identity.User{UUID: c.Subject, DisplayName: c.DisplayName, Level: identity.Level(c.Level)}, nil
}
