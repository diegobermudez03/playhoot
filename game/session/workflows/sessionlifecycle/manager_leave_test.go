package sessionlifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/diegobermudez03/playhoot/game/session"
	internalrepo "github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle/internal/repo"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gorm.io/gorm"
)

func TestManagerLeave(t *testing.T) {
	type test struct {
		ctx            context.Context
		sessionUUID    SessionUUID
		userUUID       UserUUID
		idempotencyKey IdempotencyKey
		expectedResult LeaveResult
		expectErr      bool
		errAssert      require.ErrorAssertionFunc
	}

	type svcMocks struct {
		repo *MockleaveRepoAPI
		tx   *Mocktransactor
	}

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
				ctx: context.Background(), sessionUUID: "session-uuid", userUUID: "user-uuid", idempotencyKey: "",
				expectErr: true,
				errAssert: func(tt require.TestingT, err error, _ ...interface{}) {
					require.ErrorIs(tt, err, session.ErrIdempotencyKeyRequired)
				},
			}
		},
		"rejects_session_not_found": func(t *testing.T, mocks *svcMocks) test {
			autoRunTx(mocks.tx)
			mocks.repo.EXPECT().LockSessionByUUID(gomock.Any(), gomock.Any(), "session-uuid").Return(nil, nil)
			return test{
				ctx: context.Background(), sessionUUID: "session-uuid", userUUID: "user-uuid", idempotencyKey: "key-1",
				expectErr: true,
				errAssert: func(tt require.TestingT, err error, _ ...interface{}) {
					require.ErrorIs(tt, err, session.ErrSessionNotFound)
				},
			}
		},
		"materializes_expiration_and_rejects_not_in_lobby": func(t *testing.T, mocks *svcMocks) test {
			autoRunTx(mocks.tx)
			mocks.repo.EXPECT().LockSessionByUUID(gomock.Any(), gomock.Any(), "session-uuid").Return(&internalrepo.SessionRow{
				ID: 7, UUID: "session-uuid", Phase: session.PhaseLobby, LobbyExpiresAt: fixedNow.Add(-1 * time.Minute),
			}, nil)
			mocks.repo.EXPECT().SetSessionTerminal(gomock.Any(), gomock.Any(), uint(7), fixedNow.Add(-1*time.Minute), session.TerminalReasonLobbyExpired, fixedNow).Return(nil)
			mocks.repo.EXPECT().RevokeActiveJoinCode(gomock.Any(), gomock.Any(), uint(7), fixedNow).Return(nil)
			return test{
				ctx: context.Background(), sessionUUID: "session-uuid", userUUID: "user-uuid", idempotencyKey: "key-1",
				expectErr: true,
				errAssert: func(tt require.TestingT, err error, _ ...interface{}) {
					require.ErrorIs(tt, err, session.ErrNotInLobbyPhase)
				},
			}
		},
		"rejects_already_terminal_session": func(t *testing.T, mocks *svcMocks) test {
			autoRunTx(mocks.tx)
			mocks.repo.EXPECT().LockSessionByUUID(gomock.Any(), gomock.Any(), "session-uuid").Return(&internalrepo.SessionRow{
				ID: 7, UUID: "session-uuid", Phase: session.PhaseTerminal, LobbyExpiresAt: fixedNow.Add(10 * time.Minute),
			}, nil)
			return test{
				ctx: context.Background(), sessionUUID: "session-uuid", userUUID: "user-uuid", idempotencyKey: "key-1",
				expectErr: true,
				errAssert: func(tt require.TestingT, err error, _ ...interface{}) {
					require.ErrorIs(tt, err, session.ErrNotInLobbyPhase)
				},
			}
		},
		"replays_existing_completed_claim": func(t *testing.T, mocks *svcMocks) test {
			autoRunTx(mocks.tx)
			mocks.repo.EXPECT().LockSessionByUUID(gomock.Any(), gomock.Any(), "session-uuid").Return(&internalrepo.SessionRow{
				ID: 7, UUID: "session-uuid", Phase: session.PhaseLobby, LobbyExpiresAt: fixedNow.Add(10 * time.Minute),
			}, nil)
			responsePayload := `{"session_uuid":"session-uuid"}`
			mocks.repo.EXPECT().ClaimSessionRequest(gomock.Any(), gomock.Any(), operationLeave, "user-uuid", "key-1", gomock.Any(), gomock.Any()).
				Return(uint(0), false, &internalrepo.RequestRow{
					Status:          internalrepo.RequestStatusCompleted,
					RequestPayload:  `{"session_uuid":"session-uuid","user_uuid":"user-uuid"}`,
					ResponsePayload: &responsePayload,
				}, nil)
			return test{
				ctx: context.Background(), sessionUUID: "session-uuid", userUUID: "user-uuid", idempotencyKey: "key-1",
				expectedResult: LeaveResult{SessionUUID: "session-uuid"},
			}
		},
		"rejects_conflicting_claim": func(t *testing.T, mocks *svcMocks) test {
			autoRunTx(mocks.tx)
			mocks.repo.EXPECT().LockSessionByUUID(gomock.Any(), gomock.Any(), "session-uuid").Return(&internalrepo.SessionRow{
				ID: 7, UUID: "session-uuid", Phase: session.PhaseLobby, LobbyExpiresAt: fixedNow.Add(10 * time.Minute),
			}, nil)
			mocks.repo.EXPECT().ClaimSessionRequest(gomock.Any(), gomock.Any(), operationLeave, "user-uuid", "key-1", gomock.Any(), gomock.Any()).
				Return(uint(0), false, &internalrepo.RequestRow{
					Status:         internalrepo.RequestStatusCompleted,
					RequestPayload: `{"session_uuid":"different-session-uuid","user_uuid":"user-uuid"}`,
				}, nil)
			return test{
				ctx: context.Background(), sessionUUID: "session-uuid", userUUID: "user-uuid", idempotencyKey: "key-1",
				expectErr: true,
				errAssert: func(tt require.TestingT, err error, _ ...interface{}) {
					require.ErrorIs(tt, err, session.ErrIdempotencyConflict)
				},
			}
		},
		"rejects_actor_not_found_and_commits_rejection_outcome": func(t *testing.T, mocks *svcMocks) test {
			autoRunTx(mocks.tx)
			mocks.repo.EXPECT().LockSessionByUUID(gomock.Any(), gomock.Any(), "session-uuid").Return(&internalrepo.SessionRow{
				ID: 7, UUID: "session-uuid", Phase: session.PhaseLobby, LobbyExpiresAt: fixedNow.Add(10 * time.Minute),
			}, nil)
			mocks.repo.EXPECT().ClaimSessionRequest(gomock.Any(), gomock.Any(), operationLeave, "user-uuid", "key-1", gomock.Any(), gomock.Any()).
				Return(uint(9), true, nil, nil)
			mocks.repo.EXPECT().FindActor(gomock.Any(), gomock.Any(), uint(7), "user-uuid").Return(nil, nil)
			mocks.repo.EXPECT().CompleteSessionRequest(gomock.Any(), gomock.Any(), uint(9), gomock.Any(), outcomeActorNotFound, "").Return(nil)
			return test{
				ctx: context.Background(), sessionUUID: "session-uuid", userUUID: "user-uuid", idempotencyKey: "key-1",
				expectErr: true,
				errAssert: func(tt require.TestingT, err error, _ ...interface{}) {
					require.ErrorIs(tt, err, session.ErrActorNotFound)
				},
			}
		},
		"deactivates_active_participant": func(t *testing.T, mocks *svcMocks) test {
			autoRunTx(mocks.tx)
			mocks.repo.EXPECT().LockSessionByUUID(gomock.Any(), gomock.Any(), "session-uuid").Return(&internalrepo.SessionRow{
				ID: 7, UUID: "session-uuid", Phase: session.PhaseLobby, LobbyExpiresAt: fixedNow.Add(10 * time.Minute),
			}, nil)
			mocks.repo.EXPECT().ClaimSessionRequest(gomock.Any(), gomock.Any(), operationLeave, "user-uuid", "key-1", gomock.Any(), gomock.Any()).
				Return(uint(9), true, nil, nil)
			mocks.repo.EXPECT().FindActor(gomock.Any(), gomock.Any(), uint(7), "user-uuid").Return(&internalrepo.ActorRow{ID: 55, SessionID: 7, UserUUID: "user-uuid"}, nil)
			mocks.repo.EXPECT().FindParticipant(gomock.Any(), gomock.Any(), uint(55)).Return(&internalrepo.ParticipantRow{ID: 66, SessionActorID: 55, Active: true}, nil)
			mocks.repo.EXPECT().DeactivateParticipant(gomock.Any(), gomock.Any(), uint(66), fixedNow).Return(nil)
			mocks.repo.EXPECT().CompleteSessionRequest(gomock.Any(), gomock.Any(), uint(9), gomock.Any(), outcomeLeft, gomock.Any()).Return(nil)
			return test{
				ctx: context.Background(), sessionUUID: "session-uuid", userUUID: "user-uuid", idempotencyKey: "key-1",
				expectedResult: LeaveResult{SessionUUID: "session-uuid"},
			}
		},
		"no_ops_when_participant_already_inactive": func(t *testing.T, mocks *svcMocks) test {
			autoRunTx(mocks.tx)
			mocks.repo.EXPECT().LockSessionByUUID(gomock.Any(), gomock.Any(), "session-uuid").Return(&internalrepo.SessionRow{
				ID: 7, UUID: "session-uuid", Phase: session.PhaseLobby, LobbyExpiresAt: fixedNow.Add(10 * time.Minute),
			}, nil)
			mocks.repo.EXPECT().ClaimSessionRequest(gomock.Any(), gomock.Any(), operationLeave, "user-uuid", "key-1", gomock.Any(), gomock.Any()).
				Return(uint(9), true, nil, nil)
			mocks.repo.EXPECT().FindActor(gomock.Any(), gomock.Any(), uint(7), "user-uuid").Return(&internalrepo.ActorRow{ID: 55, SessionID: 7, UserUUID: "user-uuid"}, nil)
			mocks.repo.EXPECT().FindParticipant(gomock.Any(), gomock.Any(), uint(55)).Return(&internalrepo.ParticipantRow{ID: 66, SessionActorID: 55, Active: false}, nil)
			mocks.repo.EXPECT().CompleteSessionRequest(gomock.Any(), gomock.Any(), uint(9), gomock.Any(), outcomeLeft, gomock.Any()).Return(nil)
			return test{
				ctx: context.Background(), sessionUUID: "session-uuid", userUUID: "user-uuid", idempotencyKey: "key-1",
				expectedResult: LeaveResult{SessionUUID: "session-uuid"},
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mocks := &svcMocks{
				repo: NewMockleaveRepoAPI(ctrl),
				tx:   NewMocktransactor(ctrl),
			}
			tc := setup(t, mocks)
			m := &Manager{
				leaveRepo: mocks.repo,
				tx:        mocks.tx,
				now:       func() time.Time { return fixedNow },
			}

			got, err := m.Leave(tc.ctx, tc.sessionUUID, tc.userUUID, tc.idempotencyKey)
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
