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

	// Inputs are the logical inputs SAMPLEs carry, in name order; none means
	// the game takes no SAMPLE.
	Inputs []RuntimeInput

	// Commands are the COMMANDs the game accepts, in name order.
	Commands []RuntimeCommand

	// Update means the game implements update(), which the Runtime calls
	// periodically while the game runs.
	Update bool
}

// RuntimeCommand is one declared COMMAND; its payload fields are declared
// like SAMPLE inputs, in name order.
type RuntimeCommand struct {
	Name    string
	Payload []RuntimeInput
}

// RuntimeInput is one declared SAMPLE input. Type is boolean, number,
// vector2, string or enum; Default is the value an omitted input takes, as
// JSON, already filled in when the manifest relies on the natural zero.
type RuntimeInput struct {
	Name     string
	Type     string
	Nullable bool
	Min      *float64
	Max      *float64
	Values   []string
	Default  []byte
}

// ErrSessionNotFound is returned when an operation names a Session that does
// not exist.
var ErrSessionNotFound = errors.New("session not found")

// StartEligibilityOutcome is whether a Session may start.
type StartEligibilityOutcome string

const (
	// StartEligible means the Session is in LOBBY before its expiry, the
	// user created it, the player count fits its game version and that
	// version can still be played.
	StartEligible StartEligibilityOutcome = "eligible"

	StartSessionNotFound StartEligibilityOutcome = "session_not_found"

	// StartAlreadyStarted means the Session is RUNNING.
	StartAlreadyStarted StartEligibilityOutcome = "already_started"

	// StartNotInLobby means the Session ended or its lobby expired.
	StartNotInLobby StartEligibilityOutcome = "not_in_lobby"

	StartNotCreator StartEligibilityOutcome = "not_creator"

	// StartInvalidPlayerCount means the game version does not allow that
	// many players.
	StartInvalidPlayerCount StartEligibilityOutcome = "invalid_player_count"

	// StartGameUnavailable means the pinned game version can no longer be
	// played.
	StartGameUnavailable StartEligibilityOutcome = "game_unavailable"
)

// StartEligibility is whether a Session may start and, when eligible, the
// game it starts. GameVersionUUID and Script are set only when eligible.
type StartEligibility struct {
	Outcome         StartEligibilityOutcome
	GameVersionUUID string

	// Script is the pinned game version's verified JavaScript source.
	Script []byte
}

// FrontendAccess is signed, expiring access to a game version's frontend
// package. SHA256 is the package's content hash.
type FrontendAccess struct {
	URL       string
	SHA256    string
	ExpiresAt time.Time
}

// StartParticipant maps a player to the ParticipantID the game knows them by.
type StartParticipant struct {
	UserUUID      string
	ParticipantID string
}

// StartCommit is what a Session records when it becomes RUNNING.
type StartCommit struct {
	Participants []StartParticipant

	// Checkpoint is the initial recovery checkpoint, stored in the same
	// transaction, so a RUNNING Session is always recoverable.
	Checkpoint Checkpoint
}

// Checkpoint is a recovery checkpoint as the live Runtime encoded it; Content
// is opaque to this domain.
type Checkpoint struct {
	ContextRevision int64
	Content         []byte
}

// CommitStartOutcome is the decision a CommitStart call reached.
type CommitStartOutcome string

const (
	// CommitStartCommitted means the Session is RUNNING with the roster and
	// checkpoint, and its join code no longer works.
	CommitStartCommitted CommitStartOutcome = "committed"

	// CommitStartNotInLobby means the Session left its lobby meanwhile, so
	// nothing was recorded.
	CommitStartNotInLobby CommitStartOutcome = "not_in_lobby"
)

// RunningSession is what the live Runtime needs to run a started Session
// again: its game, its roster in ParticipantID order, and its latest
// recovery checkpoint.
type RunningSession struct {
	GameVersionUUID string

	// Script is the pinned game version's verified JavaScript source.
	Script []byte

	Participants []StartParticipant
	Checkpoint   Checkpoint
}

// ErrSessionNotRunning is returned when an operation needs a RUNNING Session
// and the Session is in another status.
var ErrSessionNotRunning = errors.New("session is not running")

// AbortCause is why the platform aborts a running Session.
type AbortCause string

const (
	// AbortScriptFailure means the game's script failed twice on the same
	// execution.
	AbortScriptFailure AbortCause = "SCRIPT_FAILURE"

	// AbortUnrecoverableInfraFailure means the game could not be executed
	// for too long.
	AbortUnrecoverableInfraFailure AbortCause = "UNRECOVERABLE_INFRA_FAILURE"
)
