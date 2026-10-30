package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20261029010000IdentityLogins creates what accounts sign in with:
// the external identities bound to each account's User (an issuer and the
// subject it vouches for; email and name only observed), and login attempts
// from their start to the one-time code that completes them.
func migration20261029010000IdentityLogins() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20261029010000_identity_logins",
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`CREATE TABLE identity_external_identities (
					id BIGSERIAL PRIMARY KEY,
					issuer TEXT NOT NULL,
					subject TEXT NOT NULL,
					user_id BIGINT NOT NULL,
					email TEXT,
					name TEXT,
					created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
					last_login_at TIMESTAMPTZ NOT NULL,
					UNIQUE (issuer, subject)
				)`,
				`CREATE INDEX identity_external_identities_user_id_idx ON identity_external_identities (user_id)`,
				`CREATE TABLE identity_login_attempts (
					id BIGSERIAL PRIMARY KEY,
					state TEXT NOT NULL UNIQUE,
					provider TEXT NOT NULL,
					code_challenge TEXT NOT NULL,
					nonce TEXT NOT NULL,
					started_at TIMESTAMPTZ NOT NULL,
					answered_at TIMESTAMPTZ,
					user_id BIGINT,
					login_code TEXT UNIQUE,
					login_code_expires_at TIMESTAMPTZ,
					completed_at TIMESTAMPTZ
				)`,
			}
			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE identity_login_attempts, identity_external_identities`).Error
		},
	}
}
