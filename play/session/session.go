// Package session is the durable Session's public contract: the types its
// operations take and return. It imports only the standard library.
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

// AdminJoinEligibility is whether a user may open an admin connection to a
// Session.
type AdminJoinEligibility string

const (
	// AdminJoinEligible means the Session is in LOBBY or RUNNING and the user
	// created it.
	AdminJoinEligible AdminJoinEligibility = "eligible"

	AdminJoinSessionNotFound AdminJoinEligibility = "session_not_found"

	// AdminJoinSessionNotJoinable means the Session already ended.
	AdminJoinSessionNotJoinable AdminJoinEligibility = "session_not_joinable"

	AdminJoinNotCreator AdminJoinEligibility = "not_creator"
)

// JoinCodeOutcome is whether a join code names a Session.
type JoinCodeOutcome string

const (
	JoinCodeFound JoinCodeOutcome = "found"

	// JoinCodeNotFound means the code is unknown, released or expired; the
	// cases are deliberately not distinguished.
	JoinCodeNotFound JoinCodeOutcome = "not_found"
)

// JoinCodeResolution is the Session a join code names. SessionUUID is set
// only when Outcome is JoinCodeFound.
type JoinCodeResolution struct {
	Outcome     JoinCodeOutcome
	SessionUUID string
}

// PlayerJoinEligibility is whether a Session accepts new players.
type PlayerJoinEligibility string

const (
	// PlayerJoinEligible means the Session is in LOBBY and its lobby has not
	// expired.
	PlayerJoinEligible PlayerJoinEligibility = "eligible"

	PlayerJoinSessionNotFound PlayerJoinEligibility = "session_not_found"

	// PlayerJoinSessionStarted means the Session is RUNNING; only its
	// participants may come back, by rejoining.
	PlayerJoinSessionStarted PlayerJoinEligibility = "session_started"

	// PlayerJoinSessionNotJoinable means the Session ended or its lobby
	// expired.
	PlayerJoinSessionNotJoinable PlayerJoinEligibility = "session_not_joinable"
)

// RuntimeManifest is what the live Runtime must know about the game version
// a Session is pinned to. It never changes for a Session.
type RuntimeManifest struct {
	// MinPlayers and MaxPlayers bound how many players a game can start
	// with; 1 <= MinPlayers <= MaxPlayers.
	MinPlayers int
	MaxPlayers int
}

// ErrSessionNotFound is returned when an operation names a Session that does
// not exist.
var ErrSessionNotFound = errors.New("session not found")
