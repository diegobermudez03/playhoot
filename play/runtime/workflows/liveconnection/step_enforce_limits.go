package liveconnection

import (
	"context"
	"fmt"

	"github.com/diegobermudez03/playhoot/logging"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

//go:generate mockgen -package=liveconnection -destination=step_enforce_limits_mock_test.go . limitsLoader

// limitsLoader reads the platform-time deadlines of a Session.
type limitsLoader interface {
	LoadSessionLimits(ctx context.Context, sessionUUID string) (sessionpkg.SessionLimits, error)
}

// enforceLimits ends the Session once one of its platform-time limits is
// reached: its lobby expiring before it starts, its maximum lifetime
// whatever its activity, or a running game nobody played for the inactivity
// timeout. Failures are tried again at the next check.
func (c *sessionController) enforceLimits() {
	ctx := logging.Start(c.root)
	if err := c.loadLimits(ctx); err != nil {
		logging.LogError(ctx, err)
		logging.FinishRequestLog(ctx, c.deps.logger, "session limits check failed")
		return
	}
	cause, reached := c.reachedLimit()
	if !reached {
		return
	}
	if _, err := c.endSession(ctx, sessionpkg.SessionEnd{Cause: cause}, nil); err != nil {
		logging.LogError(ctx, err)
	}
	logging.FinishRequestLog(ctx, c.deps.logger, "session limit reached")
}

// loadLimits reads the Session's deadlines the first time they are needed.
func (c *sessionController) loadLimits(ctx context.Context) error {
	if c.limitsLoaded {
		return nil
	}
	limits, err := c.deps.limits.LoadSessionLimits(ctx, c.sessionUUID)
	if err != nil {
		return fmt.Errorf("loading session limits: %w", err)
	}
	c.limits, c.limitsLoaded = limits, true
	return nil
}

// reachedLimit returns the cause of the first limit the Session reached; the
// lifetime counts before any other.
func (c *sessionController) reachedLimit() (sessionpkg.EndCause, bool) {
	s := c.lock()
	defer c.unlock()
	now := s.now()
	switch s.phase {
	case phaseLobby, phaseRunning:
		if !now.Before(c.limits.ExpiresAt) {
			return sessionpkg.EndMaxLifetime, true
		}
	default:
		// Starting, finishing or ended: whatever it does, it ends by itself.
		return "", false
	}
	if s.phase == phaseLobby && !now.Before(c.limits.LobbyExpiresAt) {
		return sessionpkg.EndLobbyExpired, true
	}
	if s.phase == phaseRunning && now.Sub(s.lastActivity) >= c.deps.inactivityTimeout {
		return sessionpkg.EndInactivityTimeout, true
	}
	return "", false
}

// recordActivity notes that someone just played the running game. The caller
// holds the lock.
func (s *liveState) recordActivity() {
	s.lastActivity = s.now()
}
