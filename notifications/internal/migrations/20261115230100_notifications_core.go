package migrations

import (
	"time"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/notifications"
	internalrepo "github.com/diegobermudez03/playhoot/notifications/internal/repo"
)

// migration20261115230100NotificationsCore creates Notifications' tables,
// with the outbound mode starting at automatic.
func migration20261115230100NotificationsCore() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20261115230100_notifications_core",
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`CREATE TABLE notifications_entries (
					id BIGSERIAL PRIMARY KEY,
					entry_uuid UUID NOT NULL,
					-- Empty for an entry whose recipient is not a user of
					-- Playhoot, such as an access code's email-code attempt.
					user_uuid UUID,
					source_domain TEXT NOT NULL,
					type TEXT NOT NULL,
					external_token TEXT NOT NULL,
					idempotency_key TEXT NOT NULL,
					skeleton JSONB NOT NULL,
					send_at TIMESTAMPTZ NOT NULL,
					in_app BOOLEAN NOT NULL,
					email_channel BOOLEAN NOT NULL,
					necessary BOOLEAN NOT NULL,
					read_at TIMESTAMPTZ,
					content_cleared_at TIMESTAMPTZ,
					created_at TIMESTAMPTZ NOT NULL
				)`,
				`CREATE UNIQUE INDEX notifications_entries_idempotency_idx ON notifications_entries (source_domain, idempotency_key)`,
				`CREATE UNIQUE INDEX notifications_entries_uuid_idx ON notifications_entries (entry_uuid)`,
				`CREATE INDEX notifications_entries_user_idx ON notifications_entries (user_uuid, created_at)`,
				`CREATE INDEX notifications_entries_unread_idx ON notifications_entries (user_uuid) WHERE read_at IS NULL AND in_app`,
				`CREATE TABLE notifications_preferences (
					id BIGSERIAL PRIMARY KEY,
					user_uuid UUID NOT NULL,
					ai_email BOOLEAN NOT NULL,
					capacity_email BOOLEAN NOT NULL,
					review_email BOOLEAN NOT NULL,
					marketing_email BOOLEAN NOT NULL,
					created_at TIMESTAMPTZ NOT NULL
				)`,
				`CREATE INDEX notifications_preferences_user_idx ON notifications_preferences (user_uuid, created_at)`,
				`CREATE TABLE notifications_email_deliveries (
					id BIGSERIAL PRIMARY KEY,
					entry_id BIGINT NOT NULL,
					provider TEXT NOT NULL,
					state TEXT NOT NULL,
					provider_message_id TEXT NOT NULL,
					detail TEXT NOT NULL,
					closes_email BOOLEAN NOT NULL,
					decided_at TIMESTAMPTZ,
					created_at TIMESTAMPTZ NOT NULL
				)`,
				`CREATE INDEX notifications_email_deliveries_entry_idx ON notifications_email_deliveries (entry_id)`,
				`CREATE INDEX notifications_email_deliveries_created_idx ON notifications_email_deliveries (created_at)`,
				`CREATE TABLE notifications_suppressions (
					id BIGSERIAL PRIMARY KEY,
					email TEXT NOT NULL,
					reason TEXT NOT NULL,
					origin TEXT NOT NULL,
					created_at TIMESTAMPTZ NOT NULL
				)`,
				`CREATE INDEX notifications_suppressions_email_idx ON notifications_suppressions (email)`,
				`CREATE TABLE notifications_provider_settings (
					id BIGSERIAL PRIMARY KEY,
					mode TEXT NOT NULL,
					changed_by_user_uuid UUID,
					reason TEXT NOT NULL,
					created_at TIMESTAMPTZ NOT NULL
				)`,
				`CREATE INDEX notifications_provider_settings_created_idx ON notifications_provider_settings (created_at)`,
				`CREATE TABLE notifications_provider_usage (
					id BIGSERIAL PRIMARY KEY,
					provider TEXT NOT NULL,
					day DATE NOT NULL,
					sent INTEGER NOT NULL
				)`,
				`CREATE UNIQUE INDEX notifications_provider_usage_day_idx ON notifications_provider_usage (provider, day)`,
			}
			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return err
				}
			}
			start := time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)
			return tx.Create(&internalrepo.ModeChange{Mode: string(notifications.ModeAutomatic), Reason: "initial mode", CreatedAt: start}).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE notifications_provider_usage, notifications_provider_settings, notifications_suppressions, notifications_email_deliveries, notifications_preferences, notifications_entries`).Error
		},
	}
}
