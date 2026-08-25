package getgame

import (
	"context"
	"testing"

	"github.com/diegobermudez03/playhoot/game/game/internal/testdb"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRepoGetGameCurrentVersion(t *testing.T) {
	type test struct {
		ctx          context.Context
		gameUUID     string
		expectedGame *gameWithVersion
		expectErr    bool
		errAssert    require.ErrorAssertionFunc
	}

	tests := map[string]func(t *testing.T, db *gorm.DB) test{
		"returns_game_with_current_version": func(t *testing.T, db *gorm.DB) test {
			seedGetGameCurrentVersion(t, db, getGameSeed{
				gameUUID:         "11111111-1111-1111-1111-111111111111",
				name:             "Parques",
				description:      "Race game",
				ownerUUID:        "22222222-2222-2222-2222-222222222222",
				logoImageURL:     "https://example.com/logo.png",
				visibility:       "public",
				versionUUID:      "33333333-3333-3333-3333-333333333333",
				versionNumber:    2,
				script:           `{"metadata":{"id":"game"}}`,
				oldVersionUUID:   "44444444-4444-4444-4444-444444444444",
				oldVersionNumber: 1,
				oldVersionScript: `{"metadata":{"id":"old"}}`,
			})
			return test{
				ctx:      context.Background(),
				gameUUID: "11111111-1111-1111-1111-111111111111",
				expectedGame: &gameWithVersion{
					UUID:         "11111111-1111-1111-1111-111111111111",
					Name:         "Parques",
					Description:  "Race game",
					OwnerUUID:    "22222222-2222-2222-2222-222222222222",
					LogoImageURL: "https://example.com/logo.png",
					Visibility:   "public",
					VersionUUID:  "33333333-3333-3333-3333-333333333333",
					Script:       `{"metadata":{"id":"game"}}`,
				},
			}
		},
		"returns_nil_when_game_does_not_exist": func(t *testing.T, db *gorm.DB) test {
			return test{
				ctx:      context.Background(),
				gameUUID: "55555555-5555-5555-5555-555555555555",
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			db := testdb.OpenGameDB(t)
			repo := newRepo(db)
			tc := setup(t, db)

			got, err := repo.getGameCurrentVersion(tc.ctx, tc.gameUUID)
			if tc.expectErr {
				require.Error(t, err)
				if tc.errAssert != nil {
					tc.errAssert(t, err)
				}
				require.Nil(t, got)
				return
			}
			require.NoError(t, err)
			if got == nil && tc.expectedGame == nil {
				return
			}
			require.NotNil(t, got)
			require.NotNil(t, tc.expectedGame)
			require.Equal(t, *tc.expectedGame, *got)
		})
	}
}

type getGameSeed struct {
	gameUUID         string
	name             string
	description      string
	ownerUUID        string
	logoImageURL     string
	visibility       string
	versionUUID      string
	versionNumber    uint
	script           string
	oldVersionUUID   string
	oldVersionNumber uint
	oldVersionScript string
}

type testGameInsert struct {
	ID               uint   `gorm:"column:id"`
	UUID             string `gorm:"column:uuid"`
	Name             string `gorm:"column:name"`
	Description      string `gorm:"column:description"`
	OwnerUUID        string `gorm:"column:owner_uuid"`
	CurrentDefinitionID *uint `gorm:"column:current_definition_id"`
	LogoImageURL     string `gorm:"column:logo_image_url"`
	Visibility       string `gorm:"column:visibility"`
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

func seedGetGameCurrentVersion(t *testing.T, db *gorm.DB, seed getGameSeed) {
	t.Helper()

	gameRow := testGameInsert{
		UUID:         seed.gameUUID,
		Name:         seed.name,
		Description:  seed.description,
		OwnerUUID:    seed.ownerUUID,
		LogoImageURL: seed.logoImageURL,
		Visibility:   seed.visibility,
	}
	require.NoError(t, db.Create(&gameRow).Error)

	oldVersionRow := testGameVersionInsert{
		UUID:          seed.oldVersionUUID,
		GameID:        gameRow.ID,
		VersionNumber: seed.oldVersionNumber,
		Script:        seed.oldVersionScript,
	}
	require.NoError(t, db.Create(&oldVersionRow).Error)

	currentVersionRow := testGameVersionInsert{
		UUID:          seed.versionUUID,
		GameID:        gameRow.ID,
		VersionNumber: seed.versionNumber,
		Script:        seed.script,
	}
	require.NoError(t, db.Create(&currentVersionRow).Error)

	require.NoError(t, db.Exec(`
		UPDATE games
		SET current_definition_id = ?
		WHERE id = ?
	`, currentVersionRow.ID, gameRow.ID).Error)
}
