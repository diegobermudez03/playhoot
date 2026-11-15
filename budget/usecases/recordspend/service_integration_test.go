package recordspend

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/budget"
	"github.com/diegobermudez03/playhoot/budget/internal/testdb"
	"github.com/diegobermudez03/playhoot/budget/usecases/admitwork"
	"github.com/diegobermudez03/playhoot/budget/usecases/status"
	"github.com/diegobermudez03/playhoot/utils/txqueue"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m))
}

func TestRecordSpend(t *testing.T) {
	ctx := context.Background()
	db := testdb.OpenBudgetDB(t)
	queue := txqueue.NewRegistry(db, txqueue.DefaultWorkerConfig).Queue(budget.QueueDomain)
	start := time.Now().UTC().AddDate(0, 0, -1)
	ses := budget.Rate{Category: budget.CategoryEmail, Service: "ses-outbound", Unit: "emails", USDPerUnit: 0.0001}

	setup := func(t *testing.T) *Service {
		testdb.Reset(t, db)
		testdb.SetControls(t, db,
			testdb.Control{Cap: &budget.Cap{Scope: budget.ScopeGlobal, Window: budget.WindowMonth, USDLimit: 120}, At: start},
			testdb.Control{Cap: &budget.Cap{Scope: string(budget.CategoryEmail), Window: budget.WindowMonth, USDLimit: 10}, At: start},
			testdb.Control{Cap: &budget.Cap{Scope: string(budget.CategoryAI), Window: budget.WindowMonth, USDLimit: 10}, At: start},
			testdb.Control{Rate: &ses, At: start},
			testdb.Control{Rate: &budget.Rate{Category: budget.CategoryAI, Service: "model-a", Unit: "tokens", USDPerUnit: 0.00001}, At: start},
		)
		return New(db, queue)
	}
	event := func(key string) budget.SpendEvent {
		return budget.SpendEvent{SourceDomain: "notifications", Category: budget.CategoryEmail, Service: "ses-outbound", Reference: "email-1", Quantity: 1000, Unit: "emails", Measured: true, IdempotencyKey: key}
	}
	count := func(t *testing.T, query string, args ...any) int {
		var n int
		require.NoError(t, db.Raw(query, args...).Scan(&n).Error)
		return n
	}

	tests := map[string]func(t *testing.T){
		"a retried event is recorded once and priced": func(t *testing.T) {
			s := setup(t)
			outcome, err := s.RecordSpend(ctx, event("k1"))
			require.NoError(t, err)
			require.Equal(t, budget.SpendRecorded, outcome)
			outcome, err = s.RecordSpend(ctx, event("k1"))
			require.NoError(t, err)
			require.Equal(t, budget.SpendDuplicate, outcome)

			require.Equal(t, 1, count(t, `SELECT COUNT(*) FROM budget_spend_events`))
			var usd float64
			require.NoError(t, db.Raw(`SELECT usd FROM budget_spend_events`).Scan(&usd).Error)
			require.InDelta(t, 0.1, usd, 1e-9)
			require.Equal(t, 1, count(t, `SELECT COUNT(*) FROM budget_queue_tasks WHERE type = ?`, EvaluateAfterSpendTask))
		},
		"the same key from another domain is another event": func(t *testing.T) {
			s := setup(t)
			_, err := s.RecordSpend(ctx, event("k1"))
			require.NoError(t, err)
			other := event("k1")
			other.SourceDomain = "creation"
			outcome, err := s.RecordSpend(ctx, other)
			require.NoError(t, err)
			require.Equal(t, budget.SpendRecorded, outcome)
		},
		"a missing rate is kept flagged and blocks the category until fixed": func(t *testing.T) {
			s := setup(t)
			admission := admitwork.New(db)
			request := budget.WorkRequest{Category: budget.CategoryAI, Service: "model-a", Quantity: 10, Reference: "job"}
			before, err := admission.AdmitWork(ctx, request)
			require.NoError(t, err)
			require.Equal(t, budget.Admitted, before.Outcome)

			outcome, err := s.RecordSpend(ctx, budget.SpendEvent{SourceDomain: "creation", Category: budget.CategoryAI, Service: "model-b", Quantity: 5000, Unit: "tokens", Measured: true, IdempotencyKey: "k2"})
			require.NoError(t, err)
			require.Equal(t, budget.SpendRateMissing, outcome)
			require.Equal(t, 1, count(t, `SELECT COUNT(*) FROM budget_spend_events WHERE rate_missing`))

			blocked, err := admission.AdmitWork(ctx, request)
			require.NoError(t, err)
			require.Equal(t, budget.RefusedNoControl, blocked.Reason)
			emailStill, err := admission.AdmitWork(ctx, budget.WorkRequest{Category: budget.CategoryEmail, Service: "ses-outbound", Quantity: 1})
			require.NoError(t, err)
			require.Equal(t, budget.Admitted, emailStill.Outcome)

			testdb.SetControls(t, db, testdb.Control{Rate: &budget.Rate{Category: budget.CategoryAI, Service: "model-b", Unit: "tokens", USDPerUnit: 0.00002}, At: start})
			after, err := admission.AdmitWork(ctx, request)
			require.NoError(t, err)
			require.Equal(t, budget.Admitted, after.Outcome)
		},
		"a unit different from the rate's is a missing rate": func(t *testing.T) {
			s := setup(t)
			e := event("k3")
			e.Unit = "GB"
			outcome, err := s.RecordSpend(ctx, e)
			require.NoError(t, err)
			require.Equal(t, budget.SpendRateMissing, outcome)
		},
		"a measured event replaces the estimate with its reference": func(t *testing.T) {
			s := setup(t)
			estimate := event("estimate")
			estimate.Measured, estimate.Quantity = false, 5000
			_, err := s.RecordSpend(ctx, estimate)
			require.NoError(t, err)
			panel, err := status.New(db).Status(ctx)
			require.NoError(t, err)
			require.InDelta(t, 0.5, panel.Usage[2].Estimated, 1e-9) // global month

			_, err = s.RecordSpend(ctx, event("measured"))
			require.NoError(t, err)
			panel, err = status.New(db).Status(ctx)
			require.NoError(t, err)
			require.Equal(t, budget.WindowMonth, panel.Usage[2].Window)
			require.InDelta(t, 0, panel.Usage[2].Estimated, 1e-9)
			require.InDelta(t, 0.1, panel.Usage[2].Measured, 1e-9)
		},
		"an unreadable event stores nothing": func(t *testing.T) {
			s := setup(t)
			e := event("")
			outcome, err := s.RecordSpend(ctx, e)
			require.NoError(t, err)
			require.Equal(t, budget.SpendInvalid, outcome)
			e = event("k4")
			e.Category = "unknown"
			outcome, err = s.RecordSpend(ctx, e)
			require.NoError(t, err)
			require.Equal(t, budget.SpendInvalid, outcome)
			require.Equal(t, 0, count(t, `SELECT COUNT(*) FROM budget_spend_events`))
		},
	}
	for name, test := range tests {
		t.Run(name, test)
	}
}
