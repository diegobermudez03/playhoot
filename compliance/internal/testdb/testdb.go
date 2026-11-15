// Package testdb gives Compliance's repository tests one disposable database.
package testdb

import (
	"testing"

	"gorm.io/gorm"

	compliancemigration "github.com/diegobermudez03/playhoot/compliance/migration"
	"github.com/diegobermudez03/playhoot/utils"
)

var complianceDB = utils.DisposableTestDB{
	Key:        "compliance",
	NameSuffix: "compliance_pkg_test",
	Migrate:    compliancemigration.Migrate,
}

// OpenComplianceDB returns the shared test database.
func OpenComplianceDB(t *testing.T) *gorm.DB {
	t.Helper()
	return complianceDB.Open(t)
}

// Run runs a package's tests with the compliance test database and drops it
// afterwards. Every package calling OpenComplianceDB must call it from TestMain.
func Run(m *testing.M) int {
	return complianceDB.Run(m)
}
