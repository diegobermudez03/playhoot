// Package repo is versionbuilding's narrow persistence layer. It reports
// persisted facts and performs the mutations the Manager requests; it
// decides no business policy itself.
package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/diegobermudez03/playhoot/play/session/internal/business/draftsources"
)

// Repo is versionbuilding's persistence implementation. Mutations take the
// transaction the Manager opened; Repo never opens or commits one.
type Repo struct {
	db *gorm.DB
}

// New constructs a Repo bound to db.
func New(db *gorm.DB) *Repo {
	return &Repo{db: db}
}

// GetDB satisfies utils.DBServicer.
func (r *Repo) GetDB() *gorm.DB {
	return r.db
}

// ClaimedBuild is a build the worker took to run.
type ClaimedBuild struct {
	ID            uint
	UUID          string
	GameVersionID uint
	VersionUUID   string
	Revision      int64
	CreatedAt     time.Time
}

// Artifacts is what a finished build decided, written once with its end.
// Locator fields are set only when it succeeded.
type Artifacts struct {
	InputsSHA256     string
	ToolchainVersion string
	Diagnostics      []byte
	ServerObjectKey  *string
	ServerSHA256     *string
	ServerSize       *int64
	ClientObjectKey  *string
	ClientSHA256     *string
	ClientSize       *int64
	Manifest         []byte
	AspectRatio      *string
	ReusedBuildID    *uint
}

// ReusableBuild is a succeeded build whose artifacts another build with the
// same inputs may take.
type ReusableBuild struct {
	ID              uint
	ServerObjectKey string
	ServerSHA256    string
	ServerSize      int64
	ClientObjectKey string
	ClientSHA256    string
	ClientSize      int64
	Manifest        []byte
	AspectRatio     string
}

type buildStatusChangeRow struct {
	ID         uint
	BuildID    uint
	FromStatus *string
	ToStatus   string
}

func (buildStatusChangeRow) TableName() string { return "play_game_version_build_status_changes" }

type artifactObjectRow struct {
	ID          uint
	SHA256      string `gorm:"column:sha256"`
	Size        int64
	ContentType string
	ObjectKey   string
}

func (artifactObjectRow) TableName() string { return "play_build_artifact_objects" }

func (r *Repo) changeStatus(ctx context.Context, tx *gorm.DB, buildID uint, from string, to string) (bool, error) {
	result := tx.WithContext(ctx).Exec(`UPDATE play_game_version_builds SET status = ? WHERE id = ? AND status = ?`, to, buildID, from)
	if result.Error != nil {
		return false, fmt.Errorf("changing build status: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return false, nil
	}
	if err := tx.WithContext(ctx).Create(&buildStatusChangeRow{BuildID: buildID, FromStatus: &from, ToStatus: to}).Error; err != nil {
		return false, fmt.Errorf("recording build status change: %s", err)
	}
	return true, nil
}

// RequeueAll moves every build in from back to to, each change kept in the
// status history, and returns how many it moved.
func (r *Repo) RequeueAll(ctx context.Context, tx *gorm.DB, from string, to string) (int, error) {
	var ids []uint
	if err := tx.WithContext(ctx).Raw(`
		SELECT id FROM play_game_version_builds WHERE status = ? ORDER BY id FOR UPDATE
	`, from).Scan(&ids).Error; err != nil {
		return 0, fmt.Errorf("finding builds to requeue: %s", err)
	}
	for _, id := range ids {
		if _, err := r.changeStatus(ctx, tx, id, from, to); err != nil {
			return 0, err
		}
	}
	return len(ids), nil
}

// ClaimOldest moves the oldest build in from to to and returns it, or nil
// when none waits. Builds other transactions hold are skipped.
func (r *Repo) ClaimOldest(ctx context.Context, tx *gorm.DB, from string, to string) (*ClaimedBuild, error) {
	var claimed ClaimedBuild
	result := tx.WithContext(ctx).Raw(`
		SELECT b.id, b.uuid, b.game_version_id, v.uuid AS version_uuid, b.revision, b.created_at
		FROM play_game_version_builds b
		JOIN play_game_versions v ON v.id = b.game_version_id
		WHERE b.status = ?
		ORDER BY b.id
		LIMIT 1
		FOR UPDATE OF b SKIP LOCKED
	`, from).Scan(&claimed)
	if result.Error != nil {
		return nil, fmt.Errorf("claiming build: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	changed, err := r.changeStatus(ctx, tx, claimed.ID, from, to)
	if err != nil || !changed {
		return nil, err
	}
	return &claimed, nil
}

// FindRevision returns the files and asset bindings of revision number of
// versionID, or false when it has no such revision.
func (r *Repo) FindRevision(ctx context.Context, tx *gorm.DB, versionID uint, number int64) (draftsources.Revision, bool, error) {
	var rows []struct {
		Files  []byte
		Assets []byte
	}
	if err := tx.WithContext(ctx).Raw(`
		SELECT files, assets FROM play_game_version_revisions WHERE game_version_id = ? AND number = ?
	`, versionID, number).Scan(&rows).Error; err != nil {
		return draftsources.Revision{}, false, fmt.Errorf("finding revision: %s", err)
	}
	if len(rows) == 0 {
		return draftsources.Revision{}, false, nil
	}
	revision := draftsources.Revision{Files: map[string]draftsources.File{}, Assets: map[string]draftsources.AssetBinding{}}
	if err := json.Unmarshal(rows[0].Files, &revision.Files); err != nil {
		return draftsources.Revision{}, false, fmt.Errorf("decoding revision files: %s", err)
	}
	if err := json.Unmarshal(rows[0].Assets, &revision.Assets); err != nil {
		return draftsources.Revision{}, false, fmt.Errorf("decoding revision assets: %s", err)
	}
	return revision, true, nil
}

// FindSourceContents returns the stored content of each hash it has.
func (r *Repo) FindSourceContents(ctx context.Context, tx *gorm.DB, hashes []string) (map[string]string, error) {
	contents := make(map[string]string, len(hashes))
	if len(hashes) == 0 {
		return contents, nil
	}
	var rows []struct {
		SHA256  string `gorm:"column:sha256"`
		Content string
	}
	if err := tx.WithContext(ctx).Raw(`
		SELECT sha256, content FROM play_source_files WHERE sha256 IN ?
	`, hashes).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("finding source contents: %s", err)
	}
	for _, row := range rows {
		contents[row.SHA256] = row.Content
	}
	return contents, nil
}

// FindReusable returns the oldest build of versionID in status built from
// inputsSHA256 that has artifacts, or nil.
func (r *Repo) FindReusable(ctx context.Context, tx *gorm.DB, versionID uint, inputsSHA256 string, status string) (*ReusableBuild, error) {
	var reusable ReusableBuild
	result := tx.WithContext(ctx).Raw(`
		SELECT id, server_object_key, server_sha256, server_size, client_object_key, client_sha256, client_size, manifest, aspect_ratio
		FROM play_game_version_builds
		WHERE game_version_id = ? AND inputs_sha256 = ? AND status = ? AND server_object_key IS NOT NULL
		ORDER BY id
		LIMIT 1
	`, versionID, inputsSHA256, status).Scan(&reusable)
	if result.Error != nil {
		return nil, fmt.Errorf("finding reusable build: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &reusable, nil
}

// RegisterArtifactObject records content about to be stored, once per hash,
// so every stored artifact is known and can be cleaned up.
func (r *Repo) RegisterArtifactObject(ctx context.Context, tx *gorm.DB, sha256 string, size int64, contentType string, objectKey string) error {
	row := artifactObjectRow{SHA256: sha256, Size: size, ContentType: contentType, ObjectKey: objectKey}
	if err := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		return fmt.Errorf("registering artifact object: %s", err)
	}
	return nil
}

// FinishBuild moves buildID from from to to, writing what its end decided
// once. False means the build was no longer in from, and nothing changed.
func (r *Repo) FinishBuild(ctx context.Context, tx *gorm.DB, buildID uint, from string, to string, artifacts Artifacts, finishedAt time.Time) (bool, error) {
	var manifest, diagnostics any
	if len(artifacts.Manifest) > 0 {
		manifest = string(artifacts.Manifest)
	}
	if len(artifacts.Diagnostics) > 0 {
		diagnostics = string(artifacts.Diagnostics)
	}
	result := tx.WithContext(ctx).Exec(`
		UPDATE play_game_version_builds SET
			status = ?, inputs_sha256 = ?, toolchain_version = ?, diagnostics = ?,
			server_object_key = ?, server_sha256 = ?, server_size = ?,
			client_object_key = ?, client_sha256 = ?, client_size = ?,
			manifest = ?, aspect_ratio = ?, reused_build_id = ?, finished_at = ?
		WHERE id = ? AND status = ? AND finished_at IS NULL
	`, to, nullable(artifacts.InputsSHA256), nullable(artifacts.ToolchainVersion), diagnostics,
		artifacts.ServerObjectKey, artifacts.ServerSHA256, artifacts.ServerSize,
		artifacts.ClientObjectKey, artifacts.ClientSHA256, artifacts.ClientSize,
		manifest, artifacts.AspectRatio, artifacts.ReusedBuildID, finishedAt,
		buildID, from)
	if result.Error != nil {
		return false, fmt.Errorf("finishing build: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return false, nil
	}
	if err := tx.WithContext(ctx).Create(&buildStatusChangeRow{BuildID: buildID, FromStatus: &from, ToStatus: to}).Error; err != nil {
		return false, fmt.Errorf("recording build status change: %s", err)
	}
	return true, nil
}

func nullable(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
