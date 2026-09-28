package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func migration20260908000001Sessions() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260908000001_sessions",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`
				CREATE TABLE sessions (
					id BIGSERIAL PRIMARY KEY,
					uuid UUID NOT NULL UNIQUE,
					game_definition_uuid UUID NOT NULL,
					host_actor_id BIGINT NULL,
					phase TEXT NOT NULL DEFAULT 'LOBBY',
					lobby_expires_at TIMESTAMPTZ NOT NULL,
					started_at TIMESTAMPTZ NULL,
					terminal_at TIMESTAMPTZ NULL,
					terminal_reason TEXT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
				)
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE sessions`).Error
		},
	}
}
