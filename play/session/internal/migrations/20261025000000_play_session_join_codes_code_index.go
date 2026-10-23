package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20261025000000PlaySessionJoinCodesCodeIndex indexes every
// assignment of a join code, released or not, newest last, since a code is
// resolved by its most recent assignment.
func migration20261025000000PlaySessionJoinCodesCodeIndex() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20261025000000_play_session_join_codes_code_index",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`CREATE INDEX play_session_join_codes_code_idx ON play_session_join_codes (code, id)`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP INDEX play_session_join_codes_code_idx`).Error
		},
	}
}
