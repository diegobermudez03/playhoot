package setcontrols

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/budget"
	"github.com/diegobermudez03/playhoot/budget/internal/testdb"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m))
}

func TestAdminSetControls(t *testing.T) {
	ctx := context.Background()
	db := testdb.OpenBudgetDB(t)
	admin := uuid.NewString()
	count := func(t *testing.T, table string) int {
		var n int
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM `+table).Scan(&n).Error)
		return n
	}
	valid := budget.Controls{
		Caps:   []budget.Cap{{Scope: string(budget.CategoryAI), Window: budget.WindowDay, USDLimit: 2, MaxConcurrency: 3}},
		Rates:  []budget.Rate{{Category: budget.CategoryAI, Service: "model", Unit: "tokens", USDPerUnit: 0.00001}},
		Reason: "enable AI",
	}

	tests := map[string]func(t *testing.T){
		"appends caps and rates with actor and reason, keeping history": func(t *testing.T) {
			testdb.Reset(t, db)
			s := New(db)
			for i := 0; i < 2; i++ {
				outcome, err := s.AdminSetControls(ctx, admin, valid)
				require.NoError(t, err)
				require.Equal(t, budget.ControlsRecorded, outcome)
			}
			require.Equal(t, 2, count(t, "budget_caps"))
			require.Equal(t, 2, count(t, "budget_rates"))
			var actor string
			require.NoError(t, db.Raw(`SELECT created_by_user_uuid::text FROM budget_caps LIMIT 1`).Scan(&actor).Error)
			require.Equal(t, admin, actor)
		},
		"invalid changes write nothing": func(t *testing.T) {
			testdb.Reset(t, db)
			s := New(db)
			for name, change := range map[string]budget.Controls{
				"no reason":       {Caps: valid.Caps},
				"empty":           {Reason: "x"},
				"unknown scope":   {Caps: []budget.Cap{{Scope: "unknown", Window: budget.WindowDay, USDLimit: 1}}, Reason: "x"},
				"negative limit":  {Caps: []budget.Cap{{Scope: budget.ScopeGlobal, Window: budget.WindowDay, USDLimit: -1}}, Reason: "x"},
				"unknown window":  {Caps: []budget.Cap{{Scope: budget.ScopeGlobal, Window: "week", USDLimit: 1}}, Reason: "x"},
				"rate no service": {Rates: []budget.Rate{{Category: budget.CategoryAI, Unit: "x", USDPerUnit: 1}}, Reason: "x"},
				"in the past":     {Caps: valid.Caps, Reason: "x", EffectiveAt: time.Now().Add(-time.Hour)},
			} {
				outcome, err := s.AdminSetControls(ctx, admin, change)
				require.NoError(t, err, name)
				require.Equal(t, budget.ControlsInvalid, outcome, name)
			}
			require.Equal(t, 0, count(t, "budget_caps"))
			require.Equal(t, 0, count(t, "budget_rates"))
		},
	}
	for name, test := range tests {
		t.Run(name, test)
	}
}
