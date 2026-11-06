package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/diegobermudez03/playhoot/play/session/internal/business/draftsources"
)

// Provenance is where a revision came from. Origin is empty when the
// platform made it; the pointers and From* are nil or empty when they do not
// apply.
type Provenance struct {
	Kind            string
	Origin          string
	Actor           string
	ParentRevision  *int64
	RestoredFrom    *int64
	FromGameUUID    string
	FromVersionUUID string
	FromRevision    *int64
	SDK             string
}

// Revision is one entry of a version's revision history.
type Revision struct {
	Number int64
	Provenance
	Files     int
	CreatedAt time.Time
}

type revisionRow struct {
	ID              uint
	GameVersionID   uint
	Number          int64
	Files           string `gorm:"type:jsonb"`
	Assets          string `gorm:"type:jsonb"`
	Kind            string
	Origin          *string
	Actor           string
	ParentRevision  *int64
	RestoredFrom    *int64
	FromGameUUID    *string
	FromVersionUUID *string
	FromRevision    *int64
	SDK             string `gorm:"column:sdk"`
	CreatedAt       time.Time
}

func (revisionRow) TableName() string { return "play_game_version_revisions" }

type sourceFileRow struct {
	ID        uint
	SHA256    string `gorm:"column:sha256"`
	Content   string
	Size      int64
	CreatedAt time.Time
}

func (sourceFileRow) TableName() string { return "play_source_files" }

// CreateRevision records revision number of versionID with its files, asset
// bindings and provenance.
func (r *Repo) CreateRevision(ctx context.Context, tx *gorm.DB, versionID uint, number int64, revision draftsources.Revision, provenance Provenance) error {
	files, err := json.Marshal(revision.Files)
	if err != nil {
		return fmt.Errorf("encoding revision files: %s", err)
	}
	assets := revision.Assets
	if assets == nil {
		assets = map[string]draftsources.AssetBinding{}
	}
	bindings, err := json.Marshal(assets)
	if err != nil {
		return fmt.Errorf("encoding revision assets: %s", err)
	}
	row := revisionRow{
		GameVersionID: versionID, Number: number, Files: string(files), Assets: string(bindings),
		Kind: provenance.Kind, Origin: nonEmpty(provenance.Origin), Actor: provenance.Actor,
		ParentRevision: provenance.ParentRevision, RestoredFrom: provenance.RestoredFrom,
		FromGameUUID: nonEmpty(provenance.FromGameUUID), FromVersionUUID: nonEmpty(provenance.FromVersionUUID), FromRevision: provenance.FromRevision,
		SDK: provenance.SDK,
	}
	if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("creating revision: %s", err)
	}
	return nil
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

// ListRevisions returns versionID's revisions numbered below before (every
// one when before is 0), newest first, at most limit.
func (r *Repo) ListRevisions(ctx context.Context, tx *gorm.DB, versionID uint, before int64, limit int) ([]Revision, error) {
	query := `
		SELECT number, kind, COALESCE(origin, '') AS origin, actor, parent_revision, restored_from,
			COALESCE(from_game_uuid::text, '') AS from_game_uuid, COALESCE(from_version_uuid::text, '') AS from_version_uuid, from_revision, sdk,
			(SELECT count(*) FROM jsonb_object_keys(files)) AS files, created_at
		FROM play_game_version_revisions
		WHERE game_version_id = ?`
	args := []any{versionID}
	if before > 0 {
		query += ` AND number < ?`
		args = append(args, before)
	}
	query += ` ORDER BY number DESC LIMIT ?`
	args = append(args, limit)
	var revisions []Revision
	if err := tx.WithContext(ctx).Raw(query, args...).Scan(&revisions).Error; err != nil {
		return nil, fmt.Errorf("listing revisions: %s", err)
	}
	return revisions, nil
}

// StoreSourceContents stores each content under its hash, once.
func (r *Repo) StoreSourceContents(ctx context.Context, tx *gorm.DB, contents map[string]string) error {
	hashes := make([]string, 0, len(contents))
	for hash := range contents {
		hashes = append(hashes, hash)
	}
	// A stable order keeps concurrent writers from locking rows in
	// different orders.
	sort.Strings(hashes)
	for _, hash := range hashes {
		content := contents[hash]
		row := sourceFileRow{SHA256: hash, Content: content, Size: int64(len(content))}
		if err := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return fmt.Errorf("storing source content: %s", err)
		}
	}
	return nil
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

// nonEmpty is value, or nil when it is empty, for a NULL column.
func nonEmpty(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
