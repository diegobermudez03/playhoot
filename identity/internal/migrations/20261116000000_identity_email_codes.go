package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20261116000000IdentityEmailCodes creates the one-time email code
// attempts: the normalized address a code was asked for, the hash of the
// code itself (which exists only in the email), what the attempt is for,
// and the append-only history every attempt leaves behind.
func migration20261116000000IdentityEmailCodes() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20261116000000_identity_email_codes",
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`CREATE TABLE identity_email_codes (
					id BIGSERIAL PRIMARY KEY,
					attempt_uuid UUID NOT NULL UNIQUE,
					email TEXT NOT NULL,
					intent TEXT NOT NULL,
					code_sha256 TEXT NOT NULL,
					language TEXT NOT NULL,
					guest_user_id BIGINT,
					link_user_id BIGINT,
					expires_at TIMESTAMPTZ NOT NULL,
					superseded_at TIMESTAMPTZ,
					completed_at TIMESTAMPTZ,
					created_at TIMESTAMPTZ NOT NULL
				)`,
				`CREATE INDEX identity_email_codes_email_idx ON identity_email_codes (email, created_at)`,
				`CREATE TABLE identity_email_code_events (
					id BIGSERIAL PRIMARY KEY,
					code_id BIGINT NOT NULL,
					event TEXT NOT NULL,
					created_at TIMESTAMPTZ NOT NULL
				)`,
				`CREATE INDEX identity_email_code_events_code_id_idx ON identity_email_code_events (code_id)`,
			}
			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE identity_email_code_events, identity_email_codes`).Error
		},
	}
}
