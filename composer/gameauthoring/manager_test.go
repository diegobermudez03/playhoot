package gameauthoring

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/play/runtime"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

const (
	testUserUUID    = "0f8fad5b-d9cb-469f-a165-70867728950e"
	testGameUUID    = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	testVersionUUID = "16fd2706-8baf-433b-82eb-8c7fada847da"
)

type dependencies struct {
	editions  *MockeditableVersionFinder
	drafts    *MockdraftReader
	links     *MockresourceLinkReader
	simulator *MockgameSimulator
}

func newTestManager(t *testing.T) (*Manager, dependencies) {
	ctrl := gomock.NewController(t)
	deps := dependencies{editions: NewMockeditableVersionFinder(ctrl), drafts: NewMockdraftReader(ctrl), links: NewMockresourceLinkReader(ctrl), simulator: NewMockgameSimulator(ctrl)}
	return New(deps.editions, deps.drafts, deps.links, deps.simulator), deps
}

func TestReadDraft(t *testing.T) {
	type test struct {
		expected        DraftRead
		expectErr       bool
		expectedErrorIs error
	}

	ctx := context.Background()
	found := game.EditableVersion{Outcome: game.EditableVersionFound, VersionUUID: testVersionUUID}
	snapshot := sessionpkg.DraftSnapshot{VersionUUID: testVersionUUID, GameUUID: testGameUUID, Status: "DRAFT", CurrentRevision: 2, Revision: 2}

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"the_owner_reads_the_version_being_built": func(t *testing.T, deps dependencies) test {
			deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(found, nil)
			deps.drafts.EXPECT().ReadDraft(ctx, testVersionUUID, int64(0), true).Return(snapshot, nil)
			return test{expected: DraftRead{Access: game.EditableVersionFound, Snapshot: snapshot}}
		},
		"not_found_never_reaches_play": func(t *testing.T, deps dependencies) test {
			deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(game.EditableVersion{Outcome: game.EditableVersionNotFound}, nil)
			return test{expected: DraftRead{Access: game.EditableVersionNotFound}}
		},
		"not_editable_never_reaches_play": func(t *testing.T, deps dependencies) test {
			deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(game.EditableVersion{Outcome: game.EditableVersionNotEditable}, nil)
			return test{expected: DraftRead{Access: game.EditableVersionNotEditable}}
		},
		"a_revision_past_the_current_one_keeps_its_error": func(t *testing.T, deps dependencies) test {
			deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(found, nil)
			deps.drafts.EXPECT().ReadDraft(ctx, testVersionUUID, int64(0), true).Return(sessionpkg.DraftSnapshot{}, sessionpkg.ErrDraftRevisionNotFound)
			return test{expectErr: true, expectedErrorIs: sessionpkg.ErrDraftRevisionNotFound}
		},
		"a_version_play_does_not_have_yet_is_reported_missing": func(t *testing.T, deps dependencies) test {
			deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(found, nil)
			deps.drafts.EXPECT().ReadDraft(ctx, testVersionUUID, int64(0), true).Return(sessionpkg.DraftSnapshot{}, sessionpkg.ErrDraftNotFound)
			return test{expected: DraftRead{Access: game.EditableVersionFound, Missing: true}}
		},
		"unknown_access_outcome_is_broken_data": func(t *testing.T, deps dependencies) test {
			deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(game.EditableVersion{Outcome: "maybe"}, nil)
			return test{expectErr: true}
		},
		"authorization_fails": func(t *testing.T, deps dependencies) test {
			deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(game.EditableVersion{}, errors.New("database down"))
			return test{expectErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			manager, deps := newTestManager(t)
			tc := setup(t, deps)

			result, err := manager.ReadDraft(ctx, testUserUUID, testGameUUID, 0, true)

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

func TestListDraftRevisions(t *testing.T) {
	ctx := context.Background()
	revisions := []sessionpkg.DraftRevision{{Number: 1, Source: "template", Files: 5, CreatedAt: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)}}

	t.Run("the_owner_lists_the_revisions", func(t *testing.T) {
		manager, deps := newTestManager(t)
		deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(game.EditableVersion{Outcome: game.EditableVersionFound, VersionUUID: testVersionUUID}, nil)
		deps.drafts.EXPECT().ListRevisions(ctx, testVersionUUID).Return(revisions, nil)

		result, err := manager.ListDraftRevisions(ctx, testUserUUID, testGameUUID)

		require.NoError(t, err)
		require.Equal(t, DraftRevisions{Access: game.EditableVersionFound, Revisions: revisions}, result)
	})

	t.Run("a_version_play_does_not_have_yet_is_reported_missing", func(t *testing.T) {
		manager, deps := newTestManager(t)
		deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(game.EditableVersion{Outcome: game.EditableVersionFound, VersionUUID: testVersionUUID}, nil)
		deps.drafts.EXPECT().ListRevisions(ctx, testVersionUUID).Return(nil, sessionpkg.ErrDraftNotFound)

		result, err := manager.ListDraftRevisions(ctx, testUserUUID, testGameUUID)

		require.NoError(t, err)
		require.Equal(t, DraftRevisions{Access: game.EditableVersionFound, Missing: true}, result)
	})

	t.Run("not_found_never_reaches_play", func(t *testing.T) {
		manager, deps := newTestManager(t)
		deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(game.EditableVersion{Outcome: game.EditableVersionNotFound}, nil)

		result, err := manager.ListDraftRevisions(ctx, testUserUUID, testGameUUID)

		require.NoError(t, err)
		require.Equal(t, DraftRevisions{Access: game.EditableVersionNotFound}, result)
	})
}

func TestReadDraftBuild(t *testing.T) {
	ctx := context.Background()
	found := game.EditableVersion{Outcome: game.EditableVersionFound, VersionUUID: testVersionUUID}
	built := sessionpkg.DraftBuild{Revision: 2, Status: sessionpkg.DraftBuildSucceeded}

	t.Run("the_owner_reads_and_waits_for_the_build", func(t *testing.T) {
		m, deps := newTestManager(t)
		deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(found, nil)
		deps.drafts.EXPECT().ReadBuild(ctx, testVersionUUID, int64(2), 5*time.Second).Return(built, nil)
		read, err := m.ReadDraftBuild(ctx, testUserUUID, testGameUUID, 2, 5*time.Second)
		require.NoError(t, err)
		require.Equal(t, DraftBuildRead{Access: game.EditableVersionFound, Build: built}, read)
	})
	t.Run("a_non_owner_never_reaches_play", func(t *testing.T) {
		m, deps := newTestManager(t)
		deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(game.EditableVersion{Outcome: game.EditableVersionNotFound}, nil)
		read, err := m.ReadDraftBuild(ctx, testUserUUID, testGameUUID, 0, 0)
		require.NoError(t, err)
		require.Equal(t, DraftBuildRead{Access: game.EditableVersionNotFound}, read)
	})
	t.Run("a_version_play_lacks_is_missing", func(t *testing.T) {
		m, deps := newTestManager(t)
		deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(found, nil)
		deps.drafts.EXPECT().ReadBuild(ctx, testVersionUUID, int64(0), time.Duration(0)).Return(sessionpkg.DraftBuild{}, sessionpkg.ErrDraftNotFound)
		read, err := m.ReadDraftBuild(ctx, testUserUUID, testGameUUID, 0, 0)
		require.NoError(t, err)
		require.True(t, read.Missing)
	})
	t.Run("a_revision_without_build_is_its_error", func(t *testing.T) {
		m, deps := newTestManager(t)
		deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(found, nil)
		deps.drafts.EXPECT().ReadBuild(ctx, testVersionUUID, int64(9), time.Duration(0)).Return(sessionpkg.DraftBuild{}, sessionpkg.ErrDraftBuildNotFound)
		_, err := m.ReadDraftBuild(ctx, testUserUUID, testGameUUID, 9, 0)
		require.True(t, errors.Is(err, sessionpkg.ErrDraftBuildNotFound))
	})
}

func TestAILinkStatus(t *testing.T) {
	ctx := context.Background()

	t.Run("the owner sees the link even without an open edition", func(t *testing.T) {
		m, deps := newTestManager(t)
		status := identity.ResourceLinkStatus{Active: true, CreatedAt: time.Unix(1, 0), ExpiresAt: time.Unix(2, 0)}
		deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(game.EditableVersion{Outcome: game.EditableVersionNotEditable}, nil)
		deps.links.EXPECT().ResourceLinkStatus(ctx, testUserUUID, game.ResourceKind, testGameUUID).Return(status, nil)

		read, err := m.AILinkStatus(ctx, testUserUUID, testGameUUID)
		require.NoError(t, err)
		require.Equal(t, AILinkStatusRead{Access: game.EditableVersionNotEditable, Status: status}, read)
	})

	t.Run("anyone else learns nothing", func(t *testing.T) {
		m, deps := newTestManager(t)
		deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(game.EditableVersion{Outcome: game.EditableVersionNotFound}, nil)

		read, err := m.AILinkStatus(ctx, testUserUUID, testGameUUID)
		require.NoError(t, err)
		require.Equal(t, AILinkStatusRead{Access: game.EditableVersionNotFound}, read)
	})
}

func TestSimulateDraft(t *testing.T) {
	type test struct {
		expected        DraftSimulation
		expectErr       bool
		expectedErrorIs error
	}

	ctx := context.Background()
	found := game.EditableVersion{Outcome: game.EditableVersionFound, VersionUUID: testVersionUUID}
	scenario := runtime.Scenario{Players: 1, Seed: 1, DurationMs: 100}
	manifest := sessionpkg.RuntimeManifest{MinPlayers: 1, MaxPlayers: 2}
	ready := sessionpkg.BuiltRevision{Revision: 3, BuildStatus: sessionpkg.DraftBuildSucceeded, Ready: true, Script: []byte("script"), ScriptProfile: "sdk-v1", Manifest: manifest}
	ran := runtime.SimulationResult{Outcome: runtime.SimulationRan, Stopped: runtime.SimulationStoppedDuration}

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"the_owner_simulates_a_succeeded_build": func(t *testing.T, deps dependencies) test {
			deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(found, nil)
			deps.drafts.EXPECT().LoadBuiltRevision(ctx, testVersionUUID, int64(0)).Return(ready, nil)
			deps.simulator.EXPECT().Simulate(ctx, []byte("script"), "sdk-v1", manifest, scenario).Return(ran, nil)
			return test{expected: DraftSimulation{Access: game.EditableVersionFound, Revision: 3, BuildStatus: sessionpkg.DraftBuildSucceeded, Ready: true, Result: ran}}
		},
		"a_build_that_did_not_succeed_is_never_simulated": func(t *testing.T, deps dependencies) test {
			deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(found, nil)
			deps.drafts.EXPECT().LoadBuiltRevision(ctx, testVersionUUID, int64(0)).Return(sessionpkg.BuiltRevision{Revision: 3, BuildStatus: sessionpkg.DraftBuildRunning}, nil)
			return test{expected: DraftSimulation{Access: game.EditableVersionFound, Revision: 3, BuildStatus: sessionpkg.DraftBuildRunning}}
		},
		"not_found_never_reaches_play": func(t *testing.T, deps dependencies) test {
			deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(game.EditableVersion{Outcome: game.EditableVersionNotFound}, nil)
			return test{expected: DraftSimulation{Access: game.EditableVersionNotFound}}
		},
		"a_version_play_does_not_have_yet_is_reported_missing": func(t *testing.T, deps dependencies) test {
			deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(found, nil)
			deps.drafts.EXPECT().LoadBuiltRevision(ctx, testVersionUUID, int64(0)).Return(sessionpkg.BuiltRevision{}, sessionpkg.ErrDraftNotFound)
			return test{expected: DraftSimulation{Access: game.EditableVersionFound, Missing: true}}
		},
		"a_revision_without_build_keeps_its_error": func(t *testing.T, deps dependencies) test {
			deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(found, nil)
			deps.drafts.EXPECT().LoadBuiltRevision(ctx, testVersionUUID, int64(0)).Return(sessionpkg.BuiltRevision{}, sessionpkg.ErrDraftBuildNotFound)
			return test{expectErr: true, expectedErrorIs: sessionpkg.ErrDraftBuildNotFound}
		},
		"busy_keeps_its_error": func(t *testing.T, deps dependencies) test {
			deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(found, nil)
			deps.drafts.EXPECT().LoadBuiltRevision(ctx, testVersionUUID, int64(0)).Return(ready, nil)
			deps.simulator.EXPECT().Simulate(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(runtime.SimulationResult{}, runtime.ErrSimulationsBusy)
			return test{expectErr: true, expectedErrorIs: runtime.ErrSimulationsBusy}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			m, deps := newTestManager(t)
			tc := setup(t, deps)

			simulated, err := m.SimulateDraft(ctx, testUserUUID, testGameUUID, 0, scenario)

			if tc.expectErr {
				require.Error(t, err)
				if tc.expectedErrorIs != nil {
					require.ErrorIs(t, err, tc.expectedErrorIs)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, simulated)
		})
	}
}
