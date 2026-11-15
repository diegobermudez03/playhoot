// Package migration exposes Compliance's schema migrations and local seed to
// the application.
package migration

import (
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/compliance/internal/migrations"
)

// Migrate applies Compliance's migrations, the initial market matrix included.
func Migrate(db *gorm.DB) error {
	return migrations.MigrateTables(db)
}

// Seed publishes placeholder acceptance texts in every language when none of
// a kind exist. Only for local use: real texts replace them before launch.
func Seed(db *gorm.DB) error {
	return migrations.SeedDraftTexts(db)
}
