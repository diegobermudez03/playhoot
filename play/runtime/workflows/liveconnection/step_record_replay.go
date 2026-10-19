package liveconnection

import (
	"context"

	"github.com/diegobermudez03/playhoot/play/runtime/replay"
)

//go:generate mockgen -package=liveconnection -destination=step_record_replay_mock_test.go . replayRecorder

// replayRecorder keeps the replay history of the games this instance runs,
// one recording per controller that runs the game. Offer never blocks:
// recording is best effort. Finish stores a finished game's final tick
// durably, and fails when it could not.
type replayRecorder interface {
	StartRecording(sessionUUID string) string
	Offer(ctx context.Context, recordingID string, capture replay.Capture)
	Finish(ctx context.Context, recordingID string, capture replay.Capture) error
	StopRecording(recordingID string)
}

// recordStart begins the replay of the game this controller just started,
// recovered or adopted, at its starting state. Runs on the main goroutine.
func (c *sessionController) recordStart(ctx context.Context) {
	s := c.lock()
	if s.phase != phaseRunning {
		c.unlock()
		return
	}
	capture := replay.Capture{
		SessionUUID:     c.sessionUUID,
		GameVersionUUID: s.game.GameVersionUUID,
		SessionTimeMs:   s.game.ExecutedThroughSessionTimeMs,
		ContextRevision: s.game.ContextRevision,
		State:           s.game.State,
	}
	for _, participant := range s.participants {
		capture.Participants = append(capture.Participants, participant.ParticipantID)
	}
	c.unlock()
	c.deps.replay.Offer(ctx, c.recording(), capture)
}

// recording is the ID of this controller's replay recording, started the
// first time the game offers it anything. Only the main goroutine uses it.
func (c *sessionController) recording() string {
	if c.recordingID == "" {
		c.recordingID = c.deps.replay.StartRecording(c.sessionUUID)
	}
	return c.recordingID
}

// stopRecording ends this controller's replay recording, if it has one.
func (c *sessionController) stopRecording() {
	if c.recordingID != "" {
		c.deps.replay.StopRecording(c.recordingID)
		c.recordingID = ""
	}
}
