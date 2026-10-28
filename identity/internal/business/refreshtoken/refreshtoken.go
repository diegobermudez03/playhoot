// Package refreshtoken holds the rules of refresh tokens: how they are made,
// when they expire, when they rotate, and how long the one just replaced
// keeps working.
package refreshtoken

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/diegobermudez03/playhoot/identity"
)

const (
	// RotationAge is how old the active refresh token must be before a
	// renewal replaces it, so a session writes at most one row this often.
	RotationAge = 5 * time.Hour

	// PreviousGrace is how long the refresh token just replaced still renews,
	// so clients racing a rotation (another tab, a lost answer) keep working.
	PreviousGrace = 5 * time.Minute

	// AccountInactivity is how long an account's refresh token lives after
	// it was issued; it moves forward only when the token rotates.
	AccountInactivity = 30 * 24 * time.Hour

	// AccountMaxLifetime is the most an account's session may last from the
	// sign-in that opened it, whatever its activity.
	AccountMaxLifetime = 90 * 24 * time.Hour
)

// prefix makes refresh tokens recognizable to people and secret scanners.
const prefix = "phr_"

// New returns a new random refresh token.
func New() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generating refresh token: %s", err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(bytes), nil
}

// ExpiresAt is when a refresh token issued at issuedAt for a session opened at
// startedAt expires, or nil when it never does (a guest's).
func ExpiresAt(level identity.Level, issuedAt time.Time, startedAt time.Time) *time.Time {
	if level != identity.LevelAccount {
		return nil
	}
	expiresAt := issuedAt.Add(AccountInactivity)
	if maxAt := startedAt.Add(AccountMaxLifetime); maxAt.Before(expiresAt) {
		expiresAt = maxAt
	}
	return &expiresAt
}

// Expired reports whether a refresh token expiring at expiresAt (nil: never)
// has expired at now.
func Expired(expiresAt *time.Time, now time.Time) bool {
	return expiresAt != nil && !now.Before(*expiresAt)
}

// ShouldRotate reports whether an active refresh token issued at issuedAt is
// old enough at now to be replaced on renewal.
func ShouldRotate(issuedAt time.Time, now time.Time) bool {
	return now.Sub(issuedAt) > RotationAge
}

// WithinPreviousGrace reports whether a refresh token replaced at replacedAt
// still renews at now.
func WithinPreviousGrace(replacedAt time.Time, now time.Time) bool {
	return now.Sub(replacedAt) < PreviousGrace
}
