package sessionlifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/diegobermudez03/playhoot/game/language/v1/program"
	"github.com/diegobermudez03/playhoot/game/session"
	internalrepo "github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle/internal/repo"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gorm.io/gorm"
)

func TestManagerJoin(t *testing.T) {
	type test struct {
		ctx            context.Context
		joinCode       JoinCode
		userUUID       UserUUID
		displayName    DisplayName
		idempotencyKey IdempotencyKey
		expectedResult JoinResult
		expectErr      bool
		errAssert      require.ErrorAssertionFunc
	}

	type svcMocks struct {
		repo       *MockjoinRepoAPI
		tx         *Mocktransactor
		gameReader *MockgamePinnedDefinitionReader
	}

	repoErr := errors.New("repo failed")
	fixedNow := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

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
				ctx: context.Background(), joinCode: 1234, userUUID: "user-uuid", displayName: "Alice", idempotencyKey: "",
				expectErr: true,
				errAssert: func(tt require.TestingT, err error, _ ...interface{}) {
					require.ErrorIs(tt, err, session.ErrIdempotencyKeyRequired)
				},
			}
		},
		"rejects_invalid_join_code": func(t *testing.T, mocks *svcMocks) test {
			mocks.repo.EXPECT().ResolveActiveSessionForJoinCode(gomock.Any(), uint(9999)).Return(nil, nil)
			return test{
				ctx: context.Background(), joinCode: 9999, userUUID: "user-uuid", displayName: "Alice", idempotencyKey: "key-1",
				expectErr: true,
				errAssert: func(tt require.TestingT, err error, _ ...interface{}) {
					require.ErrorIs(tt, err, session.ErrJoinCodeInvalid)
				},
			}
		},
		"alerts_and_rejects_when_pinned_definition_is_missing": func(t *testing.T, mocks *svcMocks) test {
			mocks.repo.EXPECT().ResolveActiveSessionForJoinCode(gomock.Any(), uint(1234)).Return(&internalrepo.JoinCodeResolution{
				SessionID: 7, GameDefinitionUUID: "pinned-version-uuid",
			}, nil)
			mocks.gameReader.EXPECT().GetGameDefinition(gomock.Any(), "pinned-version-uuid").Return(nil, nil)
			return test{
				ctx: context.Background(), joinCode: 1234, userUUID: "user-uuid", displayName: "Alice", idempotencyKey: "key-1",
				expectErr: true,
				errAssert: func(tt require.TestingT, err error, _ ...interface{}) {
					require.ErrorIs(tt, err, session.ErrPinnedDefinitionMissing)
				},
			}
		},
		"rejects_session_not_found_after_lock_miss": func(t *testing.T, mocks *svcMocks) test {
			mocks.repo.EXPECT().ResolveActiveSessionForJoinCode(gomock.Any(), uint(1234)).Return(&internalrepo.JoinCodeResolution{
				SessionID: 7, GameDefinitionUUID: "pinned-version-uuid",
			}, nil)
			mocks.gameReader.EXPECT().GetGameDefinition(gomock.Any(), "pinned-version-uuid").Return(&program.Definition{Players: program.PlayerPolicy{Max: 4}}, nil)
			autoRunTx(mocks.tx)
			mocks.repo.EXPECT().LockSessionByID(gomock.Any(), gomock.Any(), uint(7)).Return(nil, nil)
			return test{
				ctx: context.Background(), joinCode: 1234, userUUID: "user-uuid", displayName: "Alice", idempotencyKey: "key-1",
				expectErr: true,
				errAssert: func(tt require.TestingT, err error, _ ...interface{}) {
					require.ErrorIs(tt, err, session.ErrSessionNotFound)
				},
			}
		},
		"materializes_expiration_and_rejects_without_touching_idempotency": func(t *testing.T, mocks *svcMocks) test {
			mocks.repo.EXPECT().ResolveActiveSessionForJoinCode(gomock.Any(), uint(1234)).Return(&internalrepo.JoinCodeResolution{
				SessionID: 7, GameDefinitionUUID: "pinned-version-uuid",
			}, nil)
			mocks.gameReader.EXPECT().GetGameDefinition(gomock.Any(), "pinned-version-uuid").Return(&program.Definition{Players: program.PlayerPolicy{Max: 4}}, nil)
			autoRunTx(mocks.tx)
			mocks.repo.EXPECT().LockSessionByID(gomock.Any(), gomock.Any(), uint(7)).Return(&internalrepo.SessionRow{
				ID: 7, UUID: "session-uuid", Phase: session.PhaseLobby, LobbyExpiresAt: fixedNow.Add(-1 * time.Minute),
			}, nil)
			mocks.repo.EXPECT().SetSessionTerminal(gomock.Any(), gomock.Any(), uint(7), fixedNow.Add(-1*time.Minute), session.TerminalReasonLobbyExpired, fixedNow).Return(nil)
			mocks.repo.EXPECT().RevokeActiveJoinCode(gomock.Any(), gomock.Any(), uint(7), fixedNow).Return(nil)
			// No ClaimSessionRequest/CompleteSessionRequest call is expected:
			// the rejection is discovered before any idempotency claim.
			return test{
				ctx: context.Background(), joinCode: 1234, userUUID: "user-uuid", displayName: "Alice", idempotencyKey: "key-1",
				expectErr: true,
				errAssert: func(tt require.TestingT, err error, _ ...interface{}) {
					require.ErrorIs(tt, err, session.ErrLobbyExpired)
				},
			}
		},
		"replays_existing_completed_claim": func(t *testing.T, mocks *svcMocks) test {
			mocks.repo.EXPECT().ResolveActiveSessionForJoinCode(gomock.Any(), uint(1234)).Return(&internalrepo.JoinCodeResolution{
				SessionID: 7, GameDefinitionUUID: "pinned-version-uuid",
			}, nil)
			mocks.gameReader.EXPECT().GetGameDefinition(gomock.Any(), "pinned-version-uuid").Return(&program.Definition{Players: program.PlayerPolicy{Max: 4}}, nil)
			autoRunTx(mocks.tx)
			mocks.repo.EXPECT().LockSessionByID(gomock.Any(), gomock.Any(), uint(7)).Return(&internalrepo.SessionRow{
				ID: 7, UUID: "session-uuid", Phase: session.PhaseLobby, LobbyExpiresAt: fixedNow.Add(10 * time.Minute),
			}, nil)
			responsePayload := `{"session_uuid":"session-uuid","display_name":"Alice"}`
			mocks.repo.EXPECT().ClaimSessionRequest(gomock.Any(), gomock.Any(), operationJoin, "user-uuid", "key-1", gomock.Any(), gomock.Any()).
				Return(uint(0), false, &internalrepo.RequestRow{
					Status:          internalrepo.RequestStatusCompleted,
					Outcome:         outcomeJoined,
					RequestPayload:  `{"join_code":1234,"user_uuid":"user-uuid","display_name":"Alice"}`,
					ResponsePayload: &responsePayload,
				}, nil)
			return test{
				ctx: context.Background(), joinCode: 1234, userUUID: "user-uuid", displayName: "Alice", idempotencyKey: "key-1",
				expectedResult: JoinResult{SessionUUID: "session-uuid", DisplayName: "Alice"},
			}
		},
		"rejects_conflicting_claim": func(t *testing.T, mocks *svcMocks) test {
			mocks.repo.EXPECT().ResolveActiveSessionForJoinCode(gomock.Any(), uint(1234)).Return(&internalrepo.JoinCodeResolution{
				SessionID: 7, GameDefinitionUUID: "pinned-version-uuid",
			}, nil)
			mocks.gameReader.EXPECT().GetGameDefinition(gomock.Any(), "pinned-version-uuid").Return(&program.Definition{Players: program.PlayerPolicy{Max: 4}}, nil)
			autoRunTx(mocks.tx)
			mocks.repo.EXPECT().LockSessionByID(gomock.Any(), gomock.Any(), uint(7)).Return(&internalrepo.SessionRow{
				ID: 7, UUID: "session-uuid", Phase: session.PhaseLobby, LobbyExpiresAt: fixedNow.Add(10 * time.Minute),
			}, nil)
			mocks.repo.EXPECT().ClaimSessionRequest(gomock.Any(), gomock.Any(), operationJoin, "user-uuid", "key-1", gomock.Any(), gomock.Any()).
				Return(uint(0), false, &internalrepo.RequestRow{
					Status:         internalrepo.RequestStatusCompleted,
					RequestPayload: `{"join_code":1234,"user_uuid":"user-uuid","display_name":"Different"}`,
				}, nil)
			return test{
				ctx: context.Background(), joinCode: 1234, userUUID: "user-uuid", displayName: "Alice", idempotencyKey: "key-1",
				expectErr: true,
				errAssert: func(tt require.TestingT, err error, _ ...interface{}) {
					require.ErrorIs(tt, err, session.ErrIdempotencyConflict)
				},
			}
		},
		"rejects_already_joined_for_new_token_and_commits_rejection_outcome": func(t *testing.T, mocks *svcMocks) test {
			mocks.repo.EXPECT().ResolveActiveSessionForJoinCode(gomock.Any(), uint(1234)).Return(&internalrepo.JoinCodeResolution{
				SessionID: 7, GameDefinitionUUID: "pinned-version-uuid",
			}, nil)
			mocks.gameReader.EXPECT().GetGameDefinition(gomock.Any(), "pinned-version-uuid").Return(&program.Definition{Players: program.PlayerPolicy{Max: 4}}, nil)
			autoRunTx(mocks.tx)
			mocks.repo.EXPECT().LockSessionByID(gomock.Any(), gomock.Any(), uint(7)).Return(&internalrepo.SessionRow{
				ID: 7, UUID: "session-uuid", Phase: session.PhaseLobby, LobbyExpiresAt: fixedNow.Add(10 * time.Minute),
			}, nil)
			mocks.repo.EXPECT().ClaimSessionRequest(gomock.Any(), gomock.Any(), operationJoin, "user-uuid", "key-2", gomock.Any(), gomock.Any()).
				Return(uint(9), true, nil, nil)
			mocks.repo.EXPECT().FindActor(gomock.Any(), gomock.Any(), uint(7), "user-uuid").Return(&internalrepo.ActorRow{ID: 55, SessionID: 7, UserUUID: "user-uuid"}, nil)
			mocks.repo.EXPECT().FindParticipant(gomock.Any(), gomock.Any(), uint(55)).Return(&internalrepo.ParticipantRow{ID: 66, SessionActorID: 55, Active: true}, nil)
			mocks.repo.EXPECT().CompleteSessionRequest(gomock.Any(), gomock.Any(), uint(9), gomock.Any(), outcomeAlreadyJoined, "").Return(nil)
			return test{
				ctx: context.Background(), joinCode: 1234, userUUID: "user-uuid", displayName: "Alice", idempotencyKey: "key-2",
				expectErr: true,
				errAssert: func(tt require.TestingT, err error, _ ...interface{}) {
					require.ErrorIs(tt, err, session.ErrAlreadyJoined)
				},
			}
		},
		"rejects_lobby_full_and_commits_rejection_outcome": func(t *testing.T, mocks *svcMocks) test {
			mocks.repo.EXPECT().ResolveActiveSessionForJoinCode(gomock.Any(), uint(1234)).Return(&internalrepo.JoinCodeResolution{
				SessionID: 7, GameDefinitionUUID: "pinned-version-uuid",
			}, nil)
			mocks.gameReader.EXPECT().GetGameDefinition(gomock.Any(), "pinned-version-uuid").Return(&program.Definition{Players: program.PlayerPolicy{Max: 1}}, nil)
			autoRunTx(mocks.tx)
			mocks.repo.EXPECT().LockSessionByID(gomock.Any(), gomock.Any(), uint(7)).Return(&internalrepo.SessionRow{
				ID: 7, UUID: "session-uuid", Phase: session.PhaseLobby, LobbyExpiresAt: fixedNow.Add(10 * time.Minute),
			}, nil)
			mocks.repo.EXPECT().ClaimSessionRequest(gomock.Any(), gomock.Any(), operationJoin, "user-uuid", "key-1", gomock.Any(), gomock.Any()).
				Return(uint(9), true, nil, nil)
			mocks.repo.EXPECT().FindActor(gomock.Any(), gomock.Any(), uint(7), "user-uuid").Return(nil, nil)
			mocks.repo.EXPECT().CreateActor(gomock.Any(), gomock.Any(), uint(7), "user-uuid", fixedNow).Return(uint(55), nil)
			mocks.repo.EXPECT().CountActiveParticipants(gomock.Any(), gomock.Any(), uint(7)).Return(1, nil)
			mocks.repo.EXPECT().CompleteSessionRequest(gomock.Any(), gomock.Any(), uint(9), gomock.Any(), outcomeLobbyFull, "").Return(nil)
			return test{
				ctx: context.Background(), joinCode: 1234, userUUID: "user-uuid", displayName: "Alice", idempotencyKey: "key-1",
				expectErr: true,
				errAssert: func(tt require.TestingT, err error, _ ...interface{}) {
					require.ErrorIs(tt, err, session.ErrLobbyFull)
				},
			}
		},
		"creates_actor_and_activates_new_participant": func(t *testing.T, mocks *svcMocks) test {
			mocks.repo.EXPECT().ResolveActiveSessionForJoinCode(gomock.Any(), uint(1234)).Return(&internalrepo.JoinCodeResolution{
				SessionID: 7, GameDefinitionUUID: "pinned-version-uuid",
			}, nil)
			mocks.gameReader.EXPECT().GetGameDefinition(gomock.Any(), "pinned-version-uuid").Return(&program.Definition{Players: program.PlayerPolicy{Max: 4}}, nil)
			autoRunTx(mocks.tx)
			mocks.repo.EXPECT().LockSessionByID(gomock.Any(), gomock.Any(), uint(7)).Return(&internalrepo.SessionRow{
				ID: 7, UUID: "session-uuid", Phase: session.PhaseLobby, LobbyExpiresAt: fixedNow.Add(10 * time.Minute),
			}, nil)
			mocks.repo.EXPECT().ClaimSessionRequest(gomock.Any(), gomock.Any(), operationJoin, "user-uuid", "key-1", gomock.Any(), gomock.Any()).
				Return(uint(9), true, nil, nil)
			mocks.repo.EXPECT().FindActor(gomock.Any(), gomock.Any(), uint(7), "user-uuid").Return(nil, nil)
			mocks.repo.EXPECT().CreateActor(gomock.Any(), gomock.Any(), uint(7), "user-uuid", fixedNow).Return(uint(55), nil)
			mocks.repo.EXPECT().CountActiveParticipants(gomock.Any(), gomock.Any(), uint(7)).Return(0, nil)
			mocks.repo.EXPECT().CreateParticipant(gomock.Any(), gomock.Any(), uint(55), "Alice", fixedNow).Return(nil)
			mocks.repo.EXPECT().CompleteSessionRequest(gomock.Any(), gomock.Any(), uint(9), gomock.Any(), outcomeJoined, gomock.Any()).Return(nil)
			return test{
				ctx: context.Background(), joinCode: 1234, userUUID: "user-uuid", displayName: "Alice", idempotencyKey: "key-1",
				expectedResult: JoinResult{SessionUUID: "session-uuid", DisplayName: "Alice"},
			}
		},
		"reactivates_existing_inactive_participant": func(t *testing.T, mocks *svcMocks) test {
			mocks.repo.EXPECT().ResolveActiveSessionForJoinCode(gomock.Any(), uint(1234)).Return(&internalrepo.JoinCodeResolution{
				SessionID: 7, GameDefinitionUUID: "pinned-version-uuid",
			}, nil)
			mocks.gameReader.EXPECT().GetGameDefinition(gomock.Any(), "pinned-version-uuid").Return(&program.Definition{Players: program.PlayerPolicy{Max: 4}}, nil)
			autoRunTx(mocks.tx)
			mocks.repo.EXPECT().LockSessionByID(gomock.Any(), gomock.Any(), uint(7)).Return(&internalrepo.SessionRow{
				ID: 7, UUID: "session-uuid", Phase: session.PhaseLobby, LobbyExpiresAt: fixedNow.Add(10 * time.Minute),
			}, nil)
			mocks.repo.EXPECT().ClaimSessionRequest(gomock.Any(), gomock.Any(), operationJoin, "user-uuid", "key-1", gomock.Any(), gomock.Any()).
				Return(uint(9), true, nil, nil)
			mocks.repo.EXPECT().FindActor(gomock.Any(), gomock.Any(), uint(7), "user-uuid").Return(&internalrepo.ActorRow{ID: 55, SessionID: 7, UserUUID: "user-uuid"}, nil)
			mocks.repo.EXPECT().FindParticipant(gomock.Any(), gomock.Any(), uint(55)).Return(&internalrepo.ParticipantRow{ID: 66, SessionActorID: 55, Active: false}, nil)
			mocks.repo.EXPECT().CountActiveParticipants(gomock.Any(), gomock.Any(), uint(7)).Return(0, nil)
			mocks.repo.EXPECT().ActivateParticipant(gomock.Any(), gomock.Any(), uint(66), "Alice", fixedNow).Return(nil)
			mocks.repo.EXPECT().CompleteSessionRequest(gomock.Any(), gomock.Any(), uint(9), gomock.Any(), outcomeJoined, gomock.Any()).Return(nil)
			return test{
				ctx: context.Background(), joinCode: 1234, userUUID: "user-uuid", displayName: "Alice", idempotencyKey: "key-1",
				expectedResult: JoinResult{SessionUUID: "session-uuid", DisplayName: "Alice"},
			}
		},
		"propagates_resolve_error": func(t *testing.T, mocks *svcMocks) test {
			mocks.repo.EXPECT().ResolveActiveSessionForJoinCode(gomock.Any(), uint(1234)).Return(nil, repoErr)
			return test{
				ctx: context.Background(), joinCode: 1234, userUUID: "user-uuid", displayName: "Alice", idempotencyKey: "key-1",
				expectErr: true,
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mocks := &svcMocks{
				repo:       NewMockjoinRepoAPI(ctrl),
				tx:         NewMocktransactor(ctrl),
				gameReader: NewMockgamePinnedDefinitionReader(ctrl),
			}
			tc := setup(t, mocks)
			m := &Manager{
				joinRepo:         mocks.repo,
				tx:               mocks.tx,
				pinnedGameReader: mocks.gameReader,
				now:              func() time.Time { return fixedNow },
			}

			got, err := m.Join(tc.ctx, tc.joinCode, tc.userUUID, tc.displayName, tc.idempotencyKey)
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
