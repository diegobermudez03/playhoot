// Package endcause defines why a Session reached a terminal state.
package endcause

// Cause is why a Session ended, recorded with its terminal status.
type Cause string

const (
	// GameStartFailure means the game's script failed to start twice, so the
	// Session was aborted without ever running.
	GameStartFailure Cause = "GAME_START_FAILURE"
)
