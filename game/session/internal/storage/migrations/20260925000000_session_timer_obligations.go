package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20260925000000SessionTimerObligations creates
// session_timer_obligations, the durable Timer Obligation entity a
// committed RuntimeTurn schedules (from an engine.ScheduleTimerOutput/
// ScheduleKeyedTimerOutput) and later cancels/consumes, and adds
// session_runtime_turns.source_timer_obligation_id - a third nullable
// cause pointer mirroring the already-established source_interaction_id
// pattern, for a Turn caused by a timer's expiration.
//
// There is no engine_path column: unlike questions (which carried
// engine_path/engine_slot before 20260924000000 replaced that addressing
// with engine_interaction_id), engine.ScheduleTimerOutput/CancelTimerOutput/
// ScheduleKeyedTimerOutput/CancelKeyedTimerOutput and Signal's timer-
// addressing fields (Slot, Key) have never carried a Path field in this
// repository's history - engine_path for this table was never grounded in
// an actual engine field. engine_slot alone, plus the nullable engine_key
// discriminator for a keyed timer's own authored key, is this table's full
// addressing.
func migration20260925000000SessionTimerObligations() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260925000000_session_timer_obligations",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				CREATE TABLE session_timer_obligations (
					id BIGSERIAL PRIMARY KEY,
					uuid TEXT NOT NULL UNIQUE,
					session_id BIGINT NOT NULL,
					engine_slot TEXT NOT NULL,
					engine_key JSONB NULL,
					delay_ms BIGINT NOT NULL,
					state TEXT NOT NULL,
					created_by_turn_id BIGINT NOT NULL,
					closed_by_turn_id BIGINT NULL,
					closure_reason TEXT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
				)
			`).Error; err != nil {
				return err
			}
			// At most one ACTIVE obligation per (session, slot, key) - the same
			// "slot already occupied" invariant the engine itself enforces
			// (ScheduleTimerOperation/ScheduleKeyedTimerOperation's atomic
			// occupied-tuple execution error). COALESCE normalizes engine_key's
			// NULL (ordinary timer) so two ordinary timers on the same slot
			// still collide - Postgres would otherwise treat two NULLs as
			// distinct in a unique index. This index is defense-in-depth, not
			// the sole correctness mechanism: the engine already fails the
			// whole transition atomically before any row here would be
			// written for a rejected schedule.
			return tx.Exec(`
				CREATE UNIQUE INDEX session_timer_obligations_active_key
				ON session_timer_obligations (session_id, engine_slot, (COALESCE(engine_key, 'null'::jsonb)))
				WHERE state = 'ACTIVE'
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec(`DROP INDEX session_timer_obligations_active_key`).Error; err != nil {
				return err
			}
			return tx.Exec(`DROP TABLE session_timer_obligations`).Error
		},
	}
}

// migration20260925000001SessionRuntimeTurnsSourceTimerObligationID adds
// session_runtime_turns.source_timer_obligation_id, mirroring the existing
// source_interaction_id column exactly, for a Turn caused by a timer's
// expiration (Manager.ExpireTimer).
func migration20260925000001SessionRuntimeTurnsSourceTimerObligationID() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260925000001_session_runtime_turns_source_timer_obligation_id",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`ALTER TABLE session_runtime_turns ADD COLUMN source_timer_obligation_id BIGINT NULL`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`ALTER TABLE session_runtime_turns DROP COLUMN source_timer_obligation_id`).Error
		},
	}
}
