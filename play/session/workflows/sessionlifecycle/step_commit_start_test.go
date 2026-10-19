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
	internalrepo "github.com/diegobermudez03/playhoot/play/session/workflows/sessionlifecycle/internal/repo"
)

// Identifiers shared by the tests of every Start step (eligibility, commit,
// abort).
const (
	startSessionUUID = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	startCreatorUUID = "0f8fad5b-d9cb-469f-a165-70867728950e"
	startPlayerA     = "aaaaaaaa-aaaa-4aaa-8aaa-000000000001"
	startPlayerB     = "aaaaaaaa-aaaa-4aaa-8aaa-000000000002"
	startVersionUUID = "c1d2e3f4-0000-4000-8000-000000000001"
)

var startNow = time.Date(2026, 10, 17, 12, 0, 0, 123456789, time.UTC)

func TestCommitStartInTx(t *testing.T) {
	type dependencies struct {
		repo *MockcommitStartRepo
	}
	type test struct {
		userUUID string

		expected        sessionpkg.CommitStartOutcome
		expectErr       bool
		expectedErrorIs error
	}

	ctx := context.Background()
	tx := &gorm.DB{}
	nowInDB := startNow.Truncate(time.Microsecond)
	sessionID := uint(31)
	participants := []internalrepo.Participant{{UserUUID: startPlayerA, ParticipantID: "P1"}, {UserUUID: startPlayerB, ParticipantID: "P2"}}
	checkpoint := sessionpkg.Checkpoint{ContextRevision: 1, Content: []byte(`{"round":1}`)}
	creator := startCreatorUUID
	lobby := func() *internalrepo.LockedSession {
		return &internalrepo.LockedSession{ID: sessionID, Status: "LOBBY", CreatedByUserUUID: startCreatorUUID, LobbyExpiresAt: startNow.Add(time.Minute)}
	}

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"session_starts_with_roster_and_checkpoint": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.repo.EXPECT().LockSession(ctx, tx, startSessionUUID).Return(lobby(), nil),
				deps.repo.EXPECT().CreateParticipants(ctx, tx, sessionID, participants).Return(nil),
				deps.repo.EXPECT().SaveCheckpoint(ctx, tx, sessionID, int64(1), []byte(`{"round":1}`), nowInDB).Return(nil),
				deps.repo.EXPECT().ChangeSessionStatus(ctx, tx, sessionID, "LOBBY", "RUNNING", (*string)(nil), &creator, nowInDB).Return(nil),
				deps.repo.EXPECT().RenewOwnerLease(ctx, tx, sessionID, nowInDB.Add(10*time.Minute)).Return(nil),
				deps.repo.EXPECT().ReleaseJoinCodes(ctx, tx, sessionID, nowInDB).Return(nil),
			)
			return test{expected: sessionpkg.CommitStartCommitted}
		},
		"session_that_left_its_lobby_records_nothing": func(t *testing.T, deps dependencies) test {
			cancelled := lobby()
			cancelled.Status = "CANCELLED"
			deps.repo.EXPECT().LockSession(ctx, tx, startSessionUUID).Return(cancelled, nil)
			return test{expected: sessionpkg.CommitStartNotInLobby}
		},
		"lobby_that_expired_meanwhile_records_nothing": func(t *testing.T, deps dependencies) test {
			overdue := lobby()
			overdue.LobbyExpiresAt = nowInDB
			deps.repo.EXPECT().LockSession(ctx, tx, startSessionUUID).Return(overdue, nil)
			return test{expected: sessionpkg.CommitStartNotInLobby}
		},
		"unknown_session": func(t *testing.T, deps dependencies) test {
			deps.repo.EXPECT().LockSession(ctx, tx, startSessionUUID).Return(nil, nil)
			return test{expectErr: true, expectedErrorIs: sessionpkg.ErrSessionNotFound}
		},
		"caller_is_not_the_creator": func(t *testing.T, deps dependencies) test {
			deps.repo.EXPECT().LockSession(ctx, tx, startSessionUUID).Return(lobby(), nil)
			return test{userUUID: startPlayerA, expectErr: true}
		},
		"saving_the_checkpoint_fails": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.repo.EXPECT().LockSession(ctx, tx, startSessionUUID).Return(lobby(), nil),
				deps.repo.EXPECT().CreateParticipants(ctx, tx, sessionID, participants).Return(nil),
				deps.repo.EXPECT().SaveCheckpoint(ctx, tx, sessionID, int64(1), []byte(`{"round":1}`), nowInDB).Return(errors.New("database down")),
			)
			return test{expectErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			deps := dependencies{repo: NewMockcommitStartRepo(ctrl)}
			tc := setup(t, deps)
			if tc.userUUID == "" {
				tc.userUUID = startCreatorUUID
			}
			manager := &Manager{commitStartRepo: deps.repo, now: func() time.Time { return startNow }, ownerLease: 10 * time.Minute}

			outcome, err := manager.commitStartInTx(ctx, tx, startSessionUUID, tc.userUUID, participants, checkpoint)

			if tc.expectErr {
				require.Error(t, err)
				if tc.expectedErrorIs != nil {
					require.ErrorIs(t, err, tc.expectedErrorIs)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, outcome)
		})
	}
}

func TestValidStartCommit(t *testing.T) {
	type test struct {
		commit sessionpkg.StartCommit

		expected  []internalrepo.Participant
		expectErr bool
	}

	checkpoint := sessionpkg.Checkpoint{ContextRevision: 1, Content: []byte(`{}`)}
	tests := map[string]func(t *testing.T) test{
		"roster_is_normalized": func(t *testing.T) test {
			return test{
				commit: sessionpkg.StartCommit{
					Participants: []sessionpkg.StartParticipant{{UserUUID: "AAAAAAAA-AAAA-4AAA-8AAA-000000000001", ParticipantID: "P1"}},
					Checkpoint:   checkpoint,
				},
				expected: []internalrepo.Participant{{UserUUID: startPlayerA, ParticipantID: "P1"}},
			}
		},
		"no_participants": func(t *testing.T) test {
			return test{commit: sessionpkg.StartCommit{Checkpoint: checkpoint}, expectErr: true}
		},
		"no_checkpoint": func(t *testing.T) test {
			return test{commit: sessionpkg.StartCommit{Participants: []sessionpkg.StartParticipant{{UserUUID: startPlayerA, ParticipantID: "P1"}}}, expectErr: true}
		},
		"user_that_is_not_a_uuid": func(t *testing.T) test {
			return test{commit: sessionpkg.StartCommit{Participants: []sessionpkg.StartParticipant{{UserUUID: "nobody", ParticipantID: "P1"}}, Checkpoint: checkpoint}, expectErr: true}
		},
		"participant_without_id": func(t *testing.T) test {
			return test{commit: sessionpkg.StartCommit{Participants: []sessionpkg.StartParticipant{{UserUUID: startPlayerA}}, Checkpoint: checkpoint}, expectErr: true}
		},
		"repeated_user": func(t *testing.T) test {
			return test{commit: sessionpkg.StartCommit{
				Participants: []sessionpkg.StartParticipant{{UserUUID: startPlayerA, ParticipantID: "P1"}, {UserUUID: startPlayerA, ParticipantID: "P2"}},
				Checkpoint:   checkpoint,
			}, expectErr: true}
		},
		"repeated_participant_id": func(t *testing.T) test {
			return test{commit: sessionpkg.StartCommit{
				Participants: []sessionpkg.StartParticipant{{UserUUID: startPlayerA, ParticipantID: "P1"}, {UserUUID: startPlayerB, ParticipantID: "P1"}},
				Checkpoint:   checkpoint,
			}, expectErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)

			participants, err := validStartCommit(tc.commit)

			if tc.expectErr {
				require.ErrorIs(t, err, errInvalidStartCommit)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, participants)
		})
	}
}
