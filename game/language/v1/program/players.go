package program

// PlayerPolicy declares the authored player-count contract for one game
// definition. The session layer should read this before starting a room and
// use it as the source of truth for lobby capacity.
type PlayerPolicy struct {
	// Min is the smallest number of users allowed to start a game.
	// Zero means the definition did not declare a lower bound.
	Min int

	// Max is the largest number of users allowed to start a game.
	// Zero means the definition did not declare an upper bound.
	Max int
}
