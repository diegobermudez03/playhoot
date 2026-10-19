package liveconnection

import (
	"context"
	"fmt"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
	"github.com/diegobermudez03/playhoot/play/runtime/replay"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

// finishedGame is a game its script finished, committed to the mirror, whose
// ending is not durable yet: the boundary that finished it, what it produced,
// and its final capture for replay.
type finishedGame struct {
	work     boundary
	executed execution
	capture  replay.Capture
	// recorded means replay already stored the final capture.
	recorded bool
	// failures is how many attempts to make the ending durable failed.
	failures int
}

// beginFinishing completes a game its script finished: nothing more is
// played, its context is no longer needed, and its ending is made durable.
func (c *sessionController) beginFinishing(ctx context.Context, work boundary, executed execution, capture replay.Capture) {
	c.discardContext(ctx)
	c.finishing = &finishedGame{work: work, executed: executed, capture: capture}
	c.finishGame()
}

// finishGame makes a finished game's ending durable, final replay state
// first, then FINISHED; only then do connected players get the final state,
// and every connection ends. A failure is retried at the next tick.
func (c *sessionController) finishGame() {
	ctx := logging.Start(c.root)
	finishing := c.finishing
	err := c.recordFinish(ctx, finishing)
	if err == nil {
		var result sessionpkg.EndSessionResult
		result, err = c.endSession(ctx, sessionpkg.SessionEnd{Cause: sessionpkg.EndGameFinished}, func(s *liveState) {
			s.acknowledge(ctx, finishing.work, finishing.executed)
		})
		if err == nil && result.EndCause == "" {
			err = fmt.Errorf("finished game's session is %s, not running", result.Status)
		}
	}
	if err != nil {
		logging.LogError(ctx, err)
		finishing.failures++
		if finishing.failures == maxInfraFailures {
			monitoring.Alert(ctx, "finished game could not end durably")
		}
		logging.FinishRequestLog(ctx, c.deps.logger, "game finishing failed")
		return
	}
	c.finishing = nil
	logging.FinishRequestLog(ctx, c.deps.logger, "game finished")
}

// recordFinish stores the game's final capture in replay, once.
func (c *sessionController) recordFinish(ctx context.Context, finishing *finishedGame) error {
	if finishing.recorded {
		return nil
	}
	if err := c.deps.replay.Finish(ctx, finishing.capture); err != nil {
		return fmt.Errorf("recording the final replay state: %w", err)
	}
	finishing.recorded = true
	return nil
}
