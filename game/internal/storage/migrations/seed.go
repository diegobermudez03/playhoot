package migrations

import (
	"time"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/diegobermudez03/playhoot/game"
)

// SeedTables applies pending test-data migrations, after MigrateTables and
// never in production. Append new ones in chronological order.
func SeedTables(db *gorm.DB) error {
	migrator := gormigrate.New(db, &gormigrate.Options{
		TableName:                 "migrations",
		IDColumnName:              "id",
		IDColumnSize:              255,
		UseTransaction:            true,
		ValidateUnknownMigrations: false,
	}, []*gormigrate.Migration{
		seed20261103000000PlaySeedGames(),
	})
	return migrator.Migrate()
}

// seedOwnerUUID owns the hand-testing Games: nobody signs in as it.
const seedOwnerUUID = "00000000-0000-4000-8000-000000000001"

// seedGame is one of Play's hand-testing seed Games (play/session/internal/
// migrations, seed_*), which Game Management must know as published and
// public for their Sessions to be created now that playability is checked.
type seedGame struct {
	uuid string
	name string
}

var seedGames = []seedGame{
	{uuid: "11111111-1111-4111-8111-111111111111", name: "Seed game"},
	{uuid: "33333333-3333-4333-8333-333333333333", name: "Space Drift"},
	{uuid: "44444444-4444-4444-8444-444444444444", name: "Parqués"},
	{uuid: "55555555-5555-4555-8555-555555555555", name: "Tap Race"},
}

type seedGameRow struct {
	ID          uint
	UUID        string
	Name        string
	Description string
	OwnerUUID   string
	Visibility  string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	PublishedAt *time.Time
}

func (seedGameRow) TableName() string { return "games" }

// seed20261103000000PlaySeedGames records Play's seed Games as published and
// public Games. The unavailable seed Game 22222222-... stays unknown, so it
// is not playable.
func seed20261103000000PlaySeedGames() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "seed_20261103000000_game_play_seed_games",
		Migrate: func(tx *gorm.DB) error {
			now := time.Now().UTC().Truncate(time.Microsecond)
			for _, seeded := range seedGames {
				row := seedGameRow{
					UUID: seeded.uuid, Name: seeded.name, OwnerUUID: seedOwnerUUID,
					Visibility: string(game.Public), CreatedAt: now, UpdatedAt: now, PublishedAt: &now,
				}
				if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			uuids := make([]string, len(seedGames))
			for i, seeded := range seedGames {
				uuids[i] = seeded.uuid
			}
			return tx.Exec(`DELETE FROM games WHERE uuid IN ? AND owner_uuid = ?`, uuids, seedOwnerUUID).Error
		},
	}
}
