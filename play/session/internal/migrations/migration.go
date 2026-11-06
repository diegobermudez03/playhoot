package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// MigrateTables applies every Session Runtime migration not yet recorded.
// Register a new migration by appending it to the list, in chronological
// order.
func MigrateTables(db *gorm.DB) error {
	migrations := []*gormigrate.Migration{
		migration20261015000000PlaySessions(),
		migration20261023000000PlayGameFaultReports(),
		migration20261026000000PlayGameVersionAssets(),
		migration20261026100000PlayGameVersionSignedURLs(),
		migration20261028000000PlayGameVersionInputRequirements(),
		migration20261101100000PlayDraftVersions(),
		migration20261102000000PlayDraftAssets(),
		migration20261102200000PlayDraftBuilds(),
		migration20261102300000PlayPreviews(),
		migration20261106000000PlaySimulationResults(),
	}

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
