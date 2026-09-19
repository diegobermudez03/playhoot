package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func migration20260919000000SessionRuntimeTurns() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260919000000_session_runtime_turns",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`
				CREATE TABLE session_runtime_turns (
					id BIGSERIAL PRIMARY KEY,
					session_id BIGINT NOT NULL,
					sequence BIGINT NOT NULL,
					source_kind TEXT NOT NULL,
					source_interaction_id BIGINT NULL,
					source_timer_obligation_id BIGINT NULL,
					actor_id BIGINT NULL,
					snapshot_payload JSONB NOT NULL,
					snapshot_format_version INT NOT NULL DEFAULT 1,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
				)
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE session_runtime_turns`).Error
		},
	}
}
