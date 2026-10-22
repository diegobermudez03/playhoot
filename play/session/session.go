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
	// AdminJoinEligible means the Session is in LOBBY, before its expiry, and
	// the user created it.
	AdminJoinEligible AdminJoinEligibility = "eligible"

	// AdminJoinEligibleRunning means the Session is RUNNING and the user
	// created it.
	AdminJoinEligibleRunning AdminJoinEligibility = "eligible_running"

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

	// Rejoin means the game implements playerRejoined(), so a disconnected
	// participant may ask to come back.
	Rejoin bool
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
	Viewport  Viewport
}

// Viewport is the fixed shape a game version's frontend is shown in: one of
// the aspect ratios Playhoot offers and its logical resolution, the size the
// frontend draws in and receives input in.
type Viewport struct {
	AspectRatio string
	Width       int
	Height      int
}

// StartParticipant maps a player to the ParticipantID the game knows them by.
type StartParticipant struct {
	UserUUID      string
	ParticipantID string

	// DisplayName is how the other players saw them when the game started,
	// kept so a recovered game still shows them.
	DisplayName string
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

// SaveCheckpointOutcome is the decision a SaveCheckpoint call reached.
type SaveCheckpointOutcome string

const (
	// CheckpointSaved means the checkpoint is the Session's latest, unless
	// one of a higher context revision was already stored.
	CheckpointSaved SaveCheckpointOutcome = "saved"

	// CheckpointSessionNotRunning means the Session is no longer RUNNING, so
	// nothing was stored.
	CheckpointSessionNotRunning SaveCheckpointOutcome = "session_not_running"
)

// ReplayChunk is one piece of a Session's replay history as the live Runtime
// encoded it; Content is opaque to this domain and stored as an immutable
// object. Final marks the chunk that holds the game's durable ending.
type ReplayChunk struct {
	RecordingUUID      string
	RecordingStartedAt time.Time
	Index              int
	FormatVersion      int

	FromSessionTimeMs    int64
	ThroughSessionTimeMs int64
	Final                bool

	Content []byte
}

// ErrSessionNotRunning is returned when an operation needs a RUNNING Session
// and the Session is in another status.
var ErrSessionNotRunning = errors.New("session is not running")

// EndCause is why a live Session ends; each leads to one terminal status.
type EndCause string

const (
	// EndGameFinished means the game itself finished (FINISHED).
	EndGameFinished EndCause = "GAME_FINISHED"

	// EndHostCancelled means the creator cancelled it (CANCELLED).
	EndHostCancelled EndCause = "HOST_CANCELLED"

	// EndAllPlayersLeft means none of a running Session's participants can
	// play anymore (CANCELLED).
	EndAllPlayersLeft EndCause = "ALL_PLAYERS_LEFT"

	// EndPlatformTerminated means the platform ended it by its own decision,
	// such as a usage limit (CANCELLED).
	EndPlatformTerminated EndCause = "PLATFORM_TERMINATED"

	// EndLobbyExpired means it did not start before its lobby expired
	// (EXPIRED).
	EndLobbyExpired EndCause = "LOBBY_EXPIRED"

	// EndInactivityTimeout means nobody played the running game for too long
	// (EXPIRED).
	EndInactivityTimeout EndCause = "INACTIVITY_TIMEOUT"

	// EndMaxLifetime means it reached its maximum lifetime, whatever its
	// activity (EXPIRED).
	EndMaxLifetime EndCause = "MAX_LIFETIME"

	// EndGameStartFailure means the game's script failed to start twice, so
	// the lobby never ran (ABORTED).
	EndGameStartFailure EndCause = "GAME_START_FAILURE"

	// EndScriptFailure means the game's script failed twice on the same
	// execution (ABORTED).
	EndScriptFailure EndCause = "SCRIPT_FAILURE"

	// EndUnrecoverableInfraFailure means the game could not be executed for
	// too long (ABORTED).
	EndUnrecoverableInfraFailure EndCause = "UNRECOVERABLE_INFRA_FAILURE"

	// EndOwnerLost means a running Session went too long without a live
	// owner and nobody came back to recover it (ABORTED).
	EndOwnerLost EndCause = "OWNER_LOST"
)

// GameFaultKind is how a player's client saw the game frontend of their
// running Session fail or misbehave.
type GameFaultKind string

const (
	// GameFaultArtifactUnreachable means the frontend package could not be
	// downloaded (network, expired URL, storage CORS).
	GameFaultArtifactUnreachable GameFaultKind = "ARTIFACT_UNREACHABLE"

	// GameFaultArtifactHashMismatch means the downloaded package did not
	// match the version's SHA-256, so it was not run.
	GameFaultArtifactHashMismatch GameFaultKind = "ARTIFACT_HASH_MISMATCH"

	// GameFaultReadyTimeout means the frontend never completed the bridge
	// handshake: it failed to load, crashed, or does not use the SDK.
	GameFaultReadyTimeout GameFaultKind = "READY_TIMEOUT"

	// GameFaultUnsupportedBridgeVersion means the frontend speaks no bridge
	// version the client does.
	GameFaultUnsupportedBridgeVersion GameFaultKind = "UNSUPPORTED_BRIDGE_VERSION"

	// GameFaultContainerViolation means the frontend tried to leave its
	// isolated container (navigating its frame, opening nested frames).
	GameFaultContainerViolation GameFaultKind = "CONTAINER_VIOLATION"

	// GameFaultBridgeAbuse means the frontend kept sending the client more
	// messages than allowed, so the client stopped it.
	GameFaultBridgeAbuse GameFaultKind = "BRIDGE_ABUSE"
)

// GameFault is a participant's client reporting a fault of the game frontend
// it was running. It is the client's claim, never verified: evidence for an
// audit telling a faulty game from a lying client, never a verdict. Detail is
// a small JSON object the client chose; nothing reads it but people.
type GameFault struct {
	UserUUID      string
	ParticipantID string
	Kind          GameFaultKind
	Detail        []byte
}

// RecordGameFaultOutcome is the decision a RecordGameFault call reached.
type RecordGameFaultOutcome string

const (
	// GameFaultRecorded means the fault was stored, or counted again when the
	// same participant had already reported that kind in the Session.
	GameFaultRecorded RecordGameFaultOutcome = "recorded"

	// GameFaultNotRecorded means the Session is not running, or the reporter
	// is not its participant, so nothing was stored.
	GameFaultNotRecorded RecordGameFaultOutcome = "not_recorded"
)

// LimitHit is how many times one of the platform's rate or resource limits
// refused what a user of a live Session sent, since the last time it was
// recorded. Limit names the limit, such as api.command.rate or
// runtime.command_participant_rate.
type LimitHit struct {
	SessionUUID string
	UserUUID    string
	Limit       string
	Count       int
}

// MaxGameFaultDetailBytes bounds a game fault's detail.
const MaxGameFaultDetailBytes = 2048

// OwnerHeartbeatInterval is how often the live owner of a RUNNING Session
// records a heartbeat, which renews its owner lease; a Session whose lease
// runs out has no live owner.
const OwnerHeartbeatInterval = 30 * time.Second

// OwnerHeartbeatOutcome is the decision a RecordOwnerHeartbeat call reached.
type OwnerHeartbeatOutcome string

const (
	// HeartbeatRecorded means the Session is RUNNING and its owner lease was
	// renewed.
	HeartbeatRecorded OwnerHeartbeatOutcome = "recorded"

	// HeartbeatSessionNotRunning means the Session is no longer RUNNING;
	// Status and EndCause tell where it is.
	HeartbeatSessionNotRunning OwnerHeartbeatOutcome = "session_not_running"
)

// OwnerHeartbeatResult is the outcome of a heartbeat. Status and EndCause are
// set only when the Session is not running; EndCause only once it ended.
type OwnerHeartbeatResult struct {
	Outcome  OwnerHeartbeatOutcome
	Status   string
	EndCause string
}

// SessionEnd is a request to end a live Session for Cause. ChangedBy is the
// user who asked, empty when the platform decided.
type SessionEnd struct {
	Cause     EndCause
	ChangedBy string
}

// EndSessionOutcome is the decision an EndSession call reached.
type EndSessionOutcome string

const (
	// SessionEndedNow means the Session moved to its terminal status now.
	SessionEndedNow EndSessionOutcome = "ended"

	// SessionNotEnded means the Session was not in a status the cause applies
	// to, typically because it had already ended; it was left as it is.
	SessionNotEnded EndSessionOutcome = "not_ended"
)

// EndSessionResult is what an EndSession call decided, with the Session's
// status and end cause after it (EndCause is empty while it has not ended).
type EndSessionResult struct {
	Outcome  EndSessionOutcome
	Status   string
	EndCause string
}

// SessionLimits are the platform-time deadlines of a Session: its lobby
// expires at LobbyExpiresAt if it has not started, and the Session itself at
// ExpiresAt, whatever its activity.
type SessionLimits struct {
	LobbyExpiresAt time.Time
	ExpiresAt      time.Time
}
