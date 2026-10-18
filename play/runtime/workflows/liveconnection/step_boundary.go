package liveconnection

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/play/runtime"
	"github.com/diegobermudez03/playhoot/play/runtime/internal/business/viewpatch"
	"github.com/diegobermudez03/playhoot/play/runtime/replay"
	"github.com/diegobermudez03/playhoot/play/runtime/sandbox"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

//go:generate mockgen -package=liveconnection -destination=step_boundary_mock_test.go . sessionAborter,replayRecorder

// sessionAborter ends a running Session the platform cannot continue.
type sessionAborter interface {
	AbortRunningSession(ctx context.Context, sessionUUID string, cause sessionpkg.AbortCause) error
}

// replayRecorder takes what a game offers to its replay history. Offer must
// never block: recording is best effort.
type replayRecorder interface {
	Offer(ctx context.Context, capture replay.Capture)
}

// maxInfraFailures is how many boundaries in a row may fail to execute before
// the Session is aborted: about five seconds at a 100 ms cadence.
const maxInfraFailures = 50

// maxExecutionRuns is how many times a boundary's execution runs before a
// failure counts: it is retried once in a context restored from the mirror.
const maxExecutionRuns = 2

// Boundary outcomes, as metric label values.
const (
	boundaryExecuted    = "executed"
	boundaryAckOnly     = "ack_only"
	boundaryRetried     = "infra_failed"
	boundaryAborted     = "aborted"
	boundaryInterrupted = "interrupted"
)

var boundaryOutcomeLabel = metrics.NewLabel("outcome", boundaryExecuted, boundaryAckOnly, boundaryRetried, boundaryAborted, boundaryInterrupted)

// Execution failures retried within a boundary, as metric label values.
const (
	retryScript = "script"
	retryInfra  = "infra"
)

var retryKindLabel = metrics.NewLabel("kind", retryScript, retryInfra)

var (
	boundaryDuration = metrics.NewHistogram(
		"play_runtime_boundary_duration_seconds",
		"How long a game boundary took, by outcome.",
		metrics.LatencyBuckets(),
		boundaryOutcomeLabel,
	)
	boundaryTransitions = metrics.NewHistogram(
		"play_runtime_boundary_transitions",
		"Input transitions a game boundary executed.",
		[]float64{1, 2, 4, 8, 16, 32, 64, 128},
	)
	executionRetries = metrics.NewCounter(
		"play_runtime_execution_retries_total",
		"Executions retried in a fresh context restored from the mirror, by what failed.",
		retryKindLabel,
	)
)

// errNotRunning means the game ended here before a boundary could commit.
var errNotRunning = errors.New("game no longer runs here")

// errOutOfSync means an execution did not follow the mirror's revision: the
// context drifted from the confirmed state.
var errOutOfSync = errors.New("execution does not follow the confirmed state")

// boundary is what one tick takes from the timeline to execute: the
// transitions accepted so far, in order, the Session time they bring the game
// to, and the confirmed game they run on.
type boundary struct {
	transitions   []Transition
	sessionTimeMs int64
	game          Game
	// participantIDs are every participant, whose views follow the batch.
	participantIDs []string
	// acks is how far each participant's SAMPLEs are consumed once the
	// batch commits.
	acks map[string]int64
}

// runBoundary executes what the timeline accepted since the last boundary,
// commits it to the mirror and lets every participant see it. Infrastructure
// failures put the work back for the next boundary; a script that fails
// twice, or too many failed boundaries in a row, abort the Session.
func (c *sessionController) runBoundary() {
	work, running := c.takeBoundary()
	if !running {
		return
	}
	ctx := logging.Start(c.root)
	began := time.Now()
	outcome := boundaryExecuted
	defer func() {
		boundaryDuration.ObserveDuration(ctx, time.Since(began), boundaryOutcomeLabel.Value(outcome))
		if outcome != boundaryExecuted && outcome != boundaryAckOnly {
			// Only notable boundaries are logged: there are many per second.
			logging.LogFields(ctx, logging.Field("session_uuid", c.sessionUUID), logging.Field("boundary_outcome", outcome))
			logging.FinishRequestLog(ctx, c.deps.logger, "game boundary")
		}
	}()

	if len(work.transitions) == 0 {
		outcome = boundaryAckOnly
		c.acknowledgeOnly(ctx, work)
		return
	}
	boundaryTransitions.Observe(ctx, float64(len(work.transitions)))

	game, views, err := c.execute(ctx, work)
	var scriptFailure errScript
	switch {
	case err == nil:
		err = c.commitBoundary(ctx, work, game, views)
		if errors.Is(err, errNotRunning) {
			outcome = boundaryInterrupted
			return
		}
		if err != nil {
			// The context drifted from the mirror: never trust it again.
			c.discardContext(ctx)
			break
		}
		c.infraFailures = 0
		c.deps.replay.Offer(ctx, replay.Capture{
			SessionUUID:     c.sessionUUID,
			GameVersionUUID: game.GameVersionUUID,
			SessionTimeMs:   work.sessionTimeMs,
			ContextRevision: game.ContextRevision,
			State:           game.State,
			Participants:    work.participantIDs,
		})
		return
	case c.root.Err() != nil:
		// Shutting down: the work waits for the next owner.
		c.requeue(work)
		outcome = boundaryInterrupted
		return
	case errors.As(err, &scriptFailure):
		outcome = boundaryAborted
		c.abort(ctx, sessionpkg.AbortScriptFailure, err)
		return
	}

	logging.LogError(ctx, err)
	c.requeue(work)
	c.infraFailures++
	if c.infraFailures >= maxInfraFailures {
		outcome = boundaryAborted
		c.abort(ctx, sessionpkg.AbortUnrecoverableInfraFailure, err)
		return
	}
	outcome = boundaryRetried
}

// execute runs work's transitions and every participant's projection, once
// more in a context restored from the mirror if anything fails. It runs in a
// restored context from the start when the game has none. The returned game
// is the new mirror; an errScript error means the script failed.
func (c *sessionController) execute(ctx context.Context, work boundary) (Game, map[string][]byte, error) {
	var retried string
	for run := 1; ; run++ {
		if c.contextID == "" {
			if err := c.restoreContext(ctx, work.game); err != nil {
				return Game{}, nil, err
			}
		}
		game, views, err := c.attempt(ctx, work)
		if err == nil {
			if retried == retryScript {
				monitoring.Alert(ctx, "game execution succeeded only after its script failed")
			}
			return game, views, nil
		}
		// A context is unusable after a failure.
		c.discardContext(ctx)
		if run == maxExecutionRuns || c.root.Err() != nil {
			return Game{}, nil, err
		}
		retried = retryInfra
		var scriptFailure errScript
		if errors.As(err, &scriptFailure) {
			retried = retryScript
		}
		executionRetries.Inc(ctx, retryKindLabel.Value(retried))
	}
}

// attempt runs work in the game's context.
func (c *sessionController) attempt(ctx context.Context, work boundary) (Game, map[string][]byte, error) {
	input := sandbox.SamplesInput{SessionTimeMs: work.sessionTimeMs, Transitions: make([]sandbox.Transition, len(work.transitions))}
	for i, transition := range work.transitions {
		input.Transitions[i] = sandbox.Transition{
			ParticipantID: transition.ParticipantID,
			Inputs:        []byte(transition.Inputs),
			SessionTimeMs: transition.SessionTimeMs,
		}
	}
	result, err := c.deps.sandbox.Samples(ctx, c.contextID, input)
	if err != nil {
		return Game{}, nil, fmt.Errorf("executing samples: %w", err)
	}
	if result.Outcome != sandbox.ScriptSucceeded {
		return Game{}, nil, errScript{detail: result.ScriptError}
	}
	if result.ContextRevision != work.game.ContextRevision+1 {
		return Game{}, nil, fmt.Errorf("%w: revision %d after the mirror's %d", errOutOfSync, result.ContextRevision, work.game.ContextRevision)
	}
	views, err := c.projectAll(ctx, work.participantIDs)
	if err != nil {
		return Game{}, nil, err
	}
	game := work.game
	game.ContextRevision = result.ContextRevision
	game.State = result.State
	game.PRNGState = result.PRNGState
	return game, views, nil
}

// abort ends the Session for cause: durably first, then live. A failed
// durable abort is alerted, since the game cannot go on either way.
func (c *sessionController) abort(ctx context.Context, cause sessionpkg.AbortCause, failure error) {
	logging.LogError(ctx, failure)
	logging.LogFields(ctx, logging.Field("abort_cause", string(cause)))
	if err := c.deps.aborts.AbortRunningSession(ctx, c.sessionUUID, cause); err != nil {
		logging.LogError(ctx, fmt.Errorf("aborting session: %w", err))
		monitoring.Alert(ctx, "running session could not be aborted durably")
	}
	s := c.lock()
	defer c.unlock()
	if s.phase == phaseRunning {
		c.end(s, runtime.EndSessionAborted)
	}
}

// takeBoundary moves every pending transition into a boundary, so SAMPLEs
// accepted from now on go to the next one; false means the game does not
// run here.
func (c *sessionController) takeBoundary() (boundary, bool) {
	s := c.lock()
	defer c.unlock()
	if s.phase != phaseRunning {
		return boundary{}, false
	}
	work := boundary{
		transitions:   s.timeline,
		sessionTimeMs: max(s.sessionTime(), s.game.ExecutedThroughSessionTimeMs),
		game:          s.game,
		acks:          make(map[string]int64, len(s.inputs)),
	}
	if n := len(s.timeline); n > 0 {
		work.sessionTimeMs = max(work.sessionTimeMs, s.timeline[n-1].SessionTimeMs)
	}
	s.timeline = nil
	for _, participant := range s.participants {
		work.participantIDs = append(work.participantIDs, participant.ParticipantID)
		work.acks[participant.ParticipantID] = s.inputs[participant.ParticipantID].maxAccepted
	}
	return work, true
}

// requeue puts back the transitions of a boundary that could not execute,
// ahead of any accepted since, so the timeline keeps its order.
func (c *sessionController) requeue(work boundary) {
	s := c.lock()
	defer c.unlock()
	if s.phase != phaseRunning {
		return
	}
	s.timeline = append(slices.Clone(work.transitions), s.timeline...)
}

// commitBoundary confirms what a boundary's execution produced: game is the
// new mirror, views every participant's projection. Each connected player
// gets what changed in their view and how far their SAMPLEs were consumed.
func (c *sessionController) commitBoundary(ctx context.Context, work boundary, game Game, views map[string][]byte) error {
	s := c.lock()
	defer c.unlock()
	if s.phase != phaseRunning {
		return errNotRunning
	}
	if game.ContextRevision != s.game.ContextRevision+1 {
		return fmt.Errorf("%w: revision %d after the mirror's %d", errOutOfSync, game.ContextRevision, s.game.ContextRevision)
	}
	game.ExecutedThroughSessionTimeMs = work.sessionTimeMs
	s.game = game
	s.acknowledge(ctx, work, views)
	return nil
}

// acknowledgeOnly tells players how far their SAMPLEs were consumed after a
// boundary with nothing to execute: only refreshes arrived.
func (c *sessionController) acknowledgeOnly(ctx context.Context, work boundary) {
	s := c.lock()
	defer c.unlock()
	if s.phase != phaseRunning {
		return
	}
	s.acknowledge(ctx, work, nil)
}

func (s *liveState) acknowledge(ctx context.Context, work boundary, views map[string][]byte) {
	for participantID, acked := range work.acks {
		s.inputs[participantID].consumedThrough = acked
	}
	for _, participant := range s.participants {
		player, found := s.players[participant.UserUUID]
		if !found || player.isEnded() {
			continue
		}
		s.sendView(ctx, player, participant.ParticipantID, views[participant.ParticipantID])
	}
}

// sendView sends player what changed in their view since the last one their
// connection received, with their acknowledgement; nothing when neither
// changed.
func (s *liveState) sendView(ctx context.Context, player *connection, participantID string, view []byte) {
	acked := s.inputs[participantID].consumedThrough
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
