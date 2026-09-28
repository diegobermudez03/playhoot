package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20260926000001SessionsActivityExpiresAt adds
// sessions.activity_expires_at, the durable deadline by which a RUNNING
// Session must see renewing activity before it is considered inactive.
// Nullable, distinct from lobby_expires_at (NOT NULL, meaningful only during
// LOBBY): activity_expires_at is meaningless before Start and is never
// read/written once TERMINAL.
func migration20260926000001SessionsActivityExpiresAt() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260926000001_sessions_activity_expires_at",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`ALTER TABLE sessions ADD COLUMN activity_expires_at TIMESTAMPTZ NULL`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`ALTER TABLE sessions DROP COLUMN activity_expires_at`).Error
		},
	}
}
