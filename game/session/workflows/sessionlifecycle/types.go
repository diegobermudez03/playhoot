// Package sessionlifecycle is the Session lifecycle workflow: one Manager
// exposing Create/Join/Leave (and, from Slice 2, Start) as its steps, per
// `docs/engineering/standards/domain-logic-placement.md`'s Preferred
// Workflow Package Shape. The Manager decides business/lifecycle policy
// (transaction scope, admission, expiration, idempotency-replay meaning);
// its narrow `internal/repo` persistence layer reports facts and performs
// the mutations the Manager requests.
package sessionlifecycle

import "time"

// GameUUID is a public Game identity (Create's input).
type GameUUID string

// UserUUID is an already-authenticated caller identity (GAME-ADR-0005).
// Session lifecycle operations never resolve credentials themselves.
type UserUUID string

// SessionUUID is a Session's public identity.
type SessionUUID string

// JoinCode is a lobby's human-facing numeric join code.
type JoinCode uint

// DisplayName is a Session-scoped display-name snapshot supplied by the
// trusted application/integration layer.
type DisplayName string

// IdempotencyKey is a caller-supplied opaque token scoping one logical
// command within its (UserUUID, operation) stream
// (`docs/engineering/standards/idempotency.md`).
type IdempotencyKey string

// CreatedSession is Create's logical outcome, also the shape persisted as
// the idempotency record's replayable response payload.
type CreatedSession struct {
	SessionUUID    SessionUUID `json:"session_uuid"`
	JoinCode       JoinCode    `json:"join_code"`
	LobbyExpiresAt time.Time   `json:"lobby_expires_at"`
}

// JoinOutcome is Join's expected business outcome, a value distinct from a
// Go error (GAME-ADR-0022, `docs/engineering/standards/error-handling.md`'s
// Expected Business Outcome vs. Error).
type JoinOutcome string

const (
	// JoinOutcomeJoined means admission succeeded.
	JoinOutcomeJoined JoinOutcome = "JOINED"
	// JoinOutcomeLobbyExpired means the lobby was discovered already
	// expired and materialized TERMINAL in the same transaction.
	JoinOutcomeLobbyExpired JoinOutcome = "LOBBY_EXPIRED"
	// JoinOutcomeLobbyFull means admission would exceed the pinned
	// Definition's players.max.
	JoinOutcomeLobbyFull JoinOutcome = "LOBBY_FULL"
	// JoinOutcomeAlreadyJoined means a differently-tokened Join was
	// evaluated as a new command while the caller is already an active
	// Participant (GAME-ADR-0021).
	JoinOutcomeAlreadyJoined JoinOutcome = "ALREADY_JOINED"
)

// JoinResult is Join's logical outcome, also the shape persisted as the
// idempotency record's replayable response payload. SessionUUID/DisplayName
// are only populated when Outcome is JoinOutcomeJoined.
type JoinResult struct {
	Outcome     JoinOutcome `json:"outcome"`
	SessionUUID SessionUUID `json:"session_uuid,omitempty"`
	DisplayName DisplayName `json:"display_name,omitempty"`
}

// LeaveOutcome is Leave's expected business outcome, a value distinct from a
// Go error (GAME-ADR-0022).
type LeaveOutcome string

const (
	// LeaveOutcomeLeft means an active Participant was deactivated (or the
	// Participant was already inactive/missing, a harmless no-op).
	LeaveOutcomeLeft LeaveOutcome = "LEFT"
	// LeaveOutcomeNotInLobby means the Session was no longer in LOBBY phase
	// (already TERMINAL, or discovered/materialized expired by this call).
	LeaveOutcomeNotInLobby LeaveOutcome = "NOT_IN_LOBBY"
	// LeaveOutcomeActorNotFound means the calling user never joined the
	// Session.
	LeaveOutcomeActorNotFound LeaveOutcome = "ACTOR_NOT_FOUND"
)

// LeaveResult is Leave's logical outcome, also the shape persisted as the
// idempotency record's replayable response payload. SessionUUID is only
// populated when Outcome is LeaveOutcomeLeft.
type LeaveResult struct {
	Outcome     LeaveOutcome `json:"outcome"`
	SessionUUID SessionUUID  `json:"session_uuid,omitempty"`
}
