package gameauthoring

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/compliance"
	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/orchestrator/marketgate"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

const (
	testUserUUID    = "0f8fad5b-d9cb-469f-a165-70867728950e"
	testGameUUID    = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	testVersionUUID = "16fd2706-8baf-433b-82eb-8c7fada847da"
	testKey         = "key-1"
)

type dependencies struct {
	games        *MockgameCreator
	editions     *MockeditableVersionFinder
	drafts       *MockdraftWriter
	links        *MockresourceLinkWriter
	previews     *MockpreviewStarter
	publications *MockgamePublisher
	versions     *MockversionPublisher
	cards        *MockcardNamer
	templates    *MocktemplateFinder
	market       *marketStub
}

func newTestManager(t *testing.T) (*Manager, dependencies) {
	ctrl := gomock.NewController(t)
	deps := dependencies{games: NewMockgameCreator(ctrl), editions: NewMockeditableVersionFinder(ctrl), drafts: NewMockdraftWriter(ctrl), links: NewMockresourceLinkWriter(ctrl), previews: NewMockpreviewStarter(ctrl), publications: NewMockgamePublisher(ctrl), versions: NewMockversionPublisher(ctrl), cards: NewMockcardNamer(ctrl), templates: NewMocktemplateFinder(ctrl)}
	// No release is left being published unless a test says so.
	deps.publications.EXPECT().FindPendingPublication(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	gate, market := allowingMarket()
	deps.market = market
	return New(gate, deps.games, deps.editions, deps.drafts, deps.links, deps.previews, deps.publications, deps.versions, deps.cards, deps.templates), deps
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
				deps.games.EXPECT().CreateGame(ctx, testUserUUID, "Memoria", testKey, (*game.TemplateSource)(nil)).Return(created, nil),
				deps.drafts.EXPECT().CreateDraftVersion(ctx, testGameUUID, testVersionUUID, sessionpkg.DraftStart{}).
					Return(sessionpkg.DraftVersion{Outcome: sessionpkg.DraftCreated, GameUUID: testGameUUID, VersionUUID: testVersionUUID, Revision: 1}, nil),
			)
			return test{expected: wanted}
		},
		"a_retry_completes_the_same_game": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.games.EXPECT().CreateGame(ctx, testUserUUID, "Memoria", testKey, (*game.TemplateSource)(nil)).Return(created, nil),
				deps.drafts.EXPECT().CreateDraftVersion(ctx, testGameUUID, testVersionUUID, sessionpkg.DraftStart{}).
					Return(sessionpkg.DraftVersion{Outcome: sessionpkg.DraftAlreadyCreated, GameUUID: testGameUUID, VersionUUID: testVersionUUID, Revision: 1}, nil),
			)
			return test{expected: wanted}
		},
		"another_version_being_built_is_broken_data": func(t *testing.T, deps dependencies) test {
			deps.games.EXPECT().CreateGame(ctx, testUserUUID, "Memoria", testKey, (*game.TemplateSource)(nil)).Return(created, nil)
			deps.drafts.EXPECT().CreateDraftVersion(ctx, testGameUUID, testVersionUUID, sessionpkg.DraftStart{}).
				Return(sessionpkg.DraftVersion{Outcome: sessionpkg.DraftAnotherOpen, VersionUUID: "other"}, nil)
			return test{expectErr: true}
		},
		"unknown_draft_outcome_is_broken_data": func(t *testing.T, deps dependencies) test {
			deps.games.EXPECT().CreateGame(ctx, testUserUUID, "Memoria", testKey, (*game.TemplateSource)(nil)).Return(created, nil)
			deps.drafts.EXPECT().CreateDraftVersion(ctx, testGameUUID, testVersionUUID, sessionpkg.DraftStart{}).Return(sessionpkg.DraftVersion{Outcome: "paused"}, nil)
			return test{expectErr: true}
		},
		"game_creation_fails_and_play_is_not_called": func(t *testing.T, deps dependencies) test {
			deps.games.EXPECT().CreateGame(ctx, testUserUUID, "Memoria", testKey, (*game.TemplateSource)(nil)).Return(game.CreatedGame{}, errors.New("database down"))
			return test{expectErr: true}
		},
		"draft_creation_fails": func(t *testing.T, deps dependencies) test {
			deps.games.EXPECT().CreateGame(ctx, testUserUUID, "Memoria", testKey, (*game.TemplateSource)(nil)).Return(created, nil)
			deps.drafts.EXPECT().CreateDraftVersion(ctx, testGameUUID, testVersionUUID, sessionpkg.DraftStart{}).Return(sessionpkg.DraftVersion{}, errors.New("database down"))
			return test{expectErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			manager, deps := newTestManager(t)
			tc := setup(t, deps)

			result, err := manager.CreateGame(ctx, testUserUUID, "Memoria", testKey, "", "")

			if tc.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, result)
		})
	}
}

// Creating games is gated on the market matrix: a person who has not said
// where they live is sent to declare it, and nothing is created until they
// have. A blocked market refuses as a service restriction.
func TestCreateGameMarketGate(t *testing.T) {
	ctx := context.Background()
	created := game.CreatedGame{GameUUID: testGameUUID, EditionVersionUUID: testVersionUUID, CreatedAt: time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)}

	t.Run("a person who owes a declaration creates nothing", func(t *testing.T) {
		manager, deps := newTestManager(t)
		deps.market.decision = compliance.AccessDecision{Outcome: compliance.AccessNeedsDeclaration}

		_, err := manager.CreateGame(ctx, testUserUUID, "Memoria", testKey, "", "CO")

		var required *marketgate.DeclarationRequiredError
		require.ErrorAs(t, err, &required)
		require.Equal(t, compliance.FunctionCreateGames, required.Function)
		require.Equal(t, compliance.AccessRequest{
			UserUUID: testUserUUID, Function: compliance.FunctionCreateGames, ApproximateCountry: "CO",
		}, deps.market.asked)
	})

	t.Run("once declared the same call creates the game", func(t *testing.T) {
		manager, deps := newTestManager(t)
		deps.market.decision = compliance.AccessDecision{Outcome: compliance.AccessAllowed, Country: "CO"}
		gomock.InOrder(
			deps.games.EXPECT().CreateGame(ctx, testUserUUID, "Memoria", testKey, (*game.TemplateSource)(nil)).Return(created, nil),
			deps.drafts.EXPECT().CreateDraftVersion(ctx, testGameUUID, testVersionUUID, sessionpkg.DraftStart{}).
				Return(sessionpkg.DraftVersion{Outcome: sessionpkg.DraftCreated, GameUUID: testGameUUID, VersionUUID: testVersionUUID, Revision: 1}, nil),
		)

		result, err := manager.CreateGame(ctx, testUserUUID, "Memoria", testKey, "", "CO")

		require.NoError(t, err)
		require.Equal(t, CreatedGame{GameUUID: testGameUUID, VersionUUID: testVersionUUID, Revision: 1}, result)
	})

	t.Run("a blocked market refuses as a service restriction", func(t *testing.T) {
		manager, deps := newTestManager(t)
		deps.market.decision = compliance.AccessDecision{Outcome: compliance.AccessBlocked, Reason: compliance.BlockMarketUnavailable}

		_, err := manager.CreateGame(ctx, testUserUUID, "Memoria", testKey, "", "CU")

		var restricted *marketgate.RestrictedError
		require.ErrorAs(t, err, &restricted)
		require.Equal(t, compliance.BlockMarketUnavailable, restricted.Reason)
	})
}
