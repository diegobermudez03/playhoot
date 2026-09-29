package repo

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// Repo is Game Management's persistence access. It reports persisted facts;
// it decides no business/visibility policy itself.
type Repo struct {
	db *gorm.DB
}

// New constructs a Repo bound to db.
func New(db *gorm.DB) *Repo {
	return &Repo{db: db}
}

// ResolveVisibility resolves gameUUID's current visibility value directly
// from the games table - no join, no version/definition concept. Returns
// nil, nil if no non-deleted Game with gameUUID exists.
func (r *Repo) ResolveVisibility(ctx context.Context, gameUUID string) (*string, error) {
	var resolution struct {
		Visibility string `gorm:"column:visibility"`
	}
	result := r.db.WithContext(ctx).Raw(`
		SELECT visibility
		FROM games
		WHERE uuid = ? AND deleted_at IS NULL
	`, gameUUID).Scan(&resolution)
	if result.Error != nil {
		return nil, fmt.Errorf("resolving game visibility: %s", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &resolution.Visibility, nil
}
