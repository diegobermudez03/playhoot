package getgamedefinition

import (
	"context"
	"errors"
	"testing"

	"github.com/diegobermudez03/playhoot/game/language/v1/program"
	"github.com/diegobermudez03/playhoot/game/language/v1/program/gameservice"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

//go:generate mockgen -package=getgamedefinition -destination=repo_mock_test.go . repoAPI

func TestGetGameDefinition(t *testing.T) {
	type test struct {
		ctx                context.Context
		gameDefinitionUUID string
		expectedDefinition *program.Definition
		expectErr          bool
		errAssert          require.ErrorAssertionFunc
	}

	type svcMocks struct {
		repo *MockrepoAPI
	}

	definition := program.Definition{
		Metadata: program.Metadata{ID: "parques", Name: "Parques"},
		Players:  program.PlayerPolicy{Min: 2, Max: 4},
	}
	script := encodeDefinitionForTest(t, definition)
	repoErr := errors.New("repo failed")

	tests := map[string]func(t *testing.T, mocks *svcMocks) test{
		"returns_pinned_definition": func(t *testing.T, mocks *svcMocks) test {
			mocks.repo.EXPECT().getGameDefinitionByUUID(gomock.Any(), "version-uuid").Return(&gameDefinitionRow{
				UUID:   "version-uuid",
				Script: script,
			}, nil)
			return test{
				ctx:                context.Background(),
				gameDefinitionUUID: "version-uuid",
				expectedDefinition: &definition,
			}
		},
		"returns_nil_when_definition_does_not_exist": func(t *testing.T, mocks *svcMocks) test {
			mocks.repo.EXPECT().getGameDefinitionByUUID(gomock.Any(), "missing-uuid").Return(nil, nil)
			return test{
				ctx:                context.Background(),
				gameDefinitionUUID: "missing-uuid",
			}
		},
		"returns_repo_error": func(t *testing.T, mocks *svcMocks) test {
			mocks.repo.EXPECT().getGameDefinitionByUUID(gomock.Any(), "version-uuid").Return(nil, repoErr)
			return test{
				ctx:                context.Background(),
				gameDefinitionUUID: "version-uuid",
				expectErr:          true,
			}
		},
		"returns_error_on_invalid_script": func(t *testing.T, mocks *svcMocks) test {
			mocks.repo.EXPECT().getGameDefinitionByUUID(gomock.Any(), "version-uuid").Return(&gameDefinitionRow{
				UUID:   "version-uuid",
				Script: `{"metadata":`,
			}, nil)
			return test{
				ctx:                context.Background(),
				gameDefinitionUUID: "version-uuid",
				expectErr:          true,
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mocks := &svcMocks{repo: NewMockrepoAPI(ctrl)}
			tc := setup(t, mocks)
			useCase := &UseCase{repo: mocks.repo}

			got, err := useCase.GetGameDefinition(tc.ctx, tc.gameDefinitionUUID)
			if tc.expectErr {
				require.Error(t, err)
				if tc.errAssert != nil {
					tc.errAssert(t, err)
				}
				require.Nil(t, got)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expectedDefinition, got)
		})
	}
}

func encodeDefinitionForTest(t *testing.T, definition program.Definition) string {
	t.Helper()

	data, err := gameservice.EncodeJSON(definition)
	require.NoError(t, err)
	return string(data)
}
