package main

import (
	"fmt"

	compliancemigrations "github.com/diegobermudez03/playhoot/compliance/migration"
	gamemigrations "github.com/diegobermudez03/playhoot/game/migration"
	identitymigrations "github.com/diegobermudez03/playhoot/identity/migration"
	sessionmigrations "github.com/diegobermudez03/playhoot/play/session/migration"
	"gorm.io/gorm"
)

func PostgresMigrate(db *gorm.DB) error {
	if err := gamemigrations.Migrate(db); err != nil {
		return fmt.Errorf("migrating game pkg: %w", err)
	}

	if err := sessionmigrations.Migrate(db); err != nil {
		return fmt.Errorf("migrating session pkg: %w", err)
	}

	if err := identitymigrations.Migrate(db); err != nil {
		return fmt.Errorf("migrating identity pkg: %w", err)
	}

	if err := compliancemigrations.Migrate(db); err != nil {
		return fmt.Errorf("migrating compliance pkg: %w", err)
	}

	return nil
}

// PostgresSeed fills the tables with test data. main only calls it outside
// production; it runs after PostgresMigrate.
func PostgresSeed(db *gorm.DB) error {
	if err := gamemigrations.Seed(db); err != nil {
		return fmt.Errorf("seeding game pkg: %w", err)
	}

	if err := sessionmigrations.Seed(db); err != nil {
		return fmt.Errorf("seeding session pkg: %w", err)
	}

	if err := compliancemigrations.Seed(db); err != nil {
		return fmt.Errorf("seeding compliance pkg: %w", err)
	}

	return nil
}
