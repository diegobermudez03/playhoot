package main

import (
	"fmt"

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

	return nil
}

// PostgresSeed fills the tables with test data. main only calls it outside
// production; it runs after PostgresMigrate.
func PostgresSeed(db *gorm.DB) error {
	if err := sessionmigrations.Seed(db); err != nil {
		return fmt.Errorf("seeding session pkg: %w", err)
	}

	return nil
}
