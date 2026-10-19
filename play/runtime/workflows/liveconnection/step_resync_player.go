package liveconnection

import (
	"context"

	"github.com/diegobermudez03/playhoot/monitoring/metrics"
)

var resyncsServed = metrics.NewCounter(
	"play_runtime_resyncs_total",
	"FULL views sent because a player asked for a resync, usually after missing a view revision.",
)

// resync sends player their current view whole, from the projection cache,
// with their acknowledgement and a fresh baseline for the patches that
// follow. Before the game runs, or from a replaced connection, it does
// nothing.
func (c *sessionController) resync(ctx context.Context, player *connection) {
	s := c.lock()
	defer c.unlock()
	if s.phase != phaseRunning || player.participantID == "" || s.players[player.userUUID] != player {
		return
	}
	s.sendFull(ctx, player, player.participantID)
	resyncsServed.Inc(ctx)
}
