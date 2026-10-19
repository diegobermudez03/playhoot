package liveconnection

import (
	"context"
	"fmt"

	"github.com/diegobermudez03/playhoot/play/internal/business/inputs"
	"github.com/diegobermudez03/playhoot/play/runtime"
)

// participantInputs is one participant's SAMPLE ingress: the connection whose
// SAMPLE sequence it follows (each connection numbers its own), the highest
// sequence accepted from it, how far it was consumed, and the input state the
// accepted SAMPLEs left.
type participantInputs struct {
	stream          string
	maxAccepted     int64
	consumedThrough int64
	current         inputs.Snapshot
}

// follow starts the SAMPLE sequence of the connection connectionID.
func (i *participantInputs) follow(connectionID string) {
	i.stream = connectionID
	i.maxAccepted = 0
	i.consumedThrough = 0
}

// acceptSample takes a player's SAMPLE into the timeline, without waiting for
// a boundary in flight: it joins the next one. A stale or repeated sequence
// is ignored, a refresh of the current state only moves the acknowledgement,
// and a SAMPLE the game does not declare is runtime.ErrInvalidSample.
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
	ingress.maxAccepted = sample.Sequence
	if snapshot == ingress.current {
		samplesReceived.Inc(ctx, sampleResultLabel.Value(sampleRefresh))
		return nil
	}
	s.enqueueTransition(player.participantID, snapshot)
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
