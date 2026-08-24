package getgame

import (
	"context"
	"errors"
	"testing"

	"github.com/diegobermudez03/playhoot/game/game"
	"github.com/diegobermudez03/playhoot/game/language/v1/program"
	"github.com/diegobermudez03/playhoot/game/language/v1/program/gameservice"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

//go:generate mockgen -package=getgame -destination=repo_mock_test.go . repoAPI

func TestGetGameWithCurrentVersion(t *testing.T) {
	type test struct {
		ctx          context.Context
		gameUUID     string
		expectedGame *game.Game
		expectErr    bool
		errAssert    require.ErrorAssertionFunc
		expectPanic  bool
	}

	type svcMocks struct {
		repo *MockRepoAPI
	}

	definition := program.Definition{
		Metadata: program.Metadata{ID: "parques", Name: "Parques"},
	}
	script := encodeDefinitionForGetGameTest(t, definition)
	repoErr := errors.New("repo failed")

	tests := map[string]func(t *testing.T, mocks *svcMocks) test{
		"returns_game": func(t *testing.T, mocks *svcMocks) test {
			mocks.repo.EXPECT().getGameCurrentVersion(gomock.Any(), "game-uuid").Return(&gameWithVersion{
				UUID:         "game-uuid",
				Name:         "Parques",
				Description:  "Race game",
				OwnerUUID:    "owner-uuid",
				LogoImageURL: "https://example.com/logo.png",
				Visibility:   string(game.Private),
				VersionUUID:  "version-uuid",
				Script:       script,
			}, nil)
			return test{
				ctx:      context.Background(),
				gameUUID: "game-uuid",
				expectedGame: &game.Game{
					UUID:         "game-uuid",
					Name:         "Parques",
					Description:  "Race game",
					OwnerUUID:    "owner-uuid",
					LogoImageURL: "https://example.com/logo.png",
					Visibility:   game.Private,
					VersionUUID:  "version-uuid",
					Definition:   definition,
				},
			}
		},
		"returns_nil_when_repo_has_no_game": func(t *testing.T, mocks *svcMocks) test {
			mocks.repo.EXPECT().getGameCurrentVersion(gomock.Any(), "missing-game").Return(nil, nil)
			return test{
				ctx:      context.Background(),
				gameUUID: "missing-game",
			}
		},
		"returns_repo_error": func(t *testing.T, mocks *svcMocks) test {
			mocks.repo.EXPECT().getGameCurrentVersion(gomock.Any(), "game-uuid").Return(nil, repoErr)
			return test{
				ctx:       context.Background(),
				gameUUID:  "game-uuid",
				expectErr: true,
			}
		},
		"returns_invalid_script_error": func(t *testing.T, mocks *svcMocks) test {
			mocks.repo.EXPECT().getGameCurrentVersion(gomock.Any(), "game-uuid").Return(&gameWithVersion{
				Visibility:  string(game.Public),
				VersionUUID: "version-uuid",
				Script:      `{"metadata":`,
			}, nil)
			return test{
				ctx:       context.Background(),
				gameUUID:  "game-uuid",
				expectErr: true,
			}
		},
		"panics_on_invalid_visibility": func(t *testing.T, mocks *svcMocks) test {
			mocks.repo.EXPECT().getGameCurrentVersion(gomock.Any(), "game-uuid").Return(&gameWithVersion{
				Visibility:  "bad-visibility",
				VersionUUID: "version-uuid",
				Script:      script,
			}, nil)
			return test{
				ctx:         context.Background(),
				gameUUID:    "game-uuid",
				expectPanic: true,
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mocks := &svcMocks{repo: NewMockRepoAPI(ctrl)}
			tc := setup(t, mocks)
			useCase := &UseCase{repo: mocks.repo}

			if tc.expectPanic {
				require.Panics(t, func() {
					_, _ = useCase.GetGameWithCurrentVersion(tc.ctx, tc.gameUUID)
				})
				return
			}

			got, err := useCase.GetGameWithCurrentVersion(tc.ctx, tc.gameUUID)
			if tc.expectErr {
				require.Error(t, err)
				if tc.errAssert != nil {
					tc.errAssert(t, err)
				}
				require.Nil(t, got)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expectedGame, got)
		})
	}
}

func encodeDefinitionForGetGameTest(t *testing.T, definition program.Definition) string {
	t.Helper()

	data, err := gameservice.EncodeJSON(definition)
	require.NoError(t, err)
	return string(data)
}
