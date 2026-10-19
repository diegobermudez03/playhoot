package liveconnection

import (
	"context"
	"fmt"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
	"github.com/diegobermudez03/playhoot/play/runtime"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

//go:generate mockgen -package=liveconnection -destination=step_end_session_mock_test.go . sessionEnder

// sessionEnder moves a Session to a terminal status, the only way a live
// Session ends durably.
type sessionEnder interface {
	EndSession(ctx context.Context, sessionUUID string, end sessionpkg.SessionEnd) (sessionpkg.EndSessionResult, error)
}

// endSession ends the Session for end, durably first, then live: every
// connection ends told the terminal status and cause the database holds. A
// Session the cause does not apply to, which has not ended either, stays as
// it is. onEnded, when set, runs under the lock just before the connections
// end, only when end is what ended it. Runs on the main goroutine.
func (c *sessionController) endSession(ctx context.Context, end sessionpkg.SessionEnd, onEnded func(s *liveState)) (sessionpkg.EndSessionResult, error) {
	result, err := c.deps.ends.EndSession(ctx, c.sessionUUID, end)
	if err != nil {
		return sessionpkg.EndSessionResult{}, fmt.Errorf("ending session: %w", err)
	}
	logging.LogFields(ctx,
		logging.Field("session_uuid", c.sessionUUID),
		logging.Field("end_cause", string(end.Cause)),
		logging.Field("end_session_outcome", string(result.Outcome)),
	)
	if result.EndCause == "" {
		// Still live: the cause did not apply to its status.
		return result, nil
	}
	s := c.lock()
	defer c.unlock()
	if s.phase == phaseEnded {
		return result, nil
	}
	if onEnded != nil && result.Outcome == sessionpkg.SessionEndedNow {
		onEnded(s)
	}
	c.end(s, runtime.ConnectionEnd{Reason: runtime.EndSessionEnded, SessionStatus: result.Status, EndCause: result.EndCause})
	return result, nil
}

// endIfAbandoned ends the Session once none of its participants can play and
// the game was told about each of them. A failed durable write is logged and
// tried again at the next boundary.
func (c *sessionController) endIfAbandoned(ctx context.Context) {
	if !c.isAbandoned() {
		return
	}
	if _, err := c.endSession(ctx, sessionpkg.SessionEnd{Cause: sessionpkg.EndAllPlayersLeft}, nil); err != nil {
		logging.LogError(ctx, err)
	}
}

// isAbandoned reports whether the game runs with every participant
// disconnected and no disconnection or rejoin still waiting for the game.
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
		if operation.Disconnected || operation.Rejoin != nil {
			return false
		}
	}
	return true
}

// abort ends the running game for cause: durably first, then live. A failed
// durable abort is alerted and the game still ends here, since it cannot go
// on either way; its connections then learn no terminal status.
func (c *sessionController) abort(ctx context.Context, cause sessionpkg.EndCause, failure error) {
	logging.LogError(ctx, failure)
	logging.LogFields(ctx, logging.Field("abort_cause", string(cause)))
	if _, err := c.endSession(ctx, sessionpkg.SessionEnd{Cause: cause}, nil); err != nil {
		logging.LogError(ctx, err)
		monitoring.Alert(ctx, "running session could not be aborted durably")
	}
	s := c.lock()
	defer c.unlock()
	if s.phase == phaseRunning {
		c.end(s, runtime.ConnectionEnd{Reason: runtime.EndSessionEnded})
	}
}
