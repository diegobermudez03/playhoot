package liveconnection

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/play/runtime"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

//go:generate mockgen -package=liveconnection -destination=step_attach_player_mock_test.go . playerJoinChecker

// playerJoinChecker is the durable Session's side of admitting a player.
type playerJoinChecker interface {
	EligibleToJoinAsPlayer(ctx context.Context, sessionUUID string) (sessionpkg.PlayerJoinEligibility, error)
}

var (
	playerAttachOutcomeLabel = metrics.NewLabel("outcome",
		"accepted",
		attachReattached,
		string(runtime.RejectLobbyFull),
		string(runtime.RejectParticipantDisconnected),
		string(runtime.RejectSessionStarted),
		string(runtime.RejectSessionUnavailable),
		string(runtime.RejectOwnerChanged),
	)

	playerAttaches = metrics.NewCounter(
		"play_runtime_player_attach_total",
		"Player connections attached to a live Session, into its lobby or back into its running game, by outcome.",
		playerAttachOutcomeLabel,
	)
)

// AttachPlayer hands userUUID's player connection to sessionUUID's
// controller under ownerEpoch, creating the controller if it has none. An
// error means admission could not be decided.
func (r *ControllerResolver) AttachPlayer(ctx context.Context, sessionUUID string, userUUID string, ownerEpoch int64) (runtime.AttachPlayerResult, error) {
	sessionController, ok := r.controllerFor(ctx, sessionUUID, ownerEpoch)
	if !ok {
		return rejectPlayer(ctx, runtime.RejectOwnerChanged), nil
	}
	return sessionController.attachPlayer(ctx, userUUID)
}

// attachPlayer takes userUUID's player connection back into the running game
// if they play it, or admits it into the lobby if the durable Session accepts
// players.
func (c *sessionController) attachPlayer(ctx context.Context, userUUID string) (runtime.AttachPlayerResult, error) {
	if result, participant := c.reattachPlayer(ctx, userUUID); participant {
		return result, nil
	}
	eligibility, err := c.deps.playerJoins.EligibleToJoinAsPlayer(ctx, c.sessionUUID)
	if err != nil {
		return runtime.AttachPlayerResult{}, fmt.Errorf("checking player eligibility: %w", err)
	}
	switch eligibility {
	case sessionpkg.PlayerJoinEligible:
	case sessionpkg.PlayerJoinSessionStarted:
		return rejectPlayer(ctx, runtime.RejectSessionStarted), nil
	case sessionpkg.PlayerJoinSessionNotFound, sessionpkg.PlayerJoinSessionNotJoinable:
		return rejectPlayer(ctx, runtime.RejectSessionUnavailable), nil
	default:
		return runtime.AttachPlayerResult{}, fmt.Errorf("unknown player eligibility %q", eligibility)
	}
	if err := c.loadManifest(ctx); err != nil {
		return runtime.AttachPlayerResult{}, err
	}
	connection, rejection, admitted := c.joinLobby(ctx, userUUID)
	if !admitted {
		return rejectPlayer(ctx, rejection), nil
	}
	playerAttaches.Inc(ctx, playerAttachOutcomeLabel.Value("accepted"))
	return runtime.AttachPlayerResult{Outcome: runtime.AttachAccepted, Connection: connection}, nil
}

func rejectPlayer(ctx context.Context, reason runtime.RejectReason) runtime.AttachPlayerResult {
	playerAttaches.Inc(ctx, playerAttachOutcomeLabel.Value(string(reason)))
	return runtime.AttachPlayerResult{Outcome: runtime.AttachRejected, Rejection: reason}
}

// joinLobby admits userUUID's player connection into the lobby, serialized
// with every other admission. A user already in it keeps their place: their
// older connection ends as replaced. Rejection is set only when not admitted.
func (c *sessionController) joinLobby(ctx context.Context, userUUID string) (runtime.PlayerConnection, runtime.RejectReason, bool) {
	player := newConnection(uuid.NewString(), userUUID)

	s := c.lock()
	switch s.phase {
	case phaseLobby:
	case phaseEnded:
		c.unlock()
		return runtime.PlayerConnection{}, runtime.RejectSessionUnavailable, false
	default:
		c.unlock()
		return runtime.PlayerConnection{}, runtime.RejectSessionStarted, false
	}
	if older, found := s.players[userUUID]; found {
		older.end(runtime.EndReplaced)
		playersReplaced.Inc(ctx)
		s.players[userUUID] = player
		// send message to the new player connection with current lobby state
		player.lobbyUpdates.publish(s.lobbyUpdate())
	} else {
		if len(s.lobby) >= s.manifest.MaxPlayers {
			c.unlock()
			return runtime.PlayerConnection{}, runtime.RejectLobbyFull, false
		}
		s.players[userUUID] = player
		s.lobby = append(s.lobby, userUUID)
		// send message to all players with the new lobby state
		s.broadcastLobby()
	}
	c.unlock()
	return c.playerConnection(player), "", true
}

// playerConnection is what the caller can do with player, admitted into the
// lobby or back into the running game.
func (c *sessionController) playerConnection(player *connection) runtime.PlayerConnection {
	return runtime.PlayerConnection{
		Leave: func(ctx context.Context) (runtime.LeaveResult, error) {
			return c.leave(ctx, player)
		},
		Sample: func(ctx context.Context, sample runtime.Sample) error {
			return c.acceptSample(ctx, player, sample)
		},
		Command: func(ctx context.Context, command runtime.Command) (runtime.CommandResult, error) {
			return c.acceptCommand(ctx, player, command)
		},
		// Placeholder: does nothing, since there is no view before the game runs.
		Resync: func(context.Context) error {
			return nil
		},
		NextLobbyUpdate: player.nextLobbyUpdate,
		Started: func(ctx context.Context) (runtime.PlayerStart, bool) {
			start, ok := player.sessionStart(ctx)
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
			s := c.lock()
			// Once frozen the participant stays; only the lobby forgets. A
			// cancelled Start attempt takes them out of the lobby then. In a
			// running game they keep playing for the transport grace.
			switch s.phase {
			case phaseLobby:
				s.leaveLobby(player)
			case phaseRunning:
				s.detachPlayer(c.root, player)
			}
			c.unlock()
			player.end(runtime.EndDisconnected)
		},
	}
}
