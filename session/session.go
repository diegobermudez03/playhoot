// Package session is the Session Runtime capability's public contract: the
// lobby lifecycle phase/presence/terminal-reason values, the sentinel
// errors, and every request/result type its Session lifecycle workflow's
// operations (Create/Join/Leave/Start/SubmitPlayerEvent/CancelSession/
// ExpireTimer/GetClientState) take or
// return. This package intentionally imports nothing beyond the standard
// library, so a caller depending only on this contract never transitively
// imports anything the workflow's own implementation
// (session/workflows/sessionlifecycle) needs, such as the Game
// Language engine. That implementation package refers to these types fully
// qualified (session.SessionUUID, session.StartResult, ...) rather than
// re-declaring them under local names.
package session

import "errors"

// Session lifecycle phase values.
const (
	PhaseLobby    = "LOBBY"
	PhaseRunning  = "RUNNING"
	PhaseTerminal = "TERMINAL"
)

// SessionActor semantic presence values. A normal Join begins CONNECTED;
// DISCONNECTED is not currently produced by any operation.
const (
	PresenceConnected    = "CONNECTED"
	PresenceDisconnected = "DISCONNECTED"
)

// Terminal-reason values recorded when a Session becomes TERMINAL.
const (
	TerminalReasonLobbyExpired = "LOBBY_EXPIRED"

	// TerminalReasonRuntimeExecutionFailed marks a Session terminated by a
	// deterministic game-execution failure occurring before its first Turn
	// completes.
	TerminalReasonRuntimeExecutionFailed = "RUNTIME_EXECUTION_FAILED"

	// TerminalReasonRuntimeStateInvalid marks a Session terminated because
	// its pinned Definition unexpectedly fails to recompile at Start despite
	// having compiled successfully at Create - a durable state-integrity
	// failure, not a game-execution failure.
	TerminalReasonRuntimeStateInvalid = "RUNTIME_STATE_INVALID"

	// TerminalReasonGameCompleted marks a Session terminated because the
	// authored script itself requested SESSION_COMPLETE - an ordinary,
	// expected outcome, not a failure.
	TerminalReasonGameCompleted = "GAME_COMPLETED"

	// TerminalReasonGameFailed marks a Session terminated because the
	// authored script itself requested SESSION_FAIL - distinct from
	// TerminalReasonRuntimeExecutionFailed, which marks an Executor
	// infrastructure failure rather than a deliberate authored outcome.
	TerminalReasonGameFailed = "GAME_FAILED"

	// TerminalReasonSessionCancelledByHost marks a Session terminated by an
	// explicit host cancellation command (CancelSession) that did not itself
	// result in the authored script requesting SESSION_COMPLETE/SESSION_FAIL.
	// A host cancellation always terminalizes the Session even when the
	// authored script has no reaction to SESSION_CANCELLED at all.
	TerminalReasonSessionCancelledByHost = "SESSION_CANCELLED_BY_HOST"

	// TerminalReasonRuntimeInactivityExpired marks a RUNNING Session
	// terminated because no meaningful activity renewed activity_expires_at
	// before its deadline passed - an ordinary, expected lifecycle outcome,
	// not a runtime failure; no session_runtime_failures row is created for
	// it.
	TerminalReasonRuntimeInactivityExpired = "RUNTIME_INACTIVITY_EXPIRED"
)

// session_timer_obligations.state values. A timer obligation's closure is
// either its own expiration (CONSUMED) or an explicit/terminal cancellation
// (CANCELLED).
const (
	// TimerObligationStateActive means the timer is still pending expiration.
	TimerObligationStateActive = "ACTIVE"
	// TimerObligationStateConsumed means a committed RuntimeTurn closed the
	// obligation by processing its own expiration (Manager.ExpireTimer).
	TimerObligationStateConsumed = "CONSUMED"
	// TimerObligationStateCancelled means the obligation was closed without
	// expiring - an authored CancelTimerOperation/CancelKeyedTimerOperation,
	// or terminal cleanup (closure_reason
	// TimerObligationClosureReasonSessionTerminated).
	TimerObligationStateCancelled = "CANCELLED"
)

// TimerObligationClosureReasonSessionTerminated is
// session_timer_obligations.closure_reason's value for a still-ACTIVE
// obligation cancelled by terminal cleanup rather than an authored cancel.
const TimerObligationClosureReasonSessionTerminated = "SESSION_TERMINATED"

var (
	// ErrGameNotFound is returned by CreateSession when the referenced Game
	// has no currently pinnable version in Session Runtime's own tables.
	ErrGameNotFound = errors.New("game not found")

	// ErrPinnedDefinitionMissing is returned when a Session's already-pinned
	// game_definition_uuid no longer resolves to a Game Definition. Pinned
	// Definitions are never hard-deleted, so this represents an unexpected
	// data-integrity condition rather than an ordinary rejection.
	ErrPinnedDefinitionMissing = errors.New("pinned game definition is missing")

	// ErrIdempotencyConflict is returned when the same (user_uuid,
	// operation, idempotency_key) identity is reused with a meaningfully
	// different request.
	ErrIdempotencyConflict = errors.New("idempotency key reused with a conflicting request")

	// ErrIdempotencyInFlight is returned, defensively, when an existing
	// idempotency claim is found but has not yet reached a completed
	// outcome. Under the single-transaction claim-then-commit design,
	// callers should not normally observe this.
	ErrIdempotencyInFlight = errors.New("idempotency key claim is still in flight")

	// ErrJoinCodeInvalid is returned when a JoinCode does not resolve to an
	// admissible Session (unknown or already revoked).
	ErrJoinCodeInvalid = errors.New("join code is invalid or no longer active")

	// ErrSessionNotFound is returned when a Session UUID does not resolve to
	// an existing Session.
	ErrSessionNotFound = errors.New("session not found")

	// ErrIdempotencyKeyRequired is returned by Create/Join/Leave when the
	// caller supplies a missing/empty idempotency token - every command on
	// these operations requires one.
	ErrIdempotencyKeyRequired = errors.New("idempotency key is required")

	// ErrTimerObligationNotFound is returned when a timer obligation UUID
	// does not resolve to an existing session_timer_obligations row.
	ErrTimerObligationNotFound = errors.New("timer obligation not found")
)
