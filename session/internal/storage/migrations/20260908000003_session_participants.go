package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func migration20260908000003SessionParticipants() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260908000003_session_participants",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`
				CREATE TABLE session_participants (
					id BIGSERIAL PRIMARY KEY,
					session_actor_id BIGINT NOT NULL UNIQUE,
					display_name TEXT NOT NULL,
					active BOOLEAN NOT NULL DEFAULT TRUE,
					joined_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					left_at TIMESTAMPTZ NULL
				)
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE session_participants`).Error
		},
	}
}
