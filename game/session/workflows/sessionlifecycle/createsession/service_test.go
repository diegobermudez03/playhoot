package createsession

import (
	"context"
	"errors"
	"testing"

	"github.com/diegobermudez03/playhoot/game/game"
	"github.com/diegobermudez03/playhoot/game/language/v1/program"
	"github.com/diegobermudez03/playhoot/game/session"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestCreateSession(t *testing.T) {
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

	compilableDefinition := program.Definition{
		Metadata:     program.Metadata{ID: "parques", Name: "Parques"},
		RootWorkflow: "Main",
		Workflows: []program.WorkflowDeclaration{
			{
				Name:         "Main",
				ResultType:   program.BuiltinTypeReference{Type: program.BuiltinTypeUnit},
				InitialState: "Start",
				States:       []program.WorkflowStateDeclaration{{Name: "Start"}},
			},
		},
	}
	uncompilableDefinition := program.Definition{
		Metadata:     program.Metadata{ID: "broken", Name: "Broken"},
		RootWorkflow: "does-not-exist",
	}
	repoErr := errors.New("repo failed")

	tests := map[string]func(t *testing.T, mocks *svcMocks) test{
		"creates_session_when_game_is_playable_and_compiles": func(t *testing.T, mocks *svcMocks) test {
			mocks.gameReader.EXPECT().GetPlayableGameWithCurrentVersion(gomock.Any(), "game-uuid").Return(&game.Game{
				UUID:        "game-uuid",
				VersionUUID: "version-uuid",
				Definition:  compilableDefinition,
			}, nil)
			mocks.repo.EXPECT().createSession(gomock.Any(), createParams{
				GameUUID:           "game-uuid",
				HostUserUUID:       "host-uuid",
				IdempotencyKey:     "key-1",
				GameDefinitionUUID: "version-uuid",
				LobbyTTL:           defaultLobbyTTL,
			}).Return(Result{SessionUUID: "session-uuid", JoinCode: 1234}, nil)

			return test{
				ctx: context.Background(),
				input: Input{
					GameUUID:       "game-uuid",
					HostUserUUID:   "host-uuid",
					IdempotencyKey: "key-1",
				},
				expectedResult: Result{SessionUUID: "session-uuid", JoinCode: 1234},
			}
		},
		"returns_game_not_found_when_game_reader_returns_nil": func(t *testing.T, mocks *svcMocks) test {
			mocks.gameReader.EXPECT().GetPlayableGameWithCurrentVersion(gomock.Any(), "missing-uuid").Return(nil, nil)
			return test{
				ctx:       context.Background(),
				input:     Input{GameUUID: "missing-uuid", HostUserUUID: "host-uuid", IdempotencyKey: "key-1"},
				expectErr: true,
				errAssert: func(tt require.TestingT, err error, _ ...interface{}) {
					require.ErrorIs(tt, err, session.ErrGameNotFound)
				},
			}
		},
		"propagates_non_playable_game_error": func(t *testing.T, mocks *svcMocks) test {
			mocks.gameReader.EXPECT().GetPlayableGameWithCurrentVersion(gomock.Any(), "game-uuid").Return(nil, game.ErrNonPlayableGame)
			return test{
				ctx:       context.Background(),
				input:     Input{GameUUID: "game-uuid", HostUserUUID: "host-uuid", IdempotencyKey: "key-1"},
				expectErr: true,
				errAssert: func(tt require.TestingT, err error, _ ...interface{}) {
					require.ErrorIs(tt, err, game.ErrNonPlayableGame)
				},
			}
		},
		"rejects_definition_that_does_not_compile": func(t *testing.T, mocks *svcMocks) test {
			mocks.gameReader.EXPECT().GetPlayableGameWithCurrentVersion(gomock.Any(), "game-uuid").Return(&game.Game{
				UUID:        "game-uuid",
				VersionUUID: "version-uuid",
				Definition:  uncompilableDefinition,
			}, nil)
			return test{
				ctx:       context.Background(),
				input:     Input{GameUUID: "game-uuid", HostUserUUID: "host-uuid", IdempotencyKey: "key-1"},
				expectErr: true,
				errAssert: func(tt require.TestingT, err error, _ ...interface{}) {
					require.ErrorIs(tt, err, session.ErrDefinitionDoesNotCompile)
				},
			}
		},
		"propagates_repo_error": func(t *testing.T, mocks *svcMocks) test {
			mocks.gameReader.EXPECT().GetPlayableGameWithCurrentVersion(gomock.Any(), "game-uuid").Return(&game.Game{
				UUID:        "game-uuid",
				VersionUUID: "version-uuid",
				Definition:  compilableDefinition,
			}, nil)
			mocks.repo.EXPECT().createSession(gomock.Any(), gomock.Any()).Return(Result{}, repoErr)
			return test{
				ctx:       context.Background(),
				input:     Input{GameUUID: "game-uuid", HostUserUUID: "host-uuid", IdempotencyKey: "key-1"},
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
			useCase := &UseCase{repo: mocks.repo, gameReader: mocks.gameReader, lobbyTTL: defaultLobbyTTL}

			got, err := useCase.CreateSession(tc.ctx, tc.input)
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
