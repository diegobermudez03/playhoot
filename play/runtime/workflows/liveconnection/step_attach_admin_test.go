package liveconnection

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/play/runtime"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

func TestAttachAdmin(t *testing.T) {
	type dependencies struct {
		sessions  *MockadminJoinChecker
		manifests *MockmanifestLoader
	}
	type test struct {
		// before runs against the Manager before the attach under test.
		before     func(resolver *ControllerResolver)
		ownerEpoch int64

		expectedOutcome     runtime.AttachOutcome
		expectedRejection   runtime.RejectReason
		expectedErr         bool
		expectedAdminsAfter int
	}

	const (
		sessionUUID = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
		creatorUUID = "0f8fad5b-d9cb-469f-a165-70867728950e"
	)
	ctx := context.Background()
	loadsManifest := func(deps dependencies) {
		deps.manifests.EXPECT().LoadRuntimeManifest(ctx, sessionUUID).Return(sessionpkg.RuntimeManifest{MinPlayers: 1, MaxPlayers: 8}, nil)
	}

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"first_admin_creates_the_controller": func(t *testing.T, deps dependencies) test {
			deps.sessions.EXPECT().EligibleToJoinAsAdmin(ctx, sessionUUID, creatorUUID).Return(sessionpkg.AdminJoinEligible, nil)
			loadsManifest(deps)
			return test{
				ownerEpoch:          1,
				expectedOutcome:     runtime.AttachAccepted,
				expectedAdminsAfter: 1,
			}
		},
		"admins_are_not_checked_for_a_device": func(t *testing.T, deps dependencies) test {
			deps.sessions.EXPECT().EligibleToJoinAsAdmin(ctx, sessionUUID, creatorUUID).Return(sessionpkg.AdminJoinEligible, nil)
			deps.manifests.EXPECT().LoadRuntimeManifest(ctx, sessionUUID).Return(sessionpkg.RuntimeManifest{MinPlayers: 1, MaxPlayers: 8,
				InputRequirements: sessionpkg.RuntimeInputRequirements{Devices: "mobile", UsesPointerPress: true}}, nil)
			return test{
				ownerEpoch:          1,
				expectedOutcome:     runtime.AttachAccepted,
				expectedAdminsAfter: 1,
			}
		},
		"another_admin_joins_the_existing_controller": func(t *testing.T, deps dependencies) test {
			deps.sessions.EXPECT().EligibleToJoinAsAdmin(ctx, sessionUUID, creatorUUID).Return(sessionpkg.AdminJoinEligible, nil).Times(2)
			// Only the controller's creation loads it.
			loadsManifest(deps)
			return test{
				before: func(resolver *ControllerResolver) {
					_, err := resolver.AttachAdmin(ctx, sessionUUID, creatorUUID, 1)
					require.NoError(t, err)
				},
				ownerEpoch:          1,
				expectedOutcome:     runtime.AttachAccepted,
				expectedAdminsAfter: 2,
			}
		},
		"request_under_an_older_epoch_rejects_as_owner_changed": func(t *testing.T, deps dependencies) test {
			deps.sessions.EXPECT().EligibleToJoinAsAdmin(ctx, sessionUUID, creatorUUID).Return(sessionpkg.AdminJoinEligible, nil)
			loadsManifest(deps)
			return test{
				before: func(resolver *ControllerResolver) {
					_, err := resolver.AttachAdmin(ctx, sessionUUID, creatorUUID, 2)
					require.NoError(t, err)
				},
				ownerEpoch:        1,
				expectedOutcome:   runtime.AttachRejected,
				expectedRejection: runtime.RejectOwnerChanged,
			}
		},
		"not_the_creator": func(t *testing.T, deps dependencies) test {
			deps.sessions.EXPECT().EligibleToJoinAsAdmin(ctx, sessionUUID, creatorUUID).Return(sessionpkg.AdminJoinNotCreator, nil)
			return test{
				ownerEpoch:        1,
				expectedOutcome:   runtime.AttachRejected,
				expectedRejection: runtime.RejectNotCreator,
			}
		},
		"session_not_found": func(t *testing.T, deps dependencies) test {
			deps.sessions.EXPECT().EligibleToJoinAsAdmin(ctx, sessionUUID, creatorUUID).Return(sessionpkg.AdminJoinSessionNotFound, nil)
			return test{
				ownerEpoch:        1,
				expectedOutcome:   runtime.AttachRejected,
				expectedRejection: runtime.RejectSessionUnavailable,
			}
		},
		"session_already_ended": func(t *testing.T, deps dependencies) test {
			deps.sessions.EXPECT().EligibleToJoinAsAdmin(ctx, sessionUUID, creatorUUID).Return(sessionpkg.AdminJoinSessionNotJoinable, nil)
			return test{
				ownerEpoch:        1,
				expectedOutcome:   runtime.AttachRejected,
				expectedRejection: runtime.RejectSessionUnavailable,
			}
		},
		"loading_the_manifest_fails": func(t *testing.T, deps dependencies) test {
			deps.sessions.EXPECT().EligibleToJoinAsAdmin(ctx, sessionUUID, creatorUUID).Return(sessionpkg.AdminJoinEligible, nil)
			deps.manifests.EXPECT().LoadRuntimeManifest(ctx, sessionUUID).Return(sessionpkg.RuntimeManifest{}, errors.New("database down"))
			return test{ownerEpoch: 1, expectedErr: true}
		},
		"eligibility_check_fails": func(t *testing.T, deps dependencies) test {
			deps.sessions.EXPECT().EligibleToJoinAsAdmin(ctx, sessionUUID, creatorUUID).Return(sessionpkg.AdminJoinEligibility(""), errors.New("database down"))
			return test{ownerEpoch: 1, expectedErr: true}
		},
		"unknown_eligibility": func(t *testing.T, deps dependencies) test {
			deps.sessions.EXPECT().EligibleToJoinAsAdmin(ctx, sessionUUID, creatorUUID).Return(sessionpkg.AdminJoinEligibility("something_new"), nil)
			return test{ownerEpoch: 1, expectedErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			deps := dependencies{sessions: NewMockadminJoinChecker(ctrl), manifests: NewMockmanifestLoader(ctrl)}
			tc := setup(t, deps)
			resolver := New(0, deps.sessions, nil, deps.manifests, nil, nil, nil, nil, nil, nil, nil, nil, ownersReleasing(t, sessionUUID, 1, 2), nil, nil)
			t.Cleanup(resolver.Close)
			if tc.before != nil {
				tc.before(resolver)
			}

			result, err := resolver.AttachAdmin(ctx, sessionUUID, creatorUUID, tc.ownerEpoch)

			require.Contains(t, resolver.controllers, sessionUUID, "the controller exists whatever the answer")

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
			if tc.expectedAdminsAfter > 0 {
				controller, _ := resolver.controllerFor(ctx, sessionUUID, 1)
				require.Equal(t, tc.expectedAdminsAfter, controller.AdminCount())
			}
		})
	}
}

func TestAdminConnection(t *testing.T) {
	ctrl := gomock.NewController(t)
	starts, ends := NewMocksessionStarter(ctrl), NewMocksessionEnder(ctrl)
	deps := testDependencies()
	deps.starts, deps.ends, deps.owners = starts, ends, ownersReleasing(t, "session-1", 1)
	root, stop := context.WithCancel(context.Background())
	defer stop()
	controller := withManifest(newSessionController(root, "session-1", 1, deps), Manifest{MinPlayers: 2, MaxPlayers: 8})
	go controller.run()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	player, _, ok := controller.joinLobby(ctx, runtime.Player{UserUUID: "player-1"})
	require.True(t, ok)

	first := controller.mustRegisterAdmin("creator")
	second := controller.mustRegisterAdmin("creator")
	require.Equal(t, 2, controller.AdminCount())

	update, more := first.NextLobbyUpdate(ctx)
	require.True(t, more)
	require.Equal(t, runtime.LobbyUpdate{Members: []runtime.LobbyMember{{UserUUID: "player-1"}}, MinPlayers: 2, MaxPlayers: 8}, update, "the first update is the lobby as it is, with the game's player limits")

	selected, err := first.SelectPerspective(ctx, "P1")
	require.NoError(t, err)
	require.Equal(t, runtime.SelectPerspectiveResult{Outcome: runtime.PerspectiveNotRunning}, selected, "selecting runs the controller's selection")
	require.NoError(t, first.Resync(ctx))
	starts.EXPECT().CheckStartEligibility(ctx, "session-1", "creator", 1).Return(sessionpkg.StartEligibility{Outcome: sessionpkg.StartInvalidPlayerCount}, nil)
	started, err := first.StartSession(ctx)
	require.NoError(t, err)
	require.Equal(t, runtime.StartSessionResult{Outcome: runtime.StartInvalidPlayerCount}, started, "starting runs the controller's Start")

	first.Disconnect()
	first.Disconnect()
	require.Equal(t, 1, controller.AdminCount())
	end, ended := first.Ended(ctx)
	require.True(t, ended)
	require.Equal(t, runtime.ConnectionEnd{Reason: runtime.EndDisconnected}, end)
	_, more = first.NextView(ctx)
	require.False(t, more)
	_, more = first.NextLobbyUpdate(ctx)
	require.False(t, more)
	_, more = first.Started(ctx)
	require.False(t, more, "an ended connection never sees a start")

	waiting, cancelWait := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancelWait()
	_, ended = second.Ended(waiting)
	require.False(t, ended, "the other admin connection must stay attached")

	ends.EXPECT().EndSession(ctx, "session-1", sessionpkg.SessionEnd{Cause: sessionpkg.EndHostCancelled, ChangedBy: "creator"}).
		Return(sessionpkg.EndSessionResult{Outcome: sessionpkg.SessionEndedNow, Status: "CANCELLED", EndCause: "HOST_CANCELLED"}, nil)
	cancelled, err := second.CancelSession(ctx)
	require.NoError(t, err)
	require.Equal(t, runtime.CancelSessionResult{Outcome: runtime.SessionCancelled}, cancelled, "cancelling runs the controller's cancellation")
	require.Equal(t, sessionEnded("CANCELLED", "HOST_CANCELLED"), connectionEnd(t, second.Ended))
	require.Equal(t, sessionEnded("CANCELLED", "HOST_CANCELLED"), connectionEnd(t, player.Ended))
}
