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

func TestAttachAdmin(t *testing.T) {
	type dependencies struct {
		sessions  *MockadminJoinChecker
		manifests *MockmanifestLoader
	}
	type test struct {
		// before runs against the Manager before the attach under test.
		before     func(manager *Manager)
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
		"another_admin_joins_the_existing_controller": func(t *testing.T, deps dependencies) test {
			deps.sessions.EXPECT().EligibleToJoinAsAdmin(ctx, sessionUUID, creatorUUID).Return(sessionpkg.AdminJoinEligible, nil).Times(2)
			// Only the controller's creation loads it.
			loadsManifest(deps)
			return test{
				before: func(manager *Manager) {
					_, err := manager.AttachAdmin(ctx, sessionUUID, creatorUUID, 1)
					require.NoError(t, err)
				},
				ownerEpoch:          1,
				expectedOutcome:     runtime.AttachAccepted,
				expectedAdminsAfter: 2,
			}
		},
		"request_under_an_older_epoch_rejects_as_owner_changed": func(t *testing.T, deps dependencies) test {
			deps.sessions.EXPECT().EligibleToJoinAsAdmin(ctx, sessionUUID, creatorUUID).Return(sessionpkg.AdminJoinEligible, nil).Times(2)
			loadsManifest(deps)
			return test{
				before: func(manager *Manager) {
					_, err := manager.AttachAdmin(ctx, sessionUUID, creatorUUID, 2)
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
			manager := New(deps.sessions, nil, deps.manifests)
			if tc.before != nil {
				tc.before(manager)
			}

			result, err := manager.AttachAdmin(ctx, sessionUUID, creatorUUID, tc.ownerEpoch)

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
				controller, _, err := manager.controllerFor(ctx, sessionUUID, 1)
				require.NoError(t, err)
				require.Equal(t, tc.expectedAdminsAfter, controller.AdminCount())
			}
		})
	}
}
