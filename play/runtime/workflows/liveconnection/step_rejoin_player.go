package liveconnection

import (
	"context"
	"fmt"

	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/play/runtime"
)

// What a rejoin request came to, as metric label values.
const (
	rejoinAccepted     = "accepted"
	rejoinRejected     = "rejected"
	rejoinNotSupported = "not_supported"
	rejoinNotRunning   = "session_not_running"
)

var (
	rejoinOutcomeLabel = metrics.NewLabel("outcome", rejoinAccepted, rejoinRejected, rejoinNotSupported, rejoinNotRunning)

	rejoinsRequested = metrics.NewCounter(
		"play_runtime_rejoins_total",
		"Disconnected participants asking to come back into their running game, by outcome: the game accepted or rejected, the game does not support rejoining, or the Session ended first.",
		rejoinOutcomeLabel,
	)
)

// pendingRejoin is a disconnected participant's request to come back,
// waiting in the timeline for the game's playerRejoined(). done is closed
// once outcome is set: accepted, rejected or not running.
type pendingRejoin struct {
	done    chan struct{}
	outcome string
}

// resolve answers the request. The caller holds the controller's lock.
func (p *pendingRejoin) resolve(outcome string) {
	p.outcome = outcome
	close(p.done)
}

// attachParticipant takes player's new connection into the running game
// they play: at once within the transport grace, or once the game accepts
// them back when they were disconnected. participant is false when this
// controller runs no game player plays.
func (c *sessionController) attachParticipant(ctx context.Context, player runtime.Player) (result runtime.AttachPlayerResult, participant bool, err error) {
	result, participant, disconnected := c.reattachPlayer(ctx, player)
	if !disconnected {
		return result, participant, nil
	}
	outcome, err := c.rejoin(ctx, player.UserUUID)
	if err != nil {
		return runtime.AttachPlayerResult{}, true, err
	}
	rejoinsRequested.Inc(ctx, rejoinOutcomeLabel.Value(outcome))
	switch outcome {
	case rejoinAccepted:
	case rejoinRejected:
		return rejectPlayer(ctx, runtime.RejectRejoinRejected), true, nil
	case rejoinNotSupported:
		return rejectPlayer(ctx, runtime.RejectParticipantDisconnected), true, nil
	default:
		return rejectPlayer(ctx, runtime.RejectSessionUnavailable), true, nil
	}
	result, _, disconnected = c.reattachPlayer(ctx, player)
	if disconnected {
		// Accepted, then disconnected again before this connection attached.
		return rejectPlayer(ctx, runtime.RejectParticipantDisconnected), true, nil
	}
	return result, true, nil
}

// rejoin asks the game, through the timeline and a boundary right away,
// whether userUUID's disconnected participant may come back, and waits for
// its answer. Requests of the same participant while one waits share it.
func (c *sessionController) rejoin(ctx context.Context, userUUID string) (string, error) {
	s := c.lock()
	participantID, found := s.participantOf(userUUID)
	switch {
	case s.phase != phaseRunning || !found:
		c.unlock()
		return rejoinNotRunning, nil
	case !s.manifest.Rejoin:
		c.unlock()
		return rejoinNotSupported, nil
	}
	pending, waiting := s.rejoins[participantID]
	if !waiting {
		pending = &pendingRejoin{done: make(chan struct{})}
		s.rejoins[participantID] = pending
		s.enqueue(Operation{ParticipantID: participantID, Rejoin: pending})
	}
	c.unlock()
	c.requestFlush()

	select {
	case <-pending.done:
	case <-ctx.Done():
		// It stays in the timeline; if accepted, the transport grace applies.
		return "", fmt.Errorf("waiting for rejoin: %w", ctx.Err())
	}
	// Resolved before done was closed and never again.
	return pending.outcome, nil
}

// decideRejoin applies the game's answer to participantID's request: accepted,
// they play again, with the transport grace to attach. The caller holds the
// lock.
func (s *liveState) decideRejoin(participantID string, request *pendingRejoin, accepted bool) {
	delete(s.rejoins, participantID)
	if !accepted {
		request.resolve(rejoinRejected)
		return
	}
	presence := s.presence[participantID]
	presence.disconnected, presence.disconnectCause = false, ""
	presence.await(s.now().Add(s.transportGrace), disconnectGraceExpired)
	request.resolve(rejoinAccepted)
	s.broadcastRoster()
}
