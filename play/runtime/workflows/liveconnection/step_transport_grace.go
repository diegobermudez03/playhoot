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
	disconnectGraceExpired  = "grace_expired"
	disconnectRecoveryEnded = "recovery_window_expired"
	disconnectLeft          = "left"
)

var disconnectCauseLabel = metrics.NewLabel("cause", disconnectGraceExpired, disconnectRecoveryEnded, disconnectLeft)

// participantPresence is whether a running game's participant still plays.
// A participant without a transport stays connected until graceEndsAt (the
// transport grace, or the recovery window after a recovery); once it passes
// they are disconnected for graceCause.
type participantPresence struct {
	disconnected bool
	// graceEndsAt is zero while they have a transport.
	graceEndsAt time.Time
	graceCause  string
}

// attached reports whether the participant has a transport now.
func (p *participantPresence) attached() bool {
	return !p.disconnected && p.graceEndsAt.IsZero()
}

// await gives a participant without a transport until endsAt to attach one,
// after which they are disconnected for cause.
func (p *participantPresence) await(endsAt time.Time, cause string) {
	p.graceEndsAt, p.graceCause = endsAt, cause
}

// attach marks the participant as having a transport again.
func (p *participantPresence) attach() {
	p.graceEndsAt, p.graceCause = time.Time{}, ""
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
	presence.await(s.now().Add(s.transportGrace), disconnectGraceExpired)
	transportsDetached.Inc(ctx)
}

// expireGraces tells the game, through the timeline, about every participant
// whose grace ran out; from then on they are disconnected.
func (s *liveState) expireGraces(ctx context.Context) {
	now := s.now()
	for _, participant := range s.participants {
		presence := s.presence[participant.ParticipantID]
		if presence.disconnected || presence.graceEndsAt.IsZero() || now.Before(presence.graceEndsAt) {
			continue
		}
		s.disconnect(ctx, participant.ParticipantID, presence.graceCause)
	}
}

// disconnect makes participantID stop playing for cause: their inputs return
// to the defaults and playerDisconnected() enters the timeline.
func (s *liveState) disconnect(ctx context.Context, participantID string, cause string) {
	presence := s.presence[participantID]
	presence.disconnected = true
	presence.attach()
	s.resetInputs(participantID)
	s.enqueue(Operation{ParticipantID: participantID, Disconnected: true})
	participantsDisconnected.Inc(ctx, disconnectCauseLabel.Value(cause))
}
