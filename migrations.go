package main

import (
	"fmt"

	sessionmigrations "github.com/diegobermudez03/playhoot/game/session/migration"
	"gorm.io/gorm"
)

func PostgresMigrate(db *gorm.DB) error {
	if err := sessionmigrations.Migrate(db); err != nil {
		return fmt.Errorf("migrating session pkg: %w", err)
	}

	return nil
}
