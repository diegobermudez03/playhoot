// Package completion detects that a committed RuntimeTurn's parsed platform
// Commands ended the game instance a Session runs, shared by every
// RuntimeTurn-producing sessionlifecycle step so each reacts to it the same
// way.
package completion

import (
	"github.com/diegobermudez03/playhoot/session"
	"github.com/diegobermudez03/playhoot/session/workflows/sessionlifecycle/internal/platform"
)

// Detect scans commands (already parsed via platform.ParseCommand) for a
// SessionComplete/SessionFail Command and, if present, returns the
// session.TerminalReason value describing which applied. terminated is
// false when commands contains neither, meaning the Session remains
// RUNNING. A script may request at most one of these per Turn; if it
// somehow requested both, the first one found wins - this is a defensive
// tie-break, not a documented authoring contract.
func Detect(commands []platform.Command) (reason string, terminated bool) {
	for _, command := range commands {
		switch command.(type) {
		case *platform.SessionComplete:
			return session.TerminalReasonGameCompleted, true
		case *platform.SessionFail:
			return session.TerminalReasonGameFailed, true
		}
	}
	return "", false
}
