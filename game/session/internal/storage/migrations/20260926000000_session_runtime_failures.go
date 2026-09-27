package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20260926000000SessionRuntimeFailures creates
// session_runtime_failures, GAME-ADR-0017's durable fatal-diagnostic record -
// conceptually separate from session_runtime_turns: a fatal attempted
// execution is never persisted as a Turn (no partial Turn/Step ever
// commits), so this table is the only durable trace a deterministic
// RUNTIME_EXECUTION_FAILED/RUNTIME_STATE_INVALID failure leaves behind.
// failed_step_index is deliberately not a column: the engine's current
// AdvanceTurn/StartTurn contract exposes no per-step failure position (see
// WORK-0014's own "Design Basis"), and GAME-ADR-0017 already marks that
// field optional/"when known" - adding it later, if the engine contract
// ever changes, is purely additive.
func migration20260926000000SessionRuntimeFailures() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260926000000_session_runtime_failures",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				CREATE TABLE session_runtime_failures (
					id BIGSERIAL PRIMARY KEY,
					session_id BIGINT NOT NULL,
					failure_kind TEXT NOT NULL,
					error_code TEXT NOT NULL,
					error_message TEXT NOT NULL,
					base_turn_id BIGINT NULL,
					attempted_sequence BIGINT NOT NULL,
					source_kind TEXT NOT NULL,
					source_interaction_id BIGINT NULL,
					source_timer_obligation_id BIGINT NULL,
					actor_id BIGINT NULL,
					diagnostic_payload JSONB NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
				)
			`).Error; err != nil {
				return err
			}

			// Supports GAME-ADR-0017's failure-rate-by-error-code query goal and
			// listing every failure a given Session produced (in practice at
			// most one, since a fatal materialization always terminalizes the
			// Session, but the index makes the lookup direct either way).
			if err := tx.Exec(`
				CREATE INDEX session_runtime_failures_session_idx
				ON session_runtime_failures (session_id)
			`).Error; err != nil {
				return err
			}

			return tx.Exec(`
				CREATE INDEX session_runtime_failures_error_code_idx
				ON session_runtime_failures (error_code)
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE session_runtime_failures`).Error
		},
	}
}
