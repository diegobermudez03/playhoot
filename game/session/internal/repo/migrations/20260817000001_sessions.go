package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func migration20260817000001Sessions() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260817000001_sessions",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`
				CREATE TABLE sessions (
					id BIGSERIAL PRIMARY KEY,
					uuid UUID NOT NULL UNIQUE,
					game_version_uuid UUID NOT NULL,
					started_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					ended_at TIMESTAMPTZ NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					CONSTRAINT sessions_started_at_matches_created_at
						CHECK (started_at = created_at)
				)
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE sessions`).Error
		},
	}
}
