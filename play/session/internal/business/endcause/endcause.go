// Package endcause defines why a Session reached a terminal state, and which
// terminal state each cause leads to from which states.
package endcause

import "github.com/diegobermudez03/playhoot/play/session/internal/business/sessionstatus"

// Cause is why a Session ended, recorded with its terminal status.
type Cause string

const (
	// GameFinished means the game itself ended the Session.
	GameFinished Cause = "GAME_FINISHED"

	// HostCancelled means the Session's creator cancelled it.
	HostCancelled Cause = "HOST_CANCELLED"

	// AllPlayersLeft means no participant of a running Session could play
	// anymore: every one left or was disconnected.
	AllPlayersLeft Cause = "ALL_PLAYERS_LEFT"

	// PlatformTerminated means the platform ended the Session by its own
	// decision, such as a usage limit.
	PlatformTerminated Cause = "PLATFORM_TERMINATED"

	// LobbyExpired means the Session did not start before its lobby expired.
	LobbyExpired Cause = "LOBBY_EXPIRED"

	// InactivityTimeout means nobody played a running Session for too long.
	InactivityTimeout Cause = "INACTIVITY_TIMEOUT"

	// MaxLifetime means the Session reached the most time any Session may
	// live, whatever its activity.
	MaxLifetime Cause = "MAX_LIFETIME"

	// GameStartFailure means the game's script failed to start twice, so the
	// Session was aborted without ever running.
	GameStartFailure Cause = "GAME_START_FAILURE"

	// ScriptFailure means the game's script failed twice on the same
	// execution of a running Session.
	ScriptFailure Cause = "SCRIPT_FAILURE"

	// UnrecoverableInfraFailure means the platform could not execute a
	// running Session's game for too long.
	UnrecoverableInfraFailure Cause = "UNRECOVERABLE_INFRA_FAILURE"

	// OwnerLost means a running Session went too long without a live owner
	// and nobody came back to recover it.
	OwnerLost Cause = "OWNER_LOST"
)

// Ending is the terminal status a cause leads to, and the statuses a Session
// may be in for it to apply.
type Ending struct {
	Status sessionstatus.Status
	From   []sessionstatus.Status
}

var (
	lobbyOrRunning = []sessionstatus.Status{sessionstatus.Lobby, sessionstatus.Running}
	running        = []sessionstatus.Status{sessionstatus.Running}
	lobby          = []sessionstatus.Status{sessionstatus.Lobby}
)

// endings maps each cause a live Session may end for to its ending.
var endings = map[Cause]Ending{
	GameFinished:              {Status: sessionstatus.Finished, From: running},
	HostCancelled:             {Status: sessionstatus.Cancelled, From: lobbyOrRunning},
	AllPlayersLeft:            {Status: sessionstatus.Cancelled, From: running},
	PlatformTerminated:        {Status: sessionstatus.Cancelled, From: lobbyOrRunning},
	LobbyExpired:              {Status: sessionstatus.Expired, From: lobby},
	InactivityTimeout:         {Status: sessionstatus.Expired, From: running},
	MaxLifetime:               {Status: sessionstatus.Expired, From: lobbyOrRunning},
	GameStartFailure:          {Status: sessionstatus.Aborted, From: lobby},
	ScriptFailure:             {Status: sessionstatus.Aborted, From: running},
	UnrecoverableInfraFailure: {Status: sessionstatus.Aborted, From: running},
	OwnerLost:                 {Status: sessionstatus.Aborted, From: running},
}

// EndingOf returns cause's ending; false when a live Session cannot end for
// it.
func EndingOf(cause Cause) (Ending, bool) {
	ending, found := endings[cause]
	return ending, found
}
