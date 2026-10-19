// Package endcause defines why a Session reached a terminal state.
package endcause

// Cause is why a Session ended, recorded with its terminal status.
type Cause string

const (
	// GameStartFailure means the game's script failed to start twice, so the
	// Session was aborted without ever running.
	GameStartFailure Cause = "GAME_START_FAILURE"

	// ScriptFailure means the game's script failed twice on the same
	// execution of a running Session.
	ScriptFailure Cause = "SCRIPT_FAILURE"

	// UnrecoverableInfraFailure means the platform could not execute a
	// running Session's game for too long.
	UnrecoverableInfraFailure Cause = "UNRECOVERABLE_INFRA_FAILURE"

	// AllPlayersLeft means no participant of a running Session could play
	// anymore: every one left or was disconnected.
	AllPlayersLeft Cause = "ALL_PLAYERS_LEFT"
)
