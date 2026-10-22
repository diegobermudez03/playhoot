package liveconnection

import (
	"context"
	"fmt"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/play/runtime"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

// gameFaultRecorder keeps game fault reports for auditing.
type gameFaultRecorder interface {
	RecordGameFault(ctx context.Context, sessionUUID string, fault sessionpkg.GameFault) (sessionpkg.RecordGameFaultOutcome, error)
}

// gameFaultKinds maps the Runtime's fault kinds to the durable ones.
var gameFaultKinds = map[runtime.GameFaultKind]sessionpkg.GameFaultKind{
	runtime.GameFaultArtifactUnreachable:      sessionpkg.GameFaultArtifactUnreachable,
	runtime.GameFaultArtifactHashMismatch:     sessionpkg.GameFaultArtifactHashMismatch,
	runtime.GameFaultReadyTimeout:             sessionpkg.GameFaultReadyTimeout,
	runtime.GameFaultUnsupportedBridgeVersion: sessionpkg.GameFaultUnsupportedBridgeVersion,
	runtime.GameFaultContainerViolation:       sessionpkg.GameFaultContainerViolation,
	runtime.GameFaultBridgeAbuse:              sessionpkg.GameFaultBridgeAbuse,
}

const (
	gameFaultDropped = "dropped"
	gameFaultFailed  = "failed"
)

var (
	gameFaultKindLabel = metrics.NewLabel("kind",
		string(runtime.GameFaultArtifactUnreachable), string(runtime.GameFaultArtifactHashMismatch),
		string(runtime.GameFaultReadyTimeout), string(runtime.GameFaultUnsupportedBridgeVersion),
		string(runtime.GameFaultContainerViolation), string(runtime.GameFaultBridgeAbuse))
	gameFaultOutcomeLabel = metrics.NewLabel("outcome",
		string(sessionpkg.GameFaultRecorded), string(sessionpkg.GameFaultNotRecorded), gameFaultDropped, gameFaultFailed)

	gameFaultsReported = metrics.NewCounter(
		"play_runtime_game_faults_reported_total",
		"Game frontend faults players' clients reported, by kind and outcome: recorded for audit, not_recorded (the Session no longer runs), dropped (not from a current participant of the running game) or failed (storing failed).",
		gameFaultKindLabel, gameFaultOutcomeLabel,
	)
)

// reportGameFault records fault for player when they play the running game
// from their current connection, on the caller's goroutine so the game never
// waits for it. It never changes the game, and a failure is only logged: a
// report is audit evidence, not an operation the client depends on.
func (c *sessionController) reportGameFault(ctx context.Context, player *connection, fault runtime.GameFault) {
	kind, known := gameFaultKinds[fault.Kind]
	if !known {
		return
	}
	s := c.lock()
	current := s.phase == phaseRunning && player.participantID != "" && s.players[player.userUUID] == player
	participantID := player.participantID
	c.unlock()
	kindLabel := gameFaultKindLabel.Value(string(fault.Kind))
	if !current {
		gameFaultsReported.Inc(ctx, kindLabel, gameFaultOutcomeLabel.Value(gameFaultDropped))
		return
	}
	outcome, err := c.deps.faults.RecordGameFault(ctx, c.sessionUUID, sessionpkg.GameFault{
		UserUUID:      player.userUUID,
		ParticipantID: participantID,
		Kind:          kind,
		Detail:        fault.Detail,
	})
	if err != nil {
		logging.LogError(ctx, fmt.Errorf("recording a game fault: %w", err))
		gameFaultsReported.Inc(ctx, kindLabel, gameFaultOutcomeLabel.Value(gameFaultFailed))
		return
	}
	gameFaultsReported.Inc(ctx, kindLabel, gameFaultOutcomeLabel.Value(string(outcome)))
}
