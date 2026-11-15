package txqueue

import (
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// Migration creates domain's queue table, for the domain's own migration list
// under id.
func Migration(id string, domain string) *gormigrate.Migration {
	if !domainName.MatchString(domain) {
		panic(fmt.Sprintf("txqueue: invalid domain name %q", domain))
	}
	table := TableName(domain)
	return &gormigrate.Migration{
		ID: id,
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				fmt.Sprintf(`CREATE TABLE %s (
					id BIGSERIAL PRIMARY KEY,
					uuid UUID NOT NULL UNIQUE,
					type TEXT NOT NULL,
					payload JSONB NOT NULL,
					idempotency_key TEXT NULL,
					status TEXT NOT NULL,
					run_at TIMESTAMPTZ NOT NULL,
					attempts INTEGER NOT NULL DEFAULT 0,
					last_error TEXT NULL,
					admin_retries INTEGER NOT NULL DEFAULT 0,
					created_at TIMESTAMPTZ NOT NULL,
					updated_at TIMESTAMPTZ NOT NULL
				)`, table),
				// Claiming due tasks, and listing dead ones newest first.
				fmt.Sprintf(`CREATE INDEX %s_status_run_at_idx ON %s (status, run_at)`, table, table),
				// One live task per type and key; a cancelled one frees the key.
				fmt.Sprintf(`CREATE UNIQUE INDEX %s_idempotency_idx ON %s (type, idempotency_key) WHERE idempotency_key IS NOT NULL AND status <> '%s'`, table, table, StatusCancelled),
			}
			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(fmt.Sprintf(`DROP TABLE %s`, table)).Error
		},
	}
}
