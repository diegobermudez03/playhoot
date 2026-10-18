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
	boundaryOperations = metrics.NewHistogram(
		"play_runtime_boundary_operations",
		"Timeline operations (input transitions and commands) a game boundary executed.",
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
// operations accepted so far, in order, the Session time they bring the game
// to, and the confirmed game they run on.
type boundary struct {
	operations    []Operation
	sessionTimeMs int64
	game          Game
	// participantIDs are every participant, whose views follow the batch.
	participantIDs []string
	// acks is how far each participant's SAMPLEs are consumed once the
	// batch commits.
	acks map[string]int64
}

// execution is what a boundary's batch produced: the new mirror, every
// participant's view, and each command's response in timeline order.
type execution struct {
	game      Game
	views     map[string][]byte
	responses [][]byte
}

// runBoundary executes what the timeline accepted since the last boundary,
// commits it to the mirror, answers its commands and lets every participant
// see it. Infrastructure failures put the work back for the next boundary; a
// script that fails twice, or too many failed boundaries in a row, abort the
// Session.
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

	if len(work.operations) == 0 {
		outcome = boundaryAckOnly
		c.acknowledgeOnly(ctx, work)
		return
	}
	boundaryOperations.Observe(ctx, float64(len(work.operations)))

	executed, err := c.execute(ctx, work)
	var scriptFailure errScript
	switch {
	case err == nil:
		err = c.commitBoundary(ctx, work, executed)
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
			SessionUUID:      c.sessionUUID,
			GameVersionUUID:  executed.game.GameVersionUUID,
			SessionTimeMs:    work.sessionTimeMs,
			ContextRevision:  executed.game.ContextRevision,
			State:            executed.game.State,
			Participants:     work.participantIDs,
			CommandResponses: commandResponses(work, executed.responses),
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

// execute runs work's operations and every participant's projection, once
// more in a context restored from the mirror if anything fails. It runs in a
// restored context from the start when the game has none. An errScript error
// means the script failed.
func (c *sessionController) execute(ctx context.Context, work boundary) (execution, error) {
	var retried string
	for run := 1; ; run++ {
		if c.contextID == "" {
			if err := c.restoreContext(ctx, work.game); err != nil {
				return execution{}, err
			}
		}
		executed, err := c.attempt(ctx, work)
		if err == nil {
			if retried == retryScript {
				monitoring.Alert(ctx, "game execution succeeded only after its script failed")
			}
			return executed, nil
		}
		// A context is unusable after a failure.
		c.discardContext(ctx)
		if run == maxExecutionRuns || c.root.Err() != nil {
			return execution{}, err
		}
		retried = retryInfra
		var scriptFailure errScript
		if errors.As(err, &scriptFailure) {
			retried = retryScript
		}
		executionRetries.Inc(ctx, retryKindLabel.Value(retried))
	}
}

// attempt runs work in the game's context as one batch.
func (c *sessionController) attempt(ctx context.Context, work boundary) (execution, error) {
	result, err := c.deps.sandbox.Execute(ctx, c.contextID, batchOf(work))
	if err != nil {
		return execution{}, fmt.Errorf("executing batch: %w", err)
	}
	if result.Outcome != sandbox.ScriptSucceeded {
		return execution{}, errScript{detail: result.ScriptError}
	}
	if result.ContextRevision != work.game.ContextRevision+1 {
		return execution{}, fmt.Errorf("%w: revision %d after the mirror's %d", errOutOfSync, result.ContextRevision, work.game.ContextRevision)
	}
	if commands := countCommands(work.operations); len(result.CommandResponses) != commands {
		return execution{}, fmt.Errorf("%w: %d responses to %d commands", errOutOfSync, len(result.CommandResponses), commands)
	}
	views, err := c.projectAll(ctx, work.participantIDs)
	if err != nil {
		return execution{}, err
	}
	game := work.game
	game.ContextRevision = result.ContextRevision
	game.State = result.State
	game.PRNGState = result.PRNGState
	return execution{game: game, views: views, responses: result.CommandResponses}, nil
}

// batchOf turns work's operations into one sandbox batch, in timeline order:
// adjacent transitions share a samples() call and each command runs alone. A
// command runs at its own time, transitions until the next operation's, or
// the boundary's when last; never before the confirmed state's time.
func batchOf(work boundary) sandbox.Batch {
	var batch sandbox.Batch
	for _, operation := range work.operations {
		if command := operation.Command; command != nil {
			batch.Operations = append(batch.Operations, sandbox.Operation{
				SessionTimeMs: operation.SessionTimeMs,
				Command:       &sandbox.Command{ParticipantID: operation.ParticipantID, Name: command.name, Payload: command.payload},
			})
			continue
		}
		transition := sandbox.Transition{ParticipantID: operation.ParticipantID, Inputs: []byte(operation.Inputs), SessionTimeMs: operation.SessionTimeMs}
		if n := len(batch.Operations); n > 0 && batch.Operations[n-1].Command == nil {
			batch.Operations[n-1].Transitions = append(batch.Operations[n-1].Transitions, transition)
			continue
		}
		batch.Operations = append(batch.Operations, sandbox.Operation{Transitions: []sandbox.Transition{transition}})
	}
	at := work.game.ExecutedThroughSessionTimeMs
	for i := range batch.Operations {
		operation := &batch.Operations[i]
		switch {
		case operation.Command != nil:
		case i+1 < len(batch.Operations):
			operation.SessionTimeMs = batch.Operations[i+1].SessionTimeMs
		default:
			operation.SessionTimeMs = work.sessionTimeMs
		}
		operation.SessionTimeMs = max(operation.SessionTimeMs, at)
		at = operation.SessionTimeMs
	}
	return batch
}

func countCommands(operations []Operation) int {
	commands := 0
	for _, operation := range operations {
		if operation.Command != nil {
			commands++
		}
	}
	return commands
}

// commandResponses pairs each command of work with its response, as replay
// records them.
func commandResponses(work boundary, responses [][]byte) []replay.CommandResponse {
	var answered []replay.CommandResponse
	for _, operation := range work.operations {
		if operation.Command == nil {
			continue
		}
		answered = append(answered, replay.CommandResponse{
			ParticipantID: operation.ParticipantID,
			SessionTimeMs: operation.SessionTimeMs,
			Order:         operation.Order,
			Response:      responses[len(answered)],
		})
	}
	return answered
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

// takeBoundary moves every pending operation into a boundary, so operations
// accepted from now on go to the next one; false means the game does not run
// here. A boundary ending with a command ends at that command's time.
func (c *sessionController) takeBoundary() (boundary, bool) {
	s := c.lock()
	defer c.unlock()
	if s.phase != phaseRunning {
		return boundary{}, false
	}
	work := boundary{
		operations:    s.timeline,
		sessionTimeMs: max(s.sessionTime(), s.game.ExecutedThroughSessionTimeMs),
		game:          s.game,
		acks:          make(map[string]int64, len(s.inputs)),
	}
	if n := len(s.timeline); n > 0 {
		last := s.timeline[n-1]
		work.sessionTimeMs = max(work.sessionTimeMs, last.SessionTimeMs)
		if last.Command != nil {
			work.sessionTimeMs = max(last.SessionTimeMs, s.game.ExecutedThroughSessionTimeMs)
		}
	}
	s.timeline = nil
	for _, participant := range s.participants {
		work.participantIDs = append(work.participantIDs, participant.ParticipantID)
		work.acks[participant.ParticipantID] = s.inputs[participant.ParticipantID].maxAccepted
	}
	return work, true
}

// requeue puts back the operations of a boundary that could not execute,
// ahead of any accepted since, so the timeline keeps its order; its commands
// stay pending.
func (c *sessionController) requeue(work boundary) {
	s := c.lock()
	defer c.unlock()
	if s.phase != phaseRunning {
		return
	}
	s.timeline = append(slices.Clone(work.operations), s.timeline...)
}

// commitBoundary confirms what a boundary's execution produced: the new
// mirror, every command executed with its response, and each connected
// player told what changed in their view and how far their SAMPLEs were
// consumed.
func (c *sessionController) commitBoundary(ctx context.Context, work boundary, executed execution) error {
	s := c.lock()
	defer c.unlock()
	if s.phase != phaseRunning {
		return errNotRunning
	}
	game := executed.game
	if game.ContextRevision != s.game.ContextRevision+1 {
		return fmt.Errorf("%w: revision %d after the mirror's %d", errOutOfSync, game.ContextRevision, s.game.ContextRevision)
	}
	game.ExecutedThroughSessionTimeMs = work.sessionTimeMs
	s.game = game
	answered := 0
	for _, operation := range work.operations {
		if operation.Command != nil {
			s.commands[operation.ParticipantID].executed(operation.Command, executed.responses[answered])
			answered++
		}
	}
	s.acknowledge(ctx, work, executed.views)
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
