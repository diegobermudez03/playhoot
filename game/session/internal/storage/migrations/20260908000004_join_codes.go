package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func migration20260908000004JoinCodes() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260908000004_join_codes",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				CREATE TABLE join_codes (
					id BIGSERIAL PRIMARY KEY,
					session_id BIGINT NOT NULL,
					code INTEGER NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					revoked_at TIMESTAMPTZ NULL,
					CONSTRAINT join_codes_code_range
						CHECK (code >= 1000 AND code <= 9999)
				)
			`).Error; err != nil {
				return err
			}

			// At most one active JoinCode per lobby.
			if err := tx.Exec(`
				CREATE UNIQUE INDEX join_codes_session_id_active_key
				ON join_codes (session_id)
				WHERE revoked_at IS NULL
			`).Error; err != nil {
				return err
			}

			// A code must resolve unambiguously to one Session while active.
			return tx.Exec(`
				CREATE UNIQUE INDEX join_codes_code_active_key
				ON join_codes (code)
				WHERE revoked_at IS NULL
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE join_codes`).Error
		},
	}
}
