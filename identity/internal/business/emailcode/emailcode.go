// Package emailcode holds the rules of the one-time code sent to an email
// address: how an address is normalized and recognized, how long a code
// lasts, how many wrong ones exhaust it, how often one address may ask for
// one, and how a code is kept without ever storing it.
package emailcode

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"time"
	"unicode"
)

const (
	// Lifetime is how long a code is valid once sent.
	Lifetime = 10 * time.Minute

	// MaxWrongCodes is how many wrong codes an attempt survives; the next
	// one exhausts it and a new code must be asked for.
	MaxWrongCodes = 5

	// BurstWindow and BurstSends bound how often one address may be sent a
	// code in a short while.
	BurstWindow = 15 * time.Minute
	BurstSends  = 3

	// DayWindow and DaySends bound how many codes one address may be sent in
	// a day.
	DayWindow = 24 * time.Hour
	DaySends  = 10

	// MaxAddressLen is the longest address accepted, well beyond what the
	// mail standards allow in practice.
	MaxAddressLen = 254

	// digits is how many digits a code has.
	digits = 6
)

// Normalize returns the stored form of an address (trimmed, lower case) and
// whether it is an address at all. It is deliberately permissive about what
// an address may contain: only control characters, spaces and a missing or
// misplaced single "@" are refused, because proving the address is what the
// code is for.
func Normalize(address string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(address))
	if len(normalized) == 0 || len(normalized) > MaxAddressLen {
		return "", false
	}
	for _, r := range normalized {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return "", false
		}
	}
	local, domain, found := strings.Cut(normalized, "@")
	if !found || local == "" || domain == "" || strings.Contains(domain, "@") {
		return "", false
	}
	if !strings.Contains(domain, ".") || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return "", false
	}
	return normalized, true
}

// NewCode returns a new code: digits only, so it is read from an email and
// typed on a phone without mistakes.
func NewCode() (string, error) {
	code := make([]byte, 0, digits)
	for range digits {
		digit, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", fmt.Errorf("generating access code: %s", err)
		}
		code = append(code, byte('0'+digit.Int64()))
	}
	return string(code), nil
}

// Hash is what is stored of a code; the code itself never is.
func Hash(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

// Matches reports whether code is the one hash was made from, in constant
// time.
func Matches(hash string, code string) bool {
	if code == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(Hash(code)), []byte(hash)) == 1
}
