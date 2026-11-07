// Package repo is findtemplate's persistence: a Game allowed as a template
// and its latest published edition.
package repo

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// Template is a published Game allowed as a template, with the version of
// its latest published edition.
type Template struct {
	GameUUID    string
	VersionUUID *string
	Name        string
	Description string
	Featured    bool
}

// Repo reads templates.
type Repo struct {
	db *gorm.DB
}

// New constructs a Repo bound to db.
func New(db *gorm.DB) *Repo {
	return &Repo{db: db}
}

// FindTemplate returns gameUUID when it is published, not deleted and its
// owner allows it as a template, or nil.
func (r *Repo) FindTemplate(ctx context.Context, gameUUID string, publishedStatus string) (*Template, error) {
	var template Template
	result := r.db.WithContext(ctx).Raw(`
		SELECT g.uuid AS game_uuid, g.name, g.description, g.template_featured AS featured,
			(SELECT e.uuid FROM game_editions e WHERE e.game_id = g.id AND e.status = ? ORDER BY e.id DESC LIMIT 1) AS version_uuid
		FROM games g
		WHERE g.uuid = ? AND g.deleted_at IS NULL AND g.published_at IS NOT NULL AND g.template_allowed
	`, publishedStatus, gameUUID).Scan(&template)
	if result.Error != nil {
		return nil, fmt.Errorf("finding template: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &template, nil
}
