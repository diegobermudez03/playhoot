package getgamedefinition

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

type repoAPI interface {
	getGameDefinitionByUUID(ctx context.Context, gameDefinitionUUID string) (*gameDefinitionRow, error)
}

type repo struct {
	db *gorm.DB
}

func newRepo(db *gorm.DB) *repo {
	return &repo{
		db: db,
	}
}

type gameDefinitionRow struct {
	UUID   string
	Script string
}

// getGameDefinitionByUUID loads a Game Definition/version directly by its own
// public UUID, never through games.current_definition_id, so a caller that
// already pinned a specific version keeps reading exactly that version
// regardless of what the game's author later publishes as current.
func (r *repo) getGameDefinitionByUUID(ctx context.Context, gameDefinitionUUID string) (*gameDefinitionRow, error) {
	var d gameDefinitionRow
	tx := r.db.WithContext(ctx).Raw(`
		SELECT uuid, script
		FROM game_definitions
		WHERE uuid = ?
	`, gameDefinitionUUID).Scan(&d)
	if tx.Error != nil {
		return nil, fmt.Errorf("fetching game definition by uuid in repo: %s", tx.Error)
	}
	if tx.RowsAffected == 0 {
		return nil, nil
	}

	return &d, nil
}
