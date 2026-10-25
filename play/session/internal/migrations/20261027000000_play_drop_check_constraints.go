package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20261027000000PlayDropCheckConstraints leaves the database
// enforcing only keys, unique constraints, indexes and column types: the
// allowed statuses, outcomes, aspect ratios, fault kinds and counts are kept
// by the code that writes them, and checked again by the code that reads them.
func migration20261027000000PlayDropCheckConstraints() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20261027000000_play_drop_check_constraints",
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`ALTER TABLE play_game_versions DROP CONSTRAINT play_game_versions_status_check`,
				`ALTER TABLE play_game_versions DROP CONSTRAINT play_game_versions_aspect_ratio_check`,
				`ALTER TABLE play_sessions DROP CONSTRAINT play_sessions_status_check`,
				`ALTER TABLE play_create_session_requests DROP CONSTRAINT play_create_session_requests_outcome_check`,
				`ALTER TABLE play_game_faults DROP CONSTRAINT play_game_faults_kind_check`,
				`ALTER TABLE play_game_faults DROP CONSTRAINT play_game_faults_occurrences_check`,
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
				`ALTER TABLE play_game_versions ADD CONSTRAINT play_game_versions_status_check CHECK (status IN ('DRAFT', 'EXECUTABLE'))`,
				`ALTER TABLE play_game_versions ADD CONSTRAINT play_game_versions_aspect_ratio_check CHECK (aspect_ratio IN ('16:9', '9:16', '4:3', '3:4', '1:1'))`,
				`ALTER TABLE play_sessions ADD CONSTRAINT play_sessions_status_check CHECK (status IN ('LOBBY', 'RUNNING', 'FINISHED', 'CANCELLED', 'EXPIRED', 'ABORTED'))`,
				`ALTER TABLE play_create_session_requests ADD CONSTRAINT play_create_session_requests_outcome_check CHECK (outcome IN ('created', 'game_unavailable'))`,
				`ALTER TABLE play_game_faults ADD CONSTRAINT play_game_faults_kind_check CHECK (
					(source = 'CLIENT' AND participant_id IS NOT NULL AND kind IN ('ARTIFACT_UNREACHABLE', 'ARTIFACT_HASH_MISMATCH',
						'READY_TIMEOUT', 'UNSUPPORTED_BRIDGE_VERSION', 'CONTAINER_VIOLATION', 'BRIDGE_ABUSE'))
					OR (source = 'SERVER' AND kind = 'SERVER_LIMIT'))`,
				`ALTER TABLE play_game_faults ADD CONSTRAINT play_game_faults_occurrences_check CHECK (occurrences > 0)`,
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
