package gameauthoring

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/game"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

const (
	testUserUUID    = "0f8fad5b-d9cb-469f-a165-70867728950e"
	testGameUUID    = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	testVersionUUID = "16fd2706-8baf-433b-82eb-8c7fada847da"
	testKey         = "key-1"
)

type dependencies struct {
	games    *MockgameCreator
	editions *MockeditableVersionFinder
	drafts   *MockdraftWriter
	links    *MockresourceLinkWriter
}

func newTestManager(t *testing.T) (*Manager, dependencies) {
	ctrl := gomock.NewController(t)
	deps := dependencies{games: NewMockgameCreator(ctrl), editions: NewMockeditableVersionFinder(ctrl), drafts: NewMockdraftWriter(ctrl), links: NewMockresourceLinkWriter(ctrl)}
	return New(deps.games, deps.editions, deps.drafts, deps.links), deps
}

func TestCreateGame(t *testing.T) {
	type test struct {
		expected  CreatedGame
		expectErr bool
	}

	ctx := context.Background()
	created := game.CreatedGame{GameUUID: testGameUUID, EditionVersionUUID: testVersionUUID, CreatedAt: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)}
	wanted := CreatedGame{GameUUID: testGameUUID, VersionUUID: testVersionUUID, Revision: 1}

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"creates_the_game_then_its_version_being_built": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.games.EXPECT().CreateGame(ctx, testUserUUID, testKey).Return(created, nil),
				deps.drafts.EXPECT().CreateDraftVersion(ctx, testGameUUID, testVersionUUID).
					Return(sessionpkg.DraftVersion{Outcome: sessionpkg.DraftCreated, GameUUID: testGameUUID, VersionUUID: testVersionUUID, Revision: 1}, nil),
			)
			return test{expected: wanted}
		},
		"a_retry_completes_the_same_game": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.games.EXPECT().CreateGame(ctx, testUserUUID, testKey).Return(created, nil),
				deps.drafts.EXPECT().CreateDraftVersion(ctx, testGameUUID, testVersionUUID).
					Return(sessionpkg.DraftVersion{Outcome: sessionpkg.DraftAlreadyCreated, GameUUID: testGameUUID, VersionUUID: testVersionUUID, Revision: 1}, nil),
			)
			return test{expected: wanted}
		},
		"another_version_being_built_is_broken_data": func(t *testing.T, deps dependencies) test {
			deps.games.EXPECT().CreateGame(ctx, testUserUUID, testKey).Return(created, nil)
			deps.drafts.EXPECT().CreateDraftVersion(ctx, testGameUUID, testVersionUUID).
				Return(sessionpkg.DraftVersion{Outcome: sessionpkg.DraftAnotherOpen, VersionUUID: "other"}, nil)
			return test{expectErr: true}
		},
		"unknown_draft_outcome_is_broken_data": func(t *testing.T, deps dependencies) test {
			deps.games.EXPECT().CreateGame(ctx, testUserUUID, testKey).Return(created, nil)
			deps.drafts.EXPECT().CreateDraftVersion(ctx, testGameUUID, testVersionUUID).Return(sessionpkg.DraftVersion{Outcome: "paused"}, nil)
			return test{expectErr: true}
		},
		"game_creation_fails_and_play_is_not_called": func(t *testing.T, deps dependencies) test {
			deps.games.EXPECT().CreateGame(ctx, testUserUUID, testKey).Return(game.CreatedGame{}, errors.New("database down"))
			return test{expectErr: true}
		},
		"draft_creation_fails": func(t *testing.T, deps dependencies) test {
			deps.games.EXPECT().CreateGame(ctx, testUserUUID, testKey).Return(created, nil)
			deps.drafts.EXPECT().CreateDraftVersion(ctx, testGameUUID, testVersionUUID).Return(sessionpkg.DraftVersion{}, errors.New("database down"))
			return test{expectErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			manager, deps := newTestManager(t)
			tc := setup(t, deps)

			result, err := manager.CreateGame(ctx, testUserUUID, testKey)

			if tc.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, result)
		})
	}
}
