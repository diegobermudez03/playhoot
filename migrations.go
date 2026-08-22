package main

import (
	"fmt"

	gamemigrations "github.com/diegobermudez03/playhoot/game/game/migration"
	sessionmigrations "github.com/diegobermudez03/playhoot/game/session/migration"
	"gorm.io/gorm"
)

func PostgresMigrate(db *gorm.DB) error {
	if err := gamemigrations.Migrate(db); err != nil {
		return fmt.Errorf("migrating game pkg: %w", err)
	}

	if err := sessionmigrations.Migrate(db); err != nil {
		return fmt.Errorf("migrating session pkg: %w", err)
	}

	return nil
}
