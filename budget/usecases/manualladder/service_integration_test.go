package manualladder

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/budget"
	"github.com/diegobermudez03/playhoot/budget/internal/testdb"
	"github.com/diegobermudez03/playhoot/budget/usecases/admitwork"
	"github.com/diegobermudez03/playhoot/budget/workflows/evaluateladder"
	"github.com/diegobermudez03/playhoot/utils/txqueue"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m))
}

func TestManualLadder(t *testing.T) {
	ctx := context.Background()
	db := testdb.OpenBudgetDB(t)
	queue := txqueue.NewRegistry(db, txqueue.DefaultWorkerConfig).Queue(budget.QueueDomain)
	admin := uuid.NewString()
	start := time.Now().UTC().AddDate(0, 0, -1)

	setup := func(t *testing.T) *Service {
		testdb.Reset(t, db)
		testdb.SetControls(t, db,
			testdb.Control{Cap: &budget.Cap{Scope: budget.ScopeGlobal, Window: budget.WindowMonth, USDLimit: 120}, At: start},
			testdb.Control{Cap: &budget.Cap{Scope: string(budget.CategoryEmail), Window: budget.WindowMonth, USDLimit: 10}, At: start},
			testdb.Control{Rate: &budget.Rate{Category: budget.CategoryEmail, Service: "ses-outbound", Unit: "emails", USDPerUnit: 0.0001}, At: start},
		)
		return New(db, queue)
	}
	rows := func(t *testing.T) int {
		var n int
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM budget_restriction_changes`).Scan(&n).Error)
		return n
	}
	email := budget.WorkRequest{Category: budget.CategoryEmail, Service: "ses-outbound", Quantity: 1}
	admit := func(t *testing.T) budget.Admission {
		admission, err := admitwork.New(db).AdmitWork(ctx, email)
		require.NoError(t, err)
		return admission
	}
	evaluate := func(t *testing.T) budget.LadderState {
		state, err := evaluateladder.New(db, queue).Evaluate(ctx)
		require.NoError(t, err)
		return state
	}

	tests := map[string]func(t *testing.T){
		"cutoff shuts down at once and is idempotent": func(t *testing.T) {
			s := setup(t)
			outcome, err := s.AdminCutoff(ctx, admin, "suspicious spike")
			require.NoError(t, err)
			require.Equal(t, budget.CutoffApplied, outcome)
			outcome, err = s.AdminCutoff(ctx, admin, "again")
			require.NoError(t, err)
			require.Equal(t, budget.CutoffAlreadyApplied, outcome)
			require.Equal(t, 1, rows(t))
			require.Equal(t, budget.RefusedShutdown, admit(t).Reason)
			require.Equal(t, budget.LevelShutdown, evaluate(t).Level)
		},
		"a cutoff needs a reason": func(t *testing.T) {
			s := setup(t)
			outcome, err := s.AdminCutoff(ctx, admin, "")
			require.NoError(t, err)
			require.Equal(t, budget.CutoffInvalid, outcome)
			require.Equal(t, 0, rows(t))
		},
		"reopening is only valid from shutdown": func(t *testing.T) {
			s := setup(t)
			outcome, err := s.Reopen(ctx, admin, nil, "nothing to reopen")
			require.NoError(t, err)
			require.Equal(t, budget.ReopenNotShutDown, outcome)
			require.Equal(t, 0, rows(t))
		},
		"reopening is gradual, one step per call": func(t *testing.T) {
			s := setup(t)
			_, err := s.AdminCutoff(ctx, admin, "spike")
			require.NoError(t, err)

			outcome, err := s.Reopen(ctx, admin, []budget.Category{budget.CategoryEmail}, "skip ahead")
			require.NoError(t, err)
			require.Equal(t, budget.ReopenInvalid, outcome)

			outcome, err = s.Reopen(ctx, admin, nil, "reviewed")
			require.NoError(t, err)
			require.Equal(t, budget.ReopenStarted, outcome)
			state := evaluate(t)
			require.Equal(t, budget.LevelRestricted, state.Level)
			require.Len(t, state.ReopenClosed, len(budget.Categories))
			require.Equal(t, budget.RefusedPaused, admit(t).Reason)

			outcome, err = s.Reopen(ctx, admin, []budget.Category{budget.CategoryEmail}, "email first")
			require.NoError(t, err)
			require.Equal(t, budget.ReopenOpened, outcome)
			require.Equal(t, budget.Admitted, admit(t).Outcome)
			outcome, err = s.Reopen(ctx, admin, []budget.Category{budget.CategoryEmail}, "again")
			require.NoError(t, err)
			require.Equal(t, budget.ReopenAlreadyOpen, outcome)

			state = evaluate(t)
			require.Equal(t, budget.LevelRestricted, state.Level)
			require.NotContains(t, state.ReopenClosed, budget.CategoryEmail)

			var rest []budget.Category
			for _, c := range budget.Categories {
				if c != budget.CategoryEmail {
					rest = append(rest, c)
				}
			}
			outcome, err = s.Reopen(ctx, admin, rest, "the rest")
			require.NoError(t, err)
			require.Equal(t, budget.ReopenOpened, outcome)
			require.Equal(t, budget.LevelNormal, evaluate(t).Level)
			outcome, err = s.Reopen(ctx, admin, nil, "done")
			require.NoError(t, err)
			require.Equal(t, budget.ReopenNotShutDown, outcome)
		},
	}
	for name, test := range tests {
		t.Run(name, test)
	}
}
