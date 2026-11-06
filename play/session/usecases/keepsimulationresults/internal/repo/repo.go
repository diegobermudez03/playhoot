// Package repo is keepsimulationresults' persistence: play_simulation_results.
package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Repo is keepsimulationresults' persistence implementation.
type Repo struct {
	db *gorm.DB
}

// New constructs a Repo bound to db.
func New(db *gorm.DB) *Repo {
	return &Repo{db: db}
}

type resultRow struct {
	ID            uint
	UUID          string
	GameVersionID uint
	Result        string `gorm:"type:jsonb"`
	CreatedAt     time.Time
	ExpiresAt     time.Time
}

func (resultRow) TableName() string { return "play_simulation_results" }

// FindVersionID returns the id of the version versionUUID names, or false.
func (r *Repo) FindVersionID(ctx context.Context, versionUUID string) (uint, bool, error) {
	var ids []uint
	if err := r.db.WithContext(ctx).Raw(`SELECT id FROM play_game_versions WHERE uuid = ?`, versionUUID).Scan(&ids).Error; err != nil {
		return 0, false, fmt.Errorf("finding version: %s", err)
	}
	if len(ids) == 0 {
		return 0, false, nil
	}
	return ids[0], true, nil
}

// Insert keeps result as resultUUID of versionID until expiresAt.
func (r *Repo) Insert(ctx context.Context, resultUUID string, versionID uint, result json.RawMessage, createdAt time.Time, expiresAt time.Time) error {
	row := resultRow{UUID: resultUUID, GameVersionID: versionID, Result: string(result), CreatedAt: createdAt, ExpiresAt: expiresAt}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("keeping simulation result: %s", err)
	}
	return nil
}

// ClearExpired drops the result of every record expired at now (the record
// stays, for counting) and deletes the records created before forgetBefore.
func (r *Repo) ClearExpired(ctx context.Context, now time.Time, forgetBefore time.Time) error {
	if err := r.db.WithContext(ctx).Exec(`
		UPDATE play_simulation_results SET result = NULL WHERE expires_at <= ? AND result IS NOT NULL
	`, now).Error; err != nil {
		return fmt.Errorf("clearing expired simulation results: %s", err)
	}
	if err := r.db.WithContext(ctx).Exec(`DELETE FROM play_simulation_results WHERE created_at < ?`, forgetBefore).Error; err != nil {
		return fmt.Errorf("deleting old simulation records: %s", err)
	}
	return nil
}

// Find returns the result resultUUID names when it belongs to versionUUID
// and has not expired at now, or false.
func (r *Repo) Find(ctx context.Context, resultUUID string, versionUUID string, now time.Time) (json.RawMessage, bool, error) {
	var rows []struct{ Result []byte }
	if err := r.db.WithContext(ctx).Raw(`
		SELECT s.result FROM play_simulation_results s
		JOIN play_game_versions v ON v.id = s.game_version_id
		WHERE s.uuid = ? AND v.uuid = ? AND s.expires_at > ? AND s.result IS NOT NULL
	`, resultUUID, versionUUID, now).Scan(&rows).Error; err != nil {
		return nil, false, fmt.Errorf("finding simulation result: %s", err)
	}
	if len(rows) == 0 {
		return nil, false, nil
	}
	return rows[0].Result, true, nil
}

// CountSince counts versionUUID's simulations recorded after since, expired
// ones included: what a cap on simulations per version counts.
func (r *Repo) CountSince(ctx context.Context, versionUUID string, since time.Time) (int, error) {
	var count int
	if err := r.db.WithContext(ctx).Raw(`
		SELECT COUNT(*) FROM play_simulation_results s
		JOIN play_game_versions v ON v.id = s.game_version_id
		WHERE v.uuid = ? AND s.created_at > ?
	`, versionUUID, since).Scan(&count).Error; err != nil {
		return 0, fmt.Errorf("counting simulation results: %s", err)
	}
	return count, nil
}
