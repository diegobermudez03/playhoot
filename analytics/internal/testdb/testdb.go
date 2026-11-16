// Package testdb gives Analytics' tests one disposable database.
package testdb

import (
	"testing"

	"gorm.io/gorm"

	analyticsmigration "github.com/diegobermudez03/playhoot/analytics/migration"
	"github.com/diegobermudez03/playhoot/utils"
)

var analyticsDB = utils.DisposableTestDB{
	Key:        "analytics",
	NameSuffix: "analytics_pkg_test",
	Migrate:    analyticsmigration.Migrate,
}

// OpenAnalyticsDB returns the shared test database.
func OpenAnalyticsDB(t *testing.T) *gorm.DB {
	t.Helper()
	return analyticsDB.Open(t)
}

// Run runs a package's tests with the analytics test database and drops it
// afterwards. Every package calling OpenAnalyticsDB must call it from TestMain.
func Run(m *testing.M) int {
	return analyticsDB.Run(m)
}
