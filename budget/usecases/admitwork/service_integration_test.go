package admitwork

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/budget"
	"github.com/diegobermudez03/playhoot/budget/internal/testdb"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m))
}

func TestAdmitWork(t *testing.T) {
	ctx := context.Background()
	db := testdb.OpenBudgetDB(t)
	now := time.Date(2030, 3, 14, 15, 30, 0, 0, time.UTC)
	start := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	ai := budget.Rate{Category: budget.CategoryAI, Service: "model", Unit: "usd", USDPerUnit: 1}

	setup := func(t *testing.T) *Service {
		testdb.Reset(t, db)
		testdb.SetControls(t, db,
			testdb.Control{Cap: &budget.Cap{Scope: budget.ScopeGlobal, Window: budget.WindowHour, USDLimit: 1}, At: start},
			testdb.Control{Cap: &budget.Cap{Scope: budget.ScopeGlobal, Window: budget.WindowDay, USDLimit: 8}, At: start},
			testdb.Control{Cap: &budget.Cap{Scope: budget.ScopeGlobal, Window: budget.WindowMonth, USDLimit: 120}, At: start},
			testdb.Control{Cap: &budget.Cap{Scope: string(budget.CategoryAI), Window: budget.WindowMonth, USDLimit: 1000}, At: start},
			testdb.Control{Rate: &ai, At: start},
			testdb.Control{Rate: &budget.Rate{Category: budget.CategoryFiles, Service: "upload", Unit: "usd", USDPerUnit: 1}, At: start},
		)
		service := New(db)
		service.now = func() time.Time { return now }
		return service
	}
	work := func(usd float64) budget.WorkRequest {
		return budget.WorkRequest{Category: budget.CategoryAI, Service: "model", Quantity: usd, Reference: "job"}
	}
	admit := func(t *testing.T, s *Service, request budget.WorkRequest) budget.Admission {
		admission, err := s.AdmitWork(ctx, request)
		require.NoError(t, err)
		return admission
	}

	tests := map[string]func(t *testing.T){
		"admitted work reserves its priced estimate": func(t *testing.T) {
			s := setup(t)
			admission := admit(t, s, work(0.3))
			require.Equal(t, budget.Admitted, admission.Outcome)
			var reserved float64
			require.NoError(t, db.Raw(`SELECT usd_reserved FROM budget_reservations WHERE reservation_uuid = ?`, admission.ReservationUUID).Scan(&reserved).Error)
			require.InDelta(t, 0.3, reserved, 1e-9)
		},
		"the hour window counts spend, reservations and the margin": func(t *testing.T) {
			s := setup(t)
			testdb.Spend(t, db, budget.CategoryAI, 0.5, true, "a", now.Add(-10*time.Minute))
			require.Equal(t, budget.Admitted, admit(t, s, work(0.3)).Outcome)
			// Remainder 0.2, of which 0.18 is usable.
			refused := admit(t, s, work(0.19))
			require.Equal(t, budget.Refused, refused.Outcome)
			require.Equal(t, budget.RefusedCapReached, refused.Reason)
			require.Equal(t, budget.RestrictionMessage, refused.Message)
			require.Equal(t, budget.Admitted, admit(t, s, work(0.17)).Outcome)
		},
		"the day window refuses while the hour has room": func(t *testing.T) {
			s := setup(t)
			testdb.Spend(t, db, budget.CategoryAI, 7.5, false, "", now.Add(-3*time.Hour))
			require.Equal(t, budget.RefusedCapReached, admit(t, s, work(0.5)).Reason)
			require.Equal(t, budget.Admitted, admit(t, s, work(0.4)).Outcome)
		},
		"the month window refuses while hour and day have room": func(t *testing.T) {
			s := setup(t)
			testdb.Spend(t, db, budget.CategoryAI, 119.5, true, "", now.AddDate(0, 0, -5))
			require.Equal(t, budget.RefusedCapReached, admit(t, s, work(0.5)).Reason)
		},
		"spend from earlier windows does not count": func(t *testing.T) {
			s := setup(t)
			testdb.Spend(t, db, budget.CategoryAI, 119.5, true, "", now.AddDate(0, -1, 0))
			require.Equal(t, budget.Admitted, admit(t, s, work(0.5)).Outcome)
		},
		"a category without a cap or a service without a rate admits nothing": func(t *testing.T) {
			s := setup(t)
			require.Equal(t, budget.RefusedNoControl, admit(t, s, budget.WorkRequest{Category: budget.CategoryFiles, Service: "upload", Quantity: 0}).Reason)
			require.Equal(t, budget.RefusedNoControl, admit(t, s, budget.WorkRequest{Category: budget.CategoryAI, Service: "unknown-model", Quantity: 0}).Reason)
		},
		"a zero cap blocks even free work": func(t *testing.T) {
			s := setup(t)
			testdb.SetControls(t, db, testdb.Control{Cap: &budget.Cap{Scope: string(budget.CategoryAI), Window: budget.WindowMonth, USDLimit: 0}, At: now.Add(-time.Minute)})
			require.Equal(t, budget.RefusedNoControl, admit(t, s, work(0)).Reason)
		},
		"the ladder refuses before the caps": func(t *testing.T) {
			s := setup(t)
			require.NoError(t, db.Exec(`INSERT INTO budget_restriction_changes (level, paused_categories, reopen_closed, reason, actor, created_at)
				VALUES (?, '[]', '[]', 'test', ?, ?)`, string(budget.LevelRestricted), budget.ActorAutomatic, now.Add(-time.Minute)).Error)
			discretionary := work(0.1)
			discretionary.Discretionary = true
			require.Equal(t, budget.RefusedRestricted, admit(t, s, discretionary).Reason)
			require.Equal(t, budget.Admitted, admit(t, s, work(0.1)).Outcome)
		},
		"concurrent admissions never over-reserve a cap": func(t *testing.T) {
			s := setup(t)
			// Sequentially, nine admissions of 0.1 fit an hour cap of 1 with
			// the margin; the tenth would leave less than a tenth free.
			var wg sync.WaitGroup
			results := make(chan budget.Admission, 30)
			for i := 0; i < 30; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					admission, err := s.AdmitWork(ctx, work(0.1))
					if err == nil {
						results <- admission
					}
				}()
			}
			wg.Wait()
			close(results)
			admitted, total := 0, 0
			for a := range results {
				total++
				if a.Outcome == budget.Admitted {
					admitted++
				}
			}
			require.Equal(t, 30, total)
			require.Equal(t, 9, admitted)
			var reserved float64
			require.NoError(t, db.Raw(`SELECT COALESCE(SUM(usd_reserved), 0) FROM budget_reservations`).Scan(&reserved).Error)
			require.LessOrEqual(t, reserved, 1.0)
		},
		"an invalid request is refused without touching the database": func(t *testing.T) {
			s := setup(t)
			require.Equal(t, budget.RefusedInvalidRequest, admit(t, s, budget.WorkRequest{Category: "unknown", Service: "x"}).Reason)
		},
	}
	for name, test := range tests {
		t.Run(name, test)
	}
}
