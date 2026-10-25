package liveconnection

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/play/internal/business/inputrequirements"
	"github.com/diegobermudez03/playhoot/play/runtime"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

func TestAttachPlayer(t *testing.T) {
	type dependencies struct {
		players   *MockplayerJoinChecker
		manifests *MockmanifestLoader
		limits    *MocklimitsLoader
		starts    *MocksessionStarter
	}
	type test struct {
		// before runs against the Manager before the attach under test.
		before     func(resolver *ControllerResolver)
		ownerEpoch int64
		device     runtime.Device

		expectedOutcome      runtime.AttachOutcome
		expectedRejection    runtime.RejectReason
		expectedRequirements runtime.InputRequirements
		expectedErr          bool
		expectedPlayersAfter int
	}

	const (
		sessionUUID = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
		playerUUID  = "0f8fad5b-d9cb-469f-a165-70867728950e"
		otherUUID   = "aaaaaaaa-aaaa-4aaa-8aaa-000000000002"
	)
	ctx := context.Background()
	anyDevice := sessionpkg.RuntimeInputRequirements{Devices: "both", UsesPointerPress: true}
	desktopOnly := sessionpkg.RuntimeInputRequirements{Devices: "desktop", UsesKeyboard: true, UsesPointerPress: true}
	loadsManifest := func(deps dependencies, maxPlayers int, requirements sessionpkg.RuntimeInputRequirements) {
		deps.manifests.EXPECT().LoadRuntimeManifest(ctx, sessionUUID).
			Return(sessionpkg.RuntimeManifest{MinPlayers: 1, MaxPlayers: maxPlayers, InputRequirements: requirements}, nil)
	}
	// eligible makes the Session admit players, checked before the device.
	eligible := func(deps dependencies) *gomock.Call {
		return deps.players.EXPECT().EligibleToJoinAsPlayer(ctx, sessionUUID).Return(sessionpkg.PlayerJoinEligible, nil)
	}

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"first_player_creates_the_controller_and_joins_the_lobby": func(t *testing.T, deps dependencies) test {
			loadsManifest(deps, 8, anyDevice)
			deps.players.EXPECT().EligibleToJoinAsPlayer(ctx, sessionUUID).Return(sessionpkg.PlayerJoinEligible, nil)
			return test{
				ownerEpoch:           1,
				device:               runtime.DeviceDesktop,
				expectedOutcome:      runtime.AttachAccepted,
				expectedPlayersAfter: 1,
			}
		},
		"mobile_joins_a_game_for_both": func(t *testing.T, deps dependencies) test {
			loadsManifest(deps, 8, anyDevice)
			deps.players.EXPECT().EligibleToJoinAsPlayer(ctx, sessionUUID).Return(sessionpkg.PlayerJoinEligible, nil)
			return test{
				ownerEpoch:           1,
				device:               runtime.DeviceMobile,
				expectedOutcome:      runtime.AttachAccepted,
				expectedPlayersAfter: 1,
			}
		},
		"desktop_joins_a_desktop_game": func(t *testing.T, deps dependencies) test {
			loadsManifest(deps, 8, desktopOnly)
			deps.players.EXPECT().EligibleToJoinAsPlayer(ctx, sessionUUID).Return(sessionpkg.PlayerJoinEligible, nil)
			return test{
				ownerEpoch:           1,
				device:               runtime.DeviceDesktop,
				expectedOutcome:      runtime.AttachAccepted,
				expectedPlayersAfter: 1,
			}
		},
		"mobile_is_rejected_from_a_desktop_game_with_its_requirements": func(t *testing.T, deps dependencies) test {
			eligible(deps)
			loadsManifest(deps, 8, desktopOnly)
			return test{
				ownerEpoch:           1,
				device:               runtime.DeviceMobile,
				expectedOutcome:      runtime.AttachRejected,
				expectedRejection:    runtime.RejectDeviceNotSupported,
				expectedRequirements: runtime.InputRequirements{Devices: "desktop", UsesKeyboard: true, UsesPointerPress: true},
			}
		},
		"desktop_is_rejected_from_a_mobile_game": func(t *testing.T, deps dependencies) test {
			eligible(deps)
			loadsManifest(deps, 8, sessionpkg.RuntimeInputRequirements{Devices: "mobile", UsesPointerPress: true})
			return test{
				ownerEpoch:           1,
				device:               runtime.DeviceDesktop,
				expectedOutcome:      runtime.AttachRejected,
				expectedRejection:    runtime.RejectDeviceNotSupported,
				expectedRequirements: runtime.InputRequirements{Devices: "mobile", UsesPointerPress: true},
			}
		},
		"missing_device_is_unsupported": func(t *testing.T, deps dependencies) test {
			eligible(deps)
			loadsManifest(deps, 8, anyDevice)
			return test{
				ownerEpoch:           1,
				expectedOutcome:      runtime.AttachRejected,
				expectedRejection:    runtime.RejectDeviceNotSupported,
				expectedRequirements: runtime.InputRequirements{Devices: "both", UsesPointerPress: true},
			}
		},
		"unknown_device_is_unsupported": func(t *testing.T, deps dependencies) test {
			eligible(deps)
			loadsManifest(deps, 8, anyDevice)
			return test{
				ownerEpoch:           1,
				device:               runtime.Device("tablet"),
				expectedOutcome:      runtime.AttachRejected,
				expectedRejection:    runtime.RejectDeviceNotSupported,
				expectedRequirements: runtime.InputRequirements{Devices: "both", UsesPointerPress: true},
			}
		},
		"unknown_devices_of_the_game_are_broken_data": func(t *testing.T, deps dependencies) test {
			eligible(deps)
			loadsManifest(deps, 8, sessionpkg.RuntimeInputRequirements{Devices: "tv"})
			return test{ownerEpoch: 1, device: runtime.DeviceDesktop, expectedErr: true}
		},
		"full_lobby_rejects": func(t *testing.T, deps dependencies) test {
			loadsManifest(deps, 1, anyDevice)
			deps.players.EXPECT().EligibleToJoinAsPlayer(ctx, sessionUUID).Return(sessionpkg.PlayerJoinEligible, nil).Times(2)
			return test{
				before: func(resolver *ControllerResolver) {
					result, err := resolver.AttachPlayer(ctx, sessionUUID, runtime.Player{UserUUID: otherUUID, Device: runtime.DeviceDesktop}, 1)
					require.NoError(t, err)
					require.Equal(t, runtime.AttachAccepted, result.Outcome)
				},
				ownerEpoch:           1,
				device:               runtime.DeviceDesktop,
				expectedOutcome:      runtime.AttachRejected,
				expectedRejection:    runtime.RejectLobbyFull,
				expectedPlayersAfter: 1,
			}
		},
		"same_user_on_an_unsupported_device_keeps_the_older_connection": func(t *testing.T, deps dependencies) test {
			loadsManifest(deps, 8, desktopOnly)
			eligible(deps).Times(2)
			return test{
				before: func(resolver *ControllerResolver) {
					result, err := resolver.AttachPlayer(ctx, sessionUUID, runtime.Player{UserUUID: playerUUID, Device: runtime.DeviceDesktop}, 1)
					require.NoError(t, err)
					require.Equal(t, runtime.AttachAccepted, result.Outcome)
				},
				ownerEpoch:           1,
				device:               runtime.DeviceMobile,
				expectedOutcome:      runtime.AttachRejected,
				expectedRejection:    runtime.RejectDeviceNotSupported,
				expectedRequirements: runtime.InputRequirements{Devices: "desktop", UsesKeyboard: true, UsesPointerPress: true},
				expectedPlayersAfter: 1,
			}
		},
		"request_under_an_older_epoch_rejects_as_owner_changed": func(t *testing.T, deps dependencies) test {
			loadsManifest(deps, 8, anyDevice)
			deps.players.EXPECT().EligibleToJoinAsPlayer(ctx, sessionUUID).Return(sessionpkg.PlayerJoinEligible, nil)
			return test{
				before: func(resolver *ControllerResolver) {
					_, err := resolver.AttachPlayer(ctx, sessionUUID, runtime.Player{UserUUID: otherUUID, Device: runtime.DeviceDesktop}, 2)
					require.NoError(t, err)
				},
				ownerEpoch:        1,
				device:            runtime.DeviceDesktop,
				expectedOutcome:   runtime.AttachRejected,
				expectedRejection: runtime.RejectOwnerChanged,
			}
		},
		"session_that_stopped_running_while_recovering_rejects_as_started": func(t *testing.T, deps dependencies) test {
			anyCtx := gomock.Any()
			gomock.InOrder(
				deps.players.EXPECT().EligibleToJoinAsPlayer(ctx, sessionUUID).Return(sessionpkg.PlayerJoinSessionStarted, nil),
				deps.manifests.EXPECT().LoadRuntimeManifest(ctx, sessionUUID).
					Return(sessionpkg.RuntimeManifest{MinPlayers: 1, MaxPlayers: 8, InputRequirements: anyDevice}, nil),
				deps.limits.EXPECT().LoadSessionLimits(anyCtx, sessionUUID).Return(sessionpkg.SessionLimits{ExpiresAt: time.Now().Add(time.Hour)}, nil),
				deps.starts.EXPECT().LoadRunningSession(anyCtx, sessionUUID).Return(sessionpkg.RunningSession{}, sessionpkg.ErrSessionNotRunning),
			)
			return test{
				ownerEpoch:        1,
				device:            runtime.DeviceDesktop,
				expectedOutcome:   runtime.AttachRejected,
				expectedRejection: runtime.RejectSessionStarted,
			}
		},
		"session_not_found": func(t *testing.T, deps dependencies) test {
			deps.players.EXPECT().EligibleToJoinAsPlayer(ctx, sessionUUID).Return(sessionpkg.PlayerJoinSessionNotFound, nil)
			return test{
				ownerEpoch:        1,
				device:            runtime.DeviceDesktop,
				expectedOutcome:   runtime.AttachRejected,
				expectedRejection: runtime.RejectSessionUnavailable,
			}
		},
		"session_not_joinable": func(t *testing.T, deps dependencies) test {
			deps.players.EXPECT().EligibleToJoinAsPlayer(ctx, sessionUUID).Return(sessionpkg.PlayerJoinSessionNotJoinable, nil)
			return test{
				ownerEpoch:        1,
				device:            runtime.DeviceDesktop,
				expectedOutcome:   runtime.AttachRejected,
				expectedRejection: runtime.RejectSessionUnavailable,
			}
		},
		"ended_session_on_an_unsupported_device_rejects_as_unavailable": func(t *testing.T, deps dependencies) test {
			// The device is never checked: the manifest is not even read.
			deps.players.EXPECT().EligibleToJoinAsPlayer(ctx, sessionUUID).Return(sessionpkg.PlayerJoinSessionNotJoinable, nil)
			return test{
				ownerEpoch:        1,
				device:            runtime.Device("tablet"),
				expectedOutcome:   runtime.AttachRejected,
				expectedRejection: runtime.RejectSessionUnavailable,
			}
		},
		"unknown_session_without_a_device_rejects_as_unavailable": func(t *testing.T, deps dependencies) test {
			deps.players.EXPECT().EligibleToJoinAsPlayer(ctx, sessionUUID).Return(sessionpkg.PlayerJoinSessionNotFound, nil)
			return test{
				ownerEpoch:        1,
				expectedOutcome:   runtime.AttachRejected,
				expectedRejection: runtime.RejectSessionUnavailable,
			}
		},
		"running_session_rejects_a_non_participant_as_started_whatever_the_device": func(t *testing.T, deps dependencies) test {
			anyCtx := gomock.Any()
			gomock.InOrder(
				deps.players.EXPECT().EligibleToJoinAsPlayer(ctx, sessionUUID).Return(sessionpkg.PlayerJoinSessionStarted, nil),
				deps.manifests.EXPECT().LoadRuntimeManifest(ctx, sessionUUID).
					Return(sessionpkg.RuntimeManifest{MinPlayers: 1, MaxPlayers: 8, InputRequirements: desktopOnly}, nil),
				deps.limits.EXPECT().LoadSessionLimits(anyCtx, sessionUUID).Return(sessionpkg.SessionLimits{ExpiresAt: time.Now().Add(time.Hour)}, nil),
				deps.starts.EXPECT().LoadRunningSession(anyCtx, sessionUUID).Return(sessionpkg.RunningSession{}, sessionpkg.ErrSessionNotRunning),
			)
			return test{
				ownerEpoch:        1,
				device:            runtime.DeviceMobile,
				expectedOutcome:   runtime.AttachRejected,
				expectedRejection: runtime.RejectSessionStarted,
			}
		},
		"loading_the_manifest_fails": func(t *testing.T, deps dependencies) test {
			eligible(deps)
			deps.manifests.EXPECT().LoadRuntimeManifest(ctx, sessionUUID).Return(sessionpkg.RuntimeManifest{}, errors.New("database down"))
			return test{ownerEpoch: 1, device: runtime.DeviceDesktop, expectedErr: true}
		},
		"eligibility_check_fails": func(t *testing.T, deps dependencies) test {
			deps.players.EXPECT().EligibleToJoinAsPlayer(ctx, sessionUUID).Return(sessionpkg.PlayerJoinEligibility(""), errors.New("database down"))
			return test{ownerEpoch: 1, device: runtime.DeviceDesktop, expectedErr: true}
		},
		"unknown_eligibility": func(t *testing.T, deps dependencies) test {
			deps.players.EXPECT().EligibleToJoinAsPlayer(ctx, sessionUUID).Return(sessionpkg.PlayerJoinEligibility("something_new"), nil)
			return test{ownerEpoch: 1, device: runtime.DeviceDesktop, expectedErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			deps := dependencies{players: NewMockplayerJoinChecker(ctrl), manifests: NewMockmanifestLoader(ctrl), limits: NewMocklimitsLoader(ctrl), starts: NewMocksessionStarter(ctrl)}
			tc := setup(t, deps)
			resolver := New(0, nil, deps.players, deps.manifests, deps.limits, deps.starts, nil, nil, nil, nil, nil, nil, ownersReleasing(t, sessionUUID, 1, 2), nil, nil)
			t.Cleanup(resolver.Close)
			if tc.before != nil {
				tc.before(resolver)
			}

			result, err := resolver.AttachPlayer(ctx, sessionUUID, runtime.Player{UserUUID: playerUUID, Device: tc.device}, tc.ownerEpoch)

			require.Contains(t, resolver.controllers, sessionUUID, "the controller exists whatever the answer")

			if tc.expectedErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expectedOutcome, result.Outcome)
			require.Equal(t, tc.expectedRejection, result.Rejection)
			require.Equal(t, tc.expectedRequirements, result.Requirements)
			if tc.expectedOutcome == runtime.AttachAccepted {
				require.NotNil(t, result.Connection.Disconnect)
			}
			if tc.expectedPlayersAfter > 0 {
				controller, _ := resolver.controllerFor(ctx, sessionUUID, 1)
				require.Equal(t, tc.expectedPlayersAfter, controller.PlayerCount())
			}
		})
	}
}

func TestAdmitPlayer(t *testing.T) {
	type test struct {
		maxPlayers int
		// before runs against the Controller before the admission under test.
		before func(t *testing.T, controller *sessionController)
		// after runs against the admitted connection, when there is one.
		after func(t *testing.T, controller *sessionController, player runtime.PlayerConnection)

		expectedOK          bool
		expectedRejection   runtime.RejectReason
		expectedLobby       []string
		expectedPlayerLobby []string
		expectedEnd         runtime.EndReason
	}

	ctx := context.Background()
	admit := func(t *testing.T, controller *sessionController, userUUID string) runtime.PlayerConnection {
		t.Helper()
		player, _, ok := controller.joinLobby(ctx, runtime.Player{UserUUID: userUUID})
		require.True(t, ok)
		return player
	}
	// startGame moves the Controller to a running game, as Start will.
	startGame := func(controller *sessionController) {
		controller.lock().phase = phaseRunning
		controller.unlock()
	}

	tests := map[string]func(t *testing.T) test{
		"first_player_joins_the_lobby": func(t *testing.T) test {
			return test{
				maxPlayers:          8,
				expectedOK:          true,
				expectedLobby:       []string{"user-a"},
				expectedPlayerLobby: []string{"user-a"},
			}
		},
		"players_are_listed_in_the_order_they_joined": func(t *testing.T) test {
			return test{
				maxPlayers:          8,
				before:              func(t *testing.T, controller *sessionController) { admit(t, controller, "user-b") },
				expectedOK:          true,
				expectedLobby:       []string{"user-b", "user-a"},
				expectedPlayerLobby: []string{"user-b", "user-a"},
			}
		},
		"full_lobby_rejects": func(t *testing.T) test {
			return test{
				maxPlayers:        1,
				before:            func(t *testing.T, controller *sessionController) { admit(t, controller, "user-b") },
				expectedRejection: runtime.RejectLobbyFull,
				expectedLobby:     []string{"user-b"},
			}
		},
		"running_game_rejects_new_players": func(t *testing.T) test {
			return test{
				maxPlayers:        8,
				before:            func(t *testing.T, controller *sessionController) { startGame(controller) },
				expectedRejection: runtime.RejectSessionStarted,
				expectedLobby:     []string{},
			}
		},
		"disconnecting_from_a_running_game_keeps_the_participant_and_sends_no_lobby_update": func(t *testing.T) test {
			return test{
				maxPlayers: 8,
				after: func(t *testing.T, controller *sessionController, player runtime.PlayerConnection) {
					startGame(controller)
					player.Disconnect()
				},
				expectedOK:          true,
				expectedLobby:       []string{"user-a"},
				expectedPlayerLobby: []string{"user-a"},
				expectedEnd:         runtime.EndDisconnected,
			}
		},
		"same_user_replaces_their_older_connection_even_when_full": func(t *testing.T) test {
			var older runtime.PlayerConnection
			return test{
				maxPlayers: 1,
				before:     func(t *testing.T, controller *sessionController) { older = admit(t, controller, "user-a") },
				after: func(t *testing.T, controller *sessionController, _ runtime.PlayerConnection) {
					require.Equal(t, runtime.EndReplaced, endedWith(t, older.Ended))
					// The replaced connection going away keeps the user in.
					older.Disconnect()
				},
				expectedOK:          true,
				expectedLobby:       []string{"user-a"},
				expectedPlayerLobby: []string{"user-a"},
			}
		},
		"leaving_takes_the_player_out_and_they_may_come_back": func(t *testing.T) test {
			return test{
				maxPlayers: 8,
				before:     func(t *testing.T, controller *sessionController) { admit(t, controller, "user-b") },
				after: func(t *testing.T, controller *sessionController, player runtime.PlayerConnection) {
					result, err := player.Leave(ctx)
					require.NoError(t, err)
					require.Equal(t, runtime.LeaveResult{Outcome: runtime.PlayerLeft}, result)
					require.Equal(t, []string{"user-b"}, controller.lobbySnapshot())
					admit(t, controller, "user-a")
					// The ended connection no longer speaks for the user.
					_, _ = player.Leave(ctx)
				},
				expectedOK:          true,
				expectedLobby:       []string{"user-b", "user-a"},
				expectedPlayerLobby: []string{"user-b", "user-a"},
				expectedEnd:         runtime.EndLeft,
			}
		},
		"disconnecting_takes_the_player_out": func(t *testing.T) test {
			return test{
				maxPlayers: 8,
				after: func(t *testing.T, controller *sessionController, player runtime.PlayerConnection) {
					player.Disconnect()
					player.Disconnect()
				},
				expectedOK:          true,
				expectedLobby:       []string{},
				expectedPlayerLobby: []string{"user-a"},
				expectedEnd:         runtime.EndDisconnected,
			}
		},
		"commands_are_answered_as_not_running": func(t *testing.T) test {
			return test{
				maxPlayers: 8,
				after: func(t *testing.T, controller *sessionController, player runtime.PlayerConnection) {
					result, err := player.Command(ctx, runtime.Command{Token: "t1", Name: "play_card"})
					require.NoError(t, err)
					require.Equal(t, runtime.CommandResult{Token: "t1", Outcome: runtime.CommandNotRunning}, result)
					require.NoError(t, player.Sample(ctx, runtime.Sample{Sequence: 1, Inputs: []byte(`{}`)}))
					require.NoError(t, player.Resync(ctx))
				},
				expectedOK:          true,
				expectedLobby:       []string{"user-a"},
				expectedPlayerLobby: []string{"user-a"},
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			controller := newTestController(t, tc.maxPlayers)
			admin := controller.mustRegisterAdmin("creator")
			if tc.before != nil {
				tc.before(t, controller)
			}

			player, rejection, ok := controller.joinLobby(ctx, runtime.Player{UserUUID: "user-a"})

			require.Equal(t, tc.expectedOK, ok)
			require.Equal(t, tc.expectedRejection, rejection)
			if ok && tc.after != nil {
				tc.after(t, controller, player)
			}
			require.Equal(t, tc.expectedLobby, controller.lobbySnapshot())
			require.Equal(t, tc.expectedLobby, latestLobby(t, admin.NextLobbyUpdate), "admins see every change")
			if !ok {
				return
			}
			require.Equal(t, tc.expectedPlayerLobby, latestLobby(t, player.NextLobbyUpdate))
			require.Equal(t, tc.expectedEnd, endedWith(t, player.Ended))
		})
	}
}

func TestAttachPlayerChecksTheDeviceOfEveryConnection(t *testing.T) {
	type test struct {
		// before runs against the running game of user-a (P1) and user-b (P2).
		before func(t *testing.T, controller *sessionController, players map[string]runtime.PlayerConnection)
		device runtime.Device

		expectedResult   runtime.AttachPlayerResult
		expectedOlderEnd runtime.EndReason
	}

	ctx := context.Background()
	desktopOnly := inputrequirements.Requirements{UsesKeyboard: true, UsesPointerPress: true, Devices: inputrequirements.DevicesDesktop}
	rejected := runtime.AttachPlayerResult{
		Outcome:      runtime.AttachRejected,
		Rejection:    runtime.RejectDeviceNotSupported,
		Requirements: runtime.InputRequirements{Devices: "desktop", UsesKeyboard: true, UsesPointerPress: true},
	}

	tests := map[string]func(t *testing.T) test{
		"reattach_on_a_supported_device_replaces_the_older_connection": func(t *testing.T) test {
			return test{
				device:           runtime.DeviceDesktop,
				expectedResult:   runtime.AttachPlayerResult{Outcome: runtime.AttachAccepted},
				expectedOlderEnd: runtime.EndReplaced,
			}
		},
		"device_switch_to_an_unsupported_device_keeps_the_older_connection": func(t *testing.T) test {
			return test{device: runtime.DeviceMobile, expectedResult: rejected}
		},
		"reattach_without_a_device_is_rejected": func(t *testing.T) test {
			return test{expectedResult: rejected}
		},
		"rejoin_on_an_unsupported_device_is_rejected_before_the_game_is_asked": func(t *testing.T) test {
			return test{
				before: func(t *testing.T, controller *sessionController, players map[string]runtime.PlayerConnection) {
					controller.lock().manifest.Rejoin = true
					controller.unlock()
					_, err := players["user-a"].Leave(ctx)
					require.NoError(t, err)
					controller.takeBoundary()
				},
				device:           runtime.DeviceMobile,
				expectedResult:   rejected,
				expectedOlderEnd: runtime.EndLeft,
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			controller, players, _ := runningController(t, map[string][]byte{"P1": []byte(`{}`), "P2": []byte(`{}`)}, "user-a", "user-b")
			controller.lock().manifest.Requirements = desktopOnly
			controller.unlock()
			if tc.before != nil {
				tc.before(t, controller, players)
			}

			result, err := controller.attachPlayer(ctx, runtime.Player{UserUUID: "user-a", Device: tc.device})

			require.NoError(t, err)
			result.Connection = runtime.PlayerConnection{}
			require.Equal(t, tc.expectedResult, result)
			require.Equal(t, tc.expectedOlderEnd, endedWith(t, players["user-a"].Ended))
			if tc.expectedResult.Outcome == runtime.AttachRejected {
				next, _ := controller.takeBoundary()
				require.Empty(t, next.operations, "the game is not asked about a rejected device")
			}
		})
	}
}
