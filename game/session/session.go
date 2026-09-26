// Package session is the Session Runtime capability's public contract: the
// lobby lifecycle phase/presence/terminal-reason values, the sentinel
// errors, and every request/result type its Session lifecycle workflow's
// operations (Create/Join/Leave/Start/AnswerInteraction/ExpireTimer) take or
// return. This package intentionally imports nothing beyond the standard
// library, so a caller depending only on this contract never transitively
// imports anything the workflow's own implementation
// (game/session/workflows/sessionlifecycle) needs, such as the Game
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
	// authored game itself finished as designed (an authored CompleteControl
	// applied) - an ordinary, expected outcome, not a failure.
	TerminalReasonGameCompleted = "GAME_COMPLETED"

	// TerminalReasonGameFailed marks a Session terminated because the
	// authored game's own logic determined a failure condition (an authored
	// FailControl applied) - distinct from TerminalReasonRuntimeExecutionFailed,
	// which marks an engine-execution malfunction rather than a deliberate
	// authored outcome.
	TerminalReasonGameFailed = "GAME_FAILED"

	// TerminalReasonGameCancelled marks a Session terminated because the
	// authored game's own logic abandoned the instance (an authored
	// CancelControl applied) - distinct from any future session-lifecycle-level
	// cancellation a host or operator initiates directly.
	TerminalReasonGameCancelled = "GAME_CANCELLED"
)

// session_interactions.kind values: which of the engine's two
// open-question shapes produced this interaction, needed to construct the
// correct engine.SignalKind when a response is submitted - OpenQuestionOutput
// itself carries no such discriminator.
const (
	InteractionKindQuestion = "QUESTION"
	InteractionKindAskGroup = "ASK_GROUP"
)

// session_interactions.state values.
const (
	// InteractionStateActive means the interaction is still pending a
	// response.
	InteractionStateActive = "ACTIVE"
	// InteractionStateClosed means a committed RuntimeTurn closed the
	// interaction (closed_by_turn_id is set) - an accepted response, or an
	// authored CloseQuestionOperation closing it without one.
	InteractionStateClosed = "CLOSED"
	// InteractionStateTerminated means the Session itself terminalized
	// while the interaction was still ACTIVE, so terminal-cleanup closed it
	// directly instead (closed_by_turn_id NULL, closure_reason
	// InteractionClosureReasonSessionTerminated) - not gameplay closure.
	InteractionStateTerminated = "TERMINATED"
)

// InteractionClosureReasonSessionTerminated is session_interactions.
// closure_reason's value for InteractionStateTerminated rows.
const InteractionClosureReasonSessionTerminated = "SESSION_TERMINATED"

// session_timer_obligations.state values. Reuses the same "ACTIVE" value
// session_interactions already uses for a still-pending row, alongside two
// closure states of its own - a timer obligation's closure is either its own
// expiration (CONSUMED) or an explicit/terminal cancellation (CANCELLED),
// distinct from an interaction's CLOSED/TERMINATED split since a timer has
// no "answered" concept.
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
// obligation cancelled by terminal cleanup rather than an authored cancel -
// mirrors InteractionClosureReasonSessionTerminated exactly.
const TimerObligationClosureReasonSessionTerminated = "SESSION_TERMINATED"

var (
	// ErrGameNotFound is returned by CreateSession when the referenced Game
	// does not exist.
	ErrGameNotFound = errors.New("game not found")

	// ErrDefinitionDoesNotCompile is returned by CreateSession when the
	// Game's current playable Definition fails to compile and therefore
	// cannot be pinned to a new Session.
	ErrDefinitionDoesNotCompile = errors.New("game definition does not compile")

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

	// ErrInteractionNotFound is returned when an interaction UUID does not
	// resolve to an existing session_interactions row.
	ErrInteractionNotFound = errors.New("interaction not found")

	// ErrTimerObligationNotFound is returned when a timer obligation UUID
	// does not resolve to an existing session_timer_obligations row.
	ErrTimerObligationNotFound = errors.New("timer obligation not found")
)
