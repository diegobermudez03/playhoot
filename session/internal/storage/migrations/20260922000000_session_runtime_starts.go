package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20260922000000SessionRuntimeStarts creates session_runtime_starts,
// the durable one-row-per-Session record of Start's deterministic
// initialization (InitializationInput.Seed/RootParameters). This is captured
// independently and atomically with Start's own Turn 1, never derived later
// from session_participants' current state, since a Participant's roster
// slot can be released and later reoccupied by a rejoin, overwriting the
// original Start-moment membership. root_parameters is JSONB, encoding an
// engine.Value-typed record through engineservice.EncodeValue, like every
// other engine.Value Session Runtime persists. seed is stored as a signed
// BIGINT holding InitializationInput.Seed's uint64 bit pattern reinterpreted
// as int64 (an exact, lossless Go conversion in both directions) -
// PostgreSQL has no native unsigned 64-bit integer type.
func migration20260922000000SessionRuntimeStarts() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260922000000_session_runtime_starts",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				CREATE TABLE session_runtime_starts (
					id BIGSERIAL PRIMARY KEY,
					session_id BIGINT NOT NULL,
					seed BIGINT NOT NULL,
					root_parameters JSONB NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
				)
			`).Error; err != nil {
				return err
			}
			return tx.Exec(`
				CREATE UNIQUE INDEX session_runtime_starts_session_id_key ON session_runtime_starts (session_id)
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE session_runtime_starts`).Error
		},
	}
}
