package liveconnection

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/diegobermudez03/playhoot/play/runtime"
)

// attachReattached labels a player connection taken back into its running
// game, as a metric value.
const attachReattached = "reattached"

// reattachPlayer takes userUUID's new player connection into the running game
// they play, when their transport was lost within the grace or they switch
// devices: the game is not told, the older connection is replaced, their
// inputs return to the defaults and they get a FULL view. participant is
// false when this controller runs no game userUUID plays.
func (c *sessionController) reattachPlayer(ctx context.Context, userUUID string) (result runtime.AttachPlayerResult, participant bool) {
	s := c.lock()
	defer c.unlock()
	if s.phase != phaseRunning {
		return runtime.AttachPlayerResult{}, false
	}
	participantID, found := s.participantOf(userUUID)
	if !found {
		return runtime.AttachPlayerResult{}, false
	}
	presence := s.presence[participantID]
	if presence.disconnected {
		// Placeholder: rejoining a disconnected participant (playerRejoined) is not built.
		return rejectPlayer(ctx, runtime.RejectParticipantDisconnected), true
	}
	if older := s.players[userUUID]; older != nil && !older.isEnded() {
		older.end(runtime.EndReplaced)
		playersReplaced.Inc(ctx)
	}
	s.resetInputs(participantID)
	presence.detachedAt = time.Time{}

	player := newConnection(uuid.NewString(), userUUID)
	player.participantID = participantID
	s.players[userUUID] = player
	s.inputs[participantID].follow(player.id)
	player.lastView = s.views[participantID]
	player.views.publish(runtime.OwnView{PlayerView: runtime.PlayerView{ParticipantID: participantID, Full: true, State: player.lastView}})
	viewsSent.Inc(ctx, viewKindLabel.Value(viewFull))
	// Last, so everything above is visible to whoever sees the start.
	player.markStarted(s.start)

	playerAttaches.Inc(ctx, playerAttachOutcomeLabel.Value(attachReattached))
	return runtime.AttachPlayerResult{Outcome: runtime.AttachAccepted, Connection: c.playerConnection(player)}, true
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
