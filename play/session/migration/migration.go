package migration

import (
	"github.com/diegobermudez03/playhoot/play/session/internal/migrations"
	"gorm.io/gorm"
)

func Migrate(db *gorm.DB) error {
	return migrations.MigrateTables(db)
}
