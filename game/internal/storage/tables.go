package repo

import "time"

type game struct {
	ID           uint
	UUID         string // UUID is the identifier exposed to callers; ID stays internal to storage.
	Name         string // Name is limited to 32 characters.
	Description  string // Description is limited to 255 characters.
	OwnerUUID    string // OwnerUUID references an owner managed outside this schema; it has no local foreign key.
	LogoImageURL string
	Visibility   string // Visibility stores the value of game.VisibilityType as text.
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    *time.Time
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
