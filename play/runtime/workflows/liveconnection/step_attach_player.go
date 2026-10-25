package liveconnection

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/play/internal/business/inputrequirements"
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
		string(runtime.RejectRejoinRejected),
		string(runtime.RejectSessionStarted),
		string(runtime.RejectSessionUnavailable),
		string(runtime.RejectOwnerChanged),
		string(runtime.RejectServerBusy),
		string(runtime.RejectDeviceNotSupported),
	)

	playerAttaches = metrics.NewCounter(
		"play_runtime_player_attach_total",
		"Player connections attached to a live Session, into its lobby or back into its running game, by outcome.",
		playerAttachOutcomeLabel,
	)
)

// AttachPlayer hands player's connection to sessionUUID's controller under
// ownerEpoch, creating the controller if it has none. An error means
// admission could not be decided.
func (r *ControllerResolver) AttachPlayer(ctx context.Context, sessionUUID string, player runtime.Player, ownerEpoch int64) (runtime.AttachPlayerResult, error) {
	sessionController, rejection := r.controllerFor(ctx, sessionUUID, ownerEpoch)
	if sessionController == nil {
		return rejectPlayer(ctx, rejection), nil
	}
	return sessionController.attachPlayer(ctx, player)
}

// attachPlayer takes player's connection back into the running game if they
// play it, recovering the game first when the database runs it and this
// instance does not, or admits it into the lobby if the durable Session
// accepts players. Only then is the device checked, on every connection: an
// older connection is never replaced by one on a device that cannot play.
func (c *sessionController) attachPlayer(ctx context.Context, player runtime.Player) (runtime.AttachPlayerResult, error) {
	if c.playsHere(player.UserUUID) {
		if result, supported, err := c.admitDevice(ctx, player.Device); !supported {
			return result, err
		}
		if result, participant, err := c.attachParticipant(ctx, player); participant {
			return result, err
		}
	}
	eligibility, err := c.deps.playerJoins.EligibleToJoinAsPlayer(ctx, c.sessionUUID)
	if err != nil {
		return runtime.AttachPlayerResult{}, fmt.Errorf("checking player eligibility: %w", err)
	}
	switch eligibility {
	case sessionpkg.PlayerJoinEligible:
	case sessionpkg.PlayerJoinSessionStarted:
		return c.attachRecoveredPlayer(ctx, player)
	case sessionpkg.PlayerJoinSessionNotFound, sessionpkg.PlayerJoinSessionNotJoinable:
		return rejectPlayer(ctx, runtime.RejectSessionUnavailable), nil
	default:
		return runtime.AttachPlayerResult{}, fmt.Errorf("unknown player eligibility %q", eligibility)
	}
	if result, supported, err := c.admitDevice(ctx, player.Device); !supported {
		return result, err
	}
	connection, rejection, admitted := c.joinLobby(ctx, player)
	if !admitted {
		return rejectPlayer(ctx, rejection), nil
	}
	playerAttaches.Inc(ctx, playerAttachOutcomeLabel.Value("accepted"))
	return runtime.AttachPlayerResult{Outcome: runtime.AttachAccepted, Connection: connection}, nil
}

// attachRecoveredPlayer takes player's connection into the running game,
// recovered first if it does not run here; only its participants come in,
// on a device that can play it.
func (c *sessionController) attachRecoveredPlayer(ctx context.Context, player runtime.Player) (runtime.AttachPlayerResult, error) {
	if err := c.loadManifest(ctx); err != nil {
		return runtime.AttachPlayerResult{}, err
	}
	recovery, err := c.recoverIfNeeded(ctx)
	if err != nil {
		return runtime.AttachPlayerResult{}, err
	}
	if recovery == recoveryEnded {
		return rejectPlayer(ctx, runtime.RejectSessionUnavailable), nil
	}
	if !c.playsHere(player.UserUUID) {
		return rejectPlayer(ctx, runtime.RejectSessionStarted), nil
	}
	if result, supported, err := c.admitDevice(ctx, player.Device); !supported {
		return result, err
	}
	if result, participant, err := c.attachParticipant(ctx, player); participant {
		return result, err
	}
	return rejectPlayer(ctx, runtime.RejectSessionStarted), nil
}

// playsHere reports whether userUUID is a participant of the game running
// here.
func (c *sessionController) playsHere(userUUID string) bool {
	s := c.lock()
	defer c.unlock()
	_, found := s.participantOf(userUUID)
	return s.phase == phaseRunning && found
}

// admitDevice is checkDevice once the Session's manifest is loaded.
func (c *sessionController) admitDevice(ctx context.Context, device runtime.Device) (result runtime.AttachPlayerResult, supported bool, err error) {
	if err := c.loadManifest(ctx); err != nil {
		return runtime.AttachPlayerResult{}, false, err
	}
	return c.checkDevice(ctx, device)
}

// checkDevice reports whether device can play the Session's game; when it
// cannot, result is the rejection carrying the game's requirements. An error
// means the requirements could not be read.
func (c *sessionController) checkDevice(ctx context.Context, device runtime.Device) (result runtime.AttachPlayerResult, supported bool, err error) {
	s := c.lock()
	requirements := s.manifest.Requirements
	c.unlock()

	parsed, known := inputrequirements.ParseDevice(string(device))
	if known {
		logging.LogFields(ctx, logging.Field("device", string(parsed)))
	} else {
		logging.LogFields(ctx, logging.Field("device", "unknown"))
	}
	allowed, knownDevices := requirements.Devices.Allows(parsed)
	if !knownDevices {
		monitoring.Alert(ctx, "play game version has input requirements with unknown devices")
		return runtime.AttachPlayerResult{}, false, fmt.Errorf("game version allows unknown devices %q", requirements.Devices)
	}
	if allowed {
		return runtime.AttachPlayerResult{}, true, nil
	}
	result = rejectPlayer(ctx, runtime.RejectDeviceNotSupported)
	result.Requirements = runtime.InputRequirements{
		Devices:          string(requirements.Devices),
		UsesKeyboard:     requirements.UsesKeyboard,
		UsesPointerHover: requirements.UsesPointerHover,
		UsesPointerPress: requirements.UsesPointerPress,
	}
	return result, false, nil
}

func rejectPlayer(ctx context.Context, reason runtime.RejectReason) runtime.AttachPlayerResult {
	playerAttaches.Inc(ctx, playerAttachOutcomeLabel.Value(string(reason)))
	return runtime.AttachPlayerResult{Outcome: runtime.AttachRejected, Rejection: reason}
}

// joinLobby admits joining's player connection into the lobby, serialized
// with every other admission. A user already in it keeps their place: their
// older connection ends as replaced. Rejection is set only when not admitted.
func (c *sessionController) joinLobby(ctx context.Context, joining runtime.Player) (runtime.PlayerConnection, runtime.RejectReason, bool) {
	userUUID := joining.UserUUID
	player := newConnection(uuid.NewString(), joining)

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
	s.playedBy[userUUID] = true
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
		Resync: func(ctx context.Context) error {
			c.resync(ctx, player)
			return nil
		},
		LimitHit: func(ctx context.Context, limit string) {
			c.deps.auditor().Hit(ctx, c.sessionUUID, player.userUUID, limit)
		},
		ReportGameFault: func(ctx context.Context, fault runtime.GameFault) error {
			c.reportGameFault(ctx, player, fault)
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
		NextRosterUpdate: player.nextRosterUpdate,
		Ended:            player.wait,
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
