package liveconnection

import (
	"context"

	"github.com/google/uuid"

	"github.com/diegobermudez03/playhoot/play/runtime"
)

// attachReattached labels a player connection taken back into its running
// game, as a metric value.
const attachReattached = "reattached"

// reattachPlayer takes player's new connection into the game they play,
// without telling it: any older connection is replaced, inputs reset, and a
// FULL view sent. participant is false when they play no game here;
// disconnected means they must rejoin first, and nothing was attached.
func (c *sessionController) reattachPlayer(ctx context.Context, player runtime.Player) (result runtime.AttachPlayerResult, participant bool, disconnected bool) {
	s := c.lock()
	defer c.unlock()
	if s.phase != phaseRunning {
		return runtime.AttachPlayerResult{}, false, false
	}
	userUUID := player.UserUUID
	participantID, found := s.participantOf(userUUID)
	if !found {
		return runtime.AttachPlayerResult{}, false, false
	}
	presence := s.presence[participantID]
	if presence.disconnected {
		return runtime.AttachPlayerResult{}, true, true
	}
	if older := s.players[userUUID]; older != nil && !older.isEnded() {
		older.end(runtime.EndReplaced)
		playersReplaced.Inc(ctx)
	}
	s.resetInputs(participantID)
	presence.attach()

	attached := newConnection(uuid.NewString(), player)
	attached.participantID = participantID
	s.players[userUUID] = attached
	s.inputs[participantID].follow(attached.id)
	s.sendFull(ctx, attached, participantID)
	// Last, so everything above is visible to whoever sees the start.
	attached.markStarted(s.start)
	s.broadcastRoster()

	playerAttaches.Inc(ctx, playerAttachOutcomeLabel.Value(attachReattached))
	return runtime.AttachPlayerResult{Outcome: runtime.AttachAccepted, Connection: c.playerConnection(attached)}, true, false
}

// participantOf returns the ParticipantID userUUID plays the game as.
func (s *liveState) participantOf(userUUID string) (string, bool) {
	for _, participant := range s.participants {
		if participant.UserUUID == userUUID {
			return participant.ParticipantID, true
		}
	}
	return "", false
}
