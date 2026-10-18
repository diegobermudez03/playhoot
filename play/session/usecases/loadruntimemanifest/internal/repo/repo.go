// Package repo is loadruntimemanifest's persistence: it reads the manifest of
// a Session's pinned game version and decides nothing itself.
package repo

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// Repo is loadruntimemanifest's persistence implementation.
type Repo struct {
	db *gorm.DB
}

// New constructs a Repo bound to db.
func New(db *gorm.DB) *Repo {
	return &Repo{db: db}
}

// FindSessionManifest returns the raw manifest of the game version
// sessionUUID is pinned to; found is false when there is no such Session. A
// Session whose version row is missing is found with no manifest.
func (r *Repo) FindSessionManifest(ctx context.Context, sessionUUID string) (manifest []byte, found bool, err error) {
	var row struct {
		Manifest []byte
	}
	result := r.db.WithContext(ctx).Raw(`
		SELECT v.manifest
		FROM play_sessions s
		LEFT JOIN play_game_versions v ON v.id = s.game_version_id
		WHERE s.uuid = ?
	`, sessionUUID).Scan(&row)
	if result.Error != nil {
		return nil, false, fmt.Errorf("finding session manifest: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, false, nil
	}
	return row.Manifest, true, nil
}
