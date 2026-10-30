package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20261029020000IdentityTrails keeps how things got to their state:
// every step of a login attempt as an event (with what the provider said about
// the person at that login, which external identities no longer overwrite),
// and every display name a User had. An account starts without a display name
// until the person chooses one.
func migration20261029020000IdentityTrails() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20261029020000_identity_trails",
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`CREATE TABLE identity_login_attempt_events (
					id BIGSERIAL PRIMARY KEY,
					attempt_id BIGINT NOT NULL,
					event TEXT NOT NULL,
					reason TEXT,
					user_id BIGINT,
					observed_email TEXT,
					observed_name TEXT,
					occurred_at TIMESTAMPTZ NOT NULL
				)`,
				`CREATE INDEX identity_login_attempt_events_attempt_id_idx ON identity_login_attempt_events (attempt_id)`,
				`ALTER TABLE identity_external_identities DROP COLUMN last_login_at`,
				`ALTER TABLE identity_users ALTER COLUMN display_name DROP NOT NULL`,
				`CREATE TABLE identity_display_name_changes (
					id BIGSERIAL PRIMARY KEY,
					user_id BIGINT NOT NULL,
					from_name TEXT,
					to_name TEXT NOT NULL,
					changed_at TIMESTAMPTZ NOT NULL
				)`,
				`CREATE INDEX identity_display_name_changes_user_id_idx ON identity_display_name_changes (user_id)`,
				// Guests created so far were named at creation: their first change.
				`INSERT INTO identity_display_name_changes (user_id, from_name, to_name, changed_at)
					SELECT id, NULL, display_name, created_at FROM identity_users WHERE display_name IS NOT NULL`,
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
				`DROP TABLE identity_display_name_changes`,
				`UPDATE identity_users SET display_name = 'Player' WHERE display_name IS NULL`,
				`ALTER TABLE identity_users ALTER COLUMN display_name SET NOT NULL`,
				`ALTER TABLE identity_external_identities ADD COLUMN last_login_at TIMESTAMPTZ NOT NULL DEFAULT now()`,
				`DROP TABLE identity_login_attempt_events`,
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
