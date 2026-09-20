// Package session is the Session Runtime capability's public domain
// vocabulary: the lobby lifecycle phase/presence/terminal-reason values and
// the sentinel errors shared by its CreateSession/JoinSession/LeaveSession
// lifecycle operations (game/session/workflows/sessionlifecycle/...).
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
)
