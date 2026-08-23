package migrations

import (
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
		migration20260822000001GameVersions(),
		migration20260822000002GameImages(),
		migration20260822000003GameHistories(),
		migration20260822000004GameVersionHistories(),
	})

	return migrator.Migrate()
}
