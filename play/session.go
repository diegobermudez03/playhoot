// Package session is the Session Runtime capability's public contract: the
// types, values and errors a caller depends on without importing the
// implementation behind them. It intentionally imports nothing beyond the
// standard library.
//
// The contract is partial: the Session Runtime is being redesigned as a
// real-time runtime, and its public types are declared here as that design
// is accepted.
package session

import (
	"errors"
	"time"
)

// SessionUUID is the stable public identifier of a Session.
type SessionUUID string

// GameUUID identifies the Game a Session is created from.
type GameUUID string

// IdempotencyKey is the client-generated token that makes a state-changing
// request safe to retry.
type IdempotencyKey string

// JoinCode is the human-friendly code players use to find a Session. It is
// only an identifier, never proof of authorization.
type JoinCode string

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
	Outcome     CreateSessionOutcome
	SessionUUID SessionUUID
	JoinCode    JoinCode

	// ExpiresAt is when the lobby (and its join code) stops being usable if
	// the Session has not started.
	ExpiresAt time.Time
}

// ErrIdempotencyConflict is returned when an idempotency key is reused for a
// request that differs from the one it was first used with.
var ErrIdempotencyConflict = errors.New("idempotency key reused with a conflicting request")
