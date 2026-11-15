// Package migration exposes Plan's schema migrations to the application.
package migration

import (
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/plan/internal/migrations"
)

// Migrate applies Plan's migrations, the initial free limits included.
func Migrate(db *gorm.DB) error {
	return migrations.MigrateTables(db)
}
