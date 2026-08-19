package migration

import (
	"github.com/diegobermudez03/playhoot/game/session/internal/repo/migrations"
	"gorm.io/gorm"
)

func Migrate(db *gorm.DB) error {
	return migrations.MigrateTables(db)
}
