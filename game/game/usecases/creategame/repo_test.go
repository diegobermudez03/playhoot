package creategame

import (
	"context"
	"testing"
	"time"

	"github.com/diegobermudez03/playhoot/game/game"
	"github.com/diegobermudez03/playhoot/game/game/internal/testdb"
	"github.com/diegobermudez03/playhoot/game/language/v1/program"
	"github.com/diegobermudez03/playhoot/game/language/v1/program/gameservice"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRepoCreateGameDraft(t *testing.T) {
	type test struct {
		ctx       context.Context
		params    createGameDraftParams
		expectErr bool
		expected  *createdGameDraft
		verify    func(t *testing.T, db *gorm.DB, params createGameDraftParams)
	}

	definition := program.Definition{
		Metadata: program.Metadata{
			ID:              "parques",
			Name:            "Parques",
			Description:     "Race game",
			Version:         "1",
			LanguageVersion: "v1",
		},
	}
	script := encodeDefinitionForCreateGameRepoTest(t, definition)

	tests := map[string]func(t *testing.T, db *gorm.DB) test{
		"creates_game_and_initial_definition": func(t *testing.T, db *gorm.DB) test {
			params := createGameDraftParams{
				GameUUID:       "11111111-1111-4111-8111-111111111111",
				DefinitionUUID: "33333333-3333-4333-8333-333333333333",
				Name:           "Parques",
				Description:    "Race game",
				OwnerUUID:      "22222222-2222-4222-8222-222222222222",
				LogoImageURL:   "https://example.com/logo.png",
				Script:         script,
			}
			return test{
				ctx:    context.Background(),
				params: params,
				expected: &createdGameDraft{
					GameUUID:       params.GameUUID,
					DefinitionUUID: params.DefinitionUUID,
					Name:           params.Name,
					Description:    params.Description,
					OwnerUUID:      params.OwnerUUID,
					LogoImageURL:   params.LogoImageURL,
				},
				verify: func(t *testing.T, db *gorm.DB, params createGameDraftParams) {
					verifyCreatedGameDraft(t, db, params, definition)
					require.Equal(t, int64(0), countRows(t, db, "game_histories", "game_id IS NOT NULL"))
					require.Equal(t, int64(0), countRows(t, db, "game_definition_histories", "game_definition_id IS NOT NULL"))
				},
			}
		},
		"rolls_back_game_when_initial_definition_fails": func(t *testing.T, db *gorm.DB) test {
			params := createGameDraftParams{
				GameUUID:       "44444444-4444-4444-8444-444444444444",
				DefinitionUUID: "not-a-uuid",
				Name:           "Rollback",
				Description:    "Rollback game",
				OwnerUUID:      "55555555-5555-4555-8555-555555555555",
				LogoImageURL:   "https://example.com/rollback.png",
				Script:         script,
			}
			return test{
				ctx:       context.Background(),
				params:    params,
				expectErr: true,
				verify: func(t *testing.T, db *gorm.DB, params createGameDraftParams) {
					require.Equal(t, int64(0), countRows(t, db, "games", "uuid = ?", params.GameUUID))
					require.Equal(t, int64(0), countRows(t, db, "game_definitions", "game_id IN (SELECT id FROM games WHERE uuid = ?)", params.GameUUID))
				},
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			db := testdb.OpenGameDB(t)
			repo := newRepo(db)
			tc := setup(t, db)

			got, err := repo.createGameDraft(tc.ctx, tc.params)
			if tc.expectErr {
				require.Error(t, err)
				require.Nil(t, got)
				if tc.verify != nil {
					tc.verify(t, db, tc.params)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, got)
			require.NotNil(t, tc.verify)
			tc.verify(t, db, tc.params)
		})
	}
}

type persistedGameDraft struct {
	ID                  uint   `gorm:"column:id"`
	UUID                string `gorm:"column:uuid"`
	Name                string `gorm:"column:name"`
	Description         string `gorm:"column:description"`
	OwnerUUID           string `gorm:"column:owner_uuid"`
	CurrentDefinitionID uint   `gorm:"column:current_definition_id"`
	LogoImageURL        string `gorm:"column:logo_image_url"`
	Visibility          string `gorm:"column:visibility"`
}

type persistedGameDefinition struct {
	ID            uint       `gorm:"column:id"`
	UUID          string     `gorm:"column:uuid"`
	GameID        uint       `gorm:"column:game_id"`
	VersionNumber uint       `gorm:"column:version_number"`
	Script        string     `gorm:"column:script"`
	PublishedAt   *time.Time `gorm:"column:published_at"`
	DisabledAt    *time.Time `gorm:"column:disabled_at"`
}

func verifyCreatedGameDraft(t *testing.T, db *gorm.DB, params createGameDraftParams, expectedDefinition program.Definition) {
	t.Helper()

	var gameRow persistedGameDraft
	tx := db.Raw(`
		SELECT id, uuid, name, description, owner_uuid, current_definition_id, logo_image_url, visibility
		FROM games
		WHERE uuid = ?
	`, params.GameUUID).Scan(&gameRow)
	require.NoError(t, tx.Error)
	require.Equal(t, int64(1), tx.RowsAffected)
	require.Equal(t, params.Name, gameRow.Name)
	require.Equal(t, params.Description, gameRow.Description)
	require.Equal(t, params.OwnerUUID, gameRow.OwnerUUID)
	require.Equal(t, params.LogoImageURL, gameRow.LogoImageURL)
	require.Equal(t, string(game.Draft), gameRow.Visibility)

	var definitionRow persistedGameDefinition
	tx = db.Raw(`
		SELECT id, uuid, game_id, version_number, script, published_at, disabled_at
		FROM game_definitions
		WHERE uuid = ?
	`, params.DefinitionUUID).Scan(&definitionRow)
	require.NoError(t, tx.Error)
	require.Equal(t, int64(1), tx.RowsAffected)
	require.Equal(t, gameRow.ID, definitionRow.GameID)
	require.Equal(t, uint(1), definitionRow.VersionNumber)
	require.Nil(t, definitionRow.PublishedAt)
	require.Nil(t, definitionRow.DisabledAt)
	require.Equal(t, definitionRow.ID, gameRow.CurrentDefinitionID)

	decodedDefinition, err := gameservice.DecodeJSON([]byte(definitionRow.Script))
	require.NoError(t, err)
	require.Equal(t, expectedDefinition, *decodedDefinition)
}

func countRows(t *testing.T, db *gorm.DB, table string, where string, args ...any) int64 {
	t.Helper()

	var count int64
	tx := db.Raw("SELECT COUNT(*) FROM "+table+" WHERE "+where, args...).Scan(&count)
	require.NoError(t, tx.Error)
	return count
}

func encodeDefinitionForCreateGameRepoTest(t *testing.T, definition program.Definition) string {
	t.Helper()

	data, err := gameservice.EncodeJSON(definition)
	require.NoError(t, err)
	return string(data)
}
