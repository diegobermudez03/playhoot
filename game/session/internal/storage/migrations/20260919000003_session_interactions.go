package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20260919000003SessionInteractions creates session_interactions,
// the durable record of one opened question/ask-group instance and how it
// was resolved. engine_path/interaction_payload/response_payload hold
// engine.Value-typed data encoded through engineservice.EncodeValue -
// JSONB, not TEXT, so an ACTIVE row can be matched by (session_id,
// engine_path, engine_slot, session_actor_id) using Postgres's semantic
// jsonb equality rather than raw byte comparison.
func migration20260919000003SessionInteractions() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260919000003_session_interactions",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				CREATE TABLE session_interactions (
					id BIGSERIAL PRIMARY KEY,
					uuid TEXT NOT NULL UNIQUE,
					session_id BIGINT NOT NULL,
					session_actor_id BIGINT NOT NULL,
					kind TEXT NOT NULL,
					engine_path JSONB NOT NULL,
					engine_slot TEXT NOT NULL,
					interaction_payload JSONB NOT NULL,
					response_payload JSONB NULL,
					state TEXT NOT NULL,
					opened_by_turn_id BIGINT NOT NULL,
					closed_by_turn_id BIGINT NULL,
					closure_reason TEXT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
				)
			`).Error; err != nil {
				return err
			}

			// At most one ACTIVE interaction per (session, engine instance
			// path, slot, recipient) - the same "slot already occupied"
			// invariant the engine itself enforces (ExecutionErrorSlotOccupied),
			// mirrored here since this is also the exact lookup key closing a
			// CloseQuestionOutput uses to find the row it closes.
			if err := tx.Exec(`
				CREATE UNIQUE INDEX session_interactions_active_slot_key
				ON session_interactions (session_id, engine_path, engine_slot, session_actor_id)
				WHERE state = 'ACTIVE'
			`).Error; err != nil {
				return err
			}

			// Supports finding every still-ACTIVE interaction for a Session,
			// so terminalizing it can close all of them atomically.
			return tx.Exec(`
				CREATE INDEX session_interactions_session_active_idx
				ON session_interactions (session_id)
				WHERE state = 'ACTIVE'
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE session_interactions`).Error
		},
	}
}
