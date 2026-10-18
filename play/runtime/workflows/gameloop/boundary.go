package gameloop

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/play/runtime/internal/controller"
	"github.com/diegobermudez03/playhoot/play/runtime/replay"
	"github.com/diegobermudez03/playhoot/play/runtime/sandbox"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

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
	lanesActive = metrics.NewGauge(
		"play_runtime_game_lanes_active",
		"Running games whose lane this instance runs.",
	)
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

// errScript means the authored code failed; detail is its error.
type errScript struct{ detail string }

func (e errScript) Error() string { return "script failed: " + e.detail }

// boundary executes what sessionController's timeline accepted; false ends
// the lane.
func (m *Manager) boundary(sessionUUID string, sessionController *controller.Controller, state *laneState) (keepRunning bool) {
	work, running := sessionController.TakeBoundary()
	if !running {
		return false
	}
	ctx := logging.Start(m.root)
	began := time.Now()
	outcome := boundaryExecuted
	defer func() {
		boundaryDuration.ObserveDuration(ctx, time.Since(began), boundaryOutcomeLabel.Value(outcome))
		if outcome != boundaryExecuted && outcome != boundaryAckOnly {
			// Only notable boundaries are logged: there are many per second.
			logging.LogFields(ctx, logging.Field("session_uuid", sessionUUID), logging.Field("boundary_outcome", outcome))
			logging.FinishRequestLog(ctx, m.logger, "game boundary")
		}
	}()

	if len(work.Transitions) == 0 {
		outcome = boundaryAckOnly
		sessionController.AcknowledgeOnly(ctx, work)
		return true
	}
	boundaryTransitions.Observe(ctx, float64(len(work.Transitions)))

	game, views, err := m.execute(ctx, work)
	var scriptFailure errScript
	switch {
	case err == nil:
		err = sessionController.CommitBoundary(ctx, work, game, views)
		if errors.Is(err, controller.ErrNotRunning) {
			m.discard(ctx, game.ContextID)
			outcome = boundaryInterrupted
			return false
		}
		if err != nil {
			// The context drifted from the mirror: never trust it again.
			m.discard(ctx, game.ContextID)
			break
		}
		state.infraFailures = 0
		m.replay.Offer(ctx, replay.Capture{
			SessionUUID:     sessionUUID,
			GameVersionUUID: game.GameVersionUUID,
			SessionTimeMs:   work.SessionTimeMs,
			ContextRevision: game.ContextRevision,
			State:           game.State,
			Participants:    work.ParticipantIDs,
		})
		return true
	case m.root.Err() != nil:
		// Shutting down: the work waits for the next owner.
		sessionController.Requeue(work)
		outcome = boundaryInterrupted
		return false
	case errors.As(err, &scriptFailure):
		outcome = boundaryAborted
		m.abort(ctx, sessionUUID, sessionController, sessionpkg.AbortScriptFailure, err)
		return false
	}

	logging.LogError(ctx, err)
	sessionController.Requeue(work)
	state.infraFailures++
	if state.infraFailures >= maxInfraFailures {
		outcome = boundaryAborted
		m.abort(ctx, sessionUUID, sessionController, sessionpkg.AbortUnrecoverableInfraFailure, err)
		return false
	}
	outcome = boundaryRetried
	return true
}

// execute runs work's transitions and every participant's projection, once
// more in a context restored from the mirror if anything fails. The returned
// game is the new mirror; an errScript error means the script failed twice.
func (m *Manager) execute(ctx context.Context, work controller.Boundary) (controller.Game, map[string][]byte, error) {
	game, views, err := m.attempt(ctx, work, work.Game.ContextID)
	if err == nil {
		return game, views, nil
	}
	m.discard(ctx, work.Game.ContextID)
	if m.root.Err() != nil {
		return controller.Game{}, nil, err
	}
	kind := retryInfra
	var scriptFailure errScript
	if errors.As(err, &scriptFailure) {
		kind = retryScript
	}
	executionRetries.Inc(ctx, retryKindLabel.Value(kind))

	fresh, err := m.restore(ctx, work.Game)
	if err != nil {
		return controller.Game{}, nil, err
	}
	game, views, retryErr := m.attempt(ctx, work, fresh)
	if retryErr != nil {
		m.discard(ctx, fresh)
		return controller.Game{}, nil, retryErr
	}
	if kind == retryScript {
		monitoring.Alert(ctx, "game execution succeeded only after its script failed")
	}
	return game, views, nil
}

// attempt runs work in contextID; the context is unusable after a failure.
func (m *Manager) attempt(ctx context.Context, work controller.Boundary, contextID string) (controller.Game, map[string][]byte, error) {
	input := sandbox.SamplesInput{SessionTimeMs: work.SessionTimeMs, Transitions: make([]sandbox.Transition, len(work.Transitions))}
	for i, transition := range work.Transitions {
		input.Transitions[i] = sandbox.Transition{
			ParticipantID: transition.ParticipantID,
			Inputs:        []byte(transition.Inputs),
			SessionTimeMs: transition.SessionTimeMs,
		}
	}
	result, err := m.sandbox.Samples(ctx, contextID, input)
	if err != nil {
		return controller.Game{}, nil, fmt.Errorf("executing samples: %w", err)
	}
	if result.Outcome != sandbox.ScriptSucceeded {
		return controller.Game{}, nil, errScript{detail: result.ScriptError}
	}
	if result.ContextRevision != work.Game.ContextRevision+1 {
		return controller.Game{}, nil, fmt.Errorf("%w: revision %d after the mirror's %d", controller.ErrOutOfSync, result.ContextRevision, work.Game.ContextRevision)
	}
	views := make(map[string][]byte, len(work.ParticipantIDs))
	for _, participantID := range work.ParticipantIDs {
		projection, err := m.sandbox.Project(ctx, contextID, participantID)
		if err != nil {
			return controller.Game{}, nil, fmt.Errorf("projecting for %s: %w", participantID, err)
		}
		if projection.Outcome != sandbox.ScriptSucceeded {
			return controller.Game{}, nil, errScript{detail: projection.ScriptError}
		}
		views[participantID] = projection.View
	}
	game := work.Game
	game.ContextID = contextID
	game.ContextRevision = result.ContextRevision
	game.State = result.State
	game.PRNGState = result.PRNGState
	return game, views, nil
}

// restore creates a context at the mirror's confirmed state.
func (m *Manager) restore(ctx context.Context, game controller.Game) (string, error) {
	contextID, err := m.sandbox.CreateContext(ctx, sandbox.ContextSpec{GameVersionUUID: game.GameVersionUUID, Script: game.Script})
	if err != nil {
		return "", fmt.Errorf("recreating sandbox context: %w", err)
	}
	restored, err := m.sandbox.Restore(ctx, contextID, sandbox.Snapshot{
		State:           game.State,
		PRNGState:       game.PRNGState,
		ContextRevision: game.ContextRevision,
		SessionTimeMs:   game.ExecutedThroughSessionTimeMs,
	})
	if err != nil {
		m.discard(ctx, contextID)
		return "", fmt.Errorf("restoring sandbox context: %w", err)
	}
	if restored.Outcome != sandbox.ScriptSucceeded {
		m.discard(ctx, contextID)
		return "", errScript{detail: restored.ScriptError}
	}
	return contextID, nil
}

// abort ends the Session for cause: durably first, then live. A failed
// durable abort is alerted, since the game cannot go on either way.
func (m *Manager) abort(ctx context.Context, sessionUUID string, sessionController *controller.Controller, cause sessionpkg.AbortCause, failure error) {
	logging.LogError(ctx, failure)
	logging.LogFields(ctx, logging.Field("abort_cause", string(cause)))
	if err := m.sessions.AbortRunningSession(ctx, sessionUUID, cause); err != nil {
		logging.LogError(ctx, fmt.Errorf("aborting session: %w", err))
		monitoring.Alert(ctx, "running session could not be aborted durably")
	}
	if game, ok := sessionController.TakeBoundary(); ok {
		m.discard(ctx, game.Game.ContextID)
	}
	sessionController.AbortRunning()
}

// discard releases a context no execution will use; failing only leaks it.
func (m *Manager) discard(ctx context.Context, contextID string) {
	if err := m.sandbox.DiscardContext(ctx, contextID); err != nil {
		logging.LogError(ctx, fmt.Errorf("discarding sandbox context: %w", err))
	}
}
