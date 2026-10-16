package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// SeedTables applies pending test-data migrations, after MigrateTables and
// never in production. Append new ones in chronological order.
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
