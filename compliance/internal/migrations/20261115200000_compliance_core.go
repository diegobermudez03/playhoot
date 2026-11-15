package migrations

import (
	"time"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// migration20261115200000ComplianceCore creates the market matrix (seeded
// with the initial coverage), residence-and-age declarations, minority
// signals, acceptance texts and acceptances. Every table is append-only.
func migration20261115200000ComplianceCore() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20261115200000_compliance_core",
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`CREATE TABLE compliance_market_rules (
					id BIGSERIAL PRIMARY KEY,
					country TEXT NOT NULL,
					subdivision TEXT NOT NULL,
					function TEXT NOT NULL,
					enabled BOOLEAN NOT NULL,
					min_age INTEGER NOT NULL,
					conditions_note TEXT NOT NULL,
					effective_at TIMESTAMPTZ NOT NULL,
					created_by_user_uuid UUID,
					reason TEXT NOT NULL,
					created_at TIMESTAMPTZ NOT NULL
				)`,
				`CREATE INDEX compliance_market_rules_key_effective_idx
					ON compliance_market_rules (country, subdivision, function, effective_at)`,
				`CREATE TABLE compliance_declarations (
					id BIGSERIAL PRIMARY KEY,
					user_uuid UUID NOT NULL,
					country TEXT NOT NULL,
					subdivision TEXT NOT NULL,
					meets_age BOOLEAN NOT NULL,
					threshold_age INTEGER NOT NULL,
					origin TEXT NOT NULL,
					created_at TIMESTAMPTZ NOT NULL
				)`,
				`CREATE INDEX compliance_declarations_user_created_idx
					ON compliance_declarations (user_uuid, created_at)`,
				`CREATE TABLE compliance_minority_signals (
					id BIGSERIAL PRIMARY KEY,
					user_uuid UUID NOT NULL,
					origin TEXT NOT NULL,
					noted_by_user_uuid UUID,
					created_at TIMESTAMPTZ NOT NULL
				)`,
				`CREATE INDEX compliance_minority_signals_user_idx
					ON compliance_minority_signals (user_uuid)`,
				`CREATE TABLE compliance_acceptance_texts (
					id BIGSERIAL PRIMARY KEY,
					kind TEXT NOT NULL,
					version INTEGER NOT NULL,
					language TEXT NOT NULL,
					content TEXT NOT NULL,
					published_at TIMESTAMPTZ NOT NULL,
					created_at TIMESTAMPTZ NOT NULL,
					UNIQUE (kind, version, language)
				)`,
				`CREATE TABLE compliance_acceptances (
					id BIGSERIAL PRIMARY KEY,
					user_uuid UUID NOT NULL,
					text_id BIGINT NOT NULL,
					kind TEXT NOT NULL,
					action TEXT NOT NULL,
					context TEXT NOT NULL,
					created_at TIMESTAMPTZ NOT NULL
				)`,
				`CREATE INDEX compliance_acceptances_user_kind_created_idx
					ON compliance_acceptances (user_uuid, kind, created_at)`,
				`CREATE INDEX compliance_acceptances_text_idx
					ON compliance_acceptances (text_id)`,
			}
			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return err
				}
			}
			rows := initialMarketRules(time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC))
			return tx.CreateInBatches(rows, 200).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE compliance_acceptances, compliance_acceptance_texts, compliance_minority_signals, compliance_declarations, compliance_market_rules`).Error
		},
	}
}
