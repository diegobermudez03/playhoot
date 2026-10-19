package liveconnection

import (
	"context"

	"github.com/diegobermudez03/playhoot/play/runtime"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

// requestCancel cancels the Session on the main goroutine, as its creator
// userUUID asked, and waits for the outcome.
func (c *sessionController) requestCancel(ctx context.Context, userUUID string) (runtime.CancelSessionResult, error) {
	var result runtime.CancelSessionResult
	var err error
	if c.do(ctx, func() { result, err = c.cancelSession(ctx, userUUID) }) {
		return result, err
	}
	if ctx.Err() != nil {
		return runtime.CancelSessionResult{}, ctx.Err()
	}
	select {
	case <-c.ended:
		return runtime.CancelSessionResult{Outcome: runtime.SessionAlreadyEnded}, nil
	default:
		return runtime.CancelSessionResult{}, errStopped
	}
}

// cancelSession ends the Session, in its lobby or running, as cancelled by
// userUUID, without calling the game. A game its script already finished is
// already over.
func (c *sessionController) cancelSession(ctx context.Context, userUUID string) (runtime.CancelSessionResult, error) {
	if !c.isLive() {
		return runtime.CancelSessionResult{Outcome: runtime.SessionAlreadyEnded}, nil
	}
	result, err := c.endSession(ctx, sessionpkg.SessionEnd{Cause: sessionpkg.EndHostCancelled, ChangedBy: userUUID}, nil)
	if err != nil {
		return runtime.CancelSessionResult{}, err
	}
	if result.Outcome != sessionpkg.SessionEndedNow {
		return runtime.CancelSessionResult{Outcome: runtime.SessionAlreadyEnded}, nil
	}
	return runtime.CancelSessionResult{Outcome: runtime.SessionCancelled}, nil
}

// isLive reports whether the Session is in its lobby or running its game.
func (c *sessionController) isLive() bool {
	s := c.lock()
	defer c.unlock()
	return s.phase == phaseLobby || s.phase == phaseRunning
}
