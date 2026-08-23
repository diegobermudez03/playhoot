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
	return &repo{}
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
	err := r.db.WithContext(ctx).Raw(`
		SELECT g.uuid, g.name, g.description, g.owner_uuid, g.logo_image_url, g.visibility, v.version_uuid, v.script
		FROM games g 
		INNER JOIN game_versions v ON v.game_id = d.id 
		WHERE uuid=?
	`, gameUUID).Scan(&g).Error
	if err != nil {
		return nil, fmt.Errorf("fetching game by UUID in repo: %s", err)
	}

	return &g, nil
}
