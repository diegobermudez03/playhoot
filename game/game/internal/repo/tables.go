package repo

import "time"

type game struct {
	ID               uint   // PK, incremental
	UUID             string // UUID, exported one
	Name             string // string, max 32 chars
	Description      string // string, max 255 chars
	OwnerUUID        string // extenral reference
	CurrentVersionID *uint  // Reference to game_versions
	LogoImageURL     string
	Visibility       string // public, team, direct share only, private, etc...
	IsPublished      bool   // if false then its draft
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        *time.Time
}

type gameImages struct {
	ID        uint   // PK, incremental
	GameID    uint   // Reference to games table
	ImageURL  string // url string
	CreatedAt time.Time
	RemovedAt *time.Time
}

// table works as a snapshot of the changed values:
// basically, a new history record will only record the columns which value changed from the last previous non null value
// so the first history record is created at the same time the original record is created, it contains all the values
// then for each new change, we'll compare all new column values with the latest history non null value for those columns, if value changed
// then new record will store those columns new valuees
type gameHistory struct {
	ID           uint    // PK, incremental
	GameID       uint    // reference to games table
	Name         *string // Name at the given time, nullable
	Description  *string // description at that given time, nullable
	LogoImageURL *string // url at that given time, nullable
	Visibility   *string // visibility at the given time, nullable
	IsPublished  *bool   // if it was published at the given time, nullable
	CreatedAt    time.Time
}

type gameVersion struct {
	ID            uint       // PK, incremental
	UUID          string     // exported uuid
	GameID        uint       // reference to the game ID
	VersionNumber uint       // incremental for each new version of the game
	Script        string     // raw text with the script (as json)
	PublishedAt   *time.Time // once the version is published then the version cannot be updated, new version must be created
	CreatedAt     time.Time
	UpdatedAt     time.Time
	DisabledAt    *time.Time
}

// basically, a new history record will only record the columns which value changed from the last previous non null value
// so the first history record is created at the same time the original record is created, it contains all the values
// then for each new change, we'll compare all new column values with the latest history non null value for those columns, if value changed
// then new record will store those columns new valuees
type gameVersionHistory struct {
	ID            uint
	GameVersionID uint
	Script        *string    // nullable
	PublishedAt   *time.Time // nullable
	DisabledAt    *time.Time // nullable

	CreatedAt time.Time
}
