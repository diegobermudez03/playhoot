package migrations

import (
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/play/session/internal/business/gameversion"
)

// migration20261101100000PlayDraftVersions lets a DRAFT version hold authored
// sources edited in place: each change is a new revision of the same
// version. A DRAFT has no manifest until it is built. Revisions are
// write-once and are the history of current_revision; file contents are
// stored once per hash; every change sent is kept, applied or not.
func migration20261101100000PlayDraftVersions() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20261101100000_play_draft_versions",
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`ALTER TABLE play_game_versions ALTER COLUMN manifest DROP NOT NULL`,
				`ALTER TABLE play_game_versions ADD COLUMN current_revision BIGINT NULL`,
				// A game has at most one DRAFT at a time.
				fmt.Sprintf(`CREATE UNIQUE INDEX play_game_versions_one_draft_idx ON play_game_versions (game_id) WHERE status = '%s'`, gameversion.Draft),
				`CREATE TABLE play_game_version_revisions (
					id BIGSERIAL PRIMARY KEY,
					game_version_id BIGINT NOT NULL,
					number BIGINT NOT NULL,
					files JSONB NOT NULL,
					source TEXT NOT NULL,
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
				`DROP INDEX play_game_versions_one_draft_idx`,
				`ALTER TABLE play_game_versions DROP COLUMN current_revision`,
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
