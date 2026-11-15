package settlework

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/budget"
	"github.com/diegobermudez03/playhoot/budget/internal/testdb"
	"github.com/diegobermudez03/playhoot/budget/usecases/admitwork"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m))
}

func TestSettleWork(t *testing.T) {
	ctx := context.Background()
	db := testdb.OpenBudgetDB(t)
	start := time.Now().UTC().AddDate(0, 0, -1)

	setup := func(t *testing.T) (*Service, string) {
		testdb.Reset(t, db)
		testdb.SetControls(t, db,
			testdb.Control{Cap: &budget.Cap{Scope: budget.ScopeGlobal, Window: budget.WindowMonth, USDLimit: 120}, At: start},
			testdb.Control{Cap: &budget.Cap{Scope: string(budget.CategoryEmail), Window: budget.WindowMonth, USDLimit: 10}, At: start},
			testdb.Control{Rate: &budget.Rate{Category: budget.CategoryEmail, Service: "ses-outbound", Unit: "emails", USDPerUnit: 0.0001}, At: start},
		)
		admission, err := admitwork.New(db).AdmitWork(ctx, budget.WorkRequest{Category: budget.CategoryEmail, Service: "ses-outbound", Quantity: 1000, Reference: "email-1", MaxDuration: time.Minute})
		require.NoError(t, err)
		require.Equal(t, budget.Admitted, admission.Outcome)
		return New(db), admission.ReservationUUID
	}
	measured := func(t *testing.T) []float64 {
		var rows []float64
		require.NoError(t, db.Raw(`SELECT usd FROM budget_spend_events WHERE measured ORDER BY id`).Scan(&rows).Error)
		return rows
	}

	tests := map[string]func(t *testing.T){
		"settling records the measured cost once and releases the reservation": func(t *testing.T) {
			s, reservation := setup(t)
			outcome, err := s.SettleWork(ctx, reservation, budget.Settlement{Service: "ses-outbound", Quantity: 800})
			require.NoError(t, err)
			require.Equal(t, budget.Settled, outcome)
			outcome, err = s.SettleWork(ctx, reservation, budget.Settlement{Service: "ses-outbound", Quantity: 800})
			require.NoError(t, err)
			require.Equal(t, budget.AlreadyReleased, outcome)

			rows := measured(t)
			require.Len(t, rows, 1)
			require.InDelta(t, 0.08, rows[0], 1e-9)
			var settled float64
			require.NoError(t, db.Raw(`SELECT settled_usd FROM budget_reservations WHERE reservation_uuid = ? AND released_at IS NOT NULL`, reservation).Scan(&settled).Error)
			require.InDelta(t, 0.08, settled, 1e-9)
		},
		"cancelled work releases the whole reservation": func(t *testing.T) {
			s, reservation := setup(t)
			outcome, err := s.SettleWork(ctx, reservation, budget.Settlement{})
			require.NoError(t, err)
			require.Equal(t, budget.Settled, outcome)
			require.Empty(t, measured(t))
		},
		"expired work is released by the sweep and what it spent still counts": func(t *testing.T) {
			s, reservation := setup(t)
			s.now = func() time.Time { return time.Now().Add(2 * time.Minute) }
			released, err := s.SweepExpired(ctx)
			require.NoError(t, err)
			require.Equal(t, int64(1), released)
			released, err = s.SweepExpired(ctx)
			require.NoError(t, err)
			require.Equal(t, int64(0), released)

			outcome, err := s.SettleWork(ctx, reservation, budget.Settlement{Service: "ses-outbound", Quantity: 1000})
			require.NoError(t, err)
			require.Equal(t, budget.AlreadyReleased, outcome)
			require.Len(t, measured(t), 1)
			var settled *float64
			require.NoError(t, db.Raw(`SELECT settled_usd FROM budget_reservations WHERE reservation_uuid = ?`, reservation).Scan(&settled).Error)
			require.Nil(t, settled)
		},
		"an unknown reservation is reported": func(t *testing.T) {
			s, _ := setup(t)
			outcome, err := s.SettleWork(ctx, "8f14e45f-ceea-467a-9575-7d1d5c1e0b1f", budget.Settlement{})
			require.NoError(t, err)
			require.Equal(t, budget.UnknownReservation, outcome)
		},
	}
	for name, test := range tests {
		t.Run(name, test)
	}
}
