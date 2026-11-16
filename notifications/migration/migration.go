// Package migration exposes Notifications' schema migrations to the
// application.
package migration

import (
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/notifications/internal/migrations"
)

// Migrate applies Notifications' migrations.
func Migrate(db *gorm.DB) error {
	return migrations.MigrateTables(db)
}
