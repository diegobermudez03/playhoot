package controller

import (
	"context"

	"github.com/google/uuid"

	"github.com/diegobermudez03/playhoot/play/runtime"
)

// PlayerCount returns how many players are in the lobby.
func (c *Controller) PlayerCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.lobby)
}

// AdmitPlayer admits userUUID's player connection into the lobby, serialized
// with every other admission. A user already in it keeps their place: their
// older connection ends as replaced. Rejection is set only when not admitted.
func (c *Controller) AdmitPlayer(ctx context.Context, userUUID string) (runtime.PlayerConnection, runtime.RejectReason, bool) {
	player := newConnection(uuid.NewString(), userUUID)

	c.mu.Lock()
	switch c.phase {
	case phaseLobby:
	case phaseEnded:
		c.mu.Unlock()
		return runtime.PlayerConnection{}, runtime.RejectSessionUnavailable, false
	default:
		c.mu.Unlock()
		return runtime.PlayerConnection{}, runtime.RejectSessionStarted, false
	}
	if older, found := c.players[userUUID]; found {
		older.end(runtime.EndReplaced)
		playersReplaced.Inc(ctx)
		c.players[userUUID] = player
		// send message to the new player connection with current lobby state
		player.lobbyUpdates.publish(c.lobbyLocked())
	} else {
		if len(c.lobby) >= c.manifest.MaxPlayers {
			c.mu.Unlock()
			return runtime.PlayerConnection{}, runtime.RejectLobbyFull, false
		}
		c.players[userUUID] = player
		c.lobby = append(c.lobby, userUUID)
		// send message to all players with the new lobby state
		c.broadcastLobbyLocked()
	}
	c.mu.Unlock()

	return runtime.PlayerConnection{
		Leave: func(context.Context) (runtime.LeaveResult, error) {
			c.mu.Lock()
			if c.phase != phaseLobby {
				c.mu.Unlock()
				// Placeholder: leaving once the lobby is frozen (playerLeft, LEFT) is not built.
				return runtime.LeaveResult{}, ErrNotImplemented
			}
			c.leaveLobbyLocked(player)
			c.mu.Unlock()
			player.end(runtime.EndLeft)
			return runtime.LeaveResult{Outcome: runtime.PlayerLeft}, nil
		},
		// Placeholder: dropped, since no game runs yet to consume it.
		Sample: func(context.Context, runtime.Sample) error {
			return nil
		},
		Command: func(_ context.Context, command runtime.Command) (runtime.CommandResult, error) {
			c.mu.Lock()
			defer c.mu.Unlock()
			if c.phase == phaseRunning {
				// Placeholder: running a command in a game is not built.
				return runtime.CommandResult{}, ErrNotImplemented
			}
			return runtime.CommandResult{Token: command.Token, Outcome: runtime.CommandNotRunning}, nil
		},
		// Placeholder: does nothing, since there is no view before the game runs.
		Resync: func(context.Context) error {
			return nil
		},
		NextLobbyUpdate: player.nextLobbyUpdate,
		Started: func(ctx context.Context) (runtime.PlayerStart, bool) {
			start, ok := c.sessionStart(ctx, player)
			if !ok {
				return runtime.PlayerStart{}, false
			}
			// Set before the game started and never again.
			return runtime.PlayerStart{SessionStart: start, ParticipantID: player.participantID}, true
		},
		NextView: func(ctx context.Context) (runtime.OwnView, bool) {
			return player.views.next(ctx, player.ended)
		},
		Ended: player.wait,
		Disconnect: func() {
			c.mu.Lock()
			// Once frozen the participant stays; only the lobby forgets. A
			// cancelled Start attempt takes them out of the lobby then.
			// Placeholder: a running game's transport grace is not built.
			if c.phase == phaseLobby {
				c.leaveLobbyLocked(player)
			}
			c.mu.Unlock()
			player.end(runtime.EndDisconnected)
		},
	}, "", true
}
