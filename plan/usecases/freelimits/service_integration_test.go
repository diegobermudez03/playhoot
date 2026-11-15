package freelimits

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/plan"
	"github.com/diegobermudez03/playhoot/plan/internal/business/limits"
	"github.com/diegobermudez03/playhoot/plan/internal/testdb"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m))
}

// The cases share one append-only history, so they run in the order listed.
func TestFreeLimits(t *testing.T) {
	ctx := context.Background()
	db := testdb.OpenPlanDB(t)
	admin := uuid.NewString()
	start := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := start
	service := New(db)
	service.now = func() time.Time { return clock }

	lowered := limits.Defaults
	lowered.SavedGames = 1
	raised := limits.Defaults
	raised.PlayersPerSession = 50

	tests := map[string]func(t *testing.T){
		"the seeded configuration is the P27 defaults": func(t *testing.T) {
			configs, err := service.ReadFreeLimits(ctx)
			require.NoError(t, err)
			require.Equal(t, plan.Limits{SavedGames: 3, CreationsPerWindow: 2, SessionsPerWindow: 5, WindowDays: 30, PlayersPerSession: 10, SimultaneousRooms: 1, StorageMB: 100}, configs.Current.Limits)
			require.Empty(t, configs.Scheduled)
			require.Empty(t, configs.History)
		},
		"a future effective date is scheduled and not current yet": func(t *testing.T) {
			outcome, err := service.AdminSetFreeLimits(ctx, admin, raised, start.Add(48*time.Hour), "more players")
			require.NoError(t, err)
			require.Equal(t, plan.FreeLimitsRecorded, outcome)

			configs, err := service.ReadFreeLimits(ctx)
			require.NoError(t, err)
			require.Equal(t, limits.Defaults, configs.Current.Limits)
			require.Len(t, configs.Scheduled, 1)
			require.Equal(t, raised, configs.Scheduled[0].Limits)
			require.Equal(t, admin, configs.Scheduled[0].CreatedByUserUUID)

			clock = start.Add(49 * time.Hour)
			configs, err = service.ReadFreeLimits(ctx)
			require.NoError(t, err)
			require.Equal(t, raised, configs.Current.Limits)
			require.Empty(t, configs.Scheduled)
		},
		"history is preserved when limits are lowered": func(t *testing.T) {
			outcome, err := service.AdminSetFreeLimits(ctx, admin, lowered, time.Time{}, "lower games")
			require.NoError(t, err)
			require.Equal(t, plan.FreeLimitsRecorded, outcome)

			configs, err := service.ReadFreeLimits(ctx)
			require.NoError(t, err)
			require.Equal(t, lowered, configs.Current.Limits)
			require.Len(t, configs.History, 2)
			require.Equal(t, raised, configs.History[0].Limits)
			require.Equal(t, limits.Defaults, configs.History[1].Limits)
			require.Empty(t, configs.History[1].CreatedByUserUUID)
		},
		"invalid configurations are refused and nothing is written": func(t *testing.T) {
			bad := limits.Defaults
			bad.WindowDays = 0
			for name, call := range map[string]func() (plan.SetFreeLimitsOutcome, error){
				"out of range": func() (plan.SetFreeLimitsOutcome, error) {
					return service.AdminSetFreeLimits(ctx, admin, bad, time.Time{}, "r")
				},
				"no reason": func() (plan.SetFreeLimitsOutcome, error) {
					return service.AdminSetFreeLimits(ctx, admin, limits.Defaults, time.Time{}, "")
				},
				"effective in the past": func() (plan.SetFreeLimitsOutcome, error) {
					return service.AdminSetFreeLimits(ctx, admin, limits.Defaults, clock.Add(-time.Hour), "r")
				},
			} {
				before, err := service.ReadFreeLimits(ctx)
				require.NoError(t, err)
				outcome, err := call()
				require.NoError(t, err)
				require.Equal(t, plan.FreeLimitsInvalid, outcome, name)
				after, err := service.ReadFreeLimits(ctx)
				require.NoError(t, err)
				require.Equal(t, before, after, name)
			}
		},
		"a malformed admin is an error": func(t *testing.T) {
			_, err := service.AdminSetFreeLimits(ctx, "not-a-uuid", limits.Defaults, time.Time{}, "r")
			require.Error(t, err)
		},
	}
	for _, name := range []string{
		"the seeded configuration is the P27 defaults",
		"a future effective date is scheduled and not current yet",
		"history is preserved when limits are lowered",
		"invalid configurations are refused and nothing is written",
		"a malformed admin is an error",
	} {
		t.Run(name, tests[name])
	}
}
