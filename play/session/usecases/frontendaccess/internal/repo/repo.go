// Package repo is frontendaccess's persistence: it reads where a game
// version's frontend package is stored and decides nothing itself.
package repo

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// Repo is frontendaccess's persistence implementation.
type Repo struct {
	db *gorm.DB
}

// New constructs a Repo bound to db.
func New(db *gorm.DB) *Repo {
	return &Repo{db: db}
}

// Frontend is a game version's status, the locator of its frontend package
// and the aspect ratio it is shown in; each is nil when the version has none.
type Frontend struct {
	Status      string
	ObjectKey   *string
	SHA256      *string
	AspectRatio *string
}

// FindFrontend returns gameVersionUUID's frontend package, or nil when there
// is no such version.
func (r *Repo) FindFrontend(ctx context.Context, gameVersionUUID string) (*Frontend, error) {
	var frontend Frontend
	result := r.db.WithContext(ctx).Raw(`
		SELECT status, frontend_object_key AS object_key, frontend_sha256 AS sha256, aspect_ratio
		FROM play_game_versions
		WHERE uuid = ?
	`, gameVersionUUID).Scan(&frontend)
	if result.Error != nil {
		return nil, fmt.Errorf("finding frontend package: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &frontend, nil
}
