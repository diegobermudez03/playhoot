// Package play is the Live Session Coordinator: the connection-binding,
// delivery/fan-out boundary that lives outside Session Runtime, owning no
// authoritative Session truth of its own.
//
// This package's own exported API - every type below, and Coordinator's own
// methods - depends only on primitives and play-owned types, never on a
// `game` package (sessionlifecycle, engine, etc.). The SessionRuntime port
// declared here is implemented by a separate package that is expected to
// import `game` to call sessionlifecycle.Manager, so play and game can
// later become separate deployment units by swapping only that
// implementation for a network client, with no change to play itself.
package play

import (
	"context"
	"errors"
	"time"
)

// Sentinel errors a SessionRuntime implementation translates its
// underlying `game`-package sentinel errors into, so a caller of Coordinator
// (api) can branch on a specific business-outcome error via errors.Is
// without itself importing any `game` package - play's own exported API
// must never reference a `game` type, and that includes the error values
// it can return.
var (
	ErrGameNotFound             = errors.New("game not found")
	ErrDefinitionDoesNotCompile = errors.New("game definition does not compile")
	ErrJoinCodeInvalid          = errors.New("join code is invalid or no longer active")
	ErrSessionNotFound          = errors.New("session not found")
	ErrInteractionNotFound      = errors.New("interaction not found")
	ErrIdempotencyKeyRequired   = errors.New("idempotency key is required")
	ErrIdempotencyConflict      = errors.New("idempotency key reused with a conflicting request")
	ErrIdempotencyInFlight      = errors.New("idempotency key claim is still in flight")
)

//go:generate mockgen -package=play -destination=sessionruntime_mock_test.go . SessionRuntime

// SessionUUID, UserUUID, and InteractionUUID are play's own primitive
// identity types for the values a connected client and a bound connection
// are keyed by. UserUUID is supplied directly by the client at connection
// time and trusted as-is; no independent credential verification happens
// anywhere in this path.
type (
	SessionUUID     string
	UserUUID        string
	InteractionUUID string
)

// CreatedSession is Create's result, mirroring
// sessionlifecycle.CreatedSession with play-owned types.
type CreatedSession struct {
	SessionUUID    SessionUUID
	JoinCode       uint
	LobbyExpiresAt time.Time
}

// JoinOutcome mirrors sessionlifecycle.JoinOutcome with play-owned values.
type JoinOutcome string

const (
	JoinOutcomeJoined        JoinOutcome = "JOINED"
	JoinOutcomeLobbyExpired  JoinOutcome = "LOBBY_EXPIRED"
	JoinOutcomeLobbyFull     JoinOutcome = "LOBBY_FULL"
	JoinOutcomeAlreadyJoined JoinOutcome = "ALREADY_JOINED"
)

// JoinResult is Join's result, mirroring sessionlifecycle.JoinResult with
// play-owned types.
type JoinResult struct {
	Outcome     JoinOutcome
	SessionUUID SessionUUID
	DisplayName string
}

// StartOutcome mirrors sessionlifecycle.StartOutcome with play-owned values.
type StartOutcome string

const (
	StartOutcomeStarted           StartOutcome = "STARTED"
	StartOutcomeLobbyExpired      StartOutcome = "LOBBY_EXPIRED"
	StartOutcomeNotHost           StartOutcome = "NOT_HOST"
	StartOutcomeNotEnoughPlayers  StartOutcome = "NOT_ENOUGH_PLAYERS"
	StartOutcomeRuntimeInitFailed StartOutcome = "RUNTIME_INIT_FAILED"
)

// StartResult is Start's result. Events is only ever non-empty when Outcome
// is StartOutcomeStarted - the interactions opened/closed by the resulting
// RuntimeTurn, translated into the values Coordinator fans out.
type StartResult struct {
	Outcome StartOutcome
	Events  []Event
}

// AnswerOutcome mirrors sessionlifecycle.AnswerInteractionOutcome with
// play-owned values.
type AnswerOutcome string

const (
	AnswerOutcomeAnswered               AnswerOutcome = "ANSWERED"
	AnswerOutcomeRejected               AnswerOutcome = "REJECTED"
	AnswerOutcomeConflict               AnswerOutcome = "CONFLICT"
	AnswerOutcomeRuntimeExecutionFailed AnswerOutcome = "RUNTIME_EXECUTION_FAILED"
)

// AnswerInteractionResult is AnswerInteraction's result. Events is only
// ever non-empty when Outcome is AnswerOutcomeAnswered, the same rule
// StartResult.Events follows.
type AnswerInteractionResult struct {
	Outcome AnswerOutcome
	Events  []Event
}

// EventKind identifies which shape an Event carries.
type EventKind string

const (
	// EventKindInteractionOpened corresponds to a committed RuntimeTurn's
	// engine.OpenQuestionOutput, translated for its recipient.
	EventKindInteractionOpened EventKind = "INTERACTION_OPENED"
	// EventKindInteractionClosed corresponds to a committed RuntimeTurn's
	// engine.CloseQuestionOutput, translated for its recipient. It is also
	// delivered, harmlessly, to a recipient who closed their own
	// interaction by answering it (that closure produces no
	// CloseQuestionOutput at all, so this Event is not distinguishable
	// from - and is exactly as best-effort/droppable as - an explicit
	// engine-driven close; the recipient already learned the outcome
	// synchronously from AnswerInteraction's own result).
	EventKindInteractionClosed EventKind = "INTERACTION_CLOSED"
)

// Event is one fan-out message Coordinator delivers to a single recipient's
// bound connection, translated by the SessionRuntime implementation from a
// committed RuntimeTurn's Outputs. Question/Arguments are only meaningful
// when Kind is EventKindInteractionOpened. Arguments is a generic
// JSON-shaped value (never an engine.Value) so play stays free of any
// `game` type.
type Event struct {
	Recipient     UserUUID
	Kind          EventKind
	InteractionID InteractionUUID
	Question      string
	Arguments     map[string]any
}

// SessionRuntime is the port Coordinator depends on to actually mutate
// Session Runtime state: the dependency-inversion boundary that lets play
// stay free of any `game` package import. Its concrete implementation is
// expected to import `game` and call sessionlifecycle.Manager.
//
// answer is an opaque wire-shaped payload (the client's submitted response,
// exactly as received); decoding it into a typed value meaningful to the
// underlying game is the implementation's own responsibility.
type SessionRuntime interface {
	Create(ctx context.Context, gameUUID, hostUserUUID, idempotencyKey string) (CreatedSession, error)
	Join(ctx context.Context, joinCode uint, userUUID, displayName, idempotencyKey string) (JoinResult, error)
	Start(ctx context.Context, sessionUUID, userUUID, idempotencyKey string) (StartResult, error)
	AnswerInteraction(ctx context.Context, interactionUUID, userUUID string, answer []byte) (AnswerInteractionResult, error)
}
