package migration

import (
	"context"

	"github.com/diegobermudez03/playhoot/play/session/internal/migrations"
	"github.com/diegobermudez03/playhoot/play/session/storage"
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

// SeedObjects stores in store the game version content the test data
// references. It must only run where Seed runs, with storage configured.
func SeedObjects(ctx context.Context, store storage.Store) error {
	return migrations.SeedObjects(ctx, store)
}
