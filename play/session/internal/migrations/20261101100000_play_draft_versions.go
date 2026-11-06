package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20261101100000PlayDraftVersions gives a DRAFT write-once revisions
// of its sources and asset bindings, each with its provenance; file contents
// stored once per hash; and every change attempt, applied or not.
func migration20261101100000PlayDraftVersions() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20261101100000_play_draft_versions",
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`CREATE TABLE play_game_version_revisions (
					id BIGSERIAL PRIMARY KEY,
					game_version_id BIGINT NOT NULL,
					number BIGINT NOT NULL,
					files JSONB NOT NULL,
					assets JSONB NOT NULL,
					kind TEXT NOT NULL,
					origin TEXT NULL,
					actor TEXT NOT NULL,
					parent_revision BIGINT NULL,
					restored_from BIGINT NULL,
					from_game_uuid UUID NULL,
					from_version_uuid UUID NULL,
					from_revision BIGINT NULL,
					sdk TEXT NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					UNIQUE (game_version_id, number)
				)`,
				`CREATE TABLE play_source_files (
					id BIGSERIAL PRIMARY KEY,
					sha256 TEXT NOT NULL UNIQUE,
					content TEXT NOT NULL,
					size BIGINT NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE TABLE play_game_version_change_attempts (
					id BIGSERIAL PRIMARY KEY,
					game_version_id BIGINT NOT NULL,
					kind TEXT NOT NULL,
					idempotency_key TEXT NOT NULL,
					request_sha256 TEXT NOT NULL,
					base_revision BIGINT NOT NULL,
					origin TEXT NOT NULL,
					outcome TEXT NULL,
					revision BIGINT NULL,
					diagnostics JSONB NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					UNIQUE (game_version_id, idempotency_key)
				)`,
				// Serves reading a version's most recent attempts.
				`CREATE INDEX play_game_version_change_attempts_recent_idx ON play_game_version_change_attempts (game_version_id, id DESC)`,
			}
			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			statements := []string{
				`DROP TABLE play_game_version_change_attempts`,
				`DROP TABLE play_source_files`,
				`DROP TABLE play_game_version_revisions`,
			}
			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return err
				}
			}
			return nil
		},
	}
}
