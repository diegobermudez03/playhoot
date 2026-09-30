package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20260929000004SessionTimerObligationsTimerData renames
// session_timer_obligations' engine_slot/engine_key columns to timer/data,
// giving them their real name instead of a stale one inherited from the
// retired engine: a script addresses a timer by exactly one opaque Timer
// string (session/workflows/sessionlifecycle/internal/platform.
// ScheduleTimer/CancelTimer), and any accompanying data is an opaque
// echoed payload, never part of a timer's identity. The active-timer
// uniqueness index is replaced accordingly: (session_id, timer) only - data
// must never participate in a timer's identity, so the previous
// COALESCE(engine_key, ...) comparison is dropped along with the column
// name.
func migration20260929000004SessionTimerObligationsTimerData() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260929000004_session_timer_obligations_timer_data",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`DROP INDEX session_timer_obligations_active_key`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`ALTER TABLE session_timer_obligations RENAME COLUMN engine_slot TO timer`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`ALTER TABLE session_timer_obligations RENAME COLUMN engine_key TO data`).Error; err != nil {
				return err
			}
			return tx.Exec(`
				CREATE UNIQUE INDEX session_timer_obligations_active_timer
				ON session_timer_obligations (session_id, timer)
				WHERE state = 'ACTIVE'
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec(`DROP INDEX session_timer_obligations_active_timer`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`ALTER TABLE session_timer_obligations RENAME COLUMN timer TO engine_slot`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`ALTER TABLE session_timer_obligations RENAME COLUMN data TO engine_key`).Error; err != nil {
				return err
			}
			return tx.Exec(`
				CREATE UNIQUE INDEX session_timer_obligations_active_key
				ON session_timer_obligations (session_id, engine_slot, (COALESCE(engine_key, 'null'::jsonb)))
				WHERE state = 'ACTIVE'
			`).Error
		},
	}
}
