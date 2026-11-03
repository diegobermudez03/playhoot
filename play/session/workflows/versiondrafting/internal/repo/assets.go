package repo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// assetObjectRow is asset content Play may hold, registered before it is
// stored.
type assetObjectRow struct {
	ID           uint
	SHA256       string `gorm:"column:sha256"`
	Size         int64
	MIMEType     string `gorm:"column:mime_type"`
	ObjectKey    string
	CreatedAt    time.Time
	RegisteredAt time.Time
}

func (assetObjectRow) TableName() string { return "play_asset_objects" }

// RegisterAssetObject records content about to be stored, once per hash, so
// every stored object is known and can be cleaned up. Registering it again
// marks it as being stored now, and stored again if a cleanup deleted it.
func (r *Repo) RegisterAssetObject(ctx context.Context, tx *gorm.DB, sha256 string, size int64, mimeType string, objectKey string, registeredAt time.Time) error {
	row := assetObjectRow{SHA256: sha256, Size: size, MIMEType: mimeType, ObjectKey: objectKey, RegisteredAt: registeredAt}
	if err := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		return fmt.Errorf("registering asset object: %s", err)
	}
	if err := tx.WithContext(ctx).Exec(`
		UPDATE play_asset_objects SET registered_at = ?, deleted_at = NULL WHERE sha256 = ?
	`, registeredAt, sha256).Error; err != nil {
		return fmt.Errorf("registering asset object again: %s", err)
	}
	return nil
}

// FindAssetObjectKeys returns the object key of each stored hash it has.
func (r *Repo) FindAssetObjectKeys(ctx context.Context, tx *gorm.DB, hashes []string) (map[string]string, error) {
	keys := make(map[string]string, len(hashes))
	if len(hashes) == 0 {
		return keys, nil
	}
	var rows []struct {
		SHA256    string `gorm:"column:sha256"`
		ObjectKey string
	}
	if err := tx.WithContext(ctx).Raw(`
		SELECT sha256, object_key FROM play_asset_objects WHERE sha256 IN ?
	`, hashes).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("finding asset objects: %s", err)
	}
	for _, row := range rows {
		keys[row.SHA256] = row.ObjectKey
	}
	return keys, nil
}
