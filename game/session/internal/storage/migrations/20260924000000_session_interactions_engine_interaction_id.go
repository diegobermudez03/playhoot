package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20260924000000SessionInteractionsEngineInteractionID replaces
// session_interactions' (engine_path, engine_slot) identity with the Game
// Language engine's own assigned engine_interaction_id - the engine no
// longer addresses a Question/Ask Group occurrence by its owning instance
// path plus slot name, but by one engine-assigned interaction identity. This
// is a pre-launch schema replacement, not a data migration:
// session_interactions has no production deployment/data-preservation
// requirement, matching the precedent
// game/session/internal/storage/migrations/20260908000000_drop_legacy_session_schema.go
// already established for this repository.
//
// session_timer_obligations is unaffected by this migration - Timer
// occurrences are not addressed by an InteractionID. It also carries no
// engine_path column of its own: engine.ScheduleTimerOutput/
// CancelTimerOutput never had a Path field to begin with, unlike questions
// before this migration.
func migration20260924000000SessionInteractionsEngineInteractionID() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260924000000_session_interactions_engine_interaction_id",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`DROP INDEX session_interactions_active_slot_key`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`ALTER TABLE session_interactions DROP COLUMN engine_path`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`ALTER TABLE session_interactions DROP COLUMN engine_slot`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`ALTER TABLE session_interactions ADD COLUMN engine_interaction_id BIGINT NOT NULL`).Error; err != nil {
				return err
			}

			// At most one ACTIVE interaction per (session, InteractionID) -
			// InteractionID values are never reused for the lifetime of a
			// Snapshot (engine.InteractionID's own documented guarantee), so
			// this is a unique key on its own; session_id is kept as a
			// defense-in-depth co-predicate, matching the equivalent
			// engine_interaction_id = ? lookup CloseActiveInteraction uses.
			return tx.Exec(`
				CREATE UNIQUE INDEX session_interactions_active_interaction_key
				ON session_interactions (session_id, engine_interaction_id)
				WHERE state = 'ACTIVE'
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec(`DROP INDEX session_interactions_active_interaction_key`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`ALTER TABLE session_interactions DROP COLUMN engine_interaction_id`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`ALTER TABLE session_interactions ADD COLUMN engine_path JSONB NOT NULL DEFAULT '[]'::jsonb`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`ALTER TABLE session_interactions ADD COLUMN engine_slot TEXT NOT NULL DEFAULT ''`).Error; err != nil {
				return err
			}

			return tx.Exec(`
				CREATE UNIQUE INDEX session_interactions_active_slot_key
				ON session_interactions (session_id, engine_path, engine_slot, session_actor_id)
				WHERE state = 'ACTIVE'
			`).Error
		},
	}
}
