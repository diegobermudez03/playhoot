package repo

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/game/internal/business/edition"
	"github.com/diegobermudez03/playhoot/game/internal/testdb"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m))
}

type seededGame struct {
	ID           uint
	UUID         string
	Name         string
	Description  string
	OwnerUUID    string
	LogoImageURL string
	Visibility   string
	DeletedAt    *time.Time
}

func (seededGame) TableName() string { return "games" }

type seededEdition struct {
	ID        uint
	UUID      string
	GameID    uint
	Status    string
	CreatedAt time.Time
}

func (seededEdition) TableName() string { return "game_editions" }

func TestFindGameEditing(t *testing.T) {
	type test struct {
		gameUUID string
		expected *GameEditing
	}

	db := testdb.OpenGameDB(t)
	repo := New(db)
	seedGame := func(t *testing.T, db *gorm.DB, deleted bool, statuses ...string) (seededGame, []string) {
		t.Helper()
		game := seededGame{UUID: uuid.NewString(), OwnerUUID: uuid.NewString(), Visibility: "private"}
		if deleted {
			now := time.Now()
			game.DeletedAt = &now
		}
		require.NoError(t, db.Create(&game).Error)
		var uuids []string
		for _, status := range statuses {
			seeded := seededEdition{UUID: uuid.NewString(), GameID: game.ID, Status: status, CreatedAt: time.Now()}
			require.NoError(t, db.Create(&seeded).Error)
			uuids = append(uuids, seeded.UUID)
		}
		return game, uuids
	}

	tests := map[string]func(t *testing.T) test{
		"game_with_an_open_edition_after_a_published_one": func(t *testing.T) test {
			game, editions := seedGame(t, db, false, string(edition.Published), string(edition.Editing))
			return test{gameUUID: game.UUID, expected: &GameEditing{OwnerUUID: game.OwnerUUID, VersionUUID: &editions[1]}}
		},
		"game_without_an_open_edition": func(t *testing.T) test {
			game, _ := seedGame(t, db, false, string(edition.Published))
			return test{gameUUID: game.UUID, expected: &GameEditing{OwnerUUID: game.OwnerUUID}}
		},
		"deleted_game": func(t *testing.T) test {
			game, _ := seedGame(t, db, true, string(edition.Editing))
			return test{gameUUID: game.UUID}
		},
		"unknown_game": func(t *testing.T) test {
			return test{gameUUID: uuid.NewString()}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)

			editing, err := repo.FindGameEditing(context.Background(), tc.gameUUID, string(edition.Editing))

			require.NoError(t, err)
			require.Equal(t, tc.expected, editing)
		})
	}
}
