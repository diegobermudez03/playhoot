package marketrules

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/compliance"
	"github.com/diegobermudez03/playhoot/compliance/internal/testdb"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m))
}

func TestMarketRules(t *testing.T) {
	ctx := context.Background()
	db := testdb.OpenComplianceDB(t)
	service := New(db)
	admin := uuid.NewString()

	tests := map[string]func(t *testing.T){
		"the initial matrix is seeded": func(t *testing.T) {
			count := func(country string) (int, int) {
				matrix, err := service.ReadMarketRules(ctx, country)
				require.NoError(t, err)
				enabled := 0
				for _, rule := range matrix.Current {
					if rule.Enabled {
						enabled++
					}
				}
				return len(matrix.Current), enabled
			}
			for country, expected := range map[string][2]int{
				"CO": {6, 6}, "MX": {6, 6}, "AR": {6, 0}, "CL": {6, 0}, "PE": {6, 0},
				"US": {51 * 6, 51 * 4}, "CA": {12 * 6, 12 * 4}, "BR": {0, 0},
			} {
				rules, enabled := count(country)
				require.Equal(t, expected, [2]int{rules, enabled}, country)
			}
			matrix, err := service.ReadMarketRules(ctx, "AR")
			require.NoError(t, err)
			require.NotEmpty(t, matrix.Current[0].ConditionsNote)
		},
		"enabling with pending conditions is refused until the note is cleared": func(t *testing.T) {
			rule := compliance.MarketRule{Country: "AR", Function: compliance.FunctionCreateGames, Enabled: true, MinAge: 18, ConditionsNote: "AAIP pending", Reason: "try"}
			outcome, err := service.AdminSetMarketRule(ctx, admin, rule)
			require.NoError(t, err)
			require.Equal(t, compliance.MarketRuleConditionsPending, outcome)
		},
		"malformed rules are refused": func(t *testing.T) {
			now := time.Now()
			for name, rule := range map[string]compliance.MarketRule{
				"no reason":         {Country: "ZX", Function: compliance.FunctionTeach, MinAge: 18},
				"bad country":       {Country: "Colombia", Function: compliance.FunctionTeach, MinAge: 18, Reason: "r"},
				"unknown function":  {Country: "ZX", Function: "gamble", MinAge: 18, Reason: "r"},
				"adult without age": {Country: "ZX", Function: compliance.FunctionTeach, Reason: "r"},
				"minor with an age": {Country: "ZX", Function: compliance.FunctionMinorClassParticipation, MinAge: 18, Reason: "r"},
				"effective in past": {Country: "ZX", Function: compliance.FunctionTeach, MinAge: 18, Reason: "r", EffectiveAt: now.Add(-time.Hour)},
			} {
				outcome, err := service.AdminSetMarketRule(ctx, admin, rule)
				require.NoError(t, err)
				require.Equal(t, compliance.MarketRuleInvalid, outcome, name)
			}
		},
		"rules take effect on their date and the old ones stay as history": func(t *testing.T) {
			start := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
			service.now = func() time.Time { return start }
			defer func() { service.now = time.Now }()
			set := func(rule compliance.MarketRule) {
				outcome, err := service.AdminSetMarketRule(ctx, admin, rule)
				require.NoError(t, err)
				require.Equal(t, compliance.MarketRuleRecorded, outcome)
			}
			set(compliance.MarketRule{Country: "ZX", Function: compliance.FunctionTeach, Enabled: false, MinAge: 18, ConditionsNote: "pending", Reason: "prepared"})
			set(compliance.MarketRule{Country: "ZX", Function: compliance.FunctionTeach, Enabled: true, MinAge: 18, Reason: "conditions met", EffectiveAt: start.Add(24 * time.Hour)})

			matrix, err := service.ReadMarketRules(ctx, "ZX")
			require.NoError(t, err)
			require.Len(t, matrix.Current, 1)
			require.False(t, matrix.Current[0].Enabled)
			require.Equal(t, admin, matrix.Current[0].CreatedByUserUUID)
			require.Len(t, matrix.Scheduled, 1)
			require.Empty(t, matrix.History)

			service.now = func() time.Time { return start.Add(48 * time.Hour) }
			matrix, err = service.ReadMarketRules(ctx, "ZX")
			require.NoError(t, err)
			require.Len(t, matrix.Current, 1)
			require.True(t, matrix.Current[0].Enabled)
			require.Equal(t, "conditions met", matrix.Current[0].Reason)
			require.Empty(t, matrix.Scheduled)
			require.Len(t, matrix.History, 1)
			require.Equal(t, "prepared", matrix.History[0].Reason)
		},
	}
	for name, test := range tests {
		t.Run(name, test)
	}
}
