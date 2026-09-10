// Package session is the Session Runtime capability's public domain
// vocabulary: the lobby lifecycle phase/presence/terminal-reason values and
// the sentinel errors shared by its CreateSession/JoinSession/LeaveSession
// use cases (game/session/usecases/...).
package session

import "errors"

// Session lifecycle phase values. V1 (this package) only ever observes
// PhaseLobby and PhaseTerminal; RUNNING is a later slice's concern.
const (
	PhaseLobby    = "LOBBY"
	PhaseTerminal = "TERMINAL"
)

// SessionActor semantic presence values (GAME-ADR-0015). A normal Join
// begins CONNECTED; DISCONNECTED is not produced by anything in this work.
const (
	PresenceConnected    = "CONNECTED"
	PresenceDisconnected = "DISCONNECTED"
)

// Internal terminal-reason values materialized by this work.
const (
	TerminalReasonLobbyExpired = "LOBBY_EXPIRED"
)

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
	// outcome. Under this work's single-transaction claim-then-commit
	// design, callers should not normally observe this.
	ErrIdempotencyInFlight = errors.New("idempotency key claim is still in flight")

	// ErrJoinCodeInvalid is returned when a JoinCode does not resolve to an
	// admissible Session (unknown or already revoked).
	ErrJoinCodeInvalid = errors.New("join code is invalid or no longer active")

	// ErrLobbyExpired is returned when an operation discovers and
	// materializes an already-expired lobby.
	ErrLobbyExpired = errors.New("lobby has expired")

	// ErrLobbyFull is returned when Join would exceed the pinned
	// Definition's players.max.
	ErrLobbyFull = errors.New("lobby is full")

	// ErrSessionNotFound is returned when a Session UUID does not resolve to
	// an existing Session.
	ErrSessionNotFound = errors.New("session not found")

	// ErrNotInLobbyPhase is returned by Leave when the Session is no longer
	// in LOBBY phase.
	ErrNotInLobbyPhase = errors.New("session is not in lobby phase")

	// ErrActorNotFound is returned by Leave when the calling user never
	// joined the Session.
	ErrActorNotFound = errors.New("session actor not found")
)
