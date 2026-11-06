package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20261106000000PlaySimulationResults records each simulation of a
// version, its whole result as one JSON value kept until expires_at (then
// cleared), so its states can be read on demand and simulations counted.
func migration20261106000000PlaySimulationResults() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20261106000000_play_simulation_results",
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`CREATE TABLE play_simulation_results (
					id BIGSERIAL PRIMARY KEY,
					uuid UUID NOT NULL UNIQUE,
					game_version_id BIGINT NOT NULL,
					result JSONB NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					expires_at TIMESTAMPTZ NOT NULL
				)`,
				// Serves counting a version's simulations over a window.
				`CREATE INDEX play_simulation_results_version_idx ON play_simulation_results (game_version_id, created_at)`,
				// Serve clearing expired results and forgetting old records.
				`CREATE INDEX play_simulation_results_expiry_idx ON play_simulation_results (expires_at)`,
				`CREATE INDEX play_simulation_results_created_idx ON play_simulation_results (created_at)`,
			}
			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE play_simulation_results`).Error
		},
	}
}
