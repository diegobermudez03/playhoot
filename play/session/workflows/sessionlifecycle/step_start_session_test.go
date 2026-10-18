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
	"github.com/diegobermudez03/playhoot/play/session/internal/objectstore"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/workflows/sessionlifecycle/internal/repo"
)

const (
	startSessionUUID = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	startCreatorUUID = "0f8fad5b-d9cb-469f-a165-70867728950e"
	startPlayerA     = "aaaaaaaa-aaaa-4aaa-8aaa-000000000001"
	startPlayerB     = "aaaaaaaa-aaaa-4aaa-8aaa-000000000002"
	startVersionUUID = "c1d2e3f4-0000-4000-8000-000000000001"
)

var startNow = time.Date(2026, 10, 17, 12, 0, 0, 123456789, time.UTC)

func TestCheckStartEligibility(t *testing.T) {
	type dependencies struct {
		repo    *MockcheckStartRepo
		scripts *MockscriptLoader
	}
	type test struct {
		sessionUUID string
		userUUID    string
		playerCount int

		expected  sessionpkg.StartEligibility
		expectErr bool
	}

	ctx := context.Background()
	key, sha, size := "game-versions/backend-scripts/abc", "abc", int64(12)
	locator := objectstore.Locator{ObjectKey: key, SHA256: sha, Size: size}
	candidate := func() *internalrepo.StartCandidate {
		return &internalrepo.StartCandidate{
			Status:            "LOBBY",
			CreatedByUserUUID: startCreatorUUID,
			LobbyExpiresAt:    startNow.Add(time.Minute),
			GameVersionUUID:   startVersionUUID,
			VersionStatus:     "EXECUTABLE",
			Manifest:          []byte(`{"schema_version":1,"players":{"min":2,"max":4}}`),
			ScriptObjectKey:   &key,
			ScriptSHA256:      &sha,
			ScriptSize:        &size,
		}
	}
	finds := func(deps dependencies, found *internalrepo.StartCandidate) {
		deps.repo.EXPECT().FindStartCandidate(ctx, startSessionUUID).Return(found, nil)
	}

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"eligible_with_the_versions_script": func(t *testing.T, deps dependencies) test {
			finds(deps, candidate())
			deps.scripts.EXPECT().Load(ctx, locator).Return([]byte(`function start() {}`), nil)
			return test{
				playerCount: 2,
				expected:    sessionpkg.StartEligibility{Outcome: sessionpkg.StartEligible, GameVersionUUID: startVersionUUID, Script: []byte(`function start() {}`)},
			}
		},
		"creator_uuid_in_another_textual_form_is_the_creator": func(t *testing.T, deps dependencies) test {
			finds(deps, candidate())
			deps.scripts.EXPECT().Load(ctx, locator).Return([]byte(`function start() {}`), nil)
			return test{
				userUUID:    "0F8FAD5B-D9CB-469F-A165-70867728950E",
				playerCount: 4,
				expected:    sessionpkg.StartEligibility{Outcome: sessionpkg.StartEligible, GameVersionUUID: startVersionUUID, Script: []byte(`function start() {}`)},
			}
		},
		"invalid_session_uuid_is_not_found": func(t *testing.T, deps dependencies) test {
			return test{sessionUUID: "not-a-uuid", playerCount: 2, expected: sessionpkg.StartEligibility{Outcome: sessionpkg.StartSessionNotFound}}
		},
		"unknown_session": func(t *testing.T, deps dependencies) test {
			finds(deps, nil)
			return test{playerCount: 2, expected: sessionpkg.StartEligibility{Outcome: sessionpkg.StartSessionNotFound}}
		},
		"caller_is_not_the_creator": func(t *testing.T, deps dependencies) test {
			finds(deps, candidate())
			return test{userUUID: startPlayerA, playerCount: 2, expected: sessionpkg.StartEligibility{Outcome: sessionpkg.StartNotCreator}}
		},
		"running_session": func(t *testing.T, deps dependencies) test {
			running := candidate()
			running.Status = "RUNNING"
			finds(deps, running)
			return test{playerCount: 2, expected: sessionpkg.StartEligibility{Outcome: sessionpkg.StartNotInLobby}}
		},
		"overdue_lobby_has_expired": func(t *testing.T, deps dependencies) test {
			overdue := candidate()
			overdue.LobbyExpiresAt = startNow
			finds(deps, overdue)
			return test{playerCount: 2, expected: sessionpkg.StartEligibility{Outcome: sessionpkg.StartNotInLobby}}
		},
		"fewer_players_than_the_manifest_allows": func(t *testing.T, deps dependencies) test {
			finds(deps, candidate())
			return test{playerCount: 1, expected: sessionpkg.StartEligibility{Outcome: sessionpkg.StartInvalidPlayerCount}}
		},
		"more_players_than_the_manifest_allows": func(t *testing.T, deps dependencies) test {
			finds(deps, candidate())
			return test{playerCount: 5, expected: sessionpkg.StartEligibility{Outcome: sessionpkg.StartInvalidPlayerCount}}
		},
		"version_not_executable_is_broken_data": func(t *testing.T, deps dependencies) test {
			draft := candidate()
			draft.VersionStatus = "DRAFT"
			finds(deps, draft)
			return test{playerCount: 2, expectErr: true}
		},
		"invalid_manifest_is_broken_data": func(t *testing.T, deps dependencies) test {
			broken := candidate()
			broken.Manifest = []byte(`{"schema_version":1}`)
			finds(deps, broken)
			return test{playerCount: 2, expectErr: true}
		},
		"version_without_script_is_broken_data": func(t *testing.T, deps dependencies) test {
			scriptless := candidate()
			scriptless.ScriptObjectKey = nil
			finds(deps, scriptless)
			return test{playerCount: 2, expectErr: true}
		},
		"loading_the_script_fails": func(t *testing.T, deps dependencies) test {
			finds(deps, candidate())
			deps.scripts.EXPECT().Load(ctx, locator).Return(nil, objectstore.ErrHashMismatch)
			return test{playerCount: 2, expectErr: true}
		},
		"finding_the_session_fails": func(t *testing.T, deps dependencies) test {
			deps.repo.EXPECT().FindStartCandidate(ctx, startSessionUUID).Return(nil, errors.New("database down"))
			return test{playerCount: 2, expectErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			deps := dependencies{repo: NewMockcheckStartRepo(ctrl), scripts: NewMockscriptLoader(ctrl)}
			tc := setup(t, deps)
			if tc.sessionUUID == "" {
				tc.sessionUUID = startSessionUUID
			}
			if tc.userUUID == "" {
				tc.userUUID = startCreatorUUID
			}
			manager := &Manager{checkStartRepo: deps.repo, scripts: deps.scripts, now: func() time.Time { return startNow }}

			eligibility, err := manager.CheckStartEligibility(ctx, tc.sessionUUID, tc.userUUID, tc.playerCount)

			if tc.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, eligibility)
		})
	}
}

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
	lobby := func() *internalrepo.LockedSession {
		return &internalrepo.LockedSession{ID: sessionID, Status: "LOBBY", CreatedByUserUUID: startCreatorUUID, LobbyExpiresAt: startNow.Add(time.Minute)}
	}

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"session_starts_with_roster_and_checkpoint": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.repo.EXPECT().LockSession(ctx, tx, startSessionUUID).Return(lobby(), nil),
				deps.repo.EXPECT().CreateParticipants(ctx, tx, sessionID, participants).Return(nil),
				deps.repo.EXPECT().SaveCheckpoint(ctx, tx, sessionID, int64(1), []byte(`{"round":1}`), nowInDB).Return(nil),
				deps.repo.EXPECT().UpdateSessionStatus(ctx, tx, sessionID, "RUNNING", (*string)(nil), nowInDB).Return(nil),
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
			manager := &Manager{commitStartRepo: deps.repo, now: func() time.Time { return startNow }}

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

func TestAbortStartInTx(t *testing.T) {
	type dependencies struct {
		repo *MockabortStartRepo
	}
	type test struct {
		expected        bool
		expectErr       bool
		expectedErrorIs error
	}

	ctx := context.Background()
	tx := &gorm.DB{}
	nowInDB := startNow.Truncate(time.Microsecond)
	sessionID := uint(31)
	cause := "GAME_START_FAILURE"

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"lobby_is_aborted_by_the_start_failure": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.repo.EXPECT().LockSession(ctx, tx, startSessionUUID).Return(&internalrepo.LockedSession{ID: sessionID, Status: "LOBBY"}, nil),
				deps.repo.EXPECT().UpdateSessionStatus(ctx, tx, sessionID, "ABORTED", &cause, nowInDB).Return(nil),
				deps.repo.EXPECT().ReleaseJoinCodes(ctx, tx, sessionID, nowInDB).Return(nil),
			)
			return test{expected: true}
		},
		"session_no_longer_in_its_lobby_is_left_as_it_is": func(t *testing.T, deps dependencies) test {
			deps.repo.EXPECT().LockSession(ctx, tx, startSessionUUID).Return(&internalrepo.LockedSession{ID: sessionID, Status: "EXPIRED"}, nil)
			return test{expected: false}
		},
		"unknown_session": func(t *testing.T, deps dependencies) test {
			deps.repo.EXPECT().LockSession(ctx, tx, startSessionUUID).Return(nil, nil)
			return test{expectErr: true, expectedErrorIs: sessionpkg.ErrSessionNotFound}
		},
		"updating_the_status_fails": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.repo.EXPECT().LockSession(ctx, tx, startSessionUUID).Return(&internalrepo.LockedSession{ID: sessionID, Status: "LOBBY"}, nil),
				deps.repo.EXPECT().UpdateSessionStatus(ctx, tx, sessionID, "ABORTED", &cause, nowInDB).Return(errors.New("database down")),
			)
			return test{expectErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			deps := dependencies{repo: NewMockabortStartRepo(ctrl)}
			tc := setup(t, deps)
			manager := &Manager{abortStartRepo: deps.repo, now: func() time.Time { return startNow }}

			aborted, err := manager.abortStartInTx(ctx, tx, startSessionUUID)

			if tc.expectErr {
				require.Error(t, err)
				if tc.expectedErrorIs != nil {
					require.ErrorIs(t, err, tc.expectedErrorIs)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, aborted)
		})
	}
}
