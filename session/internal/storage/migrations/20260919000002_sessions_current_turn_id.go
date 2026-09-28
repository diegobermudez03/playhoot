package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20260919000002SessionsCurrentTurnID adds the current-authoritative-
// RuntimeTurn pointer directly to sessions: every caller that needs it
// already holds the locked sessions row for per-Session serialization, so
// colocating it there is free - no separate table is needed just to hold
// one pointer. It is a logical reference with no database foreign key, like
// every other cross-table reference in this schema; it stays meaningful
// even after the row it points to is eventually deleted as part of history
// archival, resolving against the archived artifact instead of a live row -
// not a broken reference.
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
