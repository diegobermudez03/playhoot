package repo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// RenewOwnerLease makes sessionID's owner lease run until expiresAt.
func (r *Repo) RenewOwnerLease(ctx context.Context, tx *gorm.DB, sessionID uint, expiresAt time.Time) error {
	result := tx.WithContext(ctx).Exec(`
		UPDATE play_sessions SET owner_lease_expires_at = ? WHERE id = ?
	`, expiresAt, sessionID)
	if result.Error != nil {
		return fmt.Errorf("renewing owner lease: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("session %d not found", sessionID)
	}
	return nil
}
