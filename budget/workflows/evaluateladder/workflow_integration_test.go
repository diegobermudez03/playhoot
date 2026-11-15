package evaluateladder

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/budget"
	"github.com/diegobermudez03/playhoot/budget/internal/testdb"
	"github.com/diegobermudez03/playhoot/utils/txqueue"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m))
}

func TestEvaluateLadder(t *testing.T) {
	ctx := context.Background()
	db := testdb.OpenBudgetDB(t)
	queue := txqueue.NewRegistry(db, txqueue.DefaultWorkerConfig).Queue(budget.QueueDomain)
	now := time.Date(2030, 3, 14, 10, 30, 0, 0, time.UTC)
	start := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)

	setup := func(t *testing.T) *Workflow {
		testdb.Reset(t, db)
		testdb.SetControls(t, db,
			testdb.Control{Cap: &budget.Cap{Scope: budget.ScopeGlobal, Window: budget.WindowHour, USDLimit: 1}, At: start},
			testdb.Control{Cap: &budget.Cap{Scope: budget.ScopeGlobal, Window: budget.WindowDay, USDLimit: 8}, At: start},
			testdb.Control{Cap: &budget.Cap{Scope: budget.ScopeGlobal, Window: budget.WindowMonth, USDLimit: 120}, At: start},
			testdb.Control{Cap: &budget.Cap{Scope: string(budget.CategoryEmail), Window: budget.WindowDay, USDLimit: 1}, At: start},
		)
		return at(New(db, queue), now)
	}
	evaluate := func(t *testing.T, w *Workflow) budget.LadderState {
		state, err := w.Evaluate(ctx)
		require.NoError(t, err)
		return state
	}
	count := func(t *testing.T, query string, args ...any) int {
		var n int
		require.NoError(t, db.Raw(query, args...).Scan(&n).Error)
		return n
	}

	tests := map[string]func(t *testing.T){
		"escalates under pressure and tells the administrator once": func(t *testing.T) {
			w := setup(t)
			require.Equal(t, budget.LevelNormal, evaluate(t, w).Level)
			testdb.Spend(t, db, budget.CategoryAI, 0.85, true, "", now.Add(-5*time.Minute))
			state := evaluate(t, w)
			require.Equal(t, budget.LevelRestricted, state.Level)
			require.Equal(t, budget.ActorAutomatic, state.Actor)
			require.Equal(t, budget.LevelRestricted, evaluate(t, w).Level)
			require.Equal(t, 1, count(t, `SELECT COUNT(*) FROM budget_restriction_changes`))
			require.Equal(t, 1, count(t, `SELECT COUNT(*) FROM budget_queue_tasks WHERE type = ?`, txqueue.TypeNotify))
		},
		"pauses the category that threatens its cap": func(t *testing.T) {
			w := setup(t)
			testdb.Spend(t, db, budget.CategoryEmail, 0.96, true, "", now.Add(-2*time.Hour))
			state := evaluate(t, w)
			require.Equal(t, budget.LevelPaused, state.Level)
			require.Equal(t, []budget.Category{budget.CategoryEmail}, state.PausedCategories)
		},
		"de-escalates automatically when pressure clears": func(t *testing.T) {
			w := setup(t)
			testdb.Spend(t, db, budget.CategoryAI, 0.85, true, "", now.Add(-5*time.Minute))
			require.Equal(t, budget.LevelRestricted, evaluate(t, w).Level)
			w = at(w, now.Add(2*time.Hour))
			require.Equal(t, budget.LevelNormal, evaluate(t, w).Level)
		},
		"shutdown sticks across a restart and a new day": func(t *testing.T) {
			w := setup(t)
			testdb.Spend(t, db, budget.CategoryAI, 1, true, "", now.Add(-5*time.Minute))
			require.Equal(t, budget.LevelShutdown, evaluate(t, w).Level)

			restarted := at(New(db, queue), now.AddDate(0, 0, 1))
			require.Equal(t, budget.LevelShutdown, evaluate(t, restarted).Level)
			nextMonth := at(New(db, queue), now.AddDate(0, 1, 0))
			require.Equal(t, budget.LevelShutdown, evaluate(t, nextMonth).Level)
			require.Equal(t, 1, count(t, `SELECT COUNT(*) FROM budget_restriction_changes`))
		},
	}
	for name, test := range tests {
		t.Run(name, test)
	}
}

func at(w *Workflow, now time.Time) *Workflow {
	w.now = func() time.Time { return now }
	return w
}
