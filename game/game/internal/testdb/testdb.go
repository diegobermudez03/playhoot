package testdb

import (
	"testing"

	gamemigration "github.com/diegobermudez03/playhoot/game/game/migration"
	"github.com/diegobermudez03/playhoot/utils"
	"gorm.io/gorm"
)

var gameDB = utils.DisposableTestDB{
	Key:        "game",
	NameSuffix: "game_pkg_test",
	Migrate: func(db *gorm.DB) error {
		return gamemigration.Migrate(db)
	},
}

func OpenGameDB(t *testing.T) *gorm.DB {
	t.Helper()
	return gameDB.Open(t)
}
