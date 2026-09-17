package joinsession

import (
	"context"
	"errors"
	"testing"

	"github.com/diegobermudez03/playhoot/game/language/v1/program"
	"github.com/diegobermudez03/playhoot/game/session"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestJoinSession(t *testing.T) {
	type test struct {
		ctx            context.Context
		input          Input
		expectedResult Result
		expectErr      bool
		errAssert      require.ErrorAssertionFunc
	}

	type svcMocks struct {
		repo       *MockrepoAPI
		gameReader *MockgameDefinitionResolver
	}

	repoErr := errors.New("repo failed")

	tests := map[string]func(t *testing.T, mocks *svcMocks) test{
		"joins_using_the_pinned_definition_not_current_version": func(t *testing.T, mocks *svcMocks) test {
			mocks.repo.EXPECT().resolveActiveSessionForJoinCode(gomock.Any(), uint(1234)).Return(&joinCodeResolution{
				SessionID:          7,
				GameDefinitionUUID: "pinned-version-uuid",
			}, nil)
			// The mockable collaborator only exposes GetGameDefinition (by
			// pinned Definition/Version UUID) - there is structurally no way
			// for this call site to invoke a "current version" lookup
			// instead, and this assertion pins the exact UUID passed.
			mocks.gameReader.EXPECT().GetGameDefinition(gomock.Any(), "pinned-version-uuid").Return(&program.Definition{
				Players: program.PlayerPolicy{Max: 4},
			}, nil)
			mocks.repo.EXPECT().joinSession(gomock.Any(), joinParams{
				SessionID:      7,
				JoinCode:       1234,
				UserUUID:       "user-uuid",
				DisplayName:    "Alice",
				IdempotencyKey: "join-key-1",
				PlayersMax:     4,
			}).Return(Result{SessionUUID: "session-uuid", DisplayName: "Alice"}, nil)

			return test{
				ctx: context.Background(),
				input: Input{
					JoinCode:       1234,
					UserUUID:       "user-uuid",
					DisplayName:    "Alice",
					IdempotencyKey: "join-key-1",
				},
				expectedResult: Result{SessionUUID: "session-uuid", DisplayName: "Alice"},
			}
		},
		"rejects_invalid_join_code": func(t *testing.T, mocks *svcMocks) test {
			mocks.repo.EXPECT().resolveActiveSessionForJoinCode(gomock.Any(), uint(9999)).Return(nil, nil)
			return test{
				ctx:       context.Background(),
				input:     Input{JoinCode: 9999, UserUUID: "user-uuid", DisplayName: "Alice", IdempotencyKey: "join-key-1"},
				expectErr: true,
				errAssert: func(tt require.TestingT, err error, _ ...interface{}) {
					require.ErrorIs(tt, err, session.ErrJoinCodeInvalid)
				},
			}
		},
		"propagates_resolve_error": func(t *testing.T, mocks *svcMocks) test {
			mocks.repo.EXPECT().resolveActiveSessionForJoinCode(gomock.Any(), uint(1234)).Return(nil, repoErr)
			return test{
				ctx:       context.Background(),
				input:     Input{JoinCode: 1234, UserUUID: "user-uuid", DisplayName: "Alice", IdempotencyKey: "join-key-1"},
				expectErr: true,
			}
		},
		"alerts_and_rejects_when_pinned_definition_is_missing": func(t *testing.T, mocks *svcMocks) test {
			mocks.repo.EXPECT().resolveActiveSessionForJoinCode(gomock.Any(), uint(1234)).Return(&joinCodeResolution{
				SessionID:          7,
				GameDefinitionUUID: "pinned-version-uuid",
			}, nil)
			mocks.gameReader.EXPECT().GetGameDefinition(gomock.Any(), "pinned-version-uuid").Return(nil, nil)
			return test{
				ctx:       context.Background(),
				input:     Input{JoinCode: 1234, UserUUID: "user-uuid", DisplayName: "Alice", IdempotencyKey: "join-key-1"},
				expectErr: true,
				errAssert: func(tt require.TestingT, err error, _ ...interface{}) {
					require.ErrorIs(tt, err, session.ErrPinnedDefinitionMissing)
				},
			}
		},
		"propagates_join_repo_error": func(t *testing.T, mocks *svcMocks) test {
			mocks.repo.EXPECT().resolveActiveSessionForJoinCode(gomock.Any(), uint(1234)).Return(&joinCodeResolution{
				SessionID:          7,
				GameDefinitionUUID: "pinned-version-uuid",
			}, nil)
			mocks.gameReader.EXPECT().GetGameDefinition(gomock.Any(), "pinned-version-uuid").Return(&program.Definition{
				Players: program.PlayerPolicy{Max: 4},
			}, nil)
			mocks.repo.EXPECT().joinSession(gomock.Any(), gomock.Any()).Return(Result{}, repoErr)
			return test{
				ctx:       context.Background(),
				input:     Input{JoinCode: 1234, UserUUID: "user-uuid", DisplayName: "Alice", IdempotencyKey: "join-key-1"},
				expectErr: true,
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mocks := &svcMocks{
				repo:       NewMockrepoAPI(ctrl),
				gameReader: NewMockgameDefinitionResolver(ctrl),
			}
			tc := setup(t, mocks)
			useCase := &UseCase{repo: mocks.repo, gameReader: mocks.gameReader}

			got, err := useCase.JoinSession(tc.ctx, tc.input)
			if tc.expectErr {
				require.Error(t, err)
				if tc.errAssert != nil {
					tc.errAssert(t, err)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expectedResult, got)
		})
	}
}
