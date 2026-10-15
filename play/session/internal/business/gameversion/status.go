// Package gameversion defines what Session Runtime needs to know about the
// state of a game version. These states are this domain's own: whether a
// Game is published or listed anywhere is not its concern.
package gameversion

// Status is how far a game version is from being playable.
type Status string

const (
	// Draft means the version may still change and its scripts are not
	// known to run.
	Draft Status = "DRAFT"

	// Executable means the version is immutable and its scripts are ready
	// to run, so Sessions can be pinned to it.
	Executable Status = "EXECUTABLE"
)

// AllowsSessions reports whether a Session can be pinned to a version in
// this status.
func (s Status) AllowsSessions() bool {
	return s == Executable
}
