package liveconnection

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/play/runtime"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

func TestAttachPlayer(t *testing.T) {
	type dependencies struct {
		players   *MockplayerJoinChecker
		manifests *MockmanifestLoader
	}
	type test struct {
		// before runs against the Manager before the attach under test.
		before     func(manager *Manager)
		ownerEpoch int64

		expectedOutcome      runtime.AttachOutcome
		expectedRejection    runtime.RejectReason
		expectedErr          bool
		expectedPlayersAfter int
	}

	const (
		sessionUUID = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
		playerUUID  = "0f8fad5b-d9cb-469f-a165-70867728950e"
		otherUUID   = "aaaaaaaa-aaaa-4aaa-8aaa-000000000002"
	)
	ctx := context.Background()
	loadsManifest := func(deps dependencies, maxPlayers int) {
		deps.manifests.EXPECT().LoadRuntimeManifest(ctx, sessionUUID).Return(sessionpkg.RuntimeManifest{MinPlayers: 1, MaxPlayers: maxPlayers}, nil)
	}

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"first_player_creates_the_controller_and_joins_the_lobby": func(t *testing.T, deps dependencies) test {
			deps.players.EXPECT().EligibleToJoinAsPlayer(ctx, sessionUUID).Return(sessionpkg.PlayerJoinEligible, nil)
			loadsManifest(deps, 8)
			return test{
				ownerEpoch:           1,
				expectedOutcome:      runtime.AttachAccepted,
				expectedPlayersAfter: 1,
			}
		},
		"full_lobby_rejects": func(t *testing.T, deps dependencies) test {
			deps.players.EXPECT().EligibleToJoinAsPlayer(ctx, sessionUUID).Return(sessionpkg.PlayerJoinEligible, nil).Times(2)
			loadsManifest(deps, 1)
			return test{
				before: func(manager *Manager) {
					result, err := manager.AttachPlayer(ctx, sessionUUID, otherUUID, 1)
					require.NoError(t, err)
					require.Equal(t, runtime.AttachAccepted, result.Outcome)
				},
				ownerEpoch:           1,
				expectedOutcome:      runtime.AttachRejected,
				expectedRejection:    runtime.RejectLobbyFull,
				expectedPlayersAfter: 1,
			}
		},
		"request_under_an_older_epoch_rejects_as_owner_changed": func(t *testing.T, deps dependencies) test {
			deps.players.EXPECT().EligibleToJoinAsPlayer(ctx, sessionUUID).Return(sessionpkg.PlayerJoinEligible, nil).Times(2)
			loadsManifest(deps, 8)
			return test{
				before: func(manager *Manager) {
					_, err := manager.AttachPlayer(ctx, sessionUUID, otherUUID, 2)
					require.NoError(t, err)
				},
				ownerEpoch:        1,
				expectedOutcome:   runtime.AttachRejected,
				expectedRejection: runtime.RejectOwnerChanged,
			}
		},
		"session_started": func(t *testing.T, deps dependencies) test {
			deps.players.EXPECT().EligibleToJoinAsPlayer(ctx, sessionUUID).Return(sessionpkg.PlayerJoinSessionStarted, nil)
			return test{
				ownerEpoch:        1,
				expectedOutcome:   runtime.AttachRejected,
				expectedRejection: runtime.RejectSessionStarted,
			}
		},
		"session_not_found": func(t *testing.T, deps dependencies) test {
			deps.players.EXPECT().EligibleToJoinAsPlayer(ctx, sessionUUID).Return(sessionpkg.PlayerJoinSessionNotFound, nil)
			return test{
				ownerEpoch:        1,
				expectedOutcome:   runtime.AttachRejected,
				expectedRejection: runtime.RejectSessionUnavailable,
			}
		},
		"session_not_joinable": func(t *testing.T, deps dependencies) test {
			deps.players.EXPECT().EligibleToJoinAsPlayer(ctx, sessionUUID).Return(sessionpkg.PlayerJoinSessionNotJoinable, nil)
			return test{
				ownerEpoch:        1,
				expectedOutcome:   runtime.AttachRejected,
				expectedRejection: runtime.RejectSessionUnavailable,
			}
		},
		"loading_the_manifest_fails": func(t *testing.T, deps dependencies) test {
			deps.players.EXPECT().EligibleToJoinAsPlayer(ctx, sessionUUID).Return(sessionpkg.PlayerJoinEligible, nil)
			deps.manifests.EXPECT().LoadRuntimeManifest(ctx, sessionUUID).Return(sessionpkg.RuntimeManifest{}, errors.New("database down"))
			return test{ownerEpoch: 1, expectedErr: true}
		},
		"eligibility_check_fails": func(t *testing.T, deps dependencies) test {
			deps.players.EXPECT().EligibleToJoinAsPlayer(ctx, sessionUUID).Return(sessionpkg.PlayerJoinEligibility(""), errors.New("database down"))
			return test{ownerEpoch: 1, expectedErr: true}
		},
		"unknown_eligibility": func(t *testing.T, deps dependencies) test {
			deps.players.EXPECT().EligibleToJoinAsPlayer(ctx, sessionUUID).Return(sessionpkg.PlayerJoinEligibility("something_new"), nil)
			return test{ownerEpoch: 1, expectedErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			deps := dependencies{players: NewMockplayerJoinChecker(ctrl), manifests: NewMockmanifestLoader(ctrl)}
			tc := setup(t, deps)
			manager := New(nil, deps.players, deps.manifests)
			if tc.before != nil {
				tc.before(manager)
			}

			result, err := manager.AttachPlayer(ctx, sessionUUID, playerUUID, tc.ownerEpoch)

			if tc.expectedErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expectedOutcome, result.Outcome)
			require.Equal(t, tc.expectedRejection, result.Rejection)
			if tc.expectedOutcome == runtime.AttachAccepted {
				require.NotNil(t, result.Connection.Disconnect)
			}
			if tc.expectedPlayersAfter > 0 {
				controller, _, err := manager.controllerFor(ctx, sessionUUID, 1)
				require.NoError(t, err)
				require.Equal(t, tc.expectedPlayersAfter, controller.PlayerCount())
			}
		})
	}
}
