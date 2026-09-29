package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20260929000000SessionRuntimeTurnsNewState adds
// session_runtime_turns.new_state: the authoritative Runtime state
// immediately after that Turn committed, stored opaque -
// Session Runtime never decodes/interprets it beyond passing it through to
// the Executor as the next call's PreviousState. "Current state" for any
// operation is simply this column for the row sessions.current_turn_id
// already points at; no second table is needed to keep in sync.
// DEFAULT '{}' only backfills any pre-existing row (none in practice, since
// nothing has ever populated this column) - new rows always supply their
// own real value explicitly.
func migration20260929000000SessionRuntimeTurnsNewState() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260929000000_session_runtime_turns_new_state",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE session_runtime_turns
					ADD COLUMN new_state JSONB NOT NULL DEFAULT '{}'::jsonb
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`ALTER TABLE session_runtime_turns DROP COLUMN new_state`).Error
		},
	}
}
