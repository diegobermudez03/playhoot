package getgamedefinition

import (
	"context"
	"testing"

	"github.com/diegobermudez03/playhoot/game/game/internal/testdb"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRepoGetGameDefinitionByUUID(t *testing.T) {
	type test struct {
		ctx                context.Context
		gameDefinitionUUID string
		expectedDefinition *gameDefinitionRow
	}

	tests := map[string]func(t *testing.T, db *gorm.DB) test{
		"returns_definition_regardless_of_current_version": func(t *testing.T, db *gorm.DB) test {
			seedGetGameDefinition(t, db, getGameDefinitionSeed{
				gameUUID:         "66666666-6666-6666-6666-666666666666",
				oldVersionUUID:   "77777777-7777-7777-7777-777777777777",
				oldVersionNumber: 1,
				oldVersionScript: `{"metadata":{"id":"old"}}`,
				newVersionUUID:   "88888888-8888-8888-8888-888888888888",
				newVersionNumber: 2,
				newVersionScript: `{"metadata":{"id":"new"}}`,
			})
			return test{
				ctx:                context.Background(),
				gameDefinitionUUID: "77777777-7777-7777-7777-777777777777",
				expectedDefinition: &gameDefinitionRow{
					UUID:   "77777777-7777-7777-7777-777777777777",
					Script: `{"metadata":{"id":"old"}}`,
				},
			}
		},
		"returns_nil_when_definition_does_not_exist": func(t *testing.T, db *gorm.DB) test {
			return test{
				ctx:                context.Background(),
				gameDefinitionUUID: "99999999-9999-9999-9999-999999999999",
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			db := testdb.OpenGameDB(t)
			repo := newRepo(db)
			tc := setup(t, db)

			got, err := repo.getGameDefinitionByUUID(tc.ctx, tc.gameDefinitionUUID)
			require.NoError(t, err)
			if got == nil && tc.expectedDefinition == nil {
				return
			}
			require.NotNil(t, got)
			require.NotNil(t, tc.expectedDefinition)
			require.Equal(t, *tc.expectedDefinition, *got)
		})
	}
}

type getGameDefinitionSeed struct {
	gameUUID         string
	oldVersionUUID   string
	oldVersionNumber uint
	oldVersionScript string
	newVersionUUID   string
	newVersionNumber uint
	newVersionScript string
}

type testGameInsert struct {
	ID           uint   `gorm:"column:id"`
	UUID         string `gorm:"column:uuid"`
	Name         string `gorm:"column:name"`
	Description  string `gorm:"column:description"`
	OwnerUUID    string `gorm:"column:owner_uuid"`
	LogoImageURL string `gorm:"column:logo_image_url"`
	Visibility   string `gorm:"column:visibility"`
}

func (testGameInsert) TableName() string {
	return "games"
}

type testGameVersionInsert struct {
	ID            uint   `gorm:"column:id"`
	UUID          string `gorm:"column:uuid"`
	GameID        uint   `gorm:"column:game_id"`
	VersionNumber uint   `gorm:"column:version_number"`
	Script        string `gorm:"column:script"`
}

func (testGameVersionInsert) TableName() string {
	return "game_definitions"
}

// seedGetGameDefinition seeds a Game whose current version is the "new"
// version, plus an older "old" version that is no longer current - proving
// the repo reads the requested definition directly rather than through
// games.current_definition_id.
func seedGetGameDefinition(t *testing.T, db *gorm.DB, seed getGameDefinitionSeed) {
	t.Helper()

	gameRow := testGameInsert{
		UUID:         seed.gameUUID,
		Name:         "Game",
		Description:  "Game description",
		OwnerUUID:    "11111111-1111-1111-1111-111111111111",
		LogoImageURL: "https://example.com/logo.png",
		Visibility:   "public",
	}
	require.NoError(t, db.Create(&gameRow).Error)

	oldVersionRow := testGameVersionInsert{
		UUID:          seed.oldVersionUUID,
		GameID:        gameRow.ID,
		VersionNumber: seed.oldVersionNumber,
		Script:        seed.oldVersionScript,
	}
	require.NoError(t, db.Create(&oldVersionRow).Error)

	newVersionRow := testGameVersionInsert{
		UUID:          seed.newVersionUUID,
		GameID:        gameRow.ID,
		VersionNumber: seed.newVersionNumber,
		Script:        seed.newVersionScript,
	}
	require.NoError(t, db.Create(&newVersionRow).Error)

	require.NoError(t, db.Exec(`
		UPDATE games
		SET current_definition_id = ?
		WHERE id = ?
	`, newVersionRow.ID, gameRow.ID).Error)
}
