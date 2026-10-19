package liveconnection

import (
	"context"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
)

// Why a running game's context had to be recreated from the mirror, as
// metric label values.
const (
	recreatedAfterEviction = "evicted"
	recreatedAfterFailure  = "failed"
)

var recreationReasonLabel = metrics.NewLabel("reason", recreatedAfterEviction, recreatedAfterFailure)

var (
	contextsEvicted = metrics.NewCounter(
		"play_runtime_sandbox_contexts_evicted_total",
		"Sandbox contexts of running games without update() discarded for being idle.",
	)
	contextsRecreated = metrics.NewCounter(
		"play_runtime_sandbox_contexts_recreated_total",
		"Sandbox contexts of running games recreated from the mirror, by why the previous one was gone.",
		recreationReasonLabel,
	)
)

// evictIdleContext discards the context of a running game without update()
// that went unused for the idle timeout; the next execution restores one
// from the mirror. A game with update() executes every tick, so it stays hot.
func (c *sessionController) evictIdleContext() {
	if c.contextID == "" || c.finishing != nil {
		return
	}
	s := c.lock()
	keepHot := s.phase != phaseRunning || s.manifest.Update
	c.unlock()
	if keepHot || c.deps.now().Sub(c.lastSandboxUse) < c.deps.contextIdleTimeout {
		return
	}
	ctx := logging.Start(c.root)
	c.discardContext(ctx)
	c.evicted = true
	contextsEvicted.Inc(ctx)
}

// recordRecreation counts a context about to be restored from the mirror
// because the game has none.
func (c *sessionController) recordRecreation(ctx context.Context) {
	reason := recreatedAfterFailure
	if c.evicted {
		reason = recreatedAfterEviction
	}
	c.evicted = false
	contextsRecreated.Inc(ctx, recreationReasonLabel.Value(reason))
}
