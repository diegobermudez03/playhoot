package liveconnection

import (
	"context"
	"encoding/json"
	"fmt"
	"unicode/utf8"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

// Bounds of what an invalid game data record says, so its detail always fits
// sessionpkg.MaxGameFaultDetailBytes even when every byte needs escaping.
const (
	maxInvalidReasonBytes  = 200
	maxInvalidCommandBytes = 64
)

var (
	invalidGameDataKindLabel = metrics.NewLabel("kind",
		string(sessionpkg.GameFaultInvalidSample), string(sessionpkg.GameFaultInvalidCommand))
	invalidGameDataOutcomeLabel = metrics.NewLabel("outcome",
		string(sessionpkg.GameFaultRecorded), string(sessionpkg.GameFaultNotRecorded), gameFaultFailed)

	invalidGameDataRecorded = metrics.NewCounter(
		"play_runtime_invalid_game_data_total",
		"SAMPLEs and COMMANDs a game's frontend sent that do not match the game's declarations, each ending the player's connection, by kind and by how recording it went: recorded for audit, not_recorded (the Session no longer runs) or failed (storing failed).",
		invalidGameDataKindLabel, invalidGameDataOutcomeLabel,
	)
)

// invalidSampleDetail says why a SAMPLE's inputs were refused.
func invalidSampleDetail(reason error) []byte {
	detail, _ := json.Marshal(struct {
		Reason string `json:"reason"`
	}{Reason: truncateUTF8(reason.Error(), maxInvalidReasonBytes)})
	return detail
}

// invalidCommandDetail says which command was refused and why.
func invalidCommandDetail(name string, reason error) []byte {
	detail, _ := json.Marshal(struct {
		Command string `json:"command"`
		Reason  string `json:"reason"`
	}{Command: truncateUTF8(name, maxInvalidCommandBytes), Reason: truncateUTF8(reason.Error(), maxInvalidReasonBytes)})
	return detail
}

// truncateUTF8 is at most the first max bytes of s, never splitting a rune.
func truncateUTF8(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

// recordInvalidGameData records, as a SERVER fault of the running game, that
// participantID's frontend sent data the game does not declare, before the
// caller ends the connection. The caller must not hold the controller's lock:
// it waits for storage on the caller's goroutine, so the game never waits for
// it. A failure is only logged: the connection ends all the same.
func (c *sessionController) recordInvalidGameData(ctx context.Context, userUUID, participantID string, kind sessionpkg.GameFaultKind, detail []byte) {
	outcome, err := c.deps.faults.RecordGameFault(ctx, c.sessionUUID, sessionpkg.GameFault{
		UserUUID:      userUUID,
		ParticipantID: participantID,
		Source:        sessionpkg.GameFaultFromServer,
		Kind:          kind,
		Detail:        detail,
	})
	kindLabel := invalidGameDataKindLabel.Value(string(kind))
	if err != nil {
		logging.LogError(ctx, fmt.Errorf("recording invalid game data (%s): %w", kind, err))
		invalidGameDataRecorded.Inc(ctx, kindLabel, invalidGameDataOutcomeLabel.Value(gameFaultFailed))
		return
	}
	invalidGameDataRecorded.Inc(ctx, kindLabel, invalidGameDataOutcomeLabel.Value(string(outcome)))
}
