package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func migration20260919000001SessionRuntimeSteps() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260919000001_session_runtime_steps",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`
				CREATE TABLE session_runtime_steps (
					id BIGSERIAL PRIMARY KEY,
					runtime_turn_id BIGINT NOT NULL,
					step_index INT NOT NULL,
					commit_payload JSONB NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
				)
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE session_runtime_steps`).Error
		},
	}
}
