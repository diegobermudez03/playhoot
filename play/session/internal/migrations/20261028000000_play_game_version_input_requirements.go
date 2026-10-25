package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20261028000000PlayGameVersionInputRequirements stores the
// interactions each game version uses and the devices that can play it, one
// row per version, as columns a game directory can filter on. The rules are
// the application's (play/internal/business/inputrequirements).
func migration20261028000000PlayGameVersionInputRequirements() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20261028000000_play_game_version_input_requirements",
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`CREATE TABLE play_game_version_input_requirements (
					id BIGSERIAL PRIMARY KEY,
					game_version_id BIGINT NOT NULL UNIQUE,
					uses_keyboard BOOLEAN NOT NULL,
					uses_pointer_hover BOOLEAN NOT NULL,
					uses_pointer_press BOOLEAN NOT NULL,
					devices TEXT NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				// Serves listing the versions a device class can play
				// without reading the table.
				`CREATE INDEX play_game_version_input_requirements_devices_idx
					ON play_game_version_input_requirements (devices, game_version_id)`,
			}
			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE play_game_version_input_requirements`).Error
		},
	}
}
