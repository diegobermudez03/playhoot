package sessionlifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gorm.io/gorm"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/endcause"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/workflows/sessionlifecycle/internal/repo"
)

func TestEndSessionInTx(t *testing.T) {
	type dependencies struct {
		repo *MockendSessionRepo
	}
	type test struct {
		end sessionpkg.SessionEnd

		expected        sessionpkg.EndSessionResult
		expectErr       bool
		expectedErrorIs error
	}

	ctx := context.Background()
	tx := &gorm.DB{}
	nowInDB := startNow.Truncate(time.Microsecond)
	sessionID := uint(31)
	creator := "0b9c2a4e-5d1f-4c7a-9e3b-2f6d8a1c4e70"
	cause := func(c sessionpkg.EndCause) *string {
		value := string(c)
		return &value
	}
	locked := func(status string) *internalrepo.LockedSession {
		return &internalrepo.LockedSession{ID: sessionID, Status: status}
	}

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"host_cancels_the_lobby_as_its_creator": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.repo.EXPECT().LockSession(ctx, tx, startSessionUUID).Return(locked("LOBBY"), nil),
				deps.repo.EXPECT().ChangeSessionStatus(ctx, tx, sessionID, "LOBBY", "CANCELLED", cause(sessionpkg.EndHostCancelled), &creator, nowInDB).Return(nil),
				deps.repo.EXPECT().ReleaseJoinCodes(ctx, tx, sessionID, nowInDB).Return(nil),
			)
			return test{
				end:      sessionpkg.SessionEnd{Cause: sessionpkg.EndHostCancelled, ChangedBy: creator},
				expected: sessionpkg.EndSessionResult{Outcome: sessionpkg.SessionEndedNow, Status: "CANCELLED", EndCause: "HOST_CANCELLED"},
			}
		},
		"host_cancels_the_running_game": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.repo.EXPECT().LockSession(ctx, tx, startSessionUUID).Return(locked("RUNNING"), nil),
				deps.repo.EXPECT().ChangeSessionStatus(ctx, tx, sessionID, "RUNNING", "CANCELLED", cause(sessionpkg.EndHostCancelled), &creator, nowInDB).Return(nil),
				deps.repo.EXPECT().ReleaseJoinCodes(ctx, tx, sessionID, nowInDB).Return(nil),
			)
			return test{
				end:      sessionpkg.SessionEnd{Cause: sessionpkg.EndHostCancelled, ChangedBy: creator},
				expected: sessionpkg.EndSessionResult{Outcome: sessionpkg.SessionEndedNow, Status: "CANCELLED", EndCause: "HOST_CANCELLED"},
			}
		},
		"game_finishes_by_the_platform": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.repo.EXPECT().LockSession(ctx, tx, startSessionUUID).Return(locked("RUNNING"), nil),
				deps.repo.EXPECT().ChangeSessionStatus(ctx, tx, sessionID, "RUNNING", "FINISHED", cause(sessionpkg.EndGameFinished), (*string)(nil), nowInDB).Return(nil),
				deps.repo.EXPECT().ReleaseJoinCodes(ctx, tx, sessionID, nowInDB).Return(nil),
			)
			return test{
				end:      sessionpkg.SessionEnd{Cause: sessionpkg.EndGameFinished},
				expected: sessionpkg.EndSessionResult{Outcome: sessionpkg.SessionEndedNow, Status: "FINISHED", EndCause: "GAME_FINISHED"},
			}
		},
		"lobby_reaching_its_lifetime_expires": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.repo.EXPECT().LockSession(ctx, tx, startSessionUUID).Return(locked("LOBBY"), nil),
				deps.repo.EXPECT().ChangeSessionStatus(ctx, tx, sessionID, "LOBBY", "EXPIRED", cause(sessionpkg.EndMaxLifetime), (*string)(nil), nowInDB).Return(nil),
				deps.repo.EXPECT().ReleaseJoinCodes(ctx, tx, sessionID, nowInDB).Return(nil),
			)
			return test{
				end:      sessionpkg.SessionEnd{Cause: sessionpkg.EndMaxLifetime},
				expected: sessionpkg.EndSessionResult{Outcome: sessionpkg.SessionEndedNow, Status: "EXPIRED", EndCause: "MAX_LIFETIME"},
			}
		},
		"cause_that_does_not_apply_to_the_status_leaves_it": func(t *testing.T, deps dependencies) test {
			deps.repo.EXPECT().LockSession(ctx, tx, startSessionUUID).Return(locked("LOBBY"), nil)
			return test{
				end:      sessionpkg.SessionEnd{Cause: sessionpkg.EndInactivityTimeout},
				expected: sessionpkg.EndSessionResult{Outcome: sessionpkg.SessionNotEnded, Status: "LOBBY"},
			}
		},
		"ended_session_is_reported_as_it_ended": func(t *testing.T, deps dependencies) test {
			ended := &internalrepo.LockedSession{ID: sessionID, Status: "ABORTED", EndCause: cause(sessionpkg.EndScriptFailure)}
			deps.repo.EXPECT().LockSession(ctx, tx, startSessionUUID).Return(ended, nil)
			return test{
				end:      sessionpkg.SessionEnd{Cause: sessionpkg.EndPlatformTerminated},
				expected: sessionpkg.EndSessionResult{Outcome: sessionpkg.SessionNotEnded, Status: "ABORTED", EndCause: "SCRIPT_FAILURE"},
			}
		},
		"unknown_session": func(t *testing.T, deps dependencies) test {
			deps.repo.EXPECT().LockSession(ctx, tx, startSessionUUID).Return(nil, nil)
			return test{end: sessionpkg.SessionEnd{Cause: sessionpkg.EndAllPlayersLeft}, expectErr: true, expectedErrorIs: sessionpkg.ErrSessionNotFound}
		},
		"changing_the_status_fails": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.repo.EXPECT().LockSession(ctx, tx, startSessionUUID).Return(locked("RUNNING"), nil),
				deps.repo.EXPECT().ChangeSessionStatus(ctx, tx, sessionID, "RUNNING", "ABORTED", cause(sessionpkg.EndScriptFailure), (*string)(nil), nowInDB).Return(errors.New("database down")),
			)
			return test{end: sessionpkg.SessionEnd{Cause: sessionpkg.EndScriptFailure}, expectErr: true}
		},
		"releasing_the_join_codes_fails": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.repo.EXPECT().LockSession(ctx, tx, startSessionUUID).Return(locked("LOBBY"), nil),
				deps.repo.EXPECT().ChangeSessionStatus(ctx, tx, sessionID, "LOBBY", "EXPIRED", cause(sessionpkg.EndLobbyExpired), (*string)(nil), nowInDB).Return(nil),
				deps.repo.EXPECT().ReleaseJoinCodes(ctx, tx, sessionID, nowInDB).Return(errors.New("database down")),
			)
			return test{end: sessionpkg.SessionEnd{Cause: sessionpkg.EndLobbyExpired}, expectErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			deps := dependencies{repo: NewMockendSessionRepo(ctrl)}
			tc := setup(t, deps)
			manager := &Manager{endSessionRepo: deps.repo, now: func() time.Time { return startNow }}
			ending, known := endcause.EndingOf(endcause.Cause(tc.end.Cause))
			require.True(t, known)

			result, err := manager.endSessionInTx(ctx, tx, startSessionUUID, tc.end, ending)

			if tc.expectErr {
				require.Error(t, err)
				if tc.expectedErrorIs != nil {
					require.ErrorIs(t, err, tc.expectedErrorIs)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestEndSession(t *testing.T) {
	type test struct {
		sessionUUID string
		end         sessionpkg.SessionEnd

		expectedErrorIs error
	}

	tests := map[string]func(t *testing.T) test{
		"session_uuid_that_is_not_a_uuid": func(t *testing.T) test {
			return test{sessionUUID: "not-a-uuid", end: sessionpkg.SessionEnd{Cause: sessionpkg.EndGameFinished}, expectedErrorIs: sessionpkg.ErrSessionNotFound}
		},
		"unknown_cause": func(t *testing.T) test {
			return test{sessionUUID: startSessionUUID, end: sessionpkg.SessionEnd{Cause: "BORED"}}
		},
		"user_that_is_not_a_uuid": func(t *testing.T) test {
			return test{sessionUUID: startSessionUUID, end: sessionpkg.SessionEnd{Cause: sessionpkg.EndHostCancelled, ChangedBy: "someone"}}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)

			_, err := (&Manager{}).EndSession(context.Background(), tc.sessionUUID, tc.end)

			require.Error(t, err)
			if tc.expectedErrorIs != nil {
				require.ErrorIs(t, err, tc.expectedErrorIs)
			}
		})
	}
}
