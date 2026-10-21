package liveconnection

import (
	"context"
	"fmt"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/play/runtime"
)

var (
	frontendSigningOutcomeLabel = metrics.NewLabel("outcome", "signed", "failed")

	frontendsSigned = metrics.NewCounter(
		"play_runtime_frontend_access_signed_total",
		"Frontend access signed again for a connection joining a running game, by outcome; a failure falls back to the access signed at the start.",
		frontendSigningOutcomeLabel,
	)
)

// signedStart is the running game's start with frontend access signed now,
// since the access signed at the start expires. ok is false when the game is
// not running or joins rejects the connection. A failed signing keeps the
// access of the start, which a client with the frontend cached never needs.
func (c *sessionController) signedStart(ctx context.Context, joins func(s *liveState) bool) (start runtime.SessionStart, ok bool) {
	s := c.lock()
	start, gameVersionUUID, wanted := s.start, s.game.GameVersionUUID, s.phase == phaseRunning && joins(s)
	c.unlock()
	if !wanted {
		return runtime.SessionStart{}, false
	}
	access, err := c.deps.frontends.FrontendAccess(ctx, gameVersionUUID)
	if err != nil {
		logging.LogError(ctx, fmt.Errorf("signing frontend access for a joining connection: %w", err))
		frontendsSigned.Inc(ctx, frontendSigningOutcomeLabel.Value("failed"))
		return start, true
	}
	frontendsSigned.Inc(ctx, frontendSigningOutcomeLabel.Value("signed"))
	start.Frontend = frontendOf(access)
	return start, true
}
