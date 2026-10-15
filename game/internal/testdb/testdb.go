package testdb

import (
	"testing"

	gamemigration "github.com/diegobermudez03/playhoot/game/migration"
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

// Run runs a package's tests with the game test database and drops it
// afterwards. Every package calling OpenGameDB must call it from TestMain.
func Run(m *testing.M) int {
	return gameDB.Run(m)
}
