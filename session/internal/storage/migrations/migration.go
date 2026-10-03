package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// MigrateTables applies every Session Runtime migration not yet recorded.
// Register a new migration by appending it to the list, in chronological
// order.
func MigrateTables(db *gorm.DB) error {
	migrations := []*gormigrate.Migration{}

	// gormigrate refuses to run with an empty list.
	if len(migrations) == 0 {
		return nil
	}

	migrator := gormigrate.New(db, &gormigrate.Options{
		TableName:      "migrations",
		IDColumnName:   "id",
		IDColumnSize:   255,
		UseTransaction: true,
		// Other domain packages store their migration IDs in this same table.
		ValidateUnknownMigrations: false,
	}, migrations)

	return migrator.Migrate()
}
