package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20261022000000PlayGameVersionAspectRatios adds the aspect ratio a
// game version's frontend is shown in, one of the ratios Playhoot offers
// (business/gameversion). Like the package locators it is nullable for a
// DRAFT, and the application treats an EXECUTABLE version without one as
// broken data. Offering another ratio changes this constraint.
func migration20261022000000PlayGameVersionAspectRatios() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20261022000000_play_game_version_aspect_ratios",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`ALTER TABLE play_game_versions ADD COLUMN aspect_ratio TEXT NULL
				CONSTRAINT play_game_versions_aspect_ratio_check CHECK (aspect_ratio IN ('16:9', '9:16', '4:3', '3:4', '1:1'))`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`ALTER TABLE play_game_versions DROP COLUMN aspect_ratio`).Error
		},
	}
}
