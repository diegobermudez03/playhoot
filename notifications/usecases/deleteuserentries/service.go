// Package deleteuserentries removes what Notifications keeps about a person
// who no longer exists: the notices they received and the choices they made
// about email. A suppression stays, because it protects an address rather
// than an account.
package deleteuserentries

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/utils"
)

var (
	deletedEntriesTotal = metrics.NewCounter(
		"notifications_user_entries_deleted_total",
		"Notification entries deleted because the person's identity was deleted.",
	)

	deletedPreferencesTotal = metrics.NewCounter(
		"notifications_user_preferences_deleted_total",
		"Notification preference rows deleted because the person's identity was deleted.",
	)
)

// Service deletes a deleted person's notifications.
type Service struct {
	db *gorm.DB
}

// New constructs a Service on db.
func New(db *gorm.DB) *Service {
	return &Service{db: db}
}

// GetDB returns the database, for transactions.
func (s *Service) GetDB() *gorm.DB {
	return s.db
}

// DeleteUserEntries deletes every entry and preference row of userUUID,
// answering how many entries went. Delivery records go with their entries;
// suppressions are untouched. Running it again deletes nothing new.
func (s *Service) DeleteUserEntries(ctx context.Context, userUUID string) (int64, error) {
	defer logging.Step(ctx, "notifications.delete_user_entries").Close()

	entries, err := utils.RunInDBTransaction(ctx, s, func(ctx context.Context, tx *gorm.DB) (int64, error) {
		// The delivery records point at the entries by row: they go first,
		// so none is left pointing at nothing.
		err := tx.WithContext(ctx).Exec(`
			DELETE FROM notifications_email_deliveries
			WHERE entry_id IN (SELECT id FROM notifications_entries WHERE user_uuid = ?)`,
			userUUID,
		).Error
		if err != nil {
			return 0, fmt.Errorf("deleting the email deliveries of a deleted person: %s", err)
		}
		result := tx.WithContext(ctx).Exec(`DELETE FROM notifications_entries WHERE user_uuid = ?`, userUUID)
		if result.Error != nil {
			return 0, fmt.Errorf("deleting the entries of a deleted person: %s", result.Error)
		}
		entries := result.RowsAffected
		preferences := tx.WithContext(ctx).Exec(`DELETE FROM notifications_preferences WHERE user_uuid = ?`, userUUID)
		if preferences.Error != nil {
			return 0, fmt.Errorf("deleting the preferences of a deleted person: %s", preferences.Error)
		}
		if preferences.RowsAffected > 0 {
			deletedPreferencesTotal.Add(ctx, float64(preferences.RowsAffected))
		}
		return entries, nil
	})
	if err != nil {
		return 0, err
	}
	if entries > 0 {
		deletedEntriesTotal.Add(ctx, float64(entries))
	}
	logging.LogFields(ctx, logging.Field("user_entries_deleted", entries))
	return entries, nil
}
