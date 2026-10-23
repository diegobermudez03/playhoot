package liveconnection

import (
	"context"

	"github.com/diegobermudez03/playhoot/play/runtime"
)

// leave takes player out of the Session at their request, ending the
// connection as left. In the lobby they leave it; in a running game they are
// disconnected at once, without the grace, and a boundary tells the game. A
// leave during a Start attempt applies once the attempt ends.
func (c *sessionController) leave(ctx context.Context, player *connection) (runtime.LeaveResult, error) {
	// The lobby is frozen during a Start attempt, so whether the player
	// leaves a lobby or a running game is only known once the attempt ends.
	s, err := c.lockSettled(ctx)
	if err != nil {
		return runtime.LeaveResult{}, err
	}
	flush := false
	switch s.phase {
	case phaseLobby:
		s.leaveLobby(player)
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
