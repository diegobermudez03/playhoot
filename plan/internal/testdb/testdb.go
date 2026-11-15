// Package testdb gives Plan's repository tests one disposable database.
package testdb

import (
	"testing"

	"gorm.io/gorm"

	planmigration "github.com/diegobermudez03/playhoot/plan/migration"
	"github.com/diegobermudez03/playhoot/utils"
)

var planDB = utils.DisposableTestDB{
	Key:        "plan",
	NameSuffix: "plan_pkg_test",
	Migrate:    planmigration.Migrate,
}

// OpenPlanDB returns the shared test database.
func OpenPlanDB(t *testing.T) *gorm.DB {
	t.Helper()
	return planDB.Open(t)
}

// Run runs a package's tests with the plan test database and drops it
// afterwards. Every package calling OpenPlanDB must call it from TestMain.
func Run(m *testing.M) int {
	return planDB.Run(m)
}
