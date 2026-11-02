// Package repo is findeditableversion's persistence: what it reads about a
// Game and its open edition.
package repo

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// GameEditing is a Game's owner and the version of its open edition, nil
// when it has none.
type GameEditing struct {
	OwnerUUID   string
	VersionUUID *string
}

// Repo reads Games and their editions.
type Repo struct {
	db *gorm.DB
}

// New constructs a Repo bound to db.
func New(db *gorm.DB) *Repo {
	return &Repo{db: db}
}

// FindGameEditing returns gameUUID's owner and open edition, or nil when no
// Game that is not deleted has that uuid.
func (r *Repo) FindGameEditing(ctx context.Context, gameUUID string, editionStatus string) (*GameEditing, error) {
	var editing GameEditing
	result := r.db.WithContext(ctx).Raw(`
		SELECT g.owner_uuid, e.uuid AS version_uuid
		FROM games g
		LEFT JOIN game_editions e ON e.game_id = g.id AND e.status = ?
		WHERE g.uuid = ? AND g.deleted_at IS NULL
	`, editionStatus, gameUUID).Scan(&editing)
	if result.Error != nil {
		return nil, fmt.Errorf("finding game editing: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &editing, nil
}
