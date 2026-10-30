// Package login holds the rules of an account's sign-in: how long an attempt
// and its one-time code last, the random values that identify them, and how
// the client's verifier proves it started the attempt.
package login

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"time"
)

const (
	// AttemptLifetime is how long a started login waits for the provider's
	// answer.
	AttemptLifetime = 5 * time.Minute

	// CodeLifetime is how long the one-time code waits for the client to
	// complete the login.
	CodeLifetime = 60 * time.Second
)

// NewState returns the value that ties the provider's answer to an attempt.
func NewState() (string, error) {
	return random("")
}

// NewNonce returns the value the provider's identity token must carry.
func NewNonce() (string, error) {
	return random("")
}

// NewCode returns a one-time login code.
func NewCode() (string, error) {
	return random("phl_")
}

// ValidChallenge reports whether challenge has the shape of an S256 code
// challenge: 32 bytes, base64url without padding.
func ValidChallenge(challenge string) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(challenge)
	return err == nil && len(decoded) == sha256.Size
}

// VerifierMatches reports whether verifier is the one challenge was made from
// (S256).
func VerifierMatches(challenge string, verifier string) bool {
	if verifier == "" {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	expected := base64.RawURLEncoding.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(expected), []byte(challenge)) == 1
}

func random(prefix string) (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generating login value: %s", err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(bytes), nil
}
