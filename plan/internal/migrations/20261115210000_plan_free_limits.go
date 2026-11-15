package migrations

import (
	"time"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/plan/internal/business/limits"
	internalrepo "github.com/diegobermudez03/playhoot/plan/internal/repo"
)

// migration20261115210000PlanFreeLimits creates the append-only free-limit
// configuration, seeded with the initial free capacities.
func migration20261115210000PlanFreeLimits() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20261115210000_plan_free_limits",
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`CREATE TABLE plan_free_limit_configs (
					id BIGSERIAL PRIMARY KEY,
					saved_games INTEGER NOT NULL,
					creations_per_window INTEGER NOT NULL,
					sessions_per_window INTEGER NOT NULL,
					window_days INTEGER NOT NULL,
					players_per_session INTEGER NOT NULL,
					simultaneous_rooms INTEGER NOT NULL,
					storage_mb INTEGER NOT NULL,
					effective_at TIMESTAMPTZ NOT NULL,
					created_by_user_uuid UUID,
					reason TEXT NOT NULL,
					created_at TIMESTAMPTZ NOT NULL
				)`,
				`CREATE INDEX plan_free_limit_configs_effective_idx
					ON plan_free_limit_configs (effective_at)`,
			}
			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return err
				}
			}
			start := time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)
			d := limits.Defaults
			return tx.Create(&internalrepo.FreeLimitConfig{
				SavedGames: d.SavedGames, CreationsPerWindow: d.CreationsPerWindow, SessionsPerWindow: d.SessionsPerWindow,
				WindowDays: d.WindowDays, PlayersPerSession: d.PlayersPerSession, SimultaneousRooms: d.SimultaneousRooms,
				StorageMB: d.StorageMB, EffectiveAt: start, Reason: "initial free limits", CreatedAt: start,
			}).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE plan_free_limit_configs`).Error
		},
	}
}
