package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20261115230000AnalyticsEvents creates the append-only event store.
func migration20261115230000AnalyticsEvents() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20261115230000_analytics_events",
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`CREATE TABLE analytics_events (
					id BIGSERIAL PRIMARY KEY,
					event_type TEXT NOT NULL,
					user_ref UUID NOT NULL,
					account_kind TEXT NOT NULL,
					plan_kind TEXT NOT NULL,
					entity_ref TEXT NOT NULL,
					properties JSONB NOT NULL,
					source_domain TEXT NOT NULL,
					idempotency_key TEXT NOT NULL,
					occurred_at TIMESTAMPTZ NOT NULL,
					created_at TIMESTAMPTZ NOT NULL
				)`,
				`CREATE UNIQUE INDEX analytics_events_source_key_idx ON analytics_events (source_domain, idempotency_key)`,
				`CREATE INDEX analytics_events_type_time_idx ON analytics_events (event_type, occurred_at)`,
				`CREATE INDEX analytics_events_user_time_idx ON analytics_events (user_ref, occurred_at)`,
			}
			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE analytics_events`).Error
		},
	}
}
