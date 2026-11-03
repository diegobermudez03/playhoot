package migration

import (
	"github.com/diegobermudez03/playhoot/game/internal/storage/migrations"
	"gorm.io/gorm"
)

func Migrate(db *gorm.DB) error {
	return migrations.MigrateTables(db)
}

// Seed fills Game Management's tables with test data; it runs after Migrate,
// never in production.
func Seed(db *gorm.DB) error {
	return migrations.SeedTables(db)
}
