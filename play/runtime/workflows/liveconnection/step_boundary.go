package liveconnection

import (
	"context"
	"errors"
	"fmt"
	"maps"
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
	updateInterval = metrics.NewHistogram(
		"play_runtime_update_interval_seconds",
		"Wall time between consecutive committed update() calls of a game; well above the cadence means skipped ticks.",
		metrics.LatencyBuckets(),
	)
	gameErrors = metrics.NewCounter(
		"play_runtime_game_errors_total",
		"Logical errors committed boundaries returned to participants.",
	)
	gameEvents = metrics.NewCounter(
		"play_runtime_game_events_total",
		"Logical events committed boundaries emitted, each counted once whoever receives it.",
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
	acks map[string]sampleAck
	// update means the batch ends with update(), at sessionTimeMs.
	update bool
}

// sampleAck is how far the SAMPLE sequence of the connection stream was
// accepted when a boundary was taken.
type sampleAck struct {
	stream  string
	through int64
}

// execution is what a boundary's batch produced: the new mirror, every
// participant's view, each command's response and the logical errors and
// events, in timeline order.
type execution struct {
	game      Game
	views     map[string][]byte
	responses [][]byte
	errors    []gameError
	events    []gameEvent
}

// gameEvent is a logical event the game emitted, placed like a gameError and
// delivered the same way.
type gameEvent struct {
	// participantIDs are who receive it; none means every participant.
	participantIDs []string
	sessionTimeMs  int64
	order          uint64
	value          []byte
}

// sentTo reports whether participantID receives the event.
func (e gameEvent) sentTo(participantID string) bool {
	return e.participantIDs == nil || slices.Contains(e.participantIDs, participantID)
}

// gameError is a logical error the game returned to a participant, placed in
// the timeline by the call that returned it; it is delivered, never kept.
type gameError struct {
	participantID string
	sessionTimeMs int64
	// order is the last timeline operation the call ran; 0 for update().
	order uint64
	value []byte
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

	if len(work.operations) == 0 && !work.update {
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
		if work.update {
			if !c.lastUpdate.IsZero() {
				updateInterval.ObserveDuration(ctx, began.Sub(c.lastUpdate))
			}
			c.lastUpdate = began
		}
		c.deps.replay.Offer(ctx, replay.Capture{
			SessionUUID:      c.sessionUUID,
			GameVersionUUID:  executed.game.GameVersionUUID,
			SessionTimeMs:    work.sessionTimeMs,
			ContextRevision:  executed.game.ContextRevision,
			State:            executed.game.State,
			Participants:     work.participantIDs,
			CommandResponses: commandResponses(work, executed.responses),
			Errors:           replayErrors(executed.errors),
			Events:           replayEvents(executed.events),
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
	batch, orders := batchOf(work)
	result, err := c.deps.sandbox.Execute(ctx, c.contextID, batch)
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
	placement := timeline{participantIDs: work.participantIDs, batch: batch, orders: orders}
	placedErrors, err := placeErrors(placement, result.Errors)
	if err != nil {
		return execution{}, err
	}
	placedEvents, err := placeEvents(placement, result.Events)
	if err != nil {
		return execution{}, err
	}
	views, err := c.projectAll(ctx, work.participantIDs)
	if err != nil {
		return execution{}, err
	}
	game := work.game
	game.ContextRevision = result.ContextRevision
	game.State = result.State
	game.PRNGState = result.PRNGState
	return execution{game: game, views: views, responses: result.CommandResponses, errors: placedErrors, events: placedEvents}, nil
}

// timeline places what a batch's calls returned at the time and order of the
// call that returned it, for participants who play.
type timeline struct {
	participantIDs []string
	batch          sandbox.Batch
	orders         []uint64
}

// at is the Session time and order of the batch's operation. An operation
// the batch does not have means the context drifted.
func (t timeline) at(operation int) (int64, uint64, error) {
	if operation < 0 || operation >= len(t.batch.Operations) {
		return 0, 0, fmt.Errorf("%w: operation %d in a batch of %d", errOutOfSync, operation, len(t.batch.Operations))
	}
	return t.batch.Operations[operation].SessionTimeMs, t.orders[operation], nil
}

// mustPlay fails as the script's failure when participantID does not play.
func (t timeline) mustPlay(participantID string) error {
	if !slices.Contains(t.participantIDs, participantID) {
		return errScript{detail: fmt.Sprintf("returned an error or event for %q, who does not play", participantID)}
	}
	return nil
}

// placeErrors places each error the batch returned.
func placeErrors(t timeline, returned []sandbox.GameError) ([]gameError, error) {
	placed := make([]gameError, 0, len(returned))
	for _, returnedError := range returned {
		sessionTimeMs, order, err := t.at(returnedError.Operation)
		if err != nil {
			return nil, err
		}
		if err := t.mustPlay(returnedError.ParticipantID); err != nil {
			return nil, err
		}
		placed = append(placed, gameError{participantID: returnedError.ParticipantID, sessionTimeMs: sessionTimeMs, order: order, value: returnedError.Error})
	}
	return placed, nil
}

// placeEvents places each event the batch emitted.
func placeEvents(t timeline, emitted []sandbox.GameEvent) ([]gameEvent, error) {
	placed := make([]gameEvent, 0, len(emitted))
	for _, emittedEvent := range emitted {
		sessionTimeMs, order, err := t.at(emittedEvent.Operation)
		if err != nil {
			return nil, err
		}
		for _, participantID := range emittedEvent.Participants {
			if err := t.mustPlay(participantID); err != nil {
				return nil, err
			}
		}
		placed = append(placed, gameEvent{participantIDs: emittedEvent.Participants, sessionTimeMs: sessionTimeMs, order: order, value: emittedEvent.Event})
	}
	return placed, nil
}

// batchOf turns work's operations into one sandbox batch, with the order of
// the last timeline operation each call runs (0 for update). Adjacent
// transitions share a samples() call, each command or disconnection runs
// alone, update() ends it; each runs at its own time, never before the
// confirmed state's.
func batchOf(work boundary) (sandbox.Batch, []uint64) {
	var batch sandbox.Batch
	var orders []uint64
	for _, operation := range work.operations {
		if operation.runsAlone() {
			alone := sandbox.Operation{SessionTimeMs: operation.SessionTimeMs}
			if command := operation.Command; command != nil {
				alone.Command = &sandbox.Command{ParticipantID: operation.ParticipantID, Name: command.name, Payload: command.payload}
			} else {
				alone.Disconnected = operation.ParticipantID
			}
			batch.Operations = append(batch.Operations, alone)
			orders = append(orders, operation.Order)
			continue
		}
		transition := sandbox.Transition{ParticipantID: operation.ParticipantID, Inputs: []byte(operation.Inputs), SessionTimeMs: operation.SessionTimeMs}
		if n := len(batch.Operations); n > 0 && len(batch.Operations[n-1].Transitions) > 0 {
			batch.Operations[n-1].Transitions = append(batch.Operations[n-1].Transitions, transition)
			orders[n-1] = operation.Order
			continue
		}
		batch.Operations = append(batch.Operations, sandbox.Operation{Transitions: []sandbox.Transition{transition}})
		orders = append(orders, operation.Order)
	}
	if work.update {
		batch.Operations = append(batch.Operations, sandbox.Operation{SessionTimeMs: work.sessionTimeMs, Update: true})
		orders = append(orders, 0)
	}
	at := work.game.ExecutedThroughSessionTimeMs
	for i := range batch.Operations {
		operation := &batch.Operations[i]
		switch {
		case operation.Command != nil, operation.Update, operation.Disconnected != "":
		case i+1 < len(batch.Operations):
			operation.SessionTimeMs = operation.Transitions[len(operation.Transitions)-1].SessionTimeMs
		default:
			operation.SessionTimeMs = work.sessionTimeMs
		}
		operation.SessionTimeMs = max(operation.SessionTimeMs, at)
		at = operation.SessionTimeMs
	}
	return batch, orders
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

// replayErrors are a boundary's logical errors, as replay records them.
func replayErrors(placed []gameError) []replay.GameError {
	var recorded []replay.GameError
	for _, placedError := range placed {
		recorded = append(recorded, replay.GameError{
			ParticipantID: placedError.participantID,
			SessionTimeMs: placedError.sessionTimeMs,
			Order:         placedError.order,
			Error:         placedError.value,
		})
	}
	return recorded
}

// replayEvents are a boundary's logical events, as replay records them.
func replayEvents(placed []gameEvent) []replay.GameEvent {
	var recorded []replay.GameEvent
	for _, placedEvent := range placed {
		recorded = append(recorded, replay.GameEvent{
			Participants:  placedEvent.participantIDs,
			SessionTimeMs: placedEvent.sessionTimeMs,
			Order:         placedEvent.order,
			Event:         placedEvent.value,
		})
	}
	return recorded
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
// here. A game with update() calls it every boundary; without it, a boundary
// ending with a command ends at that command's time.
func (c *sessionController) takeBoundary() (boundary, bool) {
	s := c.lock()
	defer c.unlock()
	if s.phase != phaseRunning {
		return boundary{}, false
	}
	s.expireGraces(c.root, c.deps.transportGrace)
	work := boundary{
		operations:    s.timeline,
		sessionTimeMs: max(s.sessionTime(), s.game.ExecutedThroughSessionTimeMs),
		game:          s.game,
		acks:          make(map[string]sampleAck, len(s.inputs)),
		update:        s.manifest.Update,
	}
	if n := len(s.timeline); n > 0 {
		last := s.timeline[n-1]
		work.sessionTimeMs = max(work.sessionTimeMs, last.SessionTimeMs)
		if last.runsAlone() && !work.update {
			work.sessionTimeMs = max(last.SessionTimeMs, s.game.ExecutedThroughSessionTimeMs)
		}
	}
	s.timeline = nil
	for _, participant := range s.participants {
		work.participantIDs = append(work.participantIDs, participant.ParticipantID)
		ingress := s.inputs[participant.ParticipantID]
		work.acks[participant.ParticipantID] = sampleAck{stream: ingress.stream, through: ingress.maxAccepted}
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
// player told what changed in their view, how far their SAMPLEs were consumed
// and the logical errors the game returned to them.
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
	maps.Copy(s.views, executed.views)
	answered := 0
	for _, operation := range work.operations {
		if operation.Command != nil {
			s.commands[operation.ParticipantID].executed(operation.Command, executed.responses[answered])
			answered++
		}
	}
	if len(executed.errors) > 0 {
		gameErrors.Add(ctx, float64(len(executed.errors)))
	}
	if len(executed.events) > 0 {
		gameEvents.Add(ctx, float64(len(executed.events)))
	}
	s.acknowledge(ctx, work, executed)
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
	s.acknowledge(ctx, work, execution{})
}

// acknowledge sends every connected player their view of executed, with how
// far their SAMPLEs were consumed and their logical errors and events.
func (s *liveState) acknowledge(ctx context.Context, work boundary, executed execution) {
	for participantID, acked := range work.acks {
		// A connection that took over since numbers its SAMPLEs anew.
		if ingress := s.inputs[participantID]; ingress.stream == acked.stream {
			ingress.consumedThrough = acked.through
		}
	}
	for _, participant := range s.participants {
		player, found := s.players[participant.UserUUID]
		if !found || player.isEnded() {
			continue
		}
		s.sendView(ctx, player, participant.ParticipantID, executed.views[participant.ParticipantID], executed.transientsFor(participant.ParticipantID))
	}
}

// transients are the logical errors and events a player receives once, with
// a view.
type transients struct {
	errors [][]byte
	events [][]byte
}

func (t transients) empty() bool {
	return len(t.errors) == 0 && len(t.events) == 0
}

// transientsFor are the errors and events of executed that participantID
// receives, in timeline order.
func (e execution) transientsFor(participantID string) transients {
	var received transients
	for _, placedError := range e.errors {
		if placedError.participantID == participantID {
			received.errors = append(received.errors, placedError.value)
		}
	}
	for _, placedEvent := range e.events {
		if placedEvent.sentTo(participantID) {
			received.events = append(received.events, placedEvent.value)
		}
	}
	return received
}

// sendView sends player what changed in their view since the last one their
// connection received, with their acknowledgement and their transients;
// nothing when none of them is new.
func (s *liveState) sendView(ctx context.Context, player *connection, participantID string, view []byte, received transients) {
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
	if !full && string(patch) == "{}" && acked == player.lastAck && received.empty() {
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
		Errors:                        received.errors,
		Events:                        received.events,
	})
}
