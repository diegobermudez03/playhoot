package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20261024000000PlayGameFaultsServerLimits lets play_game_faults
// also keep the platform limits a Session's users hit (source SERVER, kind
// SERVER_LIMIT), next to what clients report (source CLIENT).
func migration20261024000000PlayGameFaultsServerLimits() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20261024000000_play_game_faults_server_limits",
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`ALTER TABLE play_game_faults ADD COLUMN source TEXT NOT NULL DEFAULT 'CLIENT'`,
				`ALTER TABLE play_game_faults ALTER COLUMN source DROP DEFAULT`,
				// An admin hitting a limit is no participant.
				`ALTER TABLE play_game_faults ALTER COLUMN participant_id DROP NOT NULL`,
				`ALTER TABLE play_game_faults DROP CONSTRAINT play_game_faults_kind_check`,
				`ALTER TABLE play_game_faults ADD CONSTRAINT play_game_faults_kind_check CHECK (
					(source = 'CLIENT' AND participant_id IS NOT NULL AND kind IN ('ARTIFACT_UNREACHABLE', 'ARTIFACT_HASH_MISMATCH',
						'READY_TIMEOUT', 'UNSUPPORTED_BRIDGE_VERSION', 'CONTAINER_VIOLATION', 'BRIDGE_ABUSE'))
					OR (source = 'SERVER' AND kind = 'SERVER_LIMIT'))`,
			}
			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			statements := []string{
				`DELETE FROM play_game_faults WHERE source = 'SERVER'`,
				`ALTER TABLE play_game_faults DROP CONSTRAINT play_game_faults_kind_check`,
				`ALTER TABLE play_game_faults ADD CONSTRAINT play_game_faults_kind_check CHECK (kind IN ('ARTIFACT_UNREACHABLE',
					'ARTIFACT_HASH_MISMATCH', 'READY_TIMEOUT', 'UNSUPPORTED_BRIDGE_VERSION', 'CONTAINER_VIOLATION', 'BRIDGE_ABUSE'))`,
				`ALTER TABLE play_game_faults ALTER COLUMN participant_id SET NOT NULL`,
				`ALTER TABLE play_game_faults DROP COLUMN source`,
			}
			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return err
				}
			}
			return nil
		},
	}
}
