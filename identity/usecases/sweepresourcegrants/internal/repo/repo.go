// Package repo deletes long-expired single-use grants.
package repo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Repo is the sweep's persistence.
type Repo struct {
	db *gorm.DB
}

// New constructs a Repo on db.
func New(db *gorm.DB) *Repo {
	return &Repo{db: db}
}

// DeleteExpiredGrants deletes up to limit grants that expired before before,
// answering how many it deleted.
func (r *Repo) DeleteExpiredGrants(ctx context.Context, before time.Time, limit int) (int64, error) {
	result := r.db.WithContext(ctx).Exec(`
		DELETE FROM identity_resource_grants
		WHERE id IN (SELECT id FROM identity_resource_grants WHERE expires_at < ? ORDER BY expires_at LIMIT ?)`, before, limit)
	if result.Error != nil {
		return 0, fmt.Errorf("deleting expired resource grants: %s", result.Error)
	}
	return result.RowsAffected, nil
}
