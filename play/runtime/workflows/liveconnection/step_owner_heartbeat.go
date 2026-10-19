package liveconnection

import (
	"context"
	"fmt"
	"time"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/play/runtime"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

//go:generate mockgen -package=liveconnection -destination=step_owner_heartbeat_mock_test.go . ownerHeartbeater

// ownerHeartbeater records that this instance still runs a Session, which
// keeps reconciliation from ending it as ownerless.
type ownerHeartbeater interface {
	RecordOwnerHeartbeat(ctx context.Context, sessionUUID string) (sessionpkg.OwnerHeartbeatResult, error)
}

var (
	heartbeatFailures = metrics.NewCounter(
		"play_runtime_owner_heartbeat_failures_total",
		"Owner heartbeats of running Sessions that could not be recorded; tried again a second later.",
	)
	sessionsEndedElsewhere = metrics.NewCounter(
		"play_runtime_sessions_ended_elsewhere_total",
		"Running Sessions a heartbeat found already ended durably, such as by reconciliation, and ended here.",
	)
)

// beatOwnerHeartbeats records the owner heartbeat of the game this controller
// runs: as soon as it runs here, then every heartbeatInterval, checked every
// limitsInterval, until stop is closed. A heartbeat that finds the Session
// ended ends it here too.
func (c *sessionController) beatOwnerHeartbeats(stop <-chan struct{}) {
	checks := time.NewTicker(c.deps.limitsInterval)
	defer checks.Stop()
	var lastBeat time.Time
	for {
		select {
		case <-stop:
			return
		case <-checks.C:
		}
		if !c.runsGame() || (!lastBeat.IsZero() && c.deps.now().Sub(lastBeat) < c.deps.heartbeatInterval) {
			continue
		}
		if c.beatOwnerHeartbeat() {
			lastBeat = c.deps.now()
		}
	}
}

// runsGame reports whether a game runs here, or is finishing.
func (c *sessionController) runsGame() bool {
	s := c.lock()
	defer c.unlock()
	return s.phase == phaseRunning || s.phase == phaseFinishing
}

// beatOwnerHeartbeat records one heartbeat; false means it failed and must be
// tried again.
func (c *sessionController) beatOwnerHeartbeat() bool {
	ctx := logging.Start(context.WithoutCancel(c.root))
	result, err := c.deps.heartbeats.RecordOwnerHeartbeat(ctx, c.sessionUUID)
	if err != nil {
		heartbeatFailures.Inc(ctx)
		logging.LogError(ctx, fmt.Errorf("recording owner heartbeat: %w", err))
		logging.LogFields(ctx, logging.Field("session_uuid", c.sessionUUID))
		logging.FinishRequestLog(ctx, c.deps.logger, "owner heartbeat failed")
		return false
	}
	if result.Outcome == sessionpkg.HeartbeatSessionNotRunning && result.EndCause != "" {
		c.endElsewhere(ctx, result)
	}
	return true
}

// endElsewhere ends here a game the database says already ended: the
// database is the truth, so its connections learn the durable status.
func (c *sessionController) endElsewhere(ctx context.Context, ended sessionpkg.OwnerHeartbeatResult) {
	s := c.lock()
	defer c.unlock()
	if s.phase != phaseRunning && s.phase != phaseFinishing {
		return
	}
	c.end(s, runtime.ConnectionEnd{Reason: runtime.EndSessionEnded, SessionStatus: ended.Status, EndCause: ended.EndCause})
	sessionsEndedElsewhere.Inc(ctx)
	logging.LogFields(ctx, logging.Field("session_uuid", c.sessionUUID), logging.Field("end_cause", ended.EndCause))
	logging.FinishRequestLog(ctx, c.deps.logger, "running session ended elsewhere")
}
