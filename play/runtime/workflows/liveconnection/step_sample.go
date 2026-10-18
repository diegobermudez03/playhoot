package liveconnection

import (
	"context"
	"fmt"

	"github.com/diegobermudez03/playhoot/play/internal/business/inputs"
	"github.com/diegobermudez03/playhoot/play/runtime"
)

// participantInputs is one participant's SAMPLE ingress: the highest
// sequence accepted, how far it was consumed, and the input state the
// accepted SAMPLEs left.
type participantInputs struct {
	maxAccepted     int64
	consumedThrough int64
	current         inputs.Snapshot
}

// Transition is one participant's new input state on the Session's single
// timeline, at the effective Session time it was accepted. Order breaks ties
// between operations accepted at the same time.
type Transition struct {
	ParticipantID string
	Inputs        inputs.Snapshot
	SessionTimeMs int64
	Order         uint64
}

// acceptSample takes a player's SAMPLE into the timeline, without waiting for
// a boundary in flight: it joins the next one. A stale or repeated sequence
// is ignored, a refresh of the current state only moves the acknowledgement,
// and a SAMPLE the game does not declare is runtime.ErrInvalidSample.
func (c *sessionController) acceptSample(ctx context.Context, player *connection, sample runtime.Sample) error {
	s := c.lock()
	defer c.unlock()
	if s.phase != phaseRunning || player.participantID == "" {
		// Before the game runs there is nothing to consume it.
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
	s.nextOrder++
	s.timeline = append(s.timeline, Transition{
		ParticipantID: participantID,
		Inputs:        snapshot,
		SessionTimeMs: s.sessionTime(),
		Order:         s.nextOrder,
	})
}
