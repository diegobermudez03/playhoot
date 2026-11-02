package migrations

import (
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

// noAssetBindings is what a revision written before assets existed binds.
const noAssetBindings = "{}"

// migration20261102000000PlayDraftAssets lets a DRAFT's revisions pin the
// uploaded content of each declared asset: write-once upload reservations,
// asset objects once per hash (deleted_at written once by a publication's
// cleanup), and the kind of every attempt.
func migration20261102000000PlayDraftAssets() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20261102000000_play_draft_assets",
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				fmt.Sprintf(`ALTER TABLE play_game_version_revisions ADD COLUMN assets JSONB NOT NULL DEFAULT '%s'`, noAssetBindings),
				fmt.Sprintf(`ALTER TABLE play_game_version_change_attempts ADD COLUMN kind TEXT NOT NULL DEFAULT '%s'`, sessionpkg.DraftAttemptChange),
				`CREATE TABLE play_asset_uploads (
					id BIGSERIAL PRIMARY KEY,
					uuid UUID NOT NULL UNIQUE,
					game_version_id BIGINT NOT NULL,
					asset_key TEXT NOT NULL,
					asset_type TEXT NOT NULL,
					staging_object_key TEXT NOT NULL,
					expires_at TIMESTAMPTZ NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				// Serves listing a version's uploads.
				`CREATE INDEX play_asset_uploads_version_idx ON play_asset_uploads (game_version_id)`,
				`CREATE TABLE play_asset_objects (
					id BIGSERIAL PRIMARY KEY,
					sha256 TEXT NOT NULL UNIQUE,
					size BIGINT NOT NULL,
					mime_type TEXT NOT NULL,
					object_key TEXT NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					deleted_at TIMESTAMPTZ NULL
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
			statements := []string{
				`DROP TABLE play_asset_objects`,
				`DROP TABLE play_asset_uploads`,
				`ALTER TABLE play_game_version_change_attempts DROP COLUMN kind`,
				`ALTER TABLE play_game_version_revisions DROP COLUMN assets`,
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
