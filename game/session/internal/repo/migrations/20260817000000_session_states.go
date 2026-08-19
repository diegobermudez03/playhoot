package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func migration20260817000000SessionStates() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260817000000_session_states",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`
				CREATE TABLE session_states (
					id BIGSERIAL PRIMARY KEY,
					state_number BIGINT NOT NULL,
					session_id BIGINT NOT NULL REFERENCES sessions (id),
					json_state JSONB NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					CONSTRAINT session_states_session_id_state_number_key
						UNIQUE (session_id, state_number)
				)
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE session_states`).Error
		},
	}
}
