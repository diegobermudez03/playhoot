package loadruntimemanifest

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
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

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"valid_manifest": func(t *testing.T, deps dependencies) test {
			deps.manifests.EXPECT().FindSessionManifest(ctx, sessionUUID).
				Return([]byte(`{"schema_version":1,"players":{"min":2,"max":6}}`), true, nil)
			return test{sessionUUID: sessionUUID, expectedManifest: sessionpkg.RuntimeManifest{MinPlayers: 2, MaxPlayers: 6}}
		},
		"invalid_manifest_is_broken_data": func(t *testing.T, deps dependencies) test {
			deps.manifests.EXPECT().FindSessionManifest(ctx, sessionUUID).
				Return([]byte(`{"schema_version":1,"players":{"min":3,"max":2}}`), true, nil)
			return test{sessionUUID: sessionUUID, expectedErr: true}
		},
		"version_row_missing_is_broken_data": func(t *testing.T, deps dependencies) test {
			deps.manifests.EXPECT().FindSessionManifest(ctx, sessionUUID).Return(nil, true, nil)
			return test{sessionUUID: sessionUUID, expectedErr: true}
		},
		"session_not_found": func(t *testing.T, deps dependencies) test {
			deps.manifests.EXPECT().FindSessionManifest(ctx, sessionUUID).Return(nil, false, nil)
			return test{sessionUUID: sessionUUID, expectedErrIs: sessionpkg.ErrSessionNotFound}
		},
		"session_uuid_is_not_a_uuid": func(t *testing.T, deps dependencies) test {
			return test{sessionUUID: "not-a-uuid", expectedErrIs: sessionpkg.ErrSessionNotFound}
		},
		"reading_fails": func(t *testing.T, deps dependencies) test {
			deps.manifests.EXPECT().FindSessionManifest(ctx, sessionUUID).Return(nil, false, errors.New("database down"))
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
