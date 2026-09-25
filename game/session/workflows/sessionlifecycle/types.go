// Package sessionlifecycle is the Session lifecycle workflow: one Manager
// exposing Create/Join/Leave/Start/AnswerInteraction as its steps. The
// Manager decides business/lifecycle policy (transaction scope, admission,
// expiration, idempotency-replay meaning); its narrow `internal/repo`
// persistence layer reports facts and performs the mutations the Manager
// requests.
package sessionlifecycle

import (
	"time"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
)

// GameUUID is a public Game identity (Create's input).
type GameUUID string

// UserUUID is an already-authenticated caller identity. Session lifecycle
// operations never resolve credentials themselves.
type UserUUID string

// SessionUUID is a Session's public identity.
type SessionUUID string

// JoinCode is a lobby's human-facing numeric join code.
type JoinCode uint

// DisplayName is a Session-scoped display-name snapshot supplied by the
// trusted application/integration layer.
type DisplayName string

// IdempotencyKey is a caller-supplied opaque token scoping one logical
// command within its (UserUUID, operation) stream: retrying the same
// command with the same key replays its original outcome instead of
// executing it again.
type IdempotencyKey string

// CreatedSession is Create's logical outcome, also the shape persisted as
// the idempotency record's replayable response payload.
type CreatedSession struct {
	SessionUUID    SessionUUID `json:"session_uuid"`
	JoinCode       JoinCode    `json:"join_code"`
	LobbyExpiresAt time.Time   `json:"lobby_expires_at"`
}

// JoinOutcome is Join's expected business outcome, a value distinct from a
// Go error: an ordinary decline a caller should branch on, not treat as a
// failure.
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
	// Participant.
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
// Go error: an ordinary decline a caller should branch on, not treat as a
// failure.
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

// StartOutcome is Start's expected business outcome, a value distinct from
// a Go error: an ordinary decline a caller should branch on, not treat as a
// failure.
type StartOutcome string

const (
	// StartOutcomeStarted means the Session is now RUNNING with its first
	// RuntimeTurn committed.
	StartOutcomeStarted StartOutcome = "STARTED"
	// StartOutcomeLobbyExpired means the lobby was discovered already
	// expired and materialized TERMINAL in the same transaction.
	StartOutcomeLobbyExpired StartOutcome = "LOBBY_EXPIRED"
	// StartOutcomeNotHost means the resolved SessionActorID is not
	// sessions.host_actor_id.
	StartOutcomeNotHost StartOutcome = "NOT_HOST"
	// StartOutcomeNotEnoughPlayers means active Participant count is below
	// the pinned Definition's players.min, or (as a defensive check) above
	// players.max.
	StartOutcomeNotEnoughPlayers StartOutcome = "NOT_ENOUGH_PLAYERS"
	// StartOutcomeRuntimeInitFailed means the pre-first-Turn fatal path was
	// taken: the Session is now TERMINAL, started_at remains NULL, and no
	// RuntimeTurn/Step/State was persisted.
	StartOutcomeRuntimeInitFailed StartOutcome = "RUNTIME_INIT_FAILED"
)

// StartResult is Start's logical outcome, also the shape persisted as the
// idempotency record's replayable response payload. SessionUUID is only
// populated when Outcome is StartOutcomeStarted. Outputs carries the first
// committed RuntimeTurn's client-facing Effect/Presentation Outputs, in
// commit order - populated only when this call actually executed the
// engine, never persisted, and never present on a replayed retry. It is
// excluded from the persisted idempotency payload.
type StartResult struct {
	Outcome     StartOutcome    `json:"outcome"`
	SessionUUID SessionUUID     `json:"session_uuid,omitempty"`
	Outputs     []engine.Output `json:"-"`
}

// InteractionUUID is a session_interactions row's public identity.
type InteractionUUID string

// AnswerInteractionOutcome is AnswerInteraction's expected business
// outcome, a value distinct from a Go error: an ordinary decline a caller
// should branch on, not treat as a failure.
type AnswerInteractionOutcome string

const (
	// AnswerInteractionOutcomeAnswered means the response was accepted: a
	// new RuntimeTurn committed and the interaction resolved - or, for a
	// retried, semantically equivalent response to an already-resolved
	// interaction, the original outcome replayed without a second engine
	// effect.
	AnswerInteractionOutcomeAnswered AnswerInteractionOutcome = "ANSWERED"
	// AnswerInteractionOutcomeRejected means the response was declined
	// without any engine effect: the caller was not the interaction's
	// recipient, the interaction was no longer answerable (already
	// terminally closed, or resolved by a different respondent's answer -
	// unreachable for this caller), or the engine itself rejected the
	// answer as stale/duplicate/invalid.
	AnswerInteractionOutcomeRejected AnswerInteractionOutcome = "REJECTED"
	// AnswerInteractionOutcomeConflict means the interaction was already
	// resolved with a response different from the one now submitted.
	AnswerInteractionOutcomeConflict AnswerInteractionOutcome = "CONFLICT"
	// AnswerInteractionOutcomeRuntimeExecutionFailed means a deterministic
	// engine execution failure (including Step-bound overflow) terminalized
	// the Session while processing the response.
	AnswerInteractionOutcomeRuntimeExecutionFailed AnswerInteractionOutcome = "RUNTIME_EXECUTION_FAILED"
)

// AnswerInteractionResult is AnswerInteraction's logical outcome.
// SessionUUID is populated whenever the interaction's owning Session was
// resolved (every outcome except an unresolved interactionUUID, reported as
// session.ErrInteractionNotFound instead). Outputs carries the committed
// RuntimeTurn's client-facing Effect/Presentation Outputs, in commit order -
// populated only for AnswerInteractionOutcomeAnswered when this call
// actually executed the engine, never persisted, and never present on a
// replayed or declined outcome.
type AnswerInteractionResult struct {
	Outcome     AnswerInteractionOutcome `json:"outcome"`
	SessionUUID SessionUUID              `json:"session_uuid,omitempty"`
	Outputs     []engine.Output          `json:"-"`
}
