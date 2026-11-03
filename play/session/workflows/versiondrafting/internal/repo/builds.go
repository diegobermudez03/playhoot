package repo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Build is one revision's build as drafting reads it. Artifact fields are
// nil until it succeeded; StartedAt is when it last started running.
type Build struct {
	Revision      int64
	Status        string
	Diagnostics   []byte
	ServerSHA256  *string
	ServerSize    *int64
	ClientSHA256  *string
	ClientSize    *int64
	Manifest      []byte
	AspectRatio   *string
	ReusedBuildID *uint
	CreatedAt     time.Time
	StartedAt     *time.Time
	FinishedAt    *time.Time
}

type buildRow struct {
	ID            uint
	UUID          string
	GameVersionID uint
	Revision      int64
	Status        string
}

func (buildRow) TableName() string { return "play_game_version_builds" }

type buildStatusChangeRow struct {
	ID         uint
	BuildID    uint
	FromStatus *string
	ToStatus   string
}

func (buildStatusChangeRow) TableName() string { return "play_game_version_build_status_changes" }

// QueueBuild records a build of revision of versionID in queued, after
// moving every build of the version still in queued to superseded, each
// change kept in the status history. It returns how many it superseded.
func (r *Repo) QueueBuild(ctx context.Context, tx *gorm.DB, versionID uint, revision int64, buildUUID string, queued string, superseded string) (int, error) {
	var waiting []uint
	if err := tx.WithContext(ctx).Raw(`
		SELECT id FROM play_game_version_builds WHERE game_version_id = ? AND status = ? FOR UPDATE
	`, versionID, queued).Scan(&waiting).Error; err != nil {
		return 0, fmt.Errorf("finding queued builds: %s", err)
	}
	for _, id := range waiting {
		if err := tx.WithContext(ctx).Exec(`
			UPDATE play_game_version_builds SET status = ? WHERE id = ? AND status = ?
		`, superseded, id, queued).Error; err != nil {
			return 0, fmt.Errorf("superseding build: %s", err)
		}
		from := queued
		if err := tx.WithContext(ctx).Create(&buildStatusChangeRow{BuildID: id, FromStatus: &from, ToStatus: superseded}).Error; err != nil {
			return 0, fmt.Errorf("recording build status change: %s", err)
		}
	}
	row := buildRow{UUID: buildUUID, GameVersionID: versionID, Revision: revision, Status: queued}
	if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
		return 0, fmt.Errorf("queueing build: %s", err)
	}
	if err := tx.WithContext(ctx).Create(&buildStatusChangeRow{BuildID: row.ID, ToStatus: queued}).Error; err != nil {
		return 0, fmt.Errorf("recording build status change: %s", err)
	}
	return len(waiting), nil
}

const selectBuild = `
	SELECT b.revision, b.status, b.diagnostics, b.server_sha256, b.server_size, b.client_sha256, b.client_size,
		b.manifest, b.aspect_ratio, b.reused_build_id, b.created_at, b.finished_at,
		(SELECT MAX(c.changed_at) FROM play_game_version_build_status_changes c WHERE c.build_id = b.id AND c.to_status = ?) AS started_at
	FROM play_game_version_builds b
	JOIN play_game_versions v ON v.id = b.game_version_id
`

// FindBuild returns the build of revision of versionUUID, or nil. running
// is the status whose latest change is the build's start.
func (r *Repo) FindBuild(ctx context.Context, tx *gorm.DB, versionUUID string, revision int64, running string) (*Build, error) {
	var build Build
	result := tx.WithContext(ctx).Raw(selectBuild+`WHERE v.uuid = ? AND b.revision = ?`, running, versionUUID, revision).Scan(&build)
	if result.Error != nil {
		return nil, fmt.Errorf("finding build: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &build, nil
}

// FindLastBuildInStatus returns the newest build of versionUUID in status,
// or nil.
func (r *Repo) FindLastBuildInStatus(ctx context.Context, tx *gorm.DB, versionUUID string, status string, running string) (*Build, error) {
	var build Build
	result := tx.WithContext(ctx).Raw(selectBuild+`WHERE v.uuid = ? AND b.status = ? ORDER BY b.revision DESC LIMIT 1`, running, versionUUID, status).Scan(&build)
	if result.Error != nil {
		return nil, fmt.Errorf("finding last build: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &build, nil
}

// BuiltArtifacts is what running a revision's build needs: its status, its
// backend script's locator and manifest when it succeeded, and the profile
// its version's script runs under.
type BuiltArtifacts struct {
	Revision        int64
	Status          string
	ServerObjectKey *string
	ServerSHA256    *string
	ServerSize      *int64
	ClientObjectKey *string
	ClientSHA256    *string
	ClientSize      *int64
	AspectRatio     *string
	Manifest        []byte
	ScriptProfile   string
}

// FindBuiltArtifacts returns the build of revision of versionUUID with what
// running it needs, or nil when the revision has no build.
func (r *Repo) FindBuiltArtifacts(ctx context.Context, tx *gorm.DB, versionUUID string, revision int64) (*BuiltArtifacts, error) {
	var built BuiltArtifacts
	result := tx.WithContext(ctx).Raw(`
		SELECT b.revision, b.status, b.server_object_key, b.server_sha256, b.server_size,
			b.client_object_key, b.client_sha256, b.client_size, b.aspect_ratio, b.manifest, v.script_profile
		FROM play_game_version_builds b
		JOIN play_game_versions v ON v.id = b.game_version_id
		WHERE v.uuid = ? AND b.revision = ?
	`, versionUUID, revision).Scan(&built)
	if result.Error != nil {
		return nil, fmt.Errorf("finding built artifacts: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &built, nil
}
