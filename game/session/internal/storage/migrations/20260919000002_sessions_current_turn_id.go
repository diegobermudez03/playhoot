package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20260919000002SessionsCurrentTurnID adds the current-authoritative-
// RuntimeTurn pointer directly to sessions, per GAME-ADR-0023 (refining
// GAME-ADR-0007): every caller that needs it already holds the locked
// sessions row for per-Session serialization, so colocating it there is
// free - no separate session_runtime_state table. This is a logical,
// non-DB-enforced reference like every other reference in this schema
// (SESSION_RUNTIME_PERSISTENCE_MODEL.md's Relationship Types); it remains
// meaningful after session_runtime_turns is eventually hard-deleted
// following archival (GAME-ADR-0009) - it then resolves against the
// archived artifact instead of a live row, not a broken reference.
func migration20260919000002SessionsCurrentTurnID() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260919000002_sessions_current_turn_id",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`ALTER TABLE sessions ADD COLUMN current_turn_id BIGINT NULL`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`ALTER TABLE sessions DROP COLUMN current_turn_id`).Error
		},
	}
}
