// Package setting names the account settings whose changes Identity keeps in
// its settings history: what a person chose, when, and what it replaced.
package setting

// The settings a change row may be about.
const (
	// Mode is which face of the account the person uses.
	Mode = "mode"

	// Language is the interface language they read in.
	Language = "language"
)
