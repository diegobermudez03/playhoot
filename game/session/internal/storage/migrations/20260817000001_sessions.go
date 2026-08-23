package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func migration20260817000001Sessions() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260817000001_sessions",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				CREATE TABLE sessions (
					id BIGSERIAL PRIMARY KEY,
					uuid UUID NOT NULL UNIQUE,
					game_version_uuid UUID NOT NULL,
					owner_uuid UUID NOT NULL,
					started_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					ended_at TIMESTAMPTZ NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					CONSTRAINT sessions_started_at_matches_created_at
						CHECK (started_at = created_at)
				)
			`).Error; err != nil {
				return err
			}

			if err := tx.Exec(`
				CREATE TABLE session_players (
					id BIGSERIAL PRIMARY KEY,
					session_id UUID NOT NULL REFERENCES sessions (uuid),
					player_uuid UUID NOT NULL,
					joined_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					left_at TIMESTAMPTZ NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					CONSTRAINT session_players_session_id_player_uuid_key
						UNIQUE (session_id, player_uuid)
				)
			`).Error; err != nil {
				return err
			}

			return tx.Exec(`
				CREATE TABLE join_codes (
					id BIGSERIAL PRIMARY KEY,
					code INTEGER NOT NULL,
					session_id UUID NOT NULL REFERENCES sessions (uuid),
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					deleted_at TIMESTAMPTZ NULL,
					CONSTRAINT join_codes_code_range
						CHECK (code >= 1000 AND code <= 9999)
				)
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec(`DROP TABLE join_codes`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`DROP TABLE session_players`).Error; err != nil {
				return err
			}
			return tx.Exec(`DROP TABLE sessions`).Error
		},
	}
}
