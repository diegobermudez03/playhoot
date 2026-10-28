package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20261029000000IdentityCore creates Identity's persistence: Users,
// the accounts that make a User registered, sessions (one row per refresh
// token; the active one has no deactivated_at), and guest creation requests
// for idempotency.
func migration20261029000000IdentityCore() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20261029000000_identity_core",
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`CREATE TABLE identity_users (
					id BIGSERIAL PRIMARY KEY,
					user_uuid UUID NOT NULL UNIQUE,
					display_name TEXT NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT now()
				)`,
				`CREATE TABLE identity_accounts (
					id BIGSERIAL PRIMARY KEY,
					user_id BIGINT NOT NULL UNIQUE,
					created_at TIMESTAMPTZ NOT NULL DEFAULT now()
				)`,
				`CREATE TABLE identity_sessions (
					id BIGSERIAL PRIMARY KEY,
					session_uuid UUID NOT NULL,
					user_id BIGINT NOT NULL,
					refresh_token TEXT NOT NULL UNIQUE,
					issued_at TIMESTAMPTZ NOT NULL,
					started_at TIMESTAMPTZ NOT NULL,
					expires_at TIMESTAMPTZ,
					deactivated_at TIMESTAMPTZ,
					replaced_by_id BIGINT
				)`,
				// One active refresh token per session.
				`CREATE UNIQUE INDEX identity_sessions_active_session_uuid_idx
					ON identity_sessions (session_uuid) WHERE deactivated_at IS NULL`,
				`CREATE INDEX identity_sessions_user_id_idx ON identity_sessions (user_id)`,
				`CREATE TABLE identity_guest_requests (
					id BIGSERIAL PRIMARY KEY,
					idempotency_key UUID NOT NULL UNIQUE,
					display_name TEXT NOT NULL,
					user_uuid UUID NOT NULL,
					session_uuid UUID NOT NULL,
					requested_at TIMESTAMPTZ NOT NULL
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
			return tx.Exec(`DROP TABLE identity_guest_requests, identity_sessions, identity_accounts, identity_users`).Error
		},
	}
}
