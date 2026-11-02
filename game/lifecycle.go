package game

import "time"

// CreatedGame is a Game CreateGame created: unpublished, with its first
// edition open, built in the Play version EditionVersionUUID.
type CreatedGame struct {
	GameUUID           string
	EditionVersionUUID string
	CreatedAt          time.Time
}

// EditableVersionOutcome is what FindEditableVersion decided.
type EditableVersionOutcome string

const (
	// EditableVersionFound means the user may edit the Game's open edition,
	// built in EditableVersion.VersionUUID.
	EditableVersionFound EditableVersionOutcome = "found"
	// EditableVersionNotFound means no Game the user owns has that uuid: it
	// does not exist, was deleted, or belongs to someone else.
	EditableVersionNotFound EditableVersionOutcome = "not_found"
	// EditableVersionNotEditable means the user owns the Game but it has no
	// edition open for editing.
	EditableVersionNotEditable EditableVersionOutcome = "not_editable"
)

// EditableVersion is the Play version a user may edit for a Game, set only
// when Outcome is EditableVersionFound.
type EditableVersion struct {
	Outcome     EditableVersionOutcome
	VersionUUID string
}
