package session

import (
	"encoding/json"
	"time"
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

// CreatedSession is Create's logical outcome: the new Session's public
// identity, its lobby JoinCode, and when that JoinCode expires.
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

// JoinResult is Join's logical outcome. SessionUUID/DisplayName are only
// populated when Outcome is JoinOutcomeJoined.
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

// LeaveResult is Leave's logical outcome. SessionUUID is only populated
// when Outcome is LeaveOutcomeLeft.
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

// ActorRef is an opaque, session-scoped actor identity - the same identity
// the executed backend script's own SEND_EVENT recipients are already
// validated against before an OutboundEvent is ever constructed. It is
// stable within one Session only, and is deliberately not a UserUUID or any
// other transport/routing identity: whatever future delivery layer consumes
// an OutboundEvent decides how, or whether, to map this to a connection or
// routing identity of its own.
type ActorRef string

// OutboundEvent is one game-defined SEND_EVENT command a RUNNING-phase
// call's own execute pass requested. Recipients are session-scoped only,
// never resolved to any transport identity by this package. Name/Payload
// are entirely game-defined and opaque; only the envelope (Recipients) is
// Playhoot's own. Nothing here claims delivery has happened - it hasn't;
// no live-transport layer exists yet to attempt it. Delivering this event is
// best-effort and may be duplicated, delayed, or missed entirely: ID is a
// stable, deterministic per-event correlation identifier (reproducible for
// the same underlying command across a delivery retry - never randomly
// regenerated), and Revision is the sequence number of the RuntimeTurn that
// produced this event - together they let a future delivery consumer
// recognize an already-applied duplicate and discard a stale event.
type OutboundEvent struct {
	ID         string          `json:"id"`
	Recipients []ActorRef      `json:"recipients"`
	Name       string          `json:"name"`
	Payload    json.RawMessage `json:"payload"`
	Revision   uint64          `json:"revision"`
}

// StartResult is Start's logical outcome. SessionUUID is only populated
// when Outcome is StartOutcomeStarted. TerminalReason is populated (one of
// TerminalReasonGame*) whenever this same Turn also ended the Session. A
// per-player view is obtained through its own separate, dedicated call
// (GetClientState); Events carries only this call's own transient
// SEND_EVENT occurrences, in memory, never persisted (see its own doc
// comment) - it is nil whenever this call did not itself execute the
// backend script (a decline, a replayed retry, or a fatal path).
type StartResult struct {
	Outcome        StartOutcome    `json:"outcome"`
	SessionUUID    SessionUUID     `json:"session_uuid,omitempty"`
	TerminalReason string          `json:"terminal_reason,omitempty"`
	Events         []OutboundEvent `json:"-"`
}

// SubmitPlayerEventOutcome is SubmitPlayerEvent's expected business
// outcome, a value distinct from a Go error: an ordinary decline a caller
// should branch on, not treat as a failure. A submitted player event has no
// existing durable row to compare a retry against, so a retried,
// semantically equivalent submission replays via its IdempotencyKey
// instead (see ErrIdempotencyConflict for a same-key retry with a
// materially different request).
type SubmitPlayerEventOutcome string

const (
	// SubmitPlayerEventOutcomeAccepted means the event was accepted: a new
	// RuntimeTurn committed - or, for a retried submission under the same
	// IdempotencyKey, the original outcome replayed without a second
	// execution.
	SubmitPlayerEventOutcomeAccepted SubmitPlayerEventOutcome = "ACCEPTED"
	// SubmitPlayerEventOutcomeRejected means the event was declined without
	// any execution effect: the caller did not resolve to a current
	// Participant, or the authored script itself rejected it (threw, or no
	// matching game-defined reaction).
	SubmitPlayerEventOutcomeRejected SubmitPlayerEventOutcome = "REJECTED"
	// SubmitPlayerEventOutcomeRuntimeExecutionFailed means an infrastructure-
	// level Executor failure terminalized the Session while processing the
	// event.
	SubmitPlayerEventOutcomeRuntimeExecutionFailed SubmitPlayerEventOutcome = "RUNTIME_EXECUTION_FAILED"
)

// SubmitPlayerEventResult is SubmitPlayerEvent's logical outcome.
// TerminalReason is populated (one of TerminalReasonGame*) whenever this
// same event also ended the Session. See StartResult's own doc comment for
// Events' meaning and nil conditions.
type SubmitPlayerEventResult struct {
	Outcome        SubmitPlayerEventOutcome `json:"outcome"`
	SessionUUID    SessionUUID              `json:"session_uuid,omitempty"`
	TerminalReason string                   `json:"terminal_reason,omitempty"`
	Events         []OutboundEvent          `json:"-"`
}

// GetSubmitPlayerEventOutcomeOutcome is GetSubmitPlayerEventOutcome's
// expected business outcome, a value distinct from a Go error: an ordinary
// decline a caller should branch on, not treat as a failure.
type GetSubmitPlayerEventOutcomeOutcome string

const (
	// GetSubmitPlayerEventOutcomeFound means idempotencyKey's own
	// SubmitPlayerEvent request already completed; Result carries its
	// recorded outcome, exactly as the original call itself would have
	// returned it.
	GetSubmitPlayerEventOutcomeFound GetSubmitPlayerEventOutcomeOutcome = "FOUND"
	// GetSubmitPlayerEventOutcomeNotFound means no completed request exists
	// yet for idempotencyKey - it was never submitted, is still being
	// processed, or belongs to a different session than sessionUUID names.
	GetSubmitPlayerEventOutcomeNotFound GetSubmitPlayerEventOutcomeOutcome = "NOT_FOUND"
)

// GetSubmitPlayerEventOutcomeResult is GetSubmitPlayerEventOutcome's logical
// outcome. Result is only populated when Outcome is
// GetSubmitPlayerEventOutcomeFound. This is a pure read against the same
// durable record SubmitPlayerEvent's own idempotency mechanism already
// writes - it never re-executes anything and never mutates Session state,
// letting a caller that never received the original response recover it
// afterward instead of needing a durable delivery outbox.
type GetSubmitPlayerEventOutcomeResult struct {
	Outcome GetSubmitPlayerEventOutcomeOutcome `json:"outcome"`
	Result  SubmitPlayerEventResult            `json:"result,omitempty"`
}

// CancelSessionOutcome is CancelSession's expected business outcome, a value
// distinct from a Go error: an ordinary decline a caller should branch on,
// not treat as a failure. Unlike every other signal-driven capability,
// CancelSessionOutcomeCancelled covers three distinct underlying paths (the
// authored game reacting to SessionCancelled and itself reaching a terminal
// run status, reacting without reaching one, or not reacting at all) - a
// host cancel always ends the Session once accepted past authorization, so
// callers branch on TerminalReason for which happened, not on Outcome.
type CancelSessionOutcome string

const (
	// CancelSessionOutcomeCancelled means the Session is now TERMINAL as a
	// direct result of this call - whether because the authored game itself
	// reacted to SessionCancelled and reached a terminal run status
	// (TerminalReason is one of TerminalReasonGame*), or because it did not
	// (TerminalReason is TerminalReasonSessionCancelledByHost) - or, for a
	// retried call under the same IdempotencyKey, the original outcome
	// replayed without a second engine effect.
	CancelSessionOutcomeCancelled CancelSessionOutcome = "CANCELLED"
	// CancelSessionOutcomeAlreadyTerminal means the Session was already
	// TERMINAL (any TerminalReason, including a prior CancelSession call)
	// before this call - an idempotent no-op, not an error.
	CancelSessionOutcomeAlreadyTerminal CancelSessionOutcome = "ALREADY_TERMINAL"
	// CancelSessionOutcomeNotRunning means the Session was still LOBBY -
	// there is no running engine instance yet to deliver SessionCancelled
	// to.
	CancelSessionOutcomeNotRunning CancelSessionOutcome = "NOT_RUNNING"
	// CancelSessionOutcomeNotHost means the caller did not resolve to the
	// Session's own host actor.
	CancelSessionOutcomeNotHost CancelSessionOutcome = "NOT_HOST"
	// CancelSessionOutcomeRuntimeExecutionFailed means a deterministic
	// engine execution failure (unrelated to SessionCancelled itself being
	// rejected) terminalized the Session while processing the cancellation.
	CancelSessionOutcomeRuntimeExecutionFailed CancelSessionOutcome = "RUNTIME_EXECUTION_FAILED"
)

// CancelSessionResult is CancelSession's logical outcome. TerminalReason is
// always populated when Outcome is CancelSessionOutcomeCancelled. See
// StartResult's own doc comment for Events' meaning and nil conditions;
// Events is also nil whenever the authored script rejected SessionCancelled
// outright (no execute pass to gather it from).
type CancelSessionResult struct {
	Outcome        CancelSessionOutcome `json:"outcome"`
	SessionUUID    SessionUUID          `json:"session_uuid,omitempty"`
	TerminalReason string               `json:"terminal_reason,omitempty"`
	Events         []OutboundEvent      `json:"-"`
}

// TimerObligationUUID is a session_timer_obligations row's public identity -
// the handle a caller correlates a physical wall-clock timer against, and
// ExpireTimer's own input.
type TimerObligationUUID string

// ExpireTimerOutcome is ExpireTimer's expected business outcome, a value
// distinct from a Go error: an ordinary decline a caller should branch on,
// not treat as a failure.
type ExpireTimerOutcome string

const (
	// ExpireTimerOutcomeExpired means the expiration was accepted: a new
	// RuntimeTurn committed and the obligation resolved (CONSUMED).
	ExpireTimerOutcomeExpired ExpireTimerOutcome = "EXPIRED"
	// ExpireTimerOutcomeStale means the obligation was no longer ACTIVE
	// (already CONSUMED or CANCELLED) - an ordinary duplicate/late physical
	// timer delivery, no engine effect.
	ExpireTimerOutcomeStale ExpireTimerOutcome = "STALE"
	// ExpireTimerOutcomeRejected means the engine itself rejected the
	// expiration signal as stale/cancelled, no engine effect committed.
	ExpireTimerOutcomeRejected ExpireTimerOutcome = "REJECTED"
	// ExpireTimerOutcomeRuntimeExecutionFailed means a deterministic engine
	// execution failure terminalized the Session while processing the
	// expiration.
	ExpireTimerOutcomeRuntimeExecutionFailed ExpireTimerOutcome = "RUNTIME_EXECUTION_FAILED"
)

// ExpireTimerResult is ExpireTimer's logical outcome. SessionUUID is
// populated whenever the obligation's owning Session was resolved (every
// outcome except an unresolved timerObligationUUID, reported as
// ErrTimerObligationNotFound instead). TerminalReason is populated (one of
// TerminalReasonGame*) whenever this same expiration also ended the
// Session. See StartResult's own doc comment for Events' meaning and nil
// conditions.
type ExpireTimerResult struct {
	Outcome        ExpireTimerOutcome `json:"outcome"`
	SessionUUID    SessionUUID        `json:"session_uuid,omitempty"`
	TerminalReason string             `json:"terminal_reason,omitempty"`
	Events         []OutboundEvent    `json:"-"`
}

// GetClientStateOutcome is GetClientState's expected business outcome, a
// value distinct from a Go error: an ordinary decline a caller should
// branch on, not treat as a failure.
type GetClientStateOutcome string

const (
	// GetClientStateOutcomeComputed means ClientState was successfully
	// computed from data the viewer is already authorized to see. It does
	// not by itself certify the result passed every defense-in-depth check
	// available - a caller needing that distinction has its own separate
	// signal for it.
	GetClientStateOutcomeComputed GetClientStateOutcome = "COMPUTED"
	// GetClientStateOutcomeNotRunning means the Session has not yet
	// committed a first RuntimeTurn (still LOBBY) - there is no
	// authoritative state yet to project from.
	GetClientStateOutcomeNotRunning GetClientStateOutcome = "NOT_RUNNING"
	// GetClientStateOutcomeNotAParticipant means the caller did not
	// resolve to a SessionActor for this Session.
	GetClientStateOutcomeNotAParticipant GetClientStateOutcome = "NOT_A_PARTICIPANT"
	// GetClientStateOutcomeProjectionRejected means the authored script's
	// own project() function declined the input (threw, or defines no
	// project function) - a business-level outcome, not an infrastructure
	// failure.
	GetClientStateOutcomeProjectionRejected GetClientStateOutcome = "PROJECTION_REJECTED"
)

// GetClientStateResult is GetClientState's logical outcome. ClientState and
// Revision are only populated when Outcome is GetClientStateOutcomeComputed
// - ClientState is the authored script's own project() output, computed
// only from data the viewer is already authorized to see (project() never
// receives excluded data at all, rather than being trusted with everything
// and checked afterward); Revision is the sequence number of the RuntimeTurn
// ClientState was computed from, letting a caller discard a delivery no
// newer than one it already applied.
type GetClientStateResult struct {
	Outcome     GetClientStateOutcome `json:"outcome"`
	SessionUUID SessionUUID           `json:"session_uuid,omitempty"`
	ClientState json.RawMessage       `json:"client_state,omitempty"`
	Revision    uint64                `json:"revision,omitempty"`
}
