package getgame

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

type repoAPI interface {
	getGameCurrentVersion(ctx context.Context, gameUUID string) (*gameWithVersion, error)
}

type repo struct {
	db *gorm.DB
}

func newRepo(db *gorm.DB) *repo {
	return &repo{
		db: db,
	}
}

type gameWithVersion struct {
	UUID         string
	Name         string
	Description  string
	OwnerUUID    string
	LogoImageURL string
	Visibility   string
	VersionUUID  string
	Script       string
}

// getGameCurrentVersion returns the game's latest version
func (r *repo) getGameCurrentVersion(ctx context.Context, gameUUID string) (*gameWithVersion, error) {
	var g gameWithVersion
	tx := r.db.WithContext(ctx).Raw(`
		SELECT g.uuid, g.name, g.description, g.owner_uuid, g.logo_image_url, g.visibility, d.uuid AS version_uuid, d.script
		FROM games g 
		INNER JOIN game_definitions d ON d.id = g.current_definition_id
		WHERE g.uuid = ?
	`, gameUUID).Scan(&g)
	if tx.Error != nil {
		return nil, fmt.Errorf("fetching game by UUID in repo: %s", tx.Error)
	}
	if tx.RowsAffected == 0 {
		return nil, nil
	}

	return &g, nil
}
