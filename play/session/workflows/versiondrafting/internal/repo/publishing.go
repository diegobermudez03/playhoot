package repo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// FrozenArtifacts is what a frozen version keeps from the build of the
// revision it was frozen at.
type FrozenArtifacts struct {
	Manifest        []byte
	ScriptObjectKey string
	ScriptSHA256    string
	ScriptSize      int64
	FrontObjectKey  string
	FrontSHA256     string
	FrontSize       int64
	AspectRatio     string
}

// FreezeVersion moves versionID from status from to to with the artifacts
// it runs with from now on, and reports false when it is no longer in from.
func (r *Repo) FreezeVersion(ctx context.Context, tx *gorm.DB, versionID uint, from string, to string, frozen FrozenArtifacts, at time.Time) (bool, error) {
	result := tx.WithContext(ctx).Exec(`
		UPDATE play_game_versions SET
			status = ?, manifest = ?,
			script_object_key = ?, script_sha256 = ?, script_size = ?,
			frontend_object_key = ?, frontend_sha256 = ?, frontend_size = ?,
			aspect_ratio = ?, executable_at = ?
		WHERE id = ? AND status = ?
	`, to, string(frozen.Manifest),
		frozen.ScriptObjectKey, frozen.ScriptSHA256, frozen.ScriptSize,
		frozen.FrontObjectKey, frozen.FrontSHA256, frozen.FrontSize,
		frozen.AspectRatio, at, versionID, from)
	if result.Error != nil {
		return false, fmt.Errorf("freezing version: %s", result.Error)
	}
	return result.RowsAffected == 1, nil
}

// VersionAsset is one asset an executable version is delivered with.
type VersionAsset struct {
	Key       string
	Type      string
	MIMEType  string
	ObjectKey string
	SHA256    string
	Size      int64
}

type versionAssetRow struct {
	ID            uint
	GameVersionID uint
	Key           string
	Type          string
	MIMEType      string `gorm:"column:mime_type"`
	ObjectKey     string
	SHA256        string `gorm:"column:sha256"`
	Size          int64
	CreatedAt     time.Time
}

func (versionAssetRow) TableName() string { return "play_game_version_assets" }

// CreateVersionAssets records the assets versionID is delivered with.
func (r *Repo) CreateVersionAssets(ctx context.Context, tx *gorm.DB, versionID uint, assets []VersionAsset, at time.Time) error {
	for _, asset := range assets {
		row := versionAssetRow{
			GameVersionID: versionID, Key: asset.Key, Type: asset.Type, MIMEType: asset.MIMEType,
			ObjectKey: asset.ObjectKey, SHA256: asset.SHA256, Size: asset.Size, CreatedAt: at,
		}
		if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
			return fmt.Errorf("recording version asset %s: %s", asset.Key, err)
		}
	}
	return nil
}

// InputRequirements is what an executable version asks of the device it is
// played on.
type InputRequirements struct {
	UsesKeyboard     bool
	UsesPointerHover bool
	UsesPointerPress bool
	Devices          string
}

type inputRequirementsRow struct {
	ID               uint
	GameVersionID    uint
	UsesKeyboard     bool
	UsesPointerHover bool
	UsesPointerPress bool
	Devices          string
	CreatedAt        time.Time
}

func (inputRequirementsRow) TableName() string { return "play_game_version_input_requirements" }

// CreateInputRequirements records what versionID asks of the device.
func (r *Repo) CreateInputRequirements(ctx context.Context, tx *gorm.DB, versionID uint, requirements InputRequirements, at time.Time) error {
	row := inputRequirementsRow{
		GameVersionID:    versionID,
		UsesKeyboard:     requirements.UsesKeyboard,
		UsesPointerHover: requirements.UsesPointerHover,
		UsesPointerPress: requirements.UsesPointerPress,
		Devices:          requirements.Devices,
		CreatedAt:        at,
	}
	if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("recording input requirements: %s", err)
	}
	return nil
}

// SetCurrentVersion makes versionID the version new Sessions of gameID run.
func (r *Repo) SetCurrentVersion(ctx context.Context, tx *gorm.DB, gameID uint, versionID uint, at time.Time) error {
	result := tx.WithContext(ctx).Exec(`
		UPDATE play_games SET current_version_id = ?, updated_at = ? WHERE id = ?
	`, versionID, at, gameID)
	if result.Error != nil {
		return fmt.Errorf("pointing the game at its version: %s", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("game %d not found", gameID)
	}
	return nil
}

// FindGameID returns the id of gameUUID's play_games row, or zero.
func (r *Repo) FindGameID(ctx context.Context, tx *gorm.DB, gameUUID string) (uint, error) {
	var ids []uint
	if err := tx.WithContext(ctx).Raw(`SELECT id FROM play_games WHERE game_uuid = ?`, gameUUID).Scan(&ids).Error; err != nil {
		return 0, fmt.Errorf("finding game: %s", err)
	}
	if len(ids) == 0 {
		return 0, nil
	}
	return ids[0], nil
}

// StoredObject is a stored object a cleanup may delete.
type StoredObject struct {
	ID        uint
	ObjectKey string
}

// unusedAssetObjects selects asset objects still stored, registered before a
// cutoff, that no executable version is delivered with and no revision of a
// version in a given status binds. Arguments: cutoff, status.
const unusedAssetObjects = `
	SELECT o.id, o.object_key
	FROM play_asset_objects o
	WHERE o.deleted_at IS NULL AND o.registered_at < ?
		AND NOT EXISTS (SELECT 1 FROM play_game_version_assets a WHERE a.sha256 = o.sha256)
		AND NOT EXISTS (
			SELECT 1
			FROM play_game_version_revisions r
			JOIN play_game_versions v ON v.id = r.game_version_id
			CROSS JOIN LATERAL jsonb_each(r.assets) bound
			WHERE v.status = ? AND bound.value->>'sha256' = o.sha256
		)
`

// unusedArtifactObjects selects build artifact objects still stored,
// registered before a cutoff, that no version runs with and no build of a
// version in a given status produced. Arguments: cutoff, status.
const unusedArtifactObjects = `
	SELECT o.id, o.object_key
	FROM play_build_artifact_objects o
	WHERE o.deleted_at IS NULL AND o.registered_at < ?
		AND NOT EXISTS (SELECT 1 FROM play_game_versions v WHERE v.script_sha256 = o.sha256 OR v.frontend_sha256 = o.sha256)
		AND NOT EXISTS (
			SELECT 1
			FROM play_game_version_builds b
			JOIN play_game_versions v ON v.id = b.game_version_id
			WHERE v.status = ? AND (b.server_sha256 = o.sha256 OR b.client_sha256 = o.sha256)
		)
`

// FindUnusedAssetObjects returns up to limit asset objects nothing uses: no
// executable version, and no revision of a version in status.
func (r *Repo) FindUnusedAssetObjects(ctx context.Context, tx *gorm.DB, cutoff time.Time, status string, limit int) ([]StoredObject, error) {
	return findStored(ctx, tx, unusedAssetObjects+`ORDER BY o.id LIMIT ?`, "unused asset objects", cutoff, status, limit)
}

// LockUnusedAssetObject locks objectID until tx ends and returns it when it
// is still unused, or nil.
func (r *Repo) LockUnusedAssetObject(ctx context.Context, tx *gorm.DB, objectID uint, cutoff time.Time, status string) (*StoredObject, error) {
	return lockStored(ctx, tx, unusedAssetObjects+`AND o.id = ? FOR UPDATE OF o`, "unused asset object", cutoff, status, objectID)
}

// MarkAssetObjectDeleted records that objectID's content was deleted.
func (r *Repo) MarkAssetObjectDeleted(ctx context.Context, tx *gorm.DB, objectID uint, at time.Time) error {
	return markDeleted(ctx, tx, `UPDATE play_asset_objects SET deleted_at = ? WHERE id = ? AND deleted_at IS NULL`, objectID, at)
}

// FindUnusedArtifactObjects returns up to limit build artifact objects
// nothing uses: no version runs them and no build of a version in status
// produced them.
func (r *Repo) FindUnusedArtifactObjects(ctx context.Context, tx *gorm.DB, cutoff time.Time, status string, limit int) ([]StoredObject, error) {
	return findStored(ctx, tx, unusedArtifactObjects+`ORDER BY o.id LIMIT ?`, "unused artifact objects", cutoff, status, limit)
}

// LockUnusedArtifactObject locks objectID until tx ends and returns it when
// it is still unused, or nil.
func (r *Repo) LockUnusedArtifactObject(ctx context.Context, tx *gorm.DB, objectID uint, cutoff time.Time, status string) (*StoredObject, error) {
	return lockStored(ctx, tx, unusedArtifactObjects+`AND o.id = ? FOR UPDATE OF o`, "unused artifact object", cutoff, status, objectID)
}

// MarkArtifactObjectDeleted records that objectID's content was deleted.
func (r *Repo) MarkArtifactObjectDeleted(ctx context.Context, tx *gorm.DB, objectID uint, at time.Time) error {
	return markDeleted(ctx, tx, `UPDATE play_build_artifact_objects SET deleted_at = ? WHERE id = ? AND deleted_at IS NULL`, objectID, at)
}

func findStored(ctx context.Context, tx *gorm.DB, query string, what string, args ...any) ([]StoredObject, error) {
	var objects []StoredObject
	if err := tx.WithContext(ctx).Raw(query, args...).Scan(&objects).Error; err != nil {
		return nil, fmt.Errorf("finding %s: %s", what, err)
	}
	return objects, nil
}

func lockStored(ctx context.Context, tx *gorm.DB, query string, what string, args ...any) (*StoredObject, error) {
	objects, err := findStored(ctx, tx, query, what, args...)
	if err != nil || len(objects) == 0 {
		return nil, err
	}
	return &objects[0], nil
}

func markDeleted(ctx context.Context, tx *gorm.DB, query string, objectID uint, at time.Time) error {
	if err := tx.WithContext(ctx).Exec(query, at, objectID).Error; err != nil {
		return fmt.Errorf("marking object deleted: %s", err)
	}
	return nil
}
