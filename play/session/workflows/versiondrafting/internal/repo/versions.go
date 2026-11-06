package repo

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Version is a game version as drafting sees it. CurrentRevision is nil for a
// version without authored sources.
type Version struct {
	ID              uint
	UUID            string
	GameUUID        string
	Status          string
	ScriptProfile   string
	CurrentRevision *int64
	// Languages is the JSON list of the languages an executable version's
	// game is written in; nil for a DRAFT.
	Languages []byte
}

const selectVersion = `
	SELECT v.id, v.uuid, g.game_uuid, v.status, v.script_profile, v.current_revision, v.languages
	FROM play_game_versions v
	JOIN play_games g ON g.id = v.game_id
`

// FindVersion returns the version versionUUID names, or nil.
func (r *Repo) FindVersion(ctx context.Context, tx *gorm.DB, versionUUID string) (*Version, error) {
	var version Version
	result := tx.WithContext(ctx).Raw(selectVersion+`WHERE v.uuid = ?`, versionUUID).Scan(&version)
	if result.Error != nil {
		return nil, fmt.Errorf("finding version: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &version, nil
}

// LockGame returns the id of gameUUID's play_games row, inserting it when
// absent, and locks it until tx ends, so drafts of one game are created one
// at a time.
func (r *Repo) LockGame(ctx context.Context, tx *gorm.DB, gameUUID string) (uint, error) {
	game := gameRow{GameUUID: gameUUID}
	if err := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&game).Error; err != nil {
		return 0, fmt.Errorf("inserting game: %s", err)
	}
	var gameID uint
	if err := tx.WithContext(ctx).Raw(`
		SELECT id FROM play_games WHERE game_uuid = ? FOR UPDATE
	`, gameUUID).Scan(&gameID).Error; err != nil {
		return 0, fmt.Errorf("locking game: %s", err)
	}
	return gameID, nil
}

// FindVersionOfGameInStatus returns gameID's version in status, or nil; a
// game has at most one DRAFT.
func (r *Repo) FindVersionOfGameInStatus(ctx context.Context, tx *gorm.DB, gameID uint, status string) (*Version, error) {
	var version Version
	result := tx.WithContext(ctx).Raw(selectVersion+`WHERE v.game_id = ? AND v.status = ?`, gameID, status).Scan(&version)
	if result.Error != nil {
		return nil, fmt.Errorf("finding the game's draft: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &version, nil
}

// gameRow is the play_games columns a new game sets.
type gameRow struct {
	ID       uint
	GameUUID string
}

func (gameRow) TableName() string { return "play_games" }

// versionRow is the play_game_versions columns a new authored version sets;
// manifest and locators come with its first build.
type versionRow struct {
	ID              uint
	UUID            string
	GameID          uint
	Status          string
	ScriptProfile   string
	CurrentRevision *int64
}

func (versionRow) TableName() string { return "play_game_versions" }

// CreateVersion inserts a version of gameID in status with no manifest yet,
// already at revision, and returns its id.
func (r *Repo) CreateVersion(ctx context.Context, tx *gorm.DB, gameID uint, versionUUID string, status string, scriptProfile string, revision int64) (uint, error) {
	row := versionRow{UUID: versionUUID, GameID: gameID, Status: status, ScriptProfile: scriptProfile, CurrentRevision: &revision}
	if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, fmt.Errorf("creating version: %s", err)
	}
	return row.ID, nil
}

// AdvanceRevision moves a version in status from revision base to base+1, and
// reports false when it is no longer in status at base.
func (r *Repo) AdvanceRevision(ctx context.Context, tx *gorm.DB, versionID uint, status string, base int64) (bool, error) {
	result := tx.WithContext(ctx).Exec(`
		UPDATE play_game_versions SET current_revision = current_revision + 1
		WHERE id = ? AND status = ? AND current_revision = ?
	`, versionID, status, base)
	if result.Error != nil {
		return false, fmt.Errorf("advancing revision: %s", result.Error)
	}
	return result.RowsAffected == 1, nil
}
