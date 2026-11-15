package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20261115100000IdentitySecondFactor creates the administrator's
// second factor: the grant that makes an account the administrator, its TOTP
// credentials (one confirmed and one pending at most), and every code check,
// whose accepted step-up rows are the single-use proofs sensitive actions spend.
func migration20261115100000IdentitySecondFactor() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20261115100000_identity_second_factor",
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`CREATE TABLE identity_admin_grants (
					id BIGSERIAL PRIMARY KEY,
					user_id BIGINT NOT NULL,
					granted_at TIMESTAMPTZ NOT NULL,
					revoked_at TIMESTAMPTZ,
					note TEXT NOT NULL
				)`,
				`CREATE UNIQUE INDEX identity_admin_grants_active_user_idx
					ON identity_admin_grants (user_id) WHERE revoked_at IS NULL`,
				`CREATE TABLE identity_totp_credentials (
					id BIGSERIAL PRIMARY KEY,
					user_id BIGINT NOT NULL,
					secret_encrypted TEXT NOT NULL,
					created_at TIMESTAMPTZ NOT NULL,
					confirmed_at TIMESTAMPTZ,
					revoked_at TIMESTAMPTZ
				)`,
				`CREATE UNIQUE INDEX identity_totp_credentials_confirmed_user_idx
					ON identity_totp_credentials (user_id) WHERE revoked_at IS NULL AND confirmed_at IS NOT NULL`,
				`CREATE UNIQUE INDEX identity_totp_credentials_pending_user_idx
					ON identity_totp_credentials (user_id) WHERE revoked_at IS NULL AND confirmed_at IS NULL`,
				`CREATE TABLE identity_second_factor_checks (
					id BIGSERIAL PRIMARY KEY,
					user_id BIGINT NOT NULL,
					purpose TEXT NOT NULL,
					outcome TEXT NOT NULL,
					time_step BIGINT,
					proof_sha256 TEXT UNIQUE,
					expires_at TIMESTAMPTZ,
					used_for TEXT,
					used_at TIMESTAMPTZ,
					created_at TIMESTAMPTZ NOT NULL
				)`,
				`CREATE INDEX identity_second_factor_checks_user_created_idx
					ON identity_second_factor_checks (user_id, created_at)`,
			}
			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE identity_second_factor_checks, identity_totp_credentials, identity_admin_grants`).Error
		},
	}
}
