package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20261102300000PlayPreviews creates the previews of draft builds
// (play/session/workflows/previewlifecycle): kept apart from play_sessions,
// so a preview is never listed, joined, limited or reconciled as a Session.
// ended_at and end_reason are written once, when the preview ends.
func migration20261102300000PlayPreviews() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20261102300000_play_previews",
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`CREATE TABLE play_previews (
					id BIGSERIAL PRIMARY KEY,
					uuid UUID NOT NULL UNIQUE,
					game_version_id BIGINT NOT NULL,
					revision BIGINT NOT NULL,
					owner_user_uuid UUID NOT NULL,
					participants INT NOT NULL,
					created_at TIMESTAMPTZ NOT NULL,
					expires_at TIMESTAMPTZ NOT NULL,
					ended_at TIMESTAMPTZ NULL,
					end_reason TEXT NULL
				)`,
				// At most one running preview per draft version.
				`CREATE UNIQUE INDEX play_previews_one_running_idx ON play_previews (game_version_id) WHERE ended_at IS NULL`,
			}
			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE play_previews`).Error
		},
	}
}
