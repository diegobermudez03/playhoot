package repo

import "time"

type game struct {
	ID                  uint
	UUID                string // UUID is the identifier exposed to callers; ID stays internal to storage.
	Name                string // Name is limited to 32 characters.
	Description         string // Description is limited to 255 characters.
	OwnerUUID           string // OwnerUUID references an owner managed outside this schema; it has no local foreign key.
	CurrentDefinitionID *uint  // CurrentDefinitionID is nil until the game has a current definition; a definition can be current for only one game.
	LogoImageURL        string
	Visibility          string // Visibility stores the value of management.VisibilityType as text.
	CreatedAt           time.Time
	UpdatedAt           time.Time
	DeletedAt           *time.Time
}

type gameImages struct {
	ID        uint
	GameID    uint
	ImageURL  string
	CreatedAt time.Time
	RemovedAt *time.Time
}

// gameHistory stores a diff-based audit trail of a game's mutable fields. The
// record created alongside the game captures every field; each later record
// stores only the columns whose value changed since the last non-null record
// for that column, leaving the rest nil.
type gameHistory struct {
	ID           uint
	GameID       uint
	Name         *string
	Description  *string
	LogoImageURL *string
	Visibility   *string
	IsPublished  *bool
	CreatedAt    time.Time
}

type gameDefinition struct {
	ID            uint
	UUID          string // UUID is the identifier exposed to callers; ID stays internal to storage.
	GameID        uint
	VersionNumber uint       // VersionNumber increments per game, not globally.
	Script        string     // Script holds the raw JSON-encoded definition script.
	PublishedAt   *time.Time // PublishedAt marks the definition as immutable; once set, changes require a new version.
	CreatedAt     time.Time
	UpdatedAt     time.Time
	DisabledAt    *time.Time
}

// gameDefinitionHistory stores a diff-based audit trail of a game
// definition's mutable fields, using the same non-null-carries-forward
// convention as gameHistory: a record only stores the columns whose value
// changed since the last non-null record for that column.
type gameDefinitionHistory struct {
	ID               uint
	GameDefinitionID uint
	Script           *string
	PublishedAt      *time.Time
	DisabledAt       *time.Time

	CreatedAt time.Time
}
