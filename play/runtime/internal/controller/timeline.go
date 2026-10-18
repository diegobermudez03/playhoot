package controller

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/diegobermudez03/playhoot/play/internal/business/inputs"
	"github.com/diegobermudez03/playhoot/play/runtime"
	"github.com/diegobermudez03/playhoot/play/runtime/internal/business/viewpatch"
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

// Boundary is what one tick takes from the timeline to execute: the
// transitions accepted so far, in order, the Session time they bring the game
// to, and the confirmed game they run on.
type Boundary struct {
	Transitions   []Transition
	SessionTimeMs int64
	Game          Game
	// ParticipantIDs are every participant, whose views follow the batch.
	ParticipantIDs []string
	// acks is how far each participant's SAMPLEs are consumed once the
	// batch commits.
	acks map[string]int64
}

// sessionTimeLocked is the effective Session time now: monotonic, starting
// where the game's confirmed state was when it began running here.
func (c *Controller) sessionTimeLocked() int64 {
	return c.baseSessionTimeMs + c.now().Sub(c.runningSince).Milliseconds()
}

// acceptSample takes a player's SAMPLE into the timeline. A stale or repeated
// sequence is ignored, a refresh of the current state only moves the
// acknowledgement, and a SAMPLE the game does not declare is
// runtime.ErrInvalidSample.
func (c *Controller) acceptSample(ctx context.Context, player *connection, sample runtime.Sample) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.phase != phaseRunning || player.participantID == "" {
		// Before the game runs there is nothing to consume it.
		samplesReceived.Inc(ctx, sampleResultLabel.Value(sampleDropped))
		return nil
	}
	ingress := c.inputs[player.participantID]
	if sample.Sequence <= ingress.maxAccepted {
		samplesReceived.Inc(ctx, sampleResultLabel.Value(sampleStale))
		return nil
	}
	snapshot, err := c.manifest.Inputs.Normalize(sample.Inputs)
	if err != nil {
		samplesReceived.Inc(ctx, sampleResultLabel.Value(sampleInvalid))
		return fmt.Errorf("%w: %v", runtime.ErrInvalidSample, err)
	}
	ingress.maxAccepted = sample.Sequence
	if snapshot == ingress.current {
		samplesReceived.Inc(ctx, sampleResultLabel.Value(sampleRefresh))
		return nil
	}
	c.enqueueTransitionLocked(player.participantID, snapshot)
	samplesReceived.Inc(ctx, sampleResultLabel.Value(sampleTransition))
	return nil
}

// resetInputsLocked returns a participant whose transport is gone to the
// default inputs at once: nobody is pressing anything for them.
func (c *Controller) resetInputsLocked(participantID string) {
	ingress, found := c.inputs[participantID]
	if !found {
		return
	}
	if defaults := c.manifest.Inputs.Defaults(); ingress.current != defaults {
		c.enqueueTransitionLocked(participantID, defaults)
	}
}

func (c *Controller) enqueueTransitionLocked(participantID string, snapshot inputs.Snapshot) {
	c.inputs[participantID].current = snapshot
	c.nextOrder++
	c.timeline = append(c.timeline, Transition{
		ParticipantID: participantID,
		Inputs:        snapshot,
		SessionTimeMs: c.sessionTimeLocked(),
		Order:         c.nextOrder,
	})
}

// TakeBoundary moves every pending transition into a Boundary; false means
// the game no longer runs here.
func (c *Controller) TakeBoundary() (Boundary, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.phase != phaseRunning {
		return Boundary{}, false
	}
	boundary := Boundary{
		Transitions:   c.timeline,
		SessionTimeMs: max(c.sessionTimeLocked(), c.game.ExecutedThroughSessionTimeMs),
		Game:          c.game,
		acks:          make(map[string]int64, len(c.inputs)),
	}
	if n := len(c.timeline); n > 0 {
		boundary.SessionTimeMs = max(boundary.SessionTimeMs, c.timeline[n-1].SessionTimeMs)
	}
	c.timeline = nil
	for _, participant := range c.participants {
		boundary.ParticipantIDs = append(boundary.ParticipantIDs, participant.ParticipantID)
		boundary.acks[participant.ParticipantID] = c.inputs[participant.ParticipantID].maxAccepted
	}
	return boundary, true
}

// Requeue puts back the transitions of a boundary that could not execute,
// ahead of any accepted since, so the timeline keeps its order.
func (c *Controller) Requeue(boundary Boundary) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.phase != phaseRunning {
		return
	}
	c.timeline = append(slices.Clone(boundary.Transitions), c.timeline...)
}

// ErrNotRunning means the game ended here before a boundary could commit.
var ErrNotRunning = errors.New("game no longer runs here")

// ErrOutOfSync means an execution did not follow the mirror's revision: the
// context drifted from the confirmed state.
var ErrOutOfSync = errors.New("execution does not follow the confirmed state")

// CommitBoundary confirms what a boundary's execution produced: game is the
// new mirror, views every participant's projection. Each connected player
// gets what changed in their view and how far their SAMPLEs were consumed.
func (c *Controller) CommitBoundary(ctx context.Context, boundary Boundary, game Game, views map[string][]byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.phase != phaseRunning {
		return ErrNotRunning
	}
	if game.ContextRevision != c.game.ContextRevision+1 {
		return fmt.Errorf("%w: revision %d after the mirror's %d", ErrOutOfSync, game.ContextRevision, c.game.ContextRevision)
	}
	game.ExecutedThroughSessionTimeMs = boundary.SessionTimeMs
	c.game = game
	c.acknowledgeLocked(ctx, boundary, views)
	return nil
}

// AcknowledgeOnly tells players how far their SAMPLEs were consumed after a
// boundary with nothing to execute: only refreshes arrived.
func (c *Controller) AcknowledgeOnly(ctx context.Context, boundary Boundary) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.phase != phaseRunning {
		return
	}
	c.acknowledgeLocked(ctx, boundary, nil)
}

func (c *Controller) acknowledgeLocked(ctx context.Context, boundary Boundary, views map[string][]byte) {
	for participantID, acked := range boundary.acks {
		c.inputs[participantID].consumedThrough = acked
	}
	for _, participant := range c.participants {
		player, found := c.players[participant.UserUUID]
		if !found || player.isEnded() {
			continue
		}
		c.sendViewLocked(ctx, player, participant.ParticipantID, views[participant.ParticipantID])
	}
}

// sendViewLocked sends player what changed in their view since the last one
// their connection received, with their acknowledgement; nothing when neither
// changed.
func (c *Controller) sendViewLocked(ctx context.Context, player *connection, participantID string, view []byte) {
	acked := c.inputs[participantID].consumedThrough
	patch, full := []byte(`{}`), false
	if view != nil {
		diff, wasFull, err := viewpatch.Diff(player.lastView, view)
		if err != nil {
			// A view that is not JSON is the game's bug; resend it whole.
			diff, wasFull = view, true
		}
		patch, full = diff, wasFull
		player.lastView = view
	}
	if !full && string(patch) == "{}" && acked == player.lastAck {
		return
	}
	kind := viewPatch
	switch {
	case full:
		kind = viewFull
	case string(patch) == "{}":
		kind = viewAckOnly
	}
	viewsSent.Inc(ctx, viewKindLabel.Value(kind))
	player.lastAck = acked
	player.views.publish(runtime.OwnView{
		PlayerView:                    runtime.PlayerView{ParticipantID: participantID, Full: full, State: patch},
		ConsumedThroughSampleSequence: acked,
	})
}

// AbortRunning ends a running game the platform cannot continue: every
// connection ends as aborted.
func (c *Controller) AbortRunning() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.phase != phaseRunning {
		return
	}
	c.phase = phaseEnded
	c.timeline = nil
	for id, admin := range c.admins {
		admin.end(runtime.EndSessionAborted)
		delete(c.admins, id)
	}
	for userUUID, player := range c.players {
		player.end(runtime.EndSessionAborted)
		delete(c.players, userUUID)
	}
	c.stopLocked()
}

// Done is closed once the Controller no longer runs its game: it ended, or
// another instance took the Session over.
func (c *Controller) Done() <-chan struct{} {
	return c.done
}

// FlushRequests receives when an operation wants its boundary now rather than
// at the next tick.
func (c *Controller) FlushRequests() <-chan struct{} {
	return c.flush
}

// RequestFlush asks for a boundary now; requests before it runs coalesce.
func (c *Controller) RequestFlush() {
	select {
	case c.flush <- struct{}{}:
	default:
	}
}

func (c *Controller) stopLocked() {
	c.stopOnce.Do(func() { close(c.done) })
}

// nowFunc is the clock a Controller measures Session time with.
type nowFunc func() time.Time
