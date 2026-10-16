// Package session is the Session Runtime's public contract. It imports only
// the standard library and grows as the real-time design is accepted.
package session

import (
	"errors"
	"time"
)

// CreateSessionOutcome is the decision a CreateSession call reached.
type CreateSessionOutcome string

const (
	// CreateSessionCreated means the Session exists in LOBBY. A retry with
	// the same idempotency key reports this same outcome and Session.
	CreateSessionCreated CreateSessionOutcome = "created"

	// CreateSessionGameUnavailable means the Game does not exist or is not
	// currently playable for the caller. The two cases are deliberately not
	// distinguished.
	CreateSessionGameUnavailable CreateSessionOutcome = "game_unavailable"
)

// CreateSessionResult is what a CreateSession call decided. SessionUUID,
// JoinCode and ExpiresAt are only meaningful when Outcome is
// CreateSessionCreated.
type CreateSessionResult struct {
	Outcome CreateSessionOutcome

	// SessionUUID identifies the created Session.
	SessionUUID string

	// JoinCode is the human-friendly code players use to find the Session.
	// It is only an identifier, never proof of authorization.
	JoinCode string

	// ExpiresAt is when the lobby (and its join code) stops being usable if
	// the Session has not started.
	ExpiresAt time.Time
}

// ErrIdempotencyConflict is returned when an idempotency key is reused for a
// request that differs from the one it was first used with.
var ErrIdempotencyConflict = errors.New("idempotency key reused with a conflicting request")
