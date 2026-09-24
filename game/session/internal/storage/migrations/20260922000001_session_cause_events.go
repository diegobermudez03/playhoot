package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20260922000001SessionCauseEvents creates session_cause_events,
// the shared generic satellite table for a RuntimeTurn-producing cause that
// is a pure immutable occurrence with no existing normalized home (for
// example a submitted user intent, a manual cancellation, or a
// disconnect/reconnect occurrence), discriminated by cause_kind.
// session_interactions/session_timer_obligations are deliberately not
// folded into this table: both carry independent mutable lifecycle state
// (open, then later resolved) that a write-once occurrence record does not
// fit.
//
// This migration also adds session_runtime_turns.source_cause_event_id, a
// third nullable cause pointer mirroring the already-established
// source_interaction_id/source_timer_obligation_id pattern.
//
// Neither the table nor the column is populated yet: no RuntimeTurn cause
// using them is implemented today. A future capability introducing such a
// cause is expected to write into this shared table with its own
// cause_kind/payload shape rather than introduce a bespoke table of its own.
func migration20260922000001SessionCauseEvents() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260922000001_session_cause_events",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				CREATE TABLE session_cause_events (
					id BIGSERIAL PRIMARY KEY,
					session_id BIGINT NOT NULL,
					runtime_turn_id BIGINT NOT NULL,
					cause_kind TEXT NOT NULL,
					actor_id BIGINT NULL,
					payload JSONB NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
				)
			`).Error; err != nil {
				return err
			}
			// One cause event produces exactly one RuntimeTurn (1:1).
			if err := tx.Exec(`
				CREATE UNIQUE INDEX session_cause_events_runtime_turn_id_key ON session_cause_events (runtime_turn_id)
			`).Error; err != nil {
				return err
			}
			return tx.Exec(`ALTER TABLE session_runtime_turns ADD COLUMN source_cause_event_id BIGINT NULL`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec(`ALTER TABLE session_runtime_turns DROP COLUMN source_cause_event_id`).Error; err != nil {
				return err
			}
			return tx.Exec(`DROP TABLE session_cause_events`).Error
		},
	}
}
