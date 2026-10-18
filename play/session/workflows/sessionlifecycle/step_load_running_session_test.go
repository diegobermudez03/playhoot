package sessionlifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/objectstore"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/workflows/sessionlifecycle/internal/repo"
)

func TestLoadRunningSession(t *testing.T) {
	type dependencies struct {
		repo    *MockloadRunningSessionRepo
		scripts *MockscriptLoader
	}
	type test struct {
		sessionUUID string

		expected        sessionpkg.RunningSession
		expectErr       bool
		expectedErrorIs error
	}

	ctx := context.Background()
	key, sha, size := "game-versions/backend-scripts/abc", "abc", int64(12)
	revision := int64(1)
	sessionID := uint(31)
	record := func() *internalrepo.RunningSessionRecord {
		return &internalrepo.RunningSessionRecord{
			ID: sessionID, Status: "RUNNING", GameVersionUUID: startVersionUUID,
			ScriptObjectKey: &key, ScriptSHA256: &sha, ScriptSize: &size,
			CheckpointRevision: &revision, CheckpointContent: []byte(`{"schema_version":1}`),
		}
	}
	roster := []internalrepo.Participant{{UserUUID: startPlayerA, ParticipantID: "P1"}, {UserUUID: startPlayerB, ParticipantID: "P2"}}

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"running_session_with_its_script_roster_and_checkpoint": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.repo.EXPECT().FindRunningSession(ctx, startSessionUUID).Return(record(), nil),
				deps.repo.EXPECT().FindParticipants(ctx, sessionID).Return(roster, nil),
				deps.scripts.EXPECT().Load(ctx, objectstore.Locator{ObjectKey: key, SHA256: sha, Size: size}).Return([]byte(`function start() {}`), nil),
			)
			return test{expected: sessionpkg.RunningSession{
				GameVersionUUID: startVersionUUID,
				Script:          []byte(`function start() {}`),
				Participants:    []sessionpkg.StartParticipant{{UserUUID: startPlayerA, ParticipantID: "P1"}, {UserUUID: startPlayerB, ParticipantID: "P2"}},
				Checkpoint:      sessionpkg.Checkpoint{ContextRevision: 1, Content: []byte(`{"schema_version":1}`)},
			}}
		},
		"invalid_session_uuid": func(t *testing.T, deps dependencies) test {
			return test{sessionUUID: "nope", expectErr: true, expectedErrorIs: sessionpkg.ErrSessionNotFound}
		},
		"unknown_session": func(t *testing.T, deps dependencies) test {
			deps.repo.EXPECT().FindRunningSession(ctx, startSessionUUID).Return(nil, nil)
			return test{expectErr: true, expectedErrorIs: sessionpkg.ErrSessionNotFound}
		},
		"session_not_running": func(t *testing.T, deps dependencies) test {
			lobby := record()
			lobby.Status = "LOBBY"
			deps.repo.EXPECT().FindRunningSession(ctx, startSessionUUID).Return(lobby, nil)
			return test{expectErr: true, expectedErrorIs: sessionpkg.ErrSessionNotRunning}
		},
		"running_without_checkpoint_is_broken_data": func(t *testing.T, deps dependencies) test {
			broken := record()
			broken.CheckpointRevision, broken.CheckpointContent = nil, nil
			deps.repo.EXPECT().FindRunningSession(ctx, startSessionUUID).Return(broken, nil)
			return test{expectErr: true}
		},
		"running_without_participants_is_broken_data": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.repo.EXPECT().FindRunningSession(ctx, startSessionUUID).Return(record(), nil),
				deps.repo.EXPECT().FindParticipants(ctx, sessionID).Return(nil, nil),
			)
			return test{expectErr: true}
		},
		"loading_the_script_fails": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.repo.EXPECT().FindRunningSession(ctx, startSessionUUID).Return(record(), nil),
				deps.repo.EXPECT().FindParticipants(ctx, sessionID).Return(roster, nil),
				deps.scripts.EXPECT().Load(ctx, objectstore.Locator{ObjectKey: key, SHA256: sha, Size: size}).Return(nil, objectstore.ErrNotConfigured),
			)
			return test{expectErr: true}
		},
		"finding_the_session_fails": func(t *testing.T, deps dependencies) test {
			deps.repo.EXPECT().FindRunningSession(ctx, startSessionUUID).Return(nil, errors.New("database down"))
			return test{expectErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			deps := dependencies{repo: NewMockloadRunningSessionRepo(ctrl), scripts: NewMockscriptLoader(ctrl)}
			tc := setup(t, deps)
			if tc.sessionUUID == "" {
				tc.sessionUUID = startSessionUUID
			}
			manager := &Manager{loadRunningSessionRepo: deps.repo, scripts: deps.scripts}

			running, err := manager.LoadRunningSession(ctx, tc.sessionUUID)

			if tc.expectErr {
				require.Error(t, err)
				if tc.expectedErrorIs != nil {
					require.ErrorIs(t, err, tc.expectedErrorIs)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, running)
		})
	}
}
