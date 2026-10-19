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

// What an attempt to recover a running Session came to, as metric label
// values.
const (
	// recoveryRecovered means the game runs here again from its checkpoint.
	recoveryRecovered = "recovered"
	// recoveryNotNeeded means the game already runs here, or the database no
	// longer has it RUNNING.
	recoveryNotNeeded = "not_needed"
	// recoveryEnded means the Session ended instead: here, or durably.
	recoveryEnded = "ended"
	recoveryError = "error"
)

var (
	recoveryOutcomeLabel = metrics.NewLabel("outcome", recoveryRecovered, recoveryNotNeeded, recoveryEnded, recoveryError)

	sessionRecoveries = metrics.NewCounter(
		"play_runtime_session_recoveries_total",
		"Attempts to recover a RUNNING Session this instance did not run, made by the first connection to it, by outcome.",
		recoveryOutcomeLabel,
	)
	sessionRecoveryDuration = metrics.NewHistogram(
		"play_runtime_session_recovery_duration_seconds",
		"How long recovering a RUNNING Session took, by outcome.",
		metrics.LatencyBuckets(),
		recoveryOutcomeLabel,
	)
)

// recoverIfNeeded makes sure a Session the database has RUNNING runs here,
// recovering it from its latest checkpoint on the main goroutine when it does
// not; connections arriving meanwhile wait for the same recovery. It returns
// recoveryRecovered or recoveryNotNeeded, or recoveryEnded when the Session
// ended instead.
func (c *sessionController) recoverIfNeeded(ctx context.Context) (string, error) {
	if c.isRunning() {
		return recoveryNotNeeded, nil
	}
	var outcome string
	var err error
	if c.do(ctx, func() { outcome, err = c.recoverSession(ctx) }) {
		return outcome, err
	}
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	select {
	case <-c.ended:
		return recoveryEnded, nil
	default:
		return "", errStopped
	}
}

// recoverSession recovers the Session, as recoverIfNeeded describes.
func (c *sessionController) recoverSession(ctx context.Context) (string, error) {
	began := time.Now()
	outcome, err := c.runRecovery(ctx)
	label := outcome
	if err != nil {
		label = recoveryError
	}
	sessionRecoveries.Inc(ctx, recoveryOutcomeLabel.Value(label))
	sessionRecoveryDuration.ObserveDuration(ctx, time.Since(began), recoveryOutcomeLabel.Value(label))
	logging.LogFields(ctx, logging.Field("recovery_outcome", label))
	return outcome, err
}

// runRecovery freezes the lobby as a Start does, then adopts the game the
// database runs, unless it outlived its lifetime, which ends it instead. Any
// other outcome returns the controller to its lobby.
func (c *sessionController) runRecovery(ctx context.Context) (string, error) {
	_, phaseOutcome, frozen := c.beginStart()
	if !frozen {
		if phaseOutcome == runtime.StartNotInLobby {
			return recoveryEnded, nil
		}
		return recoveryNotNeeded, nil
	}
	outcome, err := c.recoverFrozenLobby(ctx)
	if err != nil || outcome != recoveryRecovered {
		c.cancelStart()
	}
	return outcome, err
}

func (c *sessionController) recoverFrozenLobby(ctx context.Context) (string, error) {
	if err := c.loadLimits(ctx); err != nil {
		return "", err
	}
	if !c.deps.now().Before(c.limits.ExpiresAt) {
		// Not worth recovering: it is over whatever happens.
		result, err := c.endSession(ctx, sessionpkg.SessionEnd{Cause: sessionpkg.EndMaxLifetime}, nil)
		if err != nil {
			return "", err
		}
		if result.EndCause == "" {
			return recoveryNotNeeded, nil
		}
		return recoveryEnded, nil
	}
	adopted, err := c.adoptRunningSession(ctx)
	if err != nil {
		return "", fmt.Errorf("recovering running session: %w", err)
	}
	switch adopted {
	case runtime.StartAlreadyStarted:
		return recoveryRecovered, nil
	case runtime.StartNotInLobby:
		return recoveryNotNeeded, nil
	default:
		// No sandbox capacity: the next connection tries again.
		return "", fmt.Errorf("recovering running session: %s", adopted)
	}
}
