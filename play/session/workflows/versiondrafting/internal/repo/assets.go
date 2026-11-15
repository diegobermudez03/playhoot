package repo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/diegobermudez03/playhoot/play/session/internal/business/gameversion"
)

// assetObjectRow is asset content Play may hold, registered before it is
// stored.
type assetObjectRow struct {
	ID           uint
	SHA256       string `gorm:"column:sha256"`
	Size         int64
	MIMEType     string `gorm:"column:mime_type"`
	ObjectKey    string
	Width        *int
	Height       *int
	DurationMs   *int64
	CreatedAt    time.Time
	RegisteredAt time.Time
}

func (assetObjectRow) TableName() string { return "play_asset_objects" }

// RegisterAssetObject records content about to be stored, once per hash, so
// every stored object is known and can be cleaned up. Registering it again
// marks it as being stored now, and stored again if a cleanup deleted it.
func (r *Repo) RegisterAssetObject(ctx context.Context, tx *gorm.DB, sha256 string, size int64, mimeType string, media gameversion.Media, objectKey string, registeredAt time.Time) error {
	row := assetObjectRow{SHA256: sha256, Size: size, MIMEType: mimeType, ObjectKey: objectKey, RegisteredAt: registeredAt,
		Width: positive(media.Width), Height: positive(media.Height), DurationMs: positive(media.DurationMs)}
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

// StoredAssetObject is a registered object's key and what its content
// measures.
type StoredAssetObject struct {
	ObjectKey string
	Media     gameversion.Media
}

// FindAssetObjects returns each registered hash's object key and measures.
func (r *Repo) FindAssetObjects(ctx context.Context, tx *gorm.DB, hashes []string) (map[string]StoredAssetObject, error) {
	objects := make(map[string]StoredAssetObject, len(hashes))
	if len(hashes) == 0 {
		return objects, nil
	}
	var rows []assetObjectRow
	if err := tx.WithContext(ctx).Raw(`
		SELECT sha256, object_key, width, height, duration_ms FROM play_asset_objects WHERE sha256 IN ?
	`, hashes).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("finding asset objects: %s", err)
	}
	for _, row := range rows {
		object := StoredAssetObject{ObjectKey: row.ObjectKey}
		if row.Width != nil && row.Height != nil {
			object.Media.Width, object.Media.Height = *row.Width, *row.Height
		}
		if row.DurationMs != nil {
			object.Media.DurationMs = *row.DurationMs
		}
		objects[row.SHA256] = object
	}
	return objects, nil
}

// positive is value, or nil when it is not above zero (unknown).
func positive[T int | int64](value T) *T {
	if value <= 0 {
		return nil
	}
	return &value
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
