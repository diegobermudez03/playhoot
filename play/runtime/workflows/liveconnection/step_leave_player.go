package liveconnection

import (
	"context"

	"github.com/diegobermudez03/playhoot/play/runtime"
)

// leave takes player out of the Session at their request. In the lobby they
// leave it. In a running game they are disconnected at once, without the
// transport grace, and a boundary runs right away to tell the game; like any
// disconnected participant, only rejoining brings them back. The connection
// ends as left.
func (c *sessionController) leave(ctx context.Context, player *connection) (runtime.LeaveResult, error) {
	s := c.lock()
	flush := false
	switch s.phase {
	case phaseLobby:
		s.leaveLobby(player)
	case phaseStarting:
		c.unlock()
		// Placeholder: leaving while the lobby is frozen for a Start attempt is not built.
		return runtime.LeaveResult{}, ErrNotImplemented
	case phaseRunning:
		// A replaced connection no longer plays, so its leave changes nothing.
		if presence, found := s.presence[player.participantID]; found && s.players[player.userUUID] == player && !presence.disconnected {
			s.disconnect(ctx, player.participantID, disconnectLeft)
			flush = true
		}
	}
	c.unlock()
	if flush {
		c.requestFlush()
	}
	player.end(runtime.EndLeft)
	return runtime.LeaveResult{Outcome: runtime.PlayerLeft}, nil
}
