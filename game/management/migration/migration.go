package migration

import (
	"github.com/diegobermudez03/playhoot/game/management/internal/storage/migrations"
	"gorm.io/gorm"
)

func Migrate(db *gorm.DB) error {
	return migrations.MigrateTables(db)
}
