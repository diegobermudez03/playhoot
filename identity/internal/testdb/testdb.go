// Package testdb gives Identity's repository tests one disposable database.
package testdb

import (
	"testing"

	"gorm.io/gorm"

	identitymigration "github.com/diegobermudez03/playhoot/identity/migration"
	"github.com/diegobermudez03/playhoot/utils"
)

var identityDB = utils.DisposableTestDB{
	Key:        "identity",
	NameSuffix: "identity_pkg_test",
	Migrate:    identitymigration.Migrate,
}

// OpenIdentityDB returns the shared test database.
func OpenIdentityDB(t *testing.T) *gorm.DB {
	t.Helper()
	return identityDB.Open(t)
}

// Run runs a package's tests with the identity test database and drops it
// afterwards. Every package calling OpenIdentityDB must call it from TestMain.
func Run(m *testing.M) int {
	return identityDB.Run(m)
}
