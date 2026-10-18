package repo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// recordStatusChange appends sessionID's move from fromStatus (nil when it was
// created) to toStatus to its history. changedBy is nil for the platform.
func recordStatusChange(ctx context.Context, tx *gorm.DB, sessionID uint, fromStatus *string, toStatus string, endCause *string, changedBy *string, changedAt time.Time) error {
	result := tx.WithContext(ctx).Exec(`
		INSERT INTO play_session_status_changes (session_id, from_status, to_status, end_cause, changed_by_user_uuid, changed_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, sessionID, fromStatus, toStatus, endCause, changedBy, changedAt)
	if result.Error != nil {
		return fmt.Errorf("recording status change: %w", result.Error)
	}
	return nil
}
