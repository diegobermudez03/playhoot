package loadruntimemanifest

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/usecases/loadruntimemanifest/internal/repo"
)

func TestLoadRuntimeManifest(t *testing.T) {
	type dependencies struct {
		manifests *MockmanifestFinder
	}
	type test struct {
		sessionUUID string

		expectedManifest sessionpkg.RuntimeManifest
		expectedErrIs    error
		expectedErr      bool
	}

	const sessionUUID = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	ctx := context.Background()
	pressOnly := internalrepo.InputRequirements{UsesPointerPress: true, Devices: "both"}
	// stored is a version holding manifest and pressOnly.
	stored := func(manifest string) internalrepo.SessionManifest {
		return internalrepo.SessionManifest{Manifest: []byte(manifest), InputRequirements: &pressOnly}
	}
	pressOnlyRuntime := sessionpkg.RuntimeInputRequirements{Devices: "both", UsesPointerPress: true}

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"valid_manifest": func(t *testing.T, deps dependencies) test {
			deps.manifests.EXPECT().FindSessionManifest(ctx, sessionUUID).
				Return(stored(`{"schema_version":1,"players":{"min":2,"max":6}}`), true, nil)
			return test{sessionUUID: sessionUUID, expectedManifest: sessionpkg.RuntimeManifest{MinPlayers: 2, MaxPlayers: 6, InputRequirements: pressOnlyRuntime}}
		},
		"commands_with_their_payloads": func(t *testing.T, deps dependencies) test {
			deps.manifests.EXPECT().FindSessionManifest(ctx, sessionUUID).
				Return(stored(`{"schema_version":1,"players":{"min":1,"max":2},"commands":{"pass":{},"play_card":{"payload":{"card":{"type":"string"}}}}}`), true, nil)
			return test{sessionUUID: sessionUUID, expectedManifest: sessionpkg.RuntimeManifest{MinPlayers: 1, MaxPlayers: 2, Commands: []sessionpkg.RuntimeCommand{
				{Name: "pass"},
				{Name: "play_card", Payload: []sessionpkg.RuntimeInput{{Name: "card", Type: "string", Default: []byte(`""`)}}},
			}, InputRequirements: pressOnlyRuntime}}
		},
		"game_with_update": func(t *testing.T, deps dependencies) test {
			deps.manifests.EXPECT().FindSessionManifest(ctx, sessionUUID).
				Return(stored(`{"schema_version":1,"players":{"min":1,"max":2},"entrypoints":["update"]}`), true, nil)
			return test{sessionUUID: sessionUUID, expectedManifest: sessionpkg.RuntimeManifest{MinPlayers: 1, MaxPlayers: 2, Update: true, InputRequirements: pressOnlyRuntime}}
		},
		"game_with_rejoin": func(t *testing.T, deps dependencies) test {
			deps.manifests.EXPECT().FindSessionManifest(ctx, sessionUUID).
				Return(stored(`{"schema_version":1,"players":{"min":1,"max":2},"entrypoints":["playerRejoined"]}`), true, nil)
			return test{sessionUUID: sessionUUID, expectedManifest: sessionpkg.RuntimeManifest{MinPlayers: 1, MaxPlayers: 2, Rejoin: true, InputRequirements: pressOnlyRuntime}}
		},
		"invalid_manifest_is_broken_data": func(t *testing.T, deps dependencies) test {
			deps.manifests.EXPECT().FindSessionManifest(ctx, sessionUUID).
				Return(stored(`{"schema_version":1,"players":{"min":3,"max":2}}`), true, nil)
			return test{sessionUUID: sessionUUID, expectedErr: true}
		},
		"version_row_missing_is_broken_data": func(t *testing.T, deps dependencies) test {
			deps.manifests.EXPECT().FindSessionManifest(ctx, sessionUUID).Return(internalrepo.SessionManifest{InputRequirements: &pressOnly}, true, nil)
			return test{sessionUUID: sessionUUID, expectedErr: true}
		},
		"keyboard_game_for_desktops": func(t *testing.T, deps dependencies) test {
			deps.manifests.EXPECT().FindSessionManifest(ctx, sessionUUID).Return(internalrepo.SessionManifest{
				Manifest:          []byte(`{"schema_version":1,"players":{"min":1,"max":2}}`),
				InputRequirements: &internalrepo.InputRequirements{UsesKeyboard: true, UsesPointerHover: true, Devices: "desktop"},
			}, true, nil)
			return test{sessionUUID: sessionUUID, expectedManifest: sessionpkg.RuntimeManifest{MinPlayers: 1, MaxPlayers: 2,
				InputRequirements: sessionpkg.RuntimeInputRequirements{Devices: "desktop", UsesKeyboard: true, UsesPointerHover: true}}}
		},
		"mobile_devices_are_known": func(t *testing.T, deps dependencies) test {
			deps.manifests.EXPECT().FindSessionManifest(ctx, sessionUUID).Return(internalrepo.SessionManifest{
				Manifest:          []byte(`{"schema_version":1,"players":{"min":1,"max":2}}`),
				InputRequirements: &internalrepo.InputRequirements{UsesPointerPress: true, Devices: "mobile"},
			}, true, nil)
			return test{sessionUUID: sessionUUID, expectedManifest: sessionpkg.RuntimeManifest{MinPlayers: 1, MaxPlayers: 2,
				InputRequirements: sessionpkg.RuntimeInputRequirements{Devices: "mobile", UsesPointerPress: true}}}
		},
		"missing_input_requirements_is_broken_data": func(t *testing.T, deps dependencies) test {
			deps.manifests.EXPECT().FindSessionManifest(ctx, sessionUUID).
				Return(internalrepo.SessionManifest{Manifest: []byte(`{"schema_version":1,"players":{"min":1,"max":2}}`)}, true, nil)
			return test{sessionUUID: sessionUUID, expectedErr: true}
		},
		"unknown_devices_is_broken_data": func(t *testing.T, deps dependencies) test {
			deps.manifests.EXPECT().FindSessionManifest(ctx, sessionUUID).Return(internalrepo.SessionManifest{
				Manifest:          []byte(`{"schema_version":1,"players":{"min":1,"max":2}}`),
				InputRequirements: &internalrepo.InputRequirements{UsesPointerPress: true, Devices: "tablet"},
			}, true, nil)
			return test{sessionUUID: sessionUUID, expectedErr: true}
		},
		"session_not_found": func(t *testing.T, deps dependencies) test {
			deps.manifests.EXPECT().FindSessionManifest(ctx, sessionUUID).Return(internalrepo.SessionManifest{}, false, nil)
			return test{sessionUUID: sessionUUID, expectedErrIs: sessionpkg.ErrSessionNotFound}
		},
		"session_uuid_is_not_a_uuid": func(t *testing.T, deps dependencies) test {
			return test{sessionUUID: "not-a-uuid", expectedErrIs: sessionpkg.ErrSessionNotFound}
		},
		"reading_fails": func(t *testing.T, deps dependencies) test {
			deps.manifests.EXPECT().FindSessionManifest(ctx, sessionUUID).Return(internalrepo.SessionManifest{}, false, errors.New("database down"))
			return test{sessionUUID: sessionUUID, expectedErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			deps := dependencies{manifests: NewMockmanifestFinder(ctrl)}
			tc := setup(t, deps)
			service := &Service{manifests: deps.manifests}

			manifest, err := service.LoadRuntimeManifest(ctx, tc.sessionUUID)

			if tc.expectedErrIs != nil {
				require.ErrorIs(t, err, tc.expectedErrIs)
				return
			}
			if tc.expectedErr {
				require.Error(t, err)
				require.NotErrorIs(t, err, sessionpkg.ErrSessionNotFound)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expectedManifest, manifest)
		})
	}
}
