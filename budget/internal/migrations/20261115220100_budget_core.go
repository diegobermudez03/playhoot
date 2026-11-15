package migrations

import (
	"time"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/budget"
	internalrepo "github.com/diegobermudez03/playhoot/budget/internal/repo"
)

// Initial caps until the creator sets real ones, and the starter rates.
// Storage and compute rates are placeholder estimates, not measured prices.
var (
	initialCaps = []budget.Cap{
		{Scope: budget.ScopeGlobal, Window: budget.WindowHour, USDLimit: 1},
		{Scope: budget.ScopeGlobal, Window: budget.WindowDay, USDLimit: 8},
		{Scope: budget.ScopeGlobal, Window: budget.WindowMonth, USDLimit: 120},
		{Scope: string(budget.CategoryEmail), Window: budget.WindowDay, USDLimit: 1},
	}
	initialRates = []struct {
		rate   budget.Rate
		reason string
	}{
		{budget.Rate{Category: budget.CategoryEmail, Service: "ses-outbound", Unit: "emails", USDPerUnit: 0.0001}, "initial rate: SES outbound email"},
		{budget.Rate{Category: budget.CategoryEmail, Service: "resend-outbound", Unit: "emails", USDPerUnit: 0}, "initial rate: Resend free tier, counted for volume"},
		{budget.Rate{Category: budget.CategoryStorage, Service: "object-storage", Unit: "GB-month", USDPerUnit: 0.02}, "placeholder estimate, not a measured price"},
		{budget.Rate{Category: budget.CategoryCompute, Service: "app-compute", Unit: "vCPU-hours", USDPerUnit: 0.05}, "placeholder estimate, not a measured price"},
	}
)

// migration20261115220100BudgetCore creates Budget's tables, seeded with the
// initial caps, the starter rates and the ladder at normal.
func migration20261115220100BudgetCore() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20261115220100_budget_core",
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`CREATE TABLE budget_rates (
					id BIGSERIAL PRIMARY KEY,
					category TEXT NOT NULL,
					service TEXT NOT NULL,
					unit TEXT NOT NULL,
					usd_per_unit NUMERIC(20, 10) NOT NULL,
					effective_at TIMESTAMPTZ NOT NULL,
					created_by_user_uuid UUID,
					reason TEXT NOT NULL,
					created_at TIMESTAMPTZ NOT NULL
				)`,
				`CREATE INDEX budget_rates_service_idx ON budget_rates (category, service, effective_at)`,
				`CREATE TABLE budget_caps (
					id BIGSERIAL PRIMARY KEY,
					scope TEXT NOT NULL,
					cap_window TEXT NOT NULL,
					usd_limit NUMERIC(20, 10) NOT NULL,
					max_concurrency INTEGER,
					max_volume INTEGER,
					effective_at TIMESTAMPTZ NOT NULL,
					created_by_user_uuid UUID,
					reason TEXT NOT NULL,
					created_at TIMESTAMPTZ NOT NULL
				)`,
				`CREATE INDEX budget_caps_scope_idx ON budget_caps (scope, cap_window, effective_at)`,
				`CREATE TABLE budget_spend_events (
					id BIGSERIAL PRIMARY KEY,
					source_domain TEXT NOT NULL,
					category TEXT NOT NULL,
					service TEXT NOT NULL,
					reference TEXT NOT NULL,
					quantity NUMERIC(24, 6) NOT NULL,
					unit TEXT NOT NULL,
					usd NUMERIC(20, 10) NOT NULL,
					measured BOOLEAN NOT NULL,
					rate_missing BOOLEAN NOT NULL,
					idempotency_key TEXT NOT NULL,
					occurred_at TIMESTAMPTZ NOT NULL,
					created_at TIMESTAMPTZ NOT NULL
				)`,
				`CREATE UNIQUE INDEX budget_spend_events_idempotency_idx ON budget_spend_events (source_domain, idempotency_key)`,
				`CREATE INDEX budget_spend_events_window_idx ON budget_spend_events (category, occurred_at)`,
				// A measured event replacing an estimate is found by its reference.
				`CREATE INDEX budget_spend_events_reference_idx ON budget_spend_events (category, reference)`,
				`CREATE TABLE budget_reservations (
					id BIGSERIAL PRIMARY KEY,
					reservation_uuid UUID NOT NULL,
					category TEXT NOT NULL,
					reference TEXT NOT NULL,
					usd_reserved NUMERIC(20, 10) NOT NULL,
					settled_usd NUMERIC(20, 10),
					released_at TIMESTAMPTZ,
					expires_at TIMESTAMPTZ NOT NULL,
					created_at TIMESTAMPTZ NOT NULL
				)`,
				`CREATE UNIQUE INDEX budget_reservations_uuid_idx ON budget_reservations (reservation_uuid)`,
				`CREATE INDEX budget_reservations_active_idx ON budget_reservations (category, released_at)`,
				// Volume caps count admissions started in each window.
				`CREATE INDEX budget_reservations_started_idx ON budget_reservations (category, created_at)`,
				`CREATE TABLE budget_restriction_changes (
					id BIGSERIAL PRIMARY KEY,
					level TEXT NOT NULL,
					paused_categories JSONB NOT NULL,
					reopen_closed JSONB NOT NULL,
					reason TEXT NOT NULL,
					actor TEXT NOT NULL,
					created_at TIMESTAMPTZ NOT NULL
				)`,
				`CREATE INDEX budget_restriction_changes_created_idx ON budget_restriction_changes (created_at)`,
			}
			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return err
				}
			}
			start := time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)
			caps := make([]internalrepo.Cap, len(initialCaps))
			for i, c := range initialCaps {
				caps[i] = internalrepo.Cap{Scope: c.Scope, CapWindow: string(c.Window), USDLimit: c.USDLimit, EffectiveAt: start, Reason: "initial local cap", CreatedAt: start}
			}
			rates := make([]internalrepo.Rate, len(initialRates))
			for i, r := range initialRates {
				rates[i] = internalrepo.Rate{Category: string(r.rate.Category), Service: r.rate.Service, Unit: r.rate.Unit, USDPerUnit: r.rate.USDPerUnit, EffectiveAt: start, Reason: r.reason, CreatedAt: start}
			}
			if err := tx.Create(&caps).Error; err != nil {
				return err
			}
			if err := tx.Create(&rates).Error; err != nil {
				return err
			}
			return tx.Create(&internalrepo.Change{
				Level: string(budget.LevelNormal), PausedCategories: "[]", ReopenClosed: "[]",
				Reason: "initial state", Actor: budget.ActorAutomatic, CreatedAt: start,
			}).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE budget_restriction_changes, budget_reservations, budget_spend_events, budget_caps, budget_rates`).Error
		},
	}
}
