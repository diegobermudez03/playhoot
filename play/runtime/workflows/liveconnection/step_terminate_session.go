package liveconnection

import (
	"context"
	"fmt"

	"github.com/diegobermudez03/playhoot/play/runtime"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

// TerminateSession ends sessionUUID, in its lobby or running, because the
// platform decided so (PLATFORM_TERMINATED, such as a usage limit), without
// calling the game. A Session this instance runs ends live too; any other is
// only ended durably. Nothing calls it yet.
func (r *ControllerResolver) TerminateSession(ctx context.Context, sessionUUID string) (runtime.TerminateSessionResult, error) {
	end := sessionpkg.SessionEnd{Cause: sessionpkg.EndPlatformTerminated}
	r.mu.Lock()
	controller := r.controllers[sessionUUID]
	r.mu.Unlock()
	if controller != nil {
		var result sessionpkg.EndSessionResult
		var err error
		if controller.do(ctx, func() { result, err = controller.endSession(ctx, end, nil) }) {
			return terminated(result, err)
		}
		if ctx.Err() != nil {
			return runtime.TerminateSessionResult{}, ctx.Err()
		}
		// The controller stopped meanwhile: only the database is left.
	}
	result, err := r.deps.ends.EndSession(ctx, sessionUUID, end)
	if err != nil {
		err = fmt.Errorf("ending session: %w", err)
	}
	return terminated(result, err)
}

func terminated(result sessionpkg.EndSessionResult, err error) (runtime.TerminateSessionResult, error) {
	if err != nil {
		return runtime.TerminateSessionResult{}, err
	}
	if result.Outcome != sessionpkg.SessionEndedNow {
		return runtime.TerminateSessionResult{Outcome: runtime.TerminateAlreadyEnded}, nil
	}
	return runtime.TerminateSessionResult{Outcome: runtime.SessionTerminated}, nil
}
