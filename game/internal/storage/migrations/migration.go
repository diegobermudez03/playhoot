package migrations

import (
	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/utils/txqueue"
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func MigrateTables(db *gorm.DB) error {
	migrator := gormigrate.New(db, &gormigrate.Options{
		TableName:                 "migrations",
		IDColumnName:              "id",
		IDColumnSize:              255,
		UseTransaction:            true,
		ValidateUnknownMigrations: false,
	}, []*gormigrate.Migration{
		migration20260822000000Games(),
		migration20260822000003GameHistories(),
		migration20261101000000GameEditions(),
		migration20261103000000GamePublishing(),
		txqueue.Migration("20261115230100_game_queue", game.QueueDomain),
	})

	return migrator.Migrate()
}
