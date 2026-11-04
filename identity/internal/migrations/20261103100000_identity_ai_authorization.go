package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20261103100000IdentityAIAuthorization keeps AI applications' OAuth
// access to a User's games (business/aiauth), apart from the User's own
// sessions: registered applications, authorization requests and their
// decision, approved connections and their rotating refresh tokens. Only
// secrets' hashes are kept; decisions, uses and revocations are written once.
func migration20261103100000IdentityAIAuthorization() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20261103100000_identity_ai_authorization",
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`CREATE TABLE identity_ai_clients (
					id BIGSERIAL PRIMARY KEY,
					client_id TEXT NOT NULL UNIQUE,
					client_name TEXT NOT NULL,
					client_uri TEXT,
					redirect_uris JSONB NOT NULL,
					grant_types JSONB NOT NULL,
					created_at TIMESTAMPTZ NOT NULL
				)`,
				`CREATE TABLE identity_ai_authorization_requests (
					id BIGSERIAL PRIMARY KEY,
					request_id TEXT NOT NULL UNIQUE,
					client_row_id BIGINT NOT NULL,
					redirect_uri TEXT NOT NULL,
					state TEXT,
					code_challenge TEXT NOT NULL,
					scope TEXT NOT NULL,
					resource TEXT NOT NULL,
					created_at TIMESTAMPTZ NOT NULL,
					expires_at TIMESTAMPTZ NOT NULL,
					decided_at TIMESTAMPTZ,
					decided_user_id BIGINT,
					approved BOOLEAN,
					code_sha256 TEXT UNIQUE,
					code_expires_at TIMESTAMPTZ,
					code_used_at TIMESTAMPTZ,
					session_row_id BIGINT
				)`,
				`CREATE TABLE identity_ai_sessions (
					id BIGSERIAL PRIMARY KEY,
					connection_id TEXT NOT NULL UNIQUE,
					user_id BIGINT NOT NULL,
					client_row_id BIGINT NOT NULL,
					scope TEXT NOT NULL,
					resource TEXT NOT NULL,
					created_at TIMESTAMPTZ NOT NULL,
					revoked_at TIMESTAMPTZ,
					revoke_reason TEXT
				)`,
				`CREATE INDEX identity_ai_sessions_user_idx ON identity_ai_sessions (user_id, created_at)`,
				`CREATE TABLE identity_ai_refresh_tokens (
					id BIGSERIAL PRIMARY KEY,
					session_row_id BIGINT NOT NULL,
					token_sha256 TEXT NOT NULL UNIQUE,
					created_at TIMESTAMPTZ NOT NULL,
					expires_at TIMESTAMPTZ NOT NULL,
					used_at TIMESTAMPTZ
				)`,
				`CREATE INDEX identity_ai_refresh_tokens_session_idx ON identity_ai_refresh_tokens (session_row_id, created_at)`,
			}
			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE identity_ai_refresh_tokens, identity_ai_sessions, identity_ai_authorization_requests, identity_ai_clients`).Error
		},
	}
}
