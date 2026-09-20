// Package sessionruntime is the concrete implementation of play's
// SessionRuntime port: it calls the existing, unmodified
// sessionlifecycle.Manager and translates between play's primitive types
// and Manager's semantic types in both directions - including translating
// a committed call's resulting interactions into the play.Event values
// play's Coordinator fans out (exactly the interactions a RuntimeTurn
// opened/closed, nothing else the engine may have produced).
//
// This package is expected to import `game` (sessionlifecycle, engine,
// engineservice) - that is normal and required, since its whole job is
// bridging to game's real types. It is deliberately not part of play's own
// package/exported surface, so play itself stays free of any `game`
// import.
package sessionruntime

import (
	"context"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine/engineservice"
	"github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/play"
	"gorm.io/gorm"
)

// SessionRuntime implements play.SessionRuntime against manager. It also
// holds its own read-only db handle, used only to read back already-durably-
// committed session_interactions/sessions rows once manager's own call has
// returned - never to share a transaction or mutate anything itself. This
// is ordinary game-side data access (the same tables Session Runtime
// already owns), not a violation of play's own "no *gorm.DB" rule: play
// itself never sees db, only this implementation does.
type SessionRuntime struct {
	manager *sessionlifecycle.Manager
	db      *gorm.DB
}

// New constructs a SessionRuntime. manager is called exactly as any other
// caller of sessionlifecycle.Manager's public API would; db reads back
// session_interactions/sessions after a committed call, to build the
// Events play.Coordinator fans out.
func New(manager *sessionlifecycle.Manager, db *gorm.DB) *SessionRuntime {
	return &SessionRuntime{manager: manager, db: db}
}

var _ play.SessionRuntime = (*SessionRuntime)(nil)

func (sr *SessionRuntime) Create(ctx context.Context, gameUUID, hostUserUUID, idempotencyKey string) (play.CreatedSession, error) {
	defer logging.Step(ctx, "SessionRuntime.Create").Close()
	result, err := sr.manager.Create(ctx, sessionlifecycle.GameUUID(gameUUID), sessionlifecycle.UserUUID(hostUserUUID), sessionlifecycle.IdempotencyKey(idempotencyKey))
	if err != nil {
		return play.CreatedSession{}, translateError(err)
	}
	return play.CreatedSession{
		SessionUUID:    play.SessionUUID(result.SessionUUID),
		JoinCode:       uint(result.JoinCode),
		LobbyExpiresAt: result.LobbyExpiresAt,
	}, nil
}

func (sr *SessionRuntime) Join(ctx context.Context, joinCode uint, userUUID, displayName, idempotencyKey string) (play.JoinResult, error) {
	defer logging.Step(ctx, "SessionRuntime.Join").Close()
	result, err := sr.manager.Join(ctx, sessionlifecycle.JoinCode(joinCode), sessionlifecycle.UserUUID(userUUID), sessionlifecycle.DisplayName(displayName), sessionlifecycle.IdempotencyKey(idempotencyKey))
	if err != nil {
		return play.JoinResult{}, translateError(err)
	}
	return play.JoinResult{
		Outcome:     play.JoinOutcome(result.Outcome),
		SessionUUID: play.SessionUUID(result.SessionUUID),
		DisplayName: string(result.DisplayName),
	}, nil
}

// Start calls manager.Start and, only when it reports STARTED, reads back
// the committed Turn's interactions to build the Events play.Coordinator
// fans out. Manager itself never returns the values a RuntimeTurn produced
// to its caller, so this read-after-commit is the only way to learn them
// without changing Manager's own method signatures or internal behavior.
func (sr *SessionRuntime) Start(ctx context.Context, sessionUUID, userUUID, idempotencyKey string) (play.StartResult, error) {
	defer logging.Step(ctx, "SessionRuntime.Start").Close()
	result, err := sr.manager.Start(ctx, sessionlifecycle.SessionUUID(sessionUUID), sessionlifecycle.UserUUID(userUUID), sessionlifecycle.IdempotencyKey(idempotencyKey))
	if err != nil {
		return play.StartResult{}, translateError(err)
	}
	out := play.StartResult{Outcome: play.StartOutcome(result.Outcome)}
	if result.Outcome == sessionlifecycle.StartOutcomeStarted {
		events, err := sr.eventsForCommittedTurn(ctx, string(result.SessionUUID))
		if err != nil {
			return play.StartResult{}, err
		}
		out.Events = events
	}
	return out, nil
}

// AnswerInteraction decodes answer as the same wire format
// session_interactions.response_payload/interaction_payload already
// durably persist, calls manager.AnswerInteraction, and, only when it
// reports ANSWERED, reads back the committed Turn's interactions the same
// way Start does.
func (sr *SessionRuntime) AnswerInteraction(ctx context.Context, interactionUUID, userUUID string, answer []byte) (play.AnswerInteractionResult, error) {
	defer logging.Step(ctx, "SessionRuntime.AnswerInteraction").Close()
	value, err := engineservice.DecodeValue(answer)
	if err != nil {
		return play.AnswerInteractionResult{}, err
	}

	result, err := sr.manager.AnswerInteraction(ctx, sessionlifecycle.InteractionUUID(interactionUUID), sessionlifecycle.UserUUID(userUUID), value)
	if err != nil {
		return play.AnswerInteractionResult{}, translateError(err)
	}
	out := play.AnswerInteractionResult{Outcome: play.AnswerOutcome(result.Outcome)}
	if result.Outcome == sessionlifecycle.AnswerInteractionOutcomeAnswered {
		events, err := sr.eventsForCommittedTurn(ctx, string(result.SessionUUID))
		if err != nil {
			return play.AnswerInteractionResult{}, err
		}
		out.Events = events
	}
	return out, nil
}
