package leavesession

import (
	"context"
	"errors"
	"testing"

	"github.com/diegobermudez03/playhoot/game/session"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestLeaveSession(t *testing.T) {
	type test struct {
		ctx            context.Context
		input          Input
		expectedResult Result
		expectErr      bool
		errAssert      require.ErrorAssertionFunc
	}

	type svcMocks struct {
		repo *MockrepoAPI
	}

	repoErr := errors.New("repo failed")

	tests := map[string]func(t *testing.T, mocks *svcMocks) test{
		"leaves_active_participant": func(t *testing.T, mocks *svcMocks) test {
			mocks.repo.EXPECT().leaveSession(gomock.Any(), leaveParams{
				SessionUUID:    "session-uuid",
				UserUUID:       "user-uuid",
				IdempotencyKey: "leave-key-1",
			}).Return(Result{SessionUUID: "session-uuid"}, nil)
			return test{
				ctx: context.Background(),
				input: Input{
					SessionUUID:    "session-uuid",
					UserUUID:       "user-uuid",
					IdempotencyKey: "leave-key-1",
				},
				expectedResult: Result{SessionUUID: "session-uuid"},
			}
		},
		"propagates_not_in_lobby_phase_error": func(t *testing.T, mocks *svcMocks) test {
			mocks.repo.EXPECT().leaveSession(gomock.Any(), gomock.Any()).Return(Result{}, session.ErrNotInLobbyPhase)
			return test{
				ctx:       context.Background(),
				input:     Input{SessionUUID: "session-uuid", UserUUID: "user-uuid", IdempotencyKey: "leave-key-1"},
				expectErr: true,
				errAssert: func(tt require.TestingT, err error, _ ...interface{}) {
					require.ErrorIs(tt, err, session.ErrNotInLobbyPhase)
				},
			}
		},
		"propagates_repo_error": func(t *testing.T, mocks *svcMocks) test {
			mocks.repo.EXPECT().leaveSession(gomock.Any(), gomock.Any()).Return(Result{}, repoErr)
			return test{
				ctx:       context.Background(),
				input:     Input{SessionUUID: "session-uuid", UserUUID: "user-uuid", IdempotencyKey: "leave-key-1"},
				expectErr: true,
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mocks := &svcMocks{repo: NewMockrepoAPI(ctrl)}
			tc := setup(t, mocks)
			useCase := &UseCase{repo: mocks.repo}

			got, err := useCase.LeaveSession(tc.ctx, tc.input)
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
