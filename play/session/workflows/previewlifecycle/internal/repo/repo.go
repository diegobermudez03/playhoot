// Package repo is previewlifecycle's narrow persistence layer. It reports
// persisted facts and performs the mutations the Manager requests; it
// decides no business policy itself.
package repo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Repo is previewlifecycle's persistence implementation. Every method takes
// the transaction the Manager opened; Repo never opens or commits one.
type Repo struct {
	db *gorm.DB
}

// New constructs a Repo bound to db.
func New(db *gorm.DB) *Repo {
	return &Repo{db: db}
}

// GetDB satisfies utils.DBServicer, so the Manager opens its transactions on
// the handle Repo owns.
func (r *Repo) GetDB() *gorm.DB {
	return r.db
}

// LockVersion locks versionUUID's row until tx ends, serializing the previews
// of one version, and returns its id, or nil when there is no such version.
func (r *Repo) LockVersion(ctx context.Context, tx *gorm.DB, versionUUID string) (*uint, error) {
	var id uint
	result := tx.WithContext(ctx).Raw(`SELECT id FROM play_game_versions WHERE uuid = ? FOR UPDATE`, versionUUID).Scan(&id)
	if result.Error != nil {
		return nil, fmt.Errorf("locking game version: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &id, nil
}

// FindRunningPreview returns the uuid of versionID's running preview, or
// nil when none runs.
func (r *Repo) FindRunningPreview(ctx context.Context, tx *gorm.DB, versionID uint) (*string, error) {
	var previewUUID string
	result := tx.WithContext(ctx).Raw(`SELECT uuid FROM play_previews WHERE game_version_id = ? AND ended_at IS NULL`, versionID).Scan(&previewUUID)
	if result.Error != nil {
		return nil, fmt.Errorf("finding running preview: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &previewUUID, nil
}

// previewRow is one play_previews row as it is created.
type previewRow struct {
	ID            uint `gorm:"primaryKey"`
	UUID          string
	GameVersionID uint
	Revision      int64
	OwnerUserUUID string
	Participants  int
	CreatedAt     time.Time
	ExpiresAt     time.Time
}

func (previewRow) TableName() string { return "play_previews" }

// NewPreview is a preview to create.
type NewPreview struct {
	UUID          string
	GameVersionID uint
	Revision      int64
	OwnerUserUUID string
	Participants  int
	CreatedAt     time.Time
	ExpiresAt     time.Time
}

// CreatePreview inserts preview.
func (r *Repo) CreatePreview(ctx context.Context, tx *gorm.DB, preview NewPreview) error {
	row := previewRow{
		UUID: preview.UUID, GameVersionID: preview.GameVersionID, Revision: preview.Revision, OwnerUserUUID: preview.OwnerUserUUID,
		Participants: preview.Participants, CreatedAt: preview.CreatedAt, ExpiresAt: preview.ExpiresAt,
	}
	if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("creating preview: %s", err)
	}
	return nil
}

// EndPreview records that previewUUID ended at endedAt for reason, and
// reports whether it was still running.
func (r *Repo) EndPreview(ctx context.Context, tx *gorm.DB, previewUUID string, reason string, endedAt time.Time) (bool, error) {
	result := tx.WithContext(ctx).Exec(`UPDATE play_previews SET ended_at = ?, end_reason = ? WHERE uuid = ? AND ended_at IS NULL`, endedAt, reason, previewUUID)
	if result.Error != nil {
		return false, fmt.Errorf("ending preview: %s", result.Error)
	}
	return result.RowsAffected == 1, nil
}

// Preview is a preview as it is stored.
type Preview struct {
	UUID          string
	OwnerUserUUID string
	Revision      int64
	Participants  int
	CreatedAt     time.Time
	ExpiresAt     time.Time
	EndedAt       *time.Time
	EndReason     *string
}

// FindPreview returns previewUUID, or nil when there is no such preview.
func (r *Repo) FindPreview(ctx context.Context, tx *gorm.DB, previewUUID string) (*Preview, error) {
	var preview Preview
	result := tx.WithContext(ctx).Raw(`
		SELECT uuid, owner_user_uuid, revision, participants, created_at, expires_at, ended_at, end_reason
		FROM play_previews WHERE uuid = ?
	`, previewUUID).Scan(&preview)
	if result.Error != nil {
		return nil, fmt.Errorf("finding preview: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &preview, nil
}
