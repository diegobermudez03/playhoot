package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20261023000000PlayGameFaults stores the game frontend faults
// players' clients report, to audit which games misbehave and which clients
// lie: one row per Session, reporter and kind (play/docs/DATA_MODEL.md).
func migration20261023000000PlayGameFaults() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20261023000000_play_game_faults",
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`CREATE TABLE play_game_faults (
					id BIGSERIAL PRIMARY KEY,
					session_id BIGINT NOT NULL,
					game_version_id BIGINT NOT NULL,
					user_uuid UUID NOT NULL,
					participant_id TEXT NOT NULL,
					kind TEXT NOT NULL CHECK (kind IN ('ARTIFACT_UNREACHABLE', 'ARTIFACT_HASH_MISMATCH', 'READY_TIMEOUT',
						'UNSUPPORTED_BRIDGE_VERSION', 'CONTAINER_VIOLATION', 'BRIDGE_ABUSE')),
					detail JSONB NOT NULL,
					occurrences INTEGER NOT NULL CHECK (occurrences > 0),
					first_reported_at TIMESTAMPTZ NOT NULL,
					last_reported_at TIMESTAMPTZ NOT NULL,
					UNIQUE (session_id, user_uuid, kind)
				)`,
				`CREATE INDEX play_game_faults_game_version_id_kind_idx ON play_game_faults (game_version_id, kind)`,
				`CREATE INDEX play_game_faults_user_uuid_idx ON play_game_faults (user_uuid)`,
			}
			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE play_game_faults`).Error
		},
	}
}
