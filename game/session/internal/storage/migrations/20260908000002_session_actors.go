package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func migration20260908000002SessionActors() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260908000002_session_actors",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`
				CREATE TABLE session_actors (
					id BIGSERIAL PRIMARY KEY,
					session_id BIGINT NOT NULL,
					user_uuid UUID NOT NULL,
					semantic_presence TEXT NOT NULL DEFAULT 'CONNECTED',
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					CONSTRAINT session_actors_session_id_user_uuid_key
						UNIQUE (session_id, user_uuid)
				)
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE session_actors`).Error
		},
	}
}
