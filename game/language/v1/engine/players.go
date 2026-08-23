package engine

// PlayerPolicy is the compiled player-count contract for one game version.
// It is carried from program.Definition so application/session code can use
// the compiled Program as its lobby source of truth.
type PlayerPolicy struct {
	Min int
	Max int
}
