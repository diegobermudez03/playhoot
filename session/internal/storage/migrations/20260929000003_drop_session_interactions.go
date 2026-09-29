package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20260929000003DropSessionInteractions drops session_interactions
// and session_runtime_turns/session_runtime_failures.source_interaction_id.
// The platform's closed Event/Command vocabulary has no OPEN_INTERACTION
// concept at all - every game-defined player action is a PLAYER_EVENT - so
// the durable "an interaction is open/closed, of kind QUESTION/ASK_GROUP"
// model this table encoded has no platform-level concept left to represent.
// A PLAYER_EVENT's own idempotency/dedup reuses session_requests' existing
// claim mechanism (the same one Create/CancelSession already use), not a
// repurposed session_interactions row.
func migration20260929000003DropSessionInteractions() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260929000003_drop_session_interactions",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`ALTER TABLE session_runtime_turns DROP COLUMN source_interaction_id`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`ALTER TABLE session_runtime_failures DROP COLUMN source_interaction_id`).Error; err != nil {
				return err
			}
			return tx.Exec(`DROP TABLE session_interactions`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec(`ALTER TABLE session_runtime_failures ADD COLUMN source_interaction_id BIGINT NULL`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`
				CREATE TABLE session_interactions (
					id BIGSERIAL PRIMARY KEY,
					uuid UUID NOT NULL,
					session_id BIGINT NOT NULL,
					session_actor_id BIGINT NOT NULL,
					kind TEXT NOT NULL,
					engine_interaction_id BIGINT NOT NULL,
					interaction_payload JSONB NOT NULL,
					response_payload JSONB NULL,
					state TEXT NOT NULL,
					closure_reason TEXT NULL,
					opened_by_turn_id BIGINT NOT NULL,
					closed_by_turn_id BIGINT NULL
				)
			`).Error; err != nil {
				return err
			}
			return tx.Exec(`ALTER TABLE session_runtime_turns ADD COLUMN source_interaction_id BIGINT NULL`).Error
		},
	}
}
