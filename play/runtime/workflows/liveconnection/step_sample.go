package liveconnection

import (
	"context"
	"fmt"

	"github.com/diegobermudez03/playhoot/play/internal/business/inputs"
	"github.com/diegobermudez03/playhoot/play/runtime"
	"github.com/diegobermudez03/playhoot/utils/ratelimit"
)

// participantInputs is one participant's SAMPLE ingress: the connection whose
// SAMPLE sequence it follows (each connection numbers its own), the highest
// sequence accepted from it, how far it was consumed, the input state the
// accepted SAMPLEs left, and how often it may change.
type participantInputs struct {
	stream          string
	maxAccepted     int64
	consumedThrough int64
	current         inputs.Snapshot
	transitions     *ratelimit.Limiter
}

func newParticipantInputs(current inputs.Snapshot, limits liveLimits) *participantInputs {
	return &participantInputs{current: current, transitions: ratelimit.New(limits.sampleTransitions...)}
}

// follow starts the SAMPLE sequence of the connection connectionID.
func (i *participantInputs) follow(connectionID string) {
	i.stream = connectionID
	i.maxAccepted = 0
	i.consumedThrough = 0
}

// acceptSample takes a player's SAMPLE into the next boundary's timeline. A
// stale sequence is ignored, a refresh only moves the acknowledgement, a
// transition beyond the limits is dropped unacknowledged (the next SAMPLE
// carries the state), and undeclared inputs are runtime.ErrInvalidSample.
func (c *sessionController) acceptSample(ctx context.Context, player *connection, sample runtime.Sample) error {
	s := c.lock()
	defer c.unlock()
	if s.phase != phaseRunning || player.participantID == "" || s.players[player.userUUID] != player {
		// Before the game runs there is nothing to consume it, and a replaced
		// connection no longer plays.
		samplesReceived.Inc(ctx, sampleResultLabel.Value(sampleDropped))
		return nil
	}
	ingress := s.inputs[player.participantID]
	if sample.Sequence <= ingress.maxAccepted {
		samplesReceived.Inc(ctx, sampleResultLabel.Value(sampleStale))
		return nil
	}
	snapshot, err := s.manifest.Inputs.Normalize(sample.Inputs)
	if err != nil {
		samplesReceived.Inc(ctx, sampleResultLabel.Value(sampleInvalid))
		return fmt.Errorf("%w: %v", runtime.ErrInvalidSample, err)
	}
	if snapshot == ingress.current {
		ingress.maxAccepted = sample.Sequence
		samplesReceived.Inc(ctx, sampleResultLabel.Value(sampleRefresh))
		return nil
	}
	limit := s.timelineRoom(player.participantID)
	if limit == "" {
		if allowed, _ := ingress.transitions.Allow(s.now()); !allowed {
			limit = limitSampleRate
		}
	}
	if limit != "" {
		s.refused(ctx, player.userUUID, limit)
		samplesReceived.Inc(ctx, sampleResultLabel.Value(sampleLimited))
		return nil
	}
	ingress.maxAccepted = sample.Sequence
	ingress.current = snapshot
	s.enqueuePlayer(Operation{ParticipantID: player.participantID, Inputs: snapshot, sampleStream: ingress.stream, sampleSequence: sample.Sequence})
	// Only a distinct transition is someone playing; a refresh is not.
	s.recordActivity()
	samplesReceived.Inc(ctx, sampleResultLabel.Value(sampleTransition))
	return nil
}

// sessionTime is the effective Session time now: monotonic, starting where
// the game's confirmed state was when it began running here.
func (s *liveState) sessionTime() int64 {
	return s.baseSessionTimeMs + s.now().Sub(s.runningSince).Milliseconds()
}

// resetInputs returns a participant whose transport is gone to the default
// inputs at once: nobody is pressing anything for them.
func (s *liveState) resetInputs(participantID string) {
	ingress, found := s.inputs[participantID]
	if !found {
		return
	}
	if defaults := s.manifest.Inputs.Defaults(); ingress.current != defaults {
		s.enqueueTransition(participantID, defaults)
	}
}

func (s *liveState) enqueueTransition(participantID string, snapshot inputs.Snapshot) {
	s.inputs[participantID].current = snapshot
	s.enqueue(Operation{ParticipantID: participantID, Inputs: snapshot})
}
