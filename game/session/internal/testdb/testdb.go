package testdb

import (
	"testing"

	sessionmigration "github.com/diegobermudez03/playhoot/game/session/migration"
	"github.com/diegobermudez03/playhoot/utils"
	"gorm.io/gorm"
)

var sessionDB = utils.DisposableTestDB{
	Key:        "session",
	NameSuffix: "session_pkg_test",
	Migrate: func(db *gorm.DB) error {
		return sessionmigration.Migrate(db)
	},
}

func OpenSessionDB(t *testing.T) *gorm.DB {
	t.Helper()
	return sessionDB.Open(t)
}
