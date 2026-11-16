// Package migration exposes Analytics' schema migrations to the application.
package migration

import (
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/analytics/internal/migrations"
)

// Migrate applies Analytics' migrations.
func Migrate(db *gorm.DB) error {
	return migrations.MigrateTables(db)
}
