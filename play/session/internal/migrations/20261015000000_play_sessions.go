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
				// structural facts the live Runtime needs about the game. Its
				// backend script and frontend package live in object storage;
				// these are their locators, which an EXECUTABLE version must have.
				`CREATE TABLE play_game_versions (
					id BIGSERIAL PRIMARY KEY,
					uuid UUID NOT NULL UNIQUE,
					game_id BIGINT NOT NULL,
					status TEXT NOT NULL CHECK (status IN ('DRAFT', 'EXECUTABLE')),
					manifest JSONB NOT NULL,
					script_object_key TEXT NULL,
					script_sha256 TEXT NULL,
					script_size BIGINT NULL,
					frontend_object_key TEXT NULL,
					frontend_sha256 TEXT NULL,
					frontend_size BIGINT NULL,
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
					expires_at TIMESTAMPTZ NOT NULL,
					owner_lease_expires_at TIMESTAMPTZ NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE INDEX play_sessions_game_version_id_idx ON play_sessions (game_version_id)`,
				// Reconciliation reads the Sessions past one of their deadlines.
				// Each index holds only the Sessions its deadline applies to, so
				// a Session leaves it as soon as it ends.
				`CREATE INDEX play_sessions_lobby_expiry_idx ON play_sessions (lobby_expires_at) WHERE status = 'LOBBY'`,
				`CREATE INDEX play_sessions_running_expiry_idx ON play_sessions (expires_at) WHERE status = 'RUNNING'`,
				`CREATE INDEX play_sessions_owner_lease_idx ON play_sessions (owner_lease_expires_at) WHERE status = 'RUNNING'`,

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

				// Every status a Session went through, appended in the same
				// transaction that changes play_sessions.status. from_status is
				// NULL for its creation; changed_by_user_uuid is NULL when the
				// platform made the change.
				`CREATE TABLE play_session_status_changes (
					id BIGSERIAL PRIMARY KEY,
					session_id BIGINT NOT NULL,
					from_status TEXT NULL,
					to_status TEXT NOT NULL,
					end_cause TEXT NULL,
					changed_by_user_uuid UUID NULL,
					changed_at TIMESTAMPTZ NOT NULL
				)`,
				`CREATE INDEX play_session_status_changes_session_id_idx ON play_session_status_changes (session_id)`,

				// Who plays a started Session: written once, when it becomes
				// RUNNING. The game knows each player only by participant_id.
				`CREATE TABLE play_session_participants (
					id BIGSERIAL PRIMARY KEY,
					session_id BIGINT NOT NULL,
					user_uuid UUID NOT NULL,
					participant_id TEXT NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					UNIQUE (session_id, participant_id),
					UNIQUE (session_id, user_uuid)
				)`,

				// The latest recovery checkpoint of each started Session, written
				// with the start and replaced by newer ones. content is opaque
				// here: the live Runtime encodes and reads it.
				`CREATE TABLE play_session_checkpoints (
					id BIGSERIAL PRIMARY KEY,
					session_id BIGINT NOT NULL UNIQUE,
					context_revision BIGINT NOT NULL,
					content BYTEA NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,

				// The replay manifest of each Session that recorded any: its
				// pinned version, and once the game finished durably, where it
				// ended. Chunk bytes live in object storage, never here.
				`CREATE TABLE play_replays (
					id BIGSERIAL PRIMARY KEY,
					session_id BIGINT NOT NULL UNIQUE,
					game_version_id BIGINT NOT NULL,
					final_session_time_ms BIGINT NULL,
					finalized_at TIMESTAMPTZ NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,

				// Every replay chunk stored, registered only after its object
				// was written. A Session recorded by several Runtime runs has a
				// recording each; a later one supersedes earlier ones from its
				// first chunk's from_session_time_ms.
				`CREATE TABLE play_replay_chunks (
					id BIGSERIAL PRIMARY KEY,
					session_id BIGINT NOT NULL,
					recording_uuid UUID NOT NULL,
					recording_started_at TIMESTAMPTZ NOT NULL,
					chunk_index INT NOT NULL,
					format_version INT NOT NULL,
					from_session_time_ms BIGINT NOT NULL,
					through_session_time_ms BIGINT NOT NULL,
					final BOOLEAN NOT NULL,
					object_key TEXT NOT NULL,
					sha256 TEXT NOT NULL,
					size BIGINT NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					UNIQUE (recording_uuid, chunk_index)
				)`,
				`CREATE INDEX play_replay_chunks_session_id_idx ON play_replay_chunks (session_id)`,

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
			return tx.Exec(`DROP TABLE play_replay_chunks, play_replays, play_session_status_changes, play_session_checkpoints, play_session_participants, play_create_session_requests, play_session_join_codes, play_sessions, play_game_versions, play_games`).Error
		},
	}
}
