package migration

import (
	"github.com/diegobermudez03/playhoot/play/session/internal/migrations"
	"gorm.io/gorm"
)

func Migrate(db *gorm.DB) error {
	return migrations.MigrateTables(db)
}

// Seed fills the Session Runtime tables with test data. It must only run in
// local and test environments, after Migrate.
func Seed(db *gorm.DB) error {
	return migrations.SeedTables(db)
}
