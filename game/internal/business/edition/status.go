// Package edition holds what an edition of a Game is: the version of it being
// built, or one that was published.
package edition

// Status is where an edition is in its lifecycle.
type Status string

const (
	// Editing is an edition open for changes; a Game has at most one.
	Editing Status = "EDITING"
	// Published is an edition that was published; it never changes again.
	Published Status = "PUBLISHED"
)
