package sessionlifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/diegobermudez03/playhoot/game/game"
	"github.com/diegobermudez03/playhoot/game/language/v1/program"
	"github.com/diegobermudez03/playhoot/game/session"
	internalrepo "github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle/internal/repo"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gorm.io/gorm"
)

func TestManagerCreate(t *testing.T) {
	type test struct {
		ctx            context.Context
		gameUUID       GameUUID
		hostUserUUID   UserUUID
		idempotencyKey IdempotencyKey
		expectedResult CreatedSession
		expectErr      bool
		errAssert      require.ErrorAssertionFunc
	}

	type svcMocks struct {
		createRepo *MockcreateRepoAPI
		tx         *Mocktransactor
		gameReader *MockgameCurrentVersionReader
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

	autoRunTx := func(tx *Mocktransactor) {
		tx.EXPECT().RunInTransaction(gomock.Any(), gomock.Any()).DoAndReturn(
			func(ctx context.Context, fn func(context.Context, *gorm.DB) error) error {
				return fn(ctx, nil)
			},
		)
	}

	tests := map[string]func(t *testing.T, mocks *svcMocks) test{
		"rejects_missing_idempotency_key": func(t *testing.T, mocks *svcMocks) test {
			return test{
				ctx: context.Background(), gameUUID: "game-uuid", hostUserUUID: "host-uuid", idempotencyKey: "",
				expectErr: true,
				errAssert: func(tt require.TestingT, err error, _ ...interface{}) {
					require.ErrorIs(tt, err, session.ErrIdempotencyKeyRequired)
				},
			}
		},
		"returns_game_not_found_when_game_reader_returns_nil": func(t *testing.T, mocks *svcMocks) test {
			mocks.gameReader.EXPECT().GetPlayableGameWithCurrentVersion(gomock.Any(), "missing-uuid").Return(nil, nil)
			return test{
				ctx: context.Background(), gameUUID: "missing-uuid", hostUserUUID: "host-uuid", idempotencyKey: "key-1",
				expectErr: true,
				errAssert: func(tt require.TestingT, err error, _ ...interface{}) {
					require.ErrorIs(tt, err, session.ErrGameNotFound)
				},
			}
		},
		"propagates_non_playable_game_error": func(t *testing.T, mocks *svcMocks) test {
			mocks.gameReader.EXPECT().GetPlayableGameWithCurrentVersion(gomock.Any(), "game-uuid").Return(nil, game.ErrNonPlayableGame)
			return test{
				ctx: context.Background(), gameUUID: "game-uuid", hostUserUUID: "host-uuid", idempotencyKey: "key-1",
				expectErr: true,
				errAssert: func(tt require.TestingT, err error, _ ...interface{}) {
					require.ErrorIs(tt, err, game.ErrNonPlayableGame)
				},
			}
		},
		"rejects_definition_that_does_not_compile": func(t *testing.T, mocks *svcMocks) test {
			mocks.gameReader.EXPECT().GetPlayableGameWithCurrentVersion(gomock.Any(), "game-uuid").Return(&game.Game{
				UUID: "game-uuid", VersionUUID: "version-uuid", Definition: uncompilableDefinition,
			}, nil)
			return test{
				ctx: context.Background(), gameUUID: "game-uuid", hostUserUUID: "host-uuid", idempotencyKey: "key-1",
				expectErr: true,
				errAssert: func(tt require.TestingT, err error, _ ...interface{}) {
					require.ErrorIs(tt, err, session.ErrDefinitionDoesNotCompile)
				},
			}
		},
		"claims_and_creates_session_on_success": func(t *testing.T, mocks *svcMocks) test {
			mocks.gameReader.EXPECT().GetPlayableGameWithCurrentVersion(gomock.Any(), "game-uuid").Return(&game.Game{
				UUID: "game-uuid", VersionUUID: "version-uuid", Definition: compilableDefinition,
			}, nil)
			autoRunTx(mocks.tx)
			mocks.createRepo.EXPECT().ClaimSessionRequest(gomock.Any(), gomock.Any(), operationCreate, "host-uuid", "key-1", (*uint)(nil), gomock.Any()).
				Return(uint(1), true, nil, nil)
			mocks.createRepo.EXPECT().CreateSessionWithHost(gomock.Any(), gomock.Any(), "version-uuid", "host-uuid", gomock.Any(), gomock.Any()).
				Return(internalrepo.CreatedSession{SessionID: 7, SessionUUID: "session-uuid", HostActorID: 3, LobbyExpiresAt: time.Unix(0, 0)}, nil)
			mocks.createRepo.EXPECT().CreateJoinCode(gomock.Any(), gomock.Any(), uint(7), gomock.Any()).Return(uint(4242), nil)
			mocks.createRepo.EXPECT().CompleteSessionRequest(gomock.Any(), gomock.Any(), uint(1), gomock.Any(), outcomeCreated, gomock.Any()).Return(nil)
			return test{
				ctx: context.Background(), gameUUID: "game-uuid", hostUserUUID: "host-uuid", idempotencyKey: "key-1",
				expectedResult: CreatedSession{SessionUUID: "session-uuid", JoinCode: 4242, LobbyExpiresAt: time.Unix(0, 0)},
			}
		},
		"replays_existing_completed_claim": func(t *testing.T, mocks *svcMocks) test {
			mocks.gameReader.EXPECT().GetPlayableGameWithCurrentVersion(gomock.Any(), "game-uuid").Return(&game.Game{
				UUID: "game-uuid", VersionUUID: "version-uuid", Definition: compilableDefinition,
			}, nil)
			autoRunTx(mocks.tx)
			responsePayload := `{"session_uuid":"prior-session-uuid","join_code":1111,"lobby_expires_at":"2024-01-01T00:00:00Z"}`
			mocks.createRepo.EXPECT().ClaimSessionRequest(gomock.Any(), gomock.Any(), operationCreate, "host-uuid", "key-1", (*uint)(nil), gomock.Any()).
				Return(uint(0), false, &internalrepo.RequestRow{
					Status:          internalrepo.RequestStatusCompleted,
					RequestPayload:  `{"game_uuid":"game-uuid"}`,
					ResponsePayload: &responsePayload,
				}, nil)
			return test{
				ctx: context.Background(), gameUUID: "game-uuid", hostUserUUID: "host-uuid", idempotencyKey: "key-1",
				expectedResult: CreatedSession{SessionUUID: "prior-session-uuid", JoinCode: 1111, LobbyExpiresAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
			}
		},
		"rejects_conflicting_claim": func(t *testing.T, mocks *svcMocks) test {
			mocks.gameReader.EXPECT().GetPlayableGameWithCurrentVersion(gomock.Any(), "game-uuid").Return(&game.Game{
				UUID: "game-uuid", VersionUUID: "version-uuid", Definition: compilableDefinition,
			}, nil)
			autoRunTx(mocks.tx)
			mocks.createRepo.EXPECT().ClaimSessionRequest(gomock.Any(), gomock.Any(), operationCreate, "host-uuid", "key-1", (*uint)(nil), gomock.Any()).
				Return(uint(0), false, &internalrepo.RequestRow{
					Status:         internalrepo.RequestStatusCompleted,
					RequestPayload: `{"game_uuid":"different-game-uuid"}`,
				}, nil)
			return test{
				ctx: context.Background(), gameUUID: "game-uuid", hostUserUUID: "host-uuid", idempotencyKey: "key-1",
				expectErr: true,
				errAssert: func(tt require.TestingT, err error, _ ...interface{}) {
					require.ErrorIs(tt, err, session.ErrIdempotencyConflict)
				},
			}
		},
		"propagates_repo_error_during_creation": func(t *testing.T, mocks *svcMocks) test {
			mocks.gameReader.EXPECT().GetPlayableGameWithCurrentVersion(gomock.Any(), "game-uuid").Return(&game.Game{
				UUID: "game-uuid", VersionUUID: "version-uuid", Definition: compilableDefinition,
			}, nil)
			autoRunTx(mocks.tx)
			mocks.createRepo.EXPECT().ClaimSessionRequest(gomock.Any(), gomock.Any(), operationCreate, "host-uuid", "key-1", (*uint)(nil), gomock.Any()).
				Return(uint(1), true, nil, nil)
			mocks.createRepo.EXPECT().CreateSessionWithHost(gomock.Any(), gomock.Any(), "version-uuid", "host-uuid", gomock.Any(), gomock.Any()).
				Return(internalrepo.CreatedSession{}, repoErr)
			return test{
				ctx: context.Background(), gameUUID: "game-uuid", hostUserUUID: "host-uuid", idempotencyKey: "key-1",
				expectErr: true,
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mocks := &svcMocks{
				createRepo: NewMockcreateRepoAPI(ctrl),
				tx:         NewMocktransactor(ctrl),
				gameReader: NewMockgameCurrentVersionReader(ctrl),
			}
			tc := setup(t, mocks)
			m := &Manager{
				createRepo:        mocks.createRepo,
				tx:                mocks.tx,
				currentGameReader: mocks.gameReader,
				lobbyTTL:          defaultLobbyTTL,
				now:               func() time.Time { return time.Unix(0, 0) },
			}

			got, err := m.Create(tc.ctx, tc.gameUUID, tc.hostUserUUID, tc.idempotencyKey)
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
