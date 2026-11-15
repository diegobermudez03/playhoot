// Package testdb gives Budget's repository tests one disposable database.
package testdb

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/budget"
	internalrepo "github.com/diegobermudez03/playhoot/budget/internal/repo"
	budgetmigration "github.com/diegobermudez03/playhoot/budget/migration"
	"github.com/diegobermudez03/playhoot/utils"
)

var budgetDB = utils.DisposableTestDB{
	Key:        "budget",
	NameSuffix: "budget_pkg_test",
	Migrate:    budgetmigration.Migrate,
}

// OpenBudgetDB returns the shared test database.
func OpenBudgetDB(t *testing.T) *gorm.DB {
	t.Helper()
	return budgetDB.Open(t)
}

// Run runs a package's tests with the budget test database and drops it
// afterwards. Every package calling OpenBudgetDB must call it from TestMain.
func Run(m *testing.M) int {
	return budgetDB.Run(m)
}

// Reset empties every Budget table, seeds included, so a test starts from
// the controls it writes itself.
func Reset(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, table := range []string{"budget_rates", "budget_caps", "budget_spend_events", "budget_reservations", "budget_restriction_changes", "budget_queue_tasks"} {
		if err := db.Exec("DELETE FROM " + table).Error; err != nil {
			t.Fatalf("emptying %s: %v", table, err)
		}
	}
}

// Control is one cap or rate a test writes, effective at At.
type Control struct {
	Cap  *budget.Cap
	Rate *budget.Rate
	At   time.Time
}

// SetControls appends caps and rates as the seeded controls are written.
func SetControls(t *testing.T, db *gorm.DB, controls ...Control) {
	t.Helper()
	for _, c := range controls {
		var err error
		if c.Cap != nil {
			row := internalrepo.Cap{Scope: c.Cap.Scope, CapWindow: string(c.Cap.Window), USDLimit: c.Cap.USDLimit, EffectiveAt: c.At, Reason: "test", CreatedAt: c.At}
			if c.Cap.MaxConcurrency > 0 {
				row.MaxConcurrency = &c.Cap.MaxConcurrency
			}
			if c.Cap.MaxVolume > 0 {
				row.MaxVolume = &c.Cap.MaxVolume
			}
			err = db.Create(&row).Error
		}
		if c.Rate != nil {
			err = db.Create(&internalrepo.Rate{Category: string(c.Rate.Category), Service: c.Rate.Service, Unit: c.Rate.Unit, USDPerUnit: c.Rate.USDPerUnit, EffectiveAt: c.At, Reason: "test", CreatedAt: c.At}).Error
		}
		if err != nil {
			t.Fatalf("writing control: %v", err)
		}
	}
}

// Spend stores a priced spend event directly, as ingestion would.
func Spend(t *testing.T, db *gorm.DB, category budget.Category, usd float64, measured bool, reference string, at time.Time) {
	t.Helper()
	err := db.Create(&internalrepo.SpendEvent{
		SourceDomain: "test", Category: string(category), Service: "test", Reference: reference, Quantity: usd, Unit: "usd",
		USD: usd, Measured: measured, IdempotencyKey: uuid.NewString(), OccurredAt: at, CreatedAt: at,
	}).Error
	if err != nil {
		t.Fatalf("writing spend: %v", err)
	}
}
