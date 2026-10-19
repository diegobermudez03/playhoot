package liveconnection

import (
	"context"
	"fmt"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/play/runtime"
)

//go:generate mockgen -package=liveconnection -destination=step_end_abandoned_session_mock_test.go . sessionCanceller

// sessionCanceller ends a running Session nobody can play anymore.
type sessionCanceller interface {
	CancelAbandonedSession(ctx context.Context, sessionUUID string) error
}

// endIfAbandoned ends the Session, durably first, once none of its
// participants can play and the game was told about each of them. A failed
// durable write is logged and tried again at the next boundary.
func (c *sessionController) endIfAbandoned(ctx context.Context) {
	if !c.isAbandoned() {
		return
	}
	if err := c.deps.cancels.CancelAbandonedSession(ctx, c.sessionUUID); err != nil {
		logging.LogError(ctx, fmt.Errorf("cancelling abandoned session: %w", err))
		return
	}
	logging.LogFields(ctx, logging.Field("session_abandoned", true))
	s := c.lock()
	defer c.unlock()
	if s.phase == phaseRunning {
		c.end(s, runtime.EndSessionEnded)
	}
}

// isAbandoned reports whether the game runs with every participant
// disconnected and no disconnection still waiting to reach the game.
func (c *sessionController) isAbandoned() bool {
	s := c.lock()
	defer c.unlock()
	if s.phase != phaseRunning {
		return false
	}
	for _, participant := range s.participants {
		if !s.presence[participant.ParticipantID].disconnected {
			return false
		}
	}
	for _, operation := range s.timeline {
		if operation.Disconnected {
			return false
		}
	}
	// Placeholder: a game with playerRejoined() will keep waiting for its participants.
	return true
}
