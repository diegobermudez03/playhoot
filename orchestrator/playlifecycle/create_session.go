package playlifecycle

import (
	"context"
	"errors"
	"fmt"

	"github.com/diegobermudez03/playhoot/logging"
	sessionpkg "github.com/diegobermudez03/playhoot/play"
)

//go:generate mockgen -package=playlifecycle -destination=create_session_mock_test.go . gamePlayabilityChecker,sessionCreator

// gamePlayabilityChecker is this operation's narrow dependency on Game
// Management: whether a Game is currently playable by a given user. It only
// answers yes or no; a Game that does not exist and one that is not
// available to the user are both simply not playable.
type gamePlayabilityChecker interface {
	IsPlayable(ctx context.Context, gameUUID string, userUUID string) (bool, error)
}

// sessionCreator is this operation's narrow dependency on the Session
// domain: the durable creation of a Session in LOBBY for a Game that is
// already known to be playable. The Session domain owns idempotency, so it
// receives the key and reports a reused key with a different request as
// sessionpkg.ErrIdempotencyConflict.
type sessionCreator interface {
	CreateSession(ctx context.Context, userUUID string, gameUUID string, idempotencyKey string) (sessionpkg.CreateSessionResult, error)
}

// CreateSession creates a Session for userUUID from gameUUID. It first asks
// Game Management whether the Game is playable for that user; if it is not,
// no Session is created and the result reports the Game as unavailable.
// Otherwise the Session domain creates the Session and its result is
// returned as is.
//
// userUUID is only forwarded: this package never interprets it or talks to
// Identity.
func (m *Manager) CreateSession(ctx context.Context, userUUID string, gameUUID string, idempotencyKey string) (sessionpkg.CreateSessionResult, error) {
	step := logging.Step(ctx, "orchestrator.create_session")
	defer step.Close()

	playable, err := m.games.IsPlayable(ctx, gameUUID, userUUID)
	if err != nil {
		return sessionpkg.CreateSessionResult{}, fmt.Errorf("checking game playability: %s", err)
	}
	logging.LogFields(ctx, logging.Field("game_playable", playable))
	if !playable {
		return sessionpkg.CreateSessionResult{Outcome: sessionpkg.CreateSessionGameUnavailable}, nil
	}

	created, err := m.sessions.CreateSession(ctx, userUUID, gameUUID, idempotencyKey)
	if err != nil {
		if errors.Is(err, sessionpkg.ErrIdempotencyConflict) {
			return sessionpkg.CreateSessionResult{}, err
		}
		return sessionpkg.CreateSessionResult{}, fmt.Errorf("creating session: %s", err)
	}
	return created, nil
}
