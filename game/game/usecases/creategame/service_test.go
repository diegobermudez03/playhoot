package creategame

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

//go:generate mockgen -package=creategame -destination=repo_mock_test.go . repoAPI

func TestCreateGameDraft(t *testing.T) {
	type test struct {
		ctx       context.Context
		params    Params
		uuids     []string
		uuidErr   error
		expectErr bool
		expected  *game.Game
	}

	type svcMocks struct {
		repo *MockRepoAPI
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
	script := encodeDefinitionForCreateGameTest(t, definition)
	repoErr := errors.New("repo failed")
	uuidErr := errors.New("uuid failed")

	tests := map[string]func(t *testing.T, mocks *svcMocks) test{
		"creates_game_draft": func(t *testing.T, mocks *svcMocks) test {
			params := Params{
				Name:         "Parques",
				Description:  "Race game",
				OwnerUUID:    "22222222-2222-2222-2222-222222222222",
				LogoImageURL: "https://example.com/logo.png",
				Definition:   definition,
			}
			mocks.repo.EXPECT().createGameDraft(gomock.Any(), createGameDraftParams{
				GameUUID:       "11111111-1111-4111-8111-111111111111",
				DefinitionUUID: "33333333-3333-4333-8333-333333333333",
				Name:           params.Name,
				Description:    params.Description,
				OwnerUUID:      params.OwnerUUID,
				LogoImageURL:   params.LogoImageURL,
				Script:         script,
			}).Return(&createdGameDraft{
				GameUUID:       "11111111-1111-4111-8111-111111111111",
				DefinitionUUID: "33333333-3333-4333-8333-333333333333",
				Name:           params.Name,
				Description:    params.Description,
				OwnerUUID:      params.OwnerUUID,
				LogoImageURL:   params.LogoImageURL,
			}, nil)
			return test{
				ctx:    context.Background(),
				params: params,
				uuids: []string{
					"11111111-1111-4111-8111-111111111111",
					"33333333-3333-4333-8333-333333333333",
				},
				expected: &game.Game{
					UUID:         "11111111-1111-4111-8111-111111111111",
					Name:         params.Name,
					Description:  params.Description,
					OwnerUUID:    params.OwnerUUID,
					LogoImageURL: params.LogoImageURL,
					Visibility:   game.Draft,
					VersionUUID:  "33333333-3333-4333-8333-333333333333",
					Definition:   definition,
				},
			}
		},
		"returns_uuid_error_before_repo_call": func(t *testing.T, mocks *svcMocks) test {
			return test{
				ctx:       context.Background(),
				uuidErr:   uuidErr,
				expectErr: true,
			}
		},
		"returns_repo_error": func(t *testing.T, mocks *svcMocks) test {
			params := Params{
				Name:         "Parques",
				Description:  "Race game",
				OwnerUUID:    "22222222-2222-2222-2222-222222222222",
				LogoImageURL: "https://example.com/logo.png",
				Definition:   definition,
			}
			mocks.repo.EXPECT().createGameDraft(gomock.Any(), gomock.Any()).Return(nil, repoErr)
			return test{
				ctx: context.Background(),
				uuids: []string{
					"11111111-1111-4111-8111-111111111111",
					"33333333-3333-4333-8333-333333333333",
				},
				params:    params,
				expectErr: true,
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mocks := &svcMocks{repo: NewMockRepoAPI(ctrl)}
			tc := setup(t, mocks)
			useCase := &UseCase{
				repo: mocks.repo,
				newUUID: func() (string, error) {
					if tc.uuidErr != nil {
						return "", tc.uuidErr
					}
					require.NotEmpty(t, tc.uuids)
					uuid := tc.uuids[0]
					tc.uuids = tc.uuids[1:]
					return uuid, nil
				},
			}

			got, err := useCase.CreateGameDraft(tc.ctx, tc.params)
			if tc.expectErr {
				require.Error(t, err)
				require.Nil(t, got)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, got)
			require.Empty(t, tc.uuids)
		})
	}
}

func encodeDefinitionForCreateGameTest(t *testing.T, definition program.Definition) string {
	t.Helper()

	data, err := gameservice.EncodeJSON(definition)
	require.NoError(t, err)
	return string(data)
}
