package limits

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/plan"
)

func TestValid(t *testing.T) {
	with := func(change func(l *plan.Limits)) plan.Limits {
		l := Defaults
		change(&l)
		return l
	}
	tests := map[string]struct {
		limits plan.Limits
		valid  bool
	}{
		"defaults":            {Defaults, true},
		"zero capacities":     {with(func(l *plan.Limits) { l.SavedGames, l.StorageMB = 0, 0 }), true},
		"negative games":      {with(func(l *plan.Limits) { l.SavedGames = -1 }), false},
		"too many players":    {with(func(l *plan.Limits) { l.PlayersPerSession = MaxCount + 1 }), false},
		"window of zero days": {with(func(l *plan.Limits) { l.WindowDays = 0 }), false},
		"window over a year":  {with(func(l *plan.Limits) { l.WindowDays = MaxWindowDays + 1 }), false},
		"negative storage":    {with(func(l *plan.Limits) { l.StorageMB = -5 }), false},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.valid, Valid(tc.limits))
		})
	}
}
