package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// SeedTables applies every Session Runtime test-data migration not yet
// recorded. Test data is only for local and test environments, never for
// production, and runs after MigrateTables. Seed migrations are edited along
// with the schema they fill, the same way schema migrations are while nothing
// is deployed. Register a new one by appending it to the list, in
// chronological order.
func SeedTables(db *gorm.DB) error {
	seeds := []*gormigrate.Migration{
		seed20261015000000PlayGames(),
	}

	migrator := gormigrate.New(db, &gormigrate.Options{
		TableName:      "migrations",
		IDColumnName:   "id",
		IDColumnSize:   255,
		UseTransaction: true,
		// Schema migrations of every domain share this table.
		ValidateUnknownMigrations: false,
	}, seeds)

	return migrator.Migrate()
}
