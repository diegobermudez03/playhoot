package versiondrafting

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gorm.io/gorm"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/build"
	"github.com/diegobermudez03/playhoot/play/session/internal/objectstore"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/workflows/versiondrafting/internal/repo"
)

// nilDB hands the repo mocks a nil handle: they never use it.
type nilDB struct{}

func (nilDB) GetDB() *gorm.DB { return nil }

func TestLoadBuiltRevision(t *testing.T) {
	ctx := context.Background()
	const versionUUID = "16fd2706-8baf-433b-82eb-8c7fada847da"
	current := int64(4)
	version := &internalrepo.Version{ID: 7, UUID: versionUUID, Status: "DRAFT", CurrentRevision: &current}
	key, sha, size := "game-versions/backend-scripts/abc", "abc", int64(6)
	succeeded := &internalrepo.BuiltArtifacts{
		Revision: 4, Status: string(build.Succeeded), ServerObjectKey: &key, ServerSHA256: &sha, ServerSize: &size, ScriptProfile: "sdk-v1",
		Manifest: []byte(`{"schema_version":1,"players":{"min":1,"max":8},"commands":{"tap":{"payload":{}}},"entrypoints":["update"]}`),
	}

	type test struct {
		revision        int64
		expected        sessionpkg.BuiltRevision
		expectErr       bool
		expectedErrorIs error
	}
	tests := map[string]func(repo *MockloadBuiltRevisionRepo, scripts *MockscriptLoader) test{
		"the_current_revision's_succeeded_build_is_ready_with_its_script": func(repo *MockloadBuiltRevisionRepo, scripts *MockscriptLoader) test {
			repo.EXPECT().FindVersion(ctx, nil, versionUUID).Return(version, nil)
			repo.EXPECT().FindBuiltArtifacts(ctx, nil, versionUUID, int64(4)).Return(succeeded, nil)
			scripts.EXPECT().Load(ctx, objectstore.Locator{ObjectKey: key, SHA256: sha, Size: size}).Return([]byte("script"), nil)
			return test{expected: sessionpkg.BuiltRevision{
				Revision: 4, BuildStatus: sessionpkg.DraftBuildSucceeded, Ready: true, Script: []byte("script"), ScriptProfile: "sdk-v1",
				Manifest: sessionpkg.RuntimeManifest{MinPlayers: 1, MaxPlayers: 8, Commands: []sessionpkg.RuntimeCommand{{Name: "tap"}}, Update: true},
			}}
		},
		"a_build_that_did_not_succeed_is_not_ready": func(repo *MockloadBuiltRevisionRepo, scripts *MockscriptLoader) test {
			repo.EXPECT().FindVersion(ctx, nil, versionUUID).Return(version, nil)
			repo.EXPECT().FindBuiltArtifacts(ctx, nil, versionUUID, int64(2)).Return(&internalrepo.BuiltArtifacts{Revision: 2, Status: string(build.Failed)}, nil)
			return test{revision: 2, expected: sessionpkg.BuiltRevision{Revision: 2, BuildStatus: sessionpkg.DraftBuildFailed}}
		},
		"a_revision_without_build": func(repo *MockloadBuiltRevisionRepo, scripts *MockscriptLoader) test {
			repo.EXPECT().FindVersion(ctx, nil, versionUUID).Return(version, nil)
			repo.EXPECT().FindBuiltArtifacts(ctx, nil, versionUUID, int64(9)).Return(nil, nil)
			return test{revision: 9, expectErr: true, expectedErrorIs: sessionpkg.ErrDraftBuildNotFound}
		},
		"an_unknown_version": func(repo *MockloadBuiltRevisionRepo, scripts *MockscriptLoader) test {
			repo.EXPECT().FindVersion(ctx, nil, versionUUID).Return(nil, nil)
			return test{expectErr: true, expectedErrorIs: sessionpkg.ErrDraftNotFound}
		},
		"a_succeeded_build_without_its_script_is_broken_data": func(repo *MockloadBuiltRevisionRepo, scripts *MockscriptLoader) test {
			repo.EXPECT().FindVersion(ctx, nil, versionUUID).Return(version, nil)
			repo.EXPECT().FindBuiltArtifacts(ctx, nil, versionUUID, int64(4)).Return(&internalrepo.BuiltArtifacts{Revision: 4, Status: string(build.Succeeded)}, nil)
			return test{expectErr: true}
		},
		"loading_the_script_fails": func(repo *MockloadBuiltRevisionRepo, scripts *MockscriptLoader) test {
			repo.EXPECT().FindVersion(ctx, nil, versionUUID).Return(version, nil)
			repo.EXPECT().FindBuiltArtifacts(ctx, nil, versionUUID, int64(4)).Return(succeeded, nil)
			scripts.EXPECT().Load(ctx, gomock.Any()).Return(nil, errors.New("storage down"))
			return test{expectErr: true}
		},
	}
	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo, scripts := NewMockloadBuiltRevisionRepo(ctrl), NewMockscriptLoader(ctrl)
			tc := setup(repo, scripts)
			m := &Manager{loadBuiltRevisionRepo: repo, scripts: scripts, dbServicer: nilDB{}}

			loaded, err := m.LoadBuiltRevision(ctx, versionUUID, tc.revision)

			if tc.expectErr {
				require.Error(t, err)
				if tc.expectedErrorIs != nil {
					require.ErrorIs(t, err, tc.expectedErrorIs)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, loaded)
		})
	}
}
