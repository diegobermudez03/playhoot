package liveconnection

import (
	"context"
	"time"

	"github.com/diegobermudez03/playhoot/monitoring/metrics"
)

var (
	transportsDetached = metrics.NewCounter(
		"play_runtime_player_transports_detached_total",
		"Player transports of a running game lost without leaving, each starting the transport grace.",
	)
	participantsDisconnected = metrics.NewCounter(
		"play_runtime_participants_disconnected_total",
		"Participants of running games disconnected, so the game was told, by cause: their transport grace ran out, or they left.",
		disconnectCauseLabel,
	)
)

// Why a participant was disconnected, as metric label values.
const (
	disconnectGraceExpired = "grace_expired"
	disconnectLeft         = "left"
)

var disconnectCauseLabel = metrics.NewLabel("cause", disconnectGraceExpired, disconnectLeft)

// participantPresence is whether a running game's participant still plays.
// A participant whose transport went away stays connected for the transport
// grace, counted from detachedAt; once it runs out they are disconnected.
type participantPresence struct {
	disconnected bool
	// detachedAt is when their transport went away; zero while attached.
	detachedAt time.Time
}

// attached reports whether the participant has a transport now.
func (p *participantPresence) attached() bool {
	return !p.disconnected && p.detachedAt.IsZero()
}

// detachPlayer handles a running game's player transport going away without
// leaving: their inputs return to the defaults at once and the transport
// grace starts. Nothing happens if a newer connection already replaced it.
func (s *liveState) detachPlayer(ctx context.Context, player *connection) {
	if s.players[player.userUUID] != player {
		return
	}
	presence, found := s.presence[player.participantID]
	if !found || !presence.attached() {
		return
	}
	s.resetInputs(player.participantID)
	presence.detachedAt = s.now()
	transportsDetached.Inc(ctx)
}

// expireGraces tells the game, through the timeline, about every participant
// whose transport grace ran out; from then on they are disconnected.
func (s *liveState) expireGraces(ctx context.Context, grace time.Duration) {
	now := s.now()
	for _, participant := range s.participants {
		presence := s.presence[participant.ParticipantID]
		if presence.disconnected || presence.detachedAt.IsZero() || now.Sub(presence.detachedAt) < grace {
			continue
		}
		s.disconnect(ctx, participant.ParticipantID, disconnectGraceExpired)
	}
}

// disconnect makes participantID stop playing for cause: their inputs return
// to the defaults and playerDisconnected() enters the timeline.
func (s *liveState) disconnect(ctx context.Context, participantID string, cause string) {
	presence := s.presence[participantID]
	presence.disconnected = true
	presence.detachedAt = time.Time{}
	s.resetInputs(participantID)
	s.enqueue(Operation{ParticipantID: participantID, Disconnected: true})
	participantsDisconnected.Inc(ctx, disconnectCauseLabel.Value(cause))
}
