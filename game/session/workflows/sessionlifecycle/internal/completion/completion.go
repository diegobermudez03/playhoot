// Package completion detects that a committed RuntimeTurn ended the game
// instance a Session runs, shared by sessionlifecycle's Start/
// AnswerInteraction steps so both react to it the same way.
package completion

import (
	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/session"
)

// Detect scans outputs for an engine.RunCompletedOutput and, if present,
// returns the session.TerminalReason value describing which of the game's
// own outcomes applied. terminated is false when outputs contains no
// RunCompletedOutput, meaning the Session remains RUNNING.
func Detect(outputs []engine.Output) (reason string, terminated bool) {
	for _, output := range outputs {
		run, ok := output.(engine.RunCompletedOutput)
		if !ok {
			continue
		}
		switch run.Outcome.Kind {
		case engine.RunOutcomeCompleted:
			return session.TerminalReasonGameCompleted, true
		case engine.RunOutcomeFailed:
			return session.TerminalReasonGameFailed, true
		case engine.RunOutcomeCancelled:
			return session.TerminalReasonGameCancelled, true
		}
	}
	return "", false
}
