package migrations

import (
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/play/session/internal/business/build"
)

// migration20261102200000PlayDraftBuilds adds a build per DRAFT revision:
// its status, changed through its own history table, and what its end
// decided, written once (diagnostics, artifacts, the build it reused); and
// the registry of every stored build artifact.
func migration20261102200000PlayDraftBuilds() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20261102200000_play_draft_builds",
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`CREATE TABLE play_game_version_builds (
					id BIGSERIAL PRIMARY KEY,
					uuid UUID NOT NULL UNIQUE,
					game_version_id BIGINT NOT NULL,
					revision BIGINT NOT NULL,
					status TEXT NOT NULL,
					inputs_sha256 TEXT NULL,
					toolchain_version TEXT NULL,
					diagnostics JSONB NULL,
					server_object_key TEXT NULL,
					server_sha256 TEXT NULL,
					server_size BIGINT NULL,
					client_object_key TEXT NULL,
					client_sha256 TEXT NULL,
					client_size BIGINT NULL,
					manifest JSONB NULL,
					aspect_ratio TEXT NULL,
					reused_build_id BIGINT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					finished_at TIMESTAMPTZ NULL,
					UNIQUE (game_version_id, revision)
				)`,
				// Serves the worker taking the oldest queued build.
				fmt.Sprintf(`CREATE INDEX play_game_version_builds_queued_idx ON play_game_version_builds (id) WHERE status = '%s'`, build.Queued),
				// Serves finding a succeeded build with the same inputs to reuse.
				`CREATE INDEX play_game_version_builds_inputs_idx ON play_game_version_builds (game_version_id, inputs_sha256)`,
				`CREATE TABLE play_game_version_build_status_changes (
					id BIGSERIAL PRIMARY KEY,
					build_id BIGINT NOT NULL,
					from_status TEXT NULL,
					to_status TEXT NOT NULL,
					changed_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				// Serves a build's history in order.
				`CREATE INDEX play_game_version_build_status_changes_build_idx ON play_game_version_build_status_changes (build_id, id)`,
				`CREATE TABLE play_build_artifact_objects (
					id BIGSERIAL PRIMARY KEY,
					sha256 TEXT NOT NULL UNIQUE,
					size BIGINT NOT NULL,
					content_type TEXT NOT NULL,
					object_key TEXT NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					registered_at TIMESTAMPTZ NOT NULL,
					deleted_at TIMESTAMPTZ NULL
				)`,
				// Serves the cleanup's scan of objects still stored.
				`CREATE INDEX play_build_artifact_objects_cleanup_idx ON play_build_artifact_objects (registered_at) WHERE deleted_at IS NULL`,
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
				`DROP TABLE play_build_artifact_objects`,
				`DROP TABLE play_game_version_build_status_changes`,
				`DROP TABLE play_game_version_builds`,
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
