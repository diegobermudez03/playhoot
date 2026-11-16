// Package testdb gives Notifications' repository tests one disposable
// database.
package testdb

import (
	"testing"

	"gorm.io/gorm"

	notificationsmigration "github.com/diegobermudez03/playhoot/notifications/migration"
	"github.com/diegobermudez03/playhoot/utils"
)

var notificationsDB = utils.DisposableTestDB{
	Key:        "notifications",
	NameSuffix: "notifications_pkg_test",
	Migrate:    notificationsmigration.Migrate,
}

// Open returns the shared test database.
func Open(t *testing.T) *gorm.DB {
	t.Helper()
	return notificationsDB.Open(t)
}

// Run runs a package's tests with the test database and drops it
// afterwards. Every package calling Open must call it from TestMain.
func Run(m *testing.M) int {
	return notificationsDB.Run(m)
}

// Reset empties every Notifications table except the seeded mode.
func Reset(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, table := range []string{"notifications_entries", "notifications_preferences", "notifications_email_deliveries", "notifications_suppressions", "notifications_provider_usage", "notifications_queue_tasks"} {
		if err := db.Exec("DELETE FROM " + table).Error; err != nil {
			t.Fatalf("emptying %s: %v", table, err)
		}
	}
	if err := db.Exec(`DELETE FROM notifications_provider_settings WHERE changed_by_user_uuid IS NOT NULL`).Error; err != nil {
		t.Fatalf("emptying modes: %v", err)
	}
}
