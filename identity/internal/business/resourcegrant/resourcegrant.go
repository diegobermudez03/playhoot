// Package resourcegrant is the business of single-use grants: a short-lived
// secret that lets whoever holds it do one thing, once, for a User on one
// resource (for example, upload one asset of one game). Identity keeps only
// the token's hash and never knows what the resource or the thing is.
package resourcegrant

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// Lifetime is how long a grant may be used from its creation.
const Lifetime = 15 * time.Minute

// MaxLive is how many unused, unexpired grants a User may hold at once.
const MaxLive = 20

// TokenPrefix marks a grant token, so a leaked one is recognizable.
const TokenPrefix = "phg_"

// MaxScopeSize bounds the scope, the caller's description of the one thing a
// grant allows.
const MaxScopeSize = 256

// tokenBytes is the randomness of a token.
const tokenBytes = 32

// NewToken returns a new random token.
func NewToken() (string, error) {
	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generating grant token: %w", err)
	}
	return TokenPrefix + base64.RawURLEncoding.EncodeToString(raw), nil
}

// Hash is what Identity keeps of token: never the token itself.
func Hash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// WellFormed reports whether token could be a grant token, before any lookup.
func WellFormed(token string) bool {
	encoded, found := strings.CutPrefix(token, TokenPrefix)
	if !found {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	return err == nil && len(raw) == tokenBytes
}

// ValidScope reports whether scope may describe what a grant allows.
func ValidScope(scope string) bool {
	return scope != "" && len(scope) <= MaxScopeSize
}
