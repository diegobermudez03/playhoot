package gamelifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/game"
	internalrepo "github.com/diegobermudez03/playhoot/game/workflows/gamelifecycle/internal/repo"
)

func TestCreateGameInTx(t *testing.T) {
	type dependencies struct {
		repo *MockcreateGameRepo
	}
	type test struct {
		template  *game.TemplateSource
		expected  createGameAttempt
		expectErr bool
	}

	const (
		ownerUUID   = "0f8fad5b-d9cb-469f-a165-70867728950e"
		key         = "key-1"
		requestID   = uint(11)
		gameID      = uint(21)
		gameUUID    = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
		editionUUID = "16fd2706-8baf-433b-82eb-8c7fada847da"
	)
	ctx := context.Background()
	tx := &gorm.DB{}
	now := time.Date(2026, 11, 1, 12, 0, 0, 123456789, time.UTC)
	nowInDB := now.Truncate(time.Microsecond)
	gameIDRef := func() *uint { id := gameID; return &id }
	newGame := internalrepo.NewGame{UUID: gameUUID, OwnerUUID: ownerUUID, Name: "Memoria", Visibility: "private", CreatedAt: nowInDB}
	created := game.CreatedGame{GameUUID: gameUUID, EditionVersionUUID: editionUUID, CreatedAt: nowInDB}

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"a_retry_with_another_name_is_a_conflict": func(t *testing.T, deps dependencies) test {
			other := "Otro"
			deps.repo.EXPECT().ClaimCreateGameRequest(ctx, tx, ownerUUID, key, "Memoria", "").Return(uint(0), &internalrepo.CreateGameRequest{ID: requestID, GameID: gameIDRef(), Name: &other}, nil)
			return test{expectErr: true}
		},
		"game_created_with_its_open_edition": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.repo.EXPECT().ClaimCreateGameRequest(ctx, tx, ownerUUID, key, "Memoria", "").Return(requestID, nil, nil),
				deps.repo.EXPECT().CreateGame(ctx, tx, newGame).Return(gameID, nil),
				deps.repo.EXPECT().CreateEdition(ctx, tx, gameID, editionUUID, "EDITING", nil, nowInDB).Return(nil),
				deps.repo.EXPECT().CompleteCreateGameRequest(ctx, tx, requestID, gameID).Return(nil),
			)
			return test{expected: createGameAttempt{created: created}}
		},
		"a_game_started_from_a_template_records_it_on_its_edition": func(t *testing.T, deps dependencies) test {
			template := &game.TemplateSource{GameUUID: "0f8fad5b-d9cb-469f-a165-70867728950e", VersionUUID: "6ba7b810-9dad-11d1-80b4-00c04fd430c8"}
			gomock.InOrder(
				deps.repo.EXPECT().ClaimCreateGameRequest(ctx, tx, ownerUUID, key, "Memoria", template.GameUUID).Return(requestID, nil, nil),
				deps.repo.EXPECT().CreateGame(ctx, tx, newGame).Return(gameID, nil),
				deps.repo.EXPECT().CreateEdition(ctx, tx, gameID, editionUUID, "EDITING", template, nowInDB).Return(nil),
				deps.repo.EXPECT().CompleteCreateGameRequest(ctx, tx, requestID, gameID).Return(nil),
			)
			return test{template: template, expected: createGameAttempt{created: created}}
		},
		"a_retry_with_another_template_is_a_conflict": func(t *testing.T, deps dependencies) test {
			name, other := "Memoria", "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
			deps.repo.EXPECT().ClaimCreateGameRequest(ctx, tx, ownerUUID, key, "Memoria", "").Return(uint(0), &internalrepo.CreateGameRequest{ID: requestID, GameID: gameIDRef(), Name: &name, TemplateGameUUID: &other}, nil)
			return test{expectErr: true}
		},
		"retry_replays_the_created_game": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.repo.EXPECT().ClaimCreateGameRequest(ctx, tx, ownerUUID, key, "Memoria", "").Return(uint(0), &internalrepo.CreateGameRequest{ID: requestID, GameID: gameIDRef()}, nil),
				deps.repo.EXPECT().FindCreatedGame(ctx, tx, gameID).Return(&internalrepo.CreatedGame{GameUUID: gameUUID, EditionVersionUUID: editionUUID, CreatedAt: nowInDB}, nil),
			)
			return test{expected: createGameAttempt{created: created, replayed: true}}
		},
		"request_without_game_is_broken_data": func(t *testing.T, deps dependencies) test {
			deps.repo.EXPECT().ClaimCreateGameRequest(ctx, tx, ownerUUID, key, "Memoria", "").Return(uint(0), &internalrepo.CreateGameRequest{ID: requestID}, nil)
			return test{expectErr: true}
		},
		"request_whose_game_has_no_edition_is_broken_data": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.repo.EXPECT().ClaimCreateGameRequest(ctx, tx, ownerUUID, key, "Memoria", "").Return(uint(0), &internalrepo.CreateGameRequest{ID: requestID, GameID: gameIDRef()}, nil),
				deps.repo.EXPECT().FindCreatedGame(ctx, tx, gameID).Return(nil, nil),
			)
			return test{expectErr: true}
		},
		"claiming_fails": func(t *testing.T, deps dependencies) test {
			deps.repo.EXPECT().ClaimCreateGameRequest(ctx, tx, ownerUUID, key, "Memoria", "").Return(uint(0), nil, errors.New("database down"))
			return test{expectErr: true}
		},
		"creating_the_edition_fails": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.repo.EXPECT().ClaimCreateGameRequest(ctx, tx, ownerUUID, key, "Memoria", "").Return(requestID, nil, nil),
				deps.repo.EXPECT().CreateGame(ctx, tx, newGame).Return(gameID, nil),
				deps.repo.EXPECT().CreateEdition(ctx, tx, gameID, editionUUID, "EDITING", nil, nowInDB).Return(errors.New("database down")),
			)
			return test{expectErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			deps := dependencies{repo: NewMockcreateGameRepo(ctrl)}
			uuids := []string{gameUUID, editionUUID}
			manager := &Manager{
				createGameRepo: deps.repo,
				now:            func() time.Time { return now },
				newUUID: func() string {
					next := uuids[0]
					uuids = uuids[1:]
					return next
				},
			}
			tc := setup(t, deps)

			attempt, err := manager.createGameInTx(ctx, tx, ownerUUID, "Memoria", key, tc.template)

			if tc.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, attempt)
		})
	}
}

func TestCreateGameValidatesItsInput(t *testing.T) {
	tests := map[string]struct {
		ownerUUID string
		key       string
	}{
		"owner_is_not_a_uuid": {ownerUUID: "nope", key: "key-1"},
		"key_is_empty":        {ownerUUID: "0f8fad5b-d9cb-469f-a165-70867728950e"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			manager := &Manager{}

			_, err := manager.CreateGame(context.Background(), tc.ownerUUID, "Memoria", tc.key, nil)

			require.Error(t, err)
		})
	}
}
