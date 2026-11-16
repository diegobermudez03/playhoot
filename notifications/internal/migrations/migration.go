package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/notifications"
	"github.com/diegobermudez03/playhoot/utils/txqueue"
)

// MigrateTables applies every Notifications migration not yet recorded.
// Register a new migration by appending it to the list, in chronological
// order.
func MigrateTables(db *gorm.DB) error {
	migrations := []*gormigrate.Migration{
		txqueue.Migration("20261115230000_notifications_queue", notifications.QueueDomain),
		migration20261115230100NotificationsCore(),
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
