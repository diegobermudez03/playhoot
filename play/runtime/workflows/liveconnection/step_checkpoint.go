package liveconnection

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/play/runtime/internal/business/checkpoint"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

//go:generate mockgen -package=liveconnection -destination=step_checkpoint_mock_test.go . checkpointStore

// checkpointStore keeps a running Session's latest recovery checkpoint.
type checkpointStore interface {
	SaveCheckpoint(ctx context.Context, sessionUUID string, checkpoint sessionpkg.Checkpoint) (sessionpkg.SaveCheckpointOutcome, error)
}

const checkpointFailed = "failed"

var (
	checkpointOutcomeLabel = metrics.NewLabel("outcome", string(sessionpkg.CheckpointSaved), string(sessionpkg.CheckpointSessionNotRunning), checkpointFailed)

	checkpointsWritten = metrics.NewCounter(
		"play_runtime_checkpoints_total",
		"Periodic recovery checkpoints written, by outcome.",
		checkpointOutcomeLabel,
	)
	checkpointDuration = metrics.NewHistogram(
		"play_runtime_checkpoint_duration_seconds",
		"How long writing a periodic recovery checkpoint took, by outcome.",
		metrics.LatencyBuckets(),
		checkpointOutcomeLabel,
	)
	checkpointSize = metrics.NewHistogram(
		"play_runtime_checkpoint_bytes",
		"Size of the periodic recovery checkpoints written.",
		[]float64{1 << 10, 4 << 10, 16 << 10, 64 << 10, 256 << 10, 1 << 20},
	)
)

// checkpointWriter holds the latest checkpoint waiting to be written; a newer
// one replaces it, so writing never queues up nor slows the game down.
type checkpointWriter struct {
	mu      sync.Mutex
	pending *checkpoint.Checkpoint
	signal  chan struct{}
}

func newCheckpointWriter() *checkpointWriter {
	return &checkpointWriter{signal: make(chan struct{}, 1)}
}

func (w *checkpointWriter) offer(snapshot checkpoint.Checkpoint) {
	w.mu.Lock()
	w.pending = &snapshot
	w.mu.Unlock()
	select {
	case w.signal <- struct{}{}:
	default:
	}
}

func (w *checkpointWriter) take() (checkpoint.Checkpoint, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.pending == nil {
		return checkpoint.Checkpoint{}, false
	}
	snapshot := *w.pending
	w.pending = nil
	return snapshot, true
}

// offerCheckpoint hands the writer a checkpoint of the game as the boundary
// just committed it, once checkpointInterval passed since the last one. Runs
// on the main goroutine.
func (c *sessionController) offerCheckpoint() {
	s := c.lock()
	defer c.unlock()
	if s.phase != phaseRunning || s.now().Sub(s.lastCheckpoint) < c.deps.checkpointInterval {
		return
	}
	s.lastCheckpoint = s.now()
	c.checkpointWriter.offer(s.checkpoint())
}

// checkpoint is the running game's state now, at the boundary last committed:
// what recovering it needs, nothing waiting in the timeline. The caller holds
// the lock.
func (s *liveState) checkpoint() checkpoint.Checkpoint {
	snapshot := checkpoint.Checkpoint{
		GameVersionUUID:              s.game.GameVersionUUID,
		ContextRevision:              s.game.ContextRevision,
		ExecutedThroughSessionTimeMs: s.game.ExecutedThroughSessionTimeMs,
		State:                        s.game.State,
		PRNGState:                    s.game.PRNGState,
		ProcessedCommandTokens:       make(map[string][]string, len(s.participants)),
	}
	for _, participant := range s.participants {
		participantID := participant.ParticipantID
		snapshot.Participants = append(snapshot.Participants, participantID)
		var tokens []string
		for token := range s.commands[participantID].processed {
			tokens = append(tokens, token)
		}
		slices.Sort(tokens)
		if tokens != nil {
			snapshot.ProcessedCommandTokens[participantID] = tokens
		}
		// One the game was not told about yet is still playing in State.
		if s.presence[participantID].disconnected && !s.disconnectionPending(participantID) {
			snapshot.Disconnected = append(snapshot.Disconnected, participantID)
		}
	}
	return snapshot
}

// disconnectionPending reports whether participantID's disconnection waits
// in the timeline.
func (s *liveState) disconnectionPending(participantID string) bool {
	return slices.ContainsFunc(s.timeline, func(operation Operation) bool {
		return operation.Disconnected && operation.ParticipantID == participantID
	})
}

// writeCheckpoints writes every checkpoint offered until stop is closed;
// a failure waits for the next one. It never touches the controller's state.
func (c *sessionController) writeCheckpoints(stop <-chan struct{}) {
	for {
		select {
		case <-stop:
			return
		case <-c.checkpointWriter.signal:
		}
		if snapshot, ok := c.checkpointWriter.take(); ok {
			c.writeCheckpoint(snapshot)
		}
	}
}

func (c *sessionController) writeCheckpoint(snapshot checkpoint.Checkpoint) {
	ctx := logging.Start(c.root)
	began := time.Now()
	outcome, err := c.storeCheckpoint(ctx, snapshot)
	label := string(outcome)
	if err != nil {
		label = checkpointFailed
		logging.LogError(ctx, err)
		logging.LogFields(ctx, logging.Field("session_uuid", c.sessionUUID), logging.Field("context_revision", snapshot.ContextRevision))
		logging.FinishRequestLog(ctx, c.deps.logger, "checkpoint failed")
	}
	checkpointsWritten.Inc(ctx, checkpointOutcomeLabel.Value(label))
	checkpointDuration.ObserveDuration(ctx, time.Since(began), checkpointOutcomeLabel.Value(label))
}

func (c *sessionController) storeCheckpoint(ctx context.Context, snapshot checkpoint.Checkpoint) (sessionpkg.SaveCheckpointOutcome, error) {
	encoded, err := checkpoint.Encode(snapshot)
	if err != nil {
		return "", err
	}
	checkpointSize.Observe(ctx, float64(len(encoded)))
	outcome, err := c.deps.checkpoints.SaveCheckpoint(ctx, c.sessionUUID, sessionpkg.Checkpoint{ContextRevision: snapshot.ContextRevision, Content: encoded})
	if err != nil {
		return "", fmt.Errorf("saving checkpoint: %w", err)
	}
	return outcome, nil
}
