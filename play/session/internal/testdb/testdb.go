package testdb

import (
	"testing"

	sessionmigration "github.com/diegobermudez03/playhoot/play/session/migration"
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

// Run runs a package's tests with the session test database and drops it
// afterwards. Every package calling OpenSessionDB must call it from TestMain.
func Run(m *testing.M) int {
	return sessionDB.Run(m)
}
