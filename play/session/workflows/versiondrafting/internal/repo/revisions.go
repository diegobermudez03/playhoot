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

// Revision is one entry of a version's revision history.
type Revision struct {
	Number    int64
	Source    string
	Files     int
	CreatedAt time.Time
}

type revisionRow struct {
	ID            uint
	GameVersionID uint
	Number        int64
	Files         string `gorm:"type:jsonb"`
	Source        string
	CreatedAt     time.Time
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

// CreateRevision records revision number of versionID with its files.
func (r *Repo) CreateRevision(ctx context.Context, tx *gorm.DB, versionID uint, number int64, files map[string]draftsources.File, source string) error {
	encoded, err := json.Marshal(files)
	if err != nil {
		return fmt.Errorf("encoding revision files: %s", err)
	}
	row := revisionRow{GameVersionID: versionID, Number: number, Files: string(encoded), Source: source}
	if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("creating revision: %s", err)
	}
	return nil
}

// FindRevisionFiles returns the files of revision number of versionID, or
// false when it has no such revision.
func (r *Repo) FindRevisionFiles(ctx context.Context, tx *gorm.DB, versionID uint, number int64) (map[string]draftsources.File, bool, error) {
	var rows []struct{ Files []byte }
	if err := tx.WithContext(ctx).Raw(`
		SELECT files FROM play_game_version_revisions WHERE game_version_id = ? AND number = ?
	`, versionID, number).Scan(&rows).Error; err != nil {
		return nil, false, fmt.Errorf("finding revision: %s", err)
	}
	if len(rows) == 0 {
		return nil, false, nil
	}
	files := make(map[string]draftsources.File)
	if err := json.Unmarshal(rows[0].Files, &files); err != nil {
		return nil, false, fmt.Errorf("decoding revision files: %s", err)
	}
	return files, true, nil
}

// ListRevisions returns versionID's revisions, oldest first.
func (r *Repo) ListRevisions(ctx context.Context, tx *gorm.DB, versionID uint) ([]Revision, error) {
	var revisions []Revision
	if err := tx.WithContext(ctx).Raw(`
		SELECT number, source, (SELECT count(*) FROM jsonb_object_keys(files)) AS files, created_at
		FROM play_game_version_revisions
		WHERE game_version_id = ?
		ORDER BY number
	`, versionID).Scan(&revisions).Error; err != nil {
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
