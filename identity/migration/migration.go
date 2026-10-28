// Package migration exposes Identity's schema migrations to the application.
package migration

import (
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/identity/internal/migrations"
)

// Migrate applies Identity's migrations.
func Migrate(db *gorm.DB) error {
	return migrations.MigrateTables(db)
}
