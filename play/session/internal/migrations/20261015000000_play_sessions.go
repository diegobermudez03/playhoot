package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20261015000000PlaySessions creates the tables Session creation
// needs. Every table of this domain is prefixed with play_ so it is easy to
// tell apart from other domains sharing the database. There are no foreign
// keys: references between tables are indexed and kept correct by the
// application layer.
func migration20261015000000PlaySessions() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20261015000000_play_sessions",
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				// A Game as play sees it: its identity and the version that is
				// played today.
				`CREATE TABLE play_games (
					id BIGSERIAL PRIMARY KEY,
					game_uuid UUID NOT NULL UNIQUE,
					current_version_id BIGINT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE INDEX play_games_current_version_id_idx ON play_games (current_version_id)`,

				// The versions of a Game. Only an EXECUTABLE version is immutable
				// and its scripts ready to run, so safe to play; newer versions may
				// exist as DRAFT. The manifest is versioned JSON holding the
				// structural facts the live Runtime needs about the game.
				`CREATE TABLE play_game_versions (
					id BIGSERIAL PRIMARY KEY,
					uuid UUID NOT NULL UNIQUE,
					game_id BIGINT NOT NULL,
					status TEXT NOT NULL CHECK (status IN ('DRAFT', 'EXECUTABLE')),
					manifest JSONB NOT NULL,
					executable_at TIMESTAMPTZ NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE INDEX play_game_versions_game_id_idx ON play_game_versions (game_id)`,

				// A Session only references the version it is pinned to; the
				// Game is reached through the version.
				`CREATE TABLE play_sessions (
					id BIGSERIAL PRIMARY KEY,
					uuid UUID NOT NULL UNIQUE,
					game_version_id BIGINT NOT NULL,
					created_by_user_uuid UUID NOT NULL,
					status TEXT NOT NULL CHECK (status IN ('LOBBY', 'RUNNING', 'FINISHED', 'CANCELLED', 'EXPIRED', 'ABORTED')),
					end_cause TEXT NULL,
					lobby_expires_at TIMESTAMPTZ NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE INDEX play_sessions_game_version_id_idx ON play_sessions (game_version_id)`,

				// Join codes have their own lifecycle: a code is active until it
				// is released, and only one active row may hold a given code, so
				// a code can be reused once its previous holder is released.
				`CREATE TABLE play_session_join_codes (
					id BIGSERIAL PRIMARY KEY,
					session_id BIGINT NOT NULL,
					code TEXT NOT NULL,
					expires_at TIMESTAMPTZ NOT NULL,
					released_at TIMESTAMPTZ NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE INDEX play_session_join_codes_session_id_idx ON play_session_join_codes (session_id)`,
				`CREATE UNIQUE INDEX play_session_join_codes_active_code_idx ON play_session_join_codes (code) WHERE released_at IS NULL`,

				// Idempotency for Session creation. A request is claimed and
				// completed in the same transaction that creates the Session, so
				// once committed its outcome is always set.
				`CREATE TABLE play_create_session_requests (
					id BIGSERIAL PRIMARY KEY,
					user_uuid UUID NOT NULL,
					idempotency_key TEXT NOT NULL,
					game_uuid UUID NOT NULL,
					outcome TEXT NULL CHECK (outcome IN ('created', 'game_unavailable')),
					session_id BIGINT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					UNIQUE (user_uuid, idempotency_key)
				)`,
				`CREATE INDEX play_create_session_requests_session_id_idx ON play_create_session_requests (session_id)`,
			}
			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE play_create_session_requests, play_session_join_codes, play_sessions, play_game_versions, play_games`).Error
		},
	}
}
