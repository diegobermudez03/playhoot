// Package migration exposes Budget's schema migrations to the application.
package migration

import (
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/budget/internal/migrations"
)

// Migrate applies Budget's migrations, the initial caps and rates included.
func Migrate(db *gorm.DB) error {
	return migrations.MigrateTables(db)
}
