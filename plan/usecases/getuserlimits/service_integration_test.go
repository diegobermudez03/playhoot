package getuserlimits

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/plan"
	"github.com/diegobermudez03/playhoot/plan/internal/business/limits"
	internalrepo "github.com/diegobermudez03/playhoot/plan/internal/repo"
	"github.com/diegobermudez03/playhoot/plan/internal/testdb"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m))
}

// The cases share one append-only history, so they run in the order listed.
func TestGetUserLimits(t *testing.T) {
	ctx := context.Background()
	db := testdb.OpenPlanDB(t)
	repo := internalrepo.New(db)
	start := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := start
	service := New(db)
	service.now = func() time.Time { return clock }

	insert := func(l plan.Limits, effectiveAt time.Time) {
		require.NoError(t, repo.InsertFreeLimitConfig(ctx, internalrepo.FreeLimitConfig{
			SavedGames: l.SavedGames, CreationsPerWindow: l.CreationsPerWindow, SessionsPerWindow: l.SessionsPerWindow,
			WindowDays: l.WindowDays, PlayersPerSession: l.PlayersPerSession, SimultaneousRooms: l.SimultaneousRooms,
			StorageMB: l.StorageMB, EffectiveAt: effectiveAt, Reason: "test", CreatedAt: start,
		}))
	}
	raised := limits.Defaults
	raised.PlayersPerSession = 50
	lowered := limits.Defaults
	lowered.SavedGames = 1

	tests := map[string]func(t *testing.T){
		"unknown and empty users get the seeded defaults": func(t *testing.T) {
			for _, user := range []string{"", uuid.NewString()} {
				got, err := service.GetUserLimits(ctx, user)
				require.NoError(t, err)
				require.Equal(t, limits.Defaults, got)
			}
		},
		"a future effective date is not active yet": func(t *testing.T) {
			insert(raised, start.Add(48*time.Hour))
			got, err := service.GetUserLimits(ctx, uuid.NewString())
			require.NoError(t, err)
			require.Equal(t, limits.Defaults, got)

			clock = start.Add(49 * time.Hour)
			got, err = service.GetUserLimits(ctx, uuid.NewString())
			require.NoError(t, err)
			require.Equal(t, raised, got)
		},
		"the latest effective configuration wins and lowering is just a read": func(t *testing.T) {
			insert(lowered, start.Add(50*time.Hour))
			got, err := service.GetUserLimits(ctx, uuid.NewString())
			require.NoError(t, err)
			require.Equal(t, raised, got)

			clock = start.Add(51 * time.Hour)
			got, err = service.GetUserLimits(ctx, uuid.NewString())
			require.NoError(t, err)
			require.Equal(t, lowered, got)
		},
	}
	for _, name := range []string{
		"unknown and empty users get the seeded defaults",
		"a future effective date is not active yet",
		"the latest effective configuration wins and lowering is just a read",
	} {
		t.Run(name, tests[name])
	}
}
