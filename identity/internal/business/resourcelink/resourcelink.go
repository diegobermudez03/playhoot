// Package resourcelink is the business of secret links to one resource: how
// their tokens look, how they are kept, and how long they live.
package resourcelink

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Lifetime is how long a link works from its creation; renewing a link is
// creating a new one.
const Lifetime = 7 * 24 * time.Hour

// TokenPrefix marks a link token, so a leaked one is recognizable.
const TokenPrefix = "phl_"

// tokenBytes is the randomness of a token.
const tokenBytes = 32

var (
	kindPattern       = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
	maxResourceIDSize = 128
)

// NewToken returns a new random token.
func NewToken() (string, error) {
	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generating link token: %w", err)
	}
	return TokenPrefix + base64.RawURLEncoding.EncodeToString(raw), nil
}

// Hash is what Identity keeps of token: never the token itself.
func Hash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// WellFormed reports whether token could be a link token, before any lookup.
func WellFormed(token string) bool {
	encoded, found := strings.CutPrefix(token, TokenPrefix)
	if !found {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	return err == nil && len(raw) == tokenBytes
}

// ValidResource reports whether kind and resourceID name a resource a link
// may point to. Identity does not know what the resource is.
func ValidResource(kind string, resourceID string) bool {
	return kindPattern.MatchString(kind) && resourceID != "" && len(resourceID) <= maxResourceIDSize
}
