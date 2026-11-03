package previewlifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

const (
	testVersionUUID = "16fd2706-8baf-433b-82eb-8c7fada847da"
	testOwnerUUID   = "0f8fad5b-d9cb-469f-a165-70867728950e"
)

func TestOpenPreviewRefusals(t *testing.T) {
	ctx := context.Background()
	tests := map[string]struct {
		built    sessionpkg.BuiltRevision
		err      error
		players  int
		expected sessionpkg.PreviewOpening
	}{
		"a_revision_without_a_build": {
			err: sessionpkg.ErrDraftBuildNotFound, players: 1,
			expected: sessionpkg.PreviewOpening{Outcome: sessionpkg.PreviewBuildNotReady},
		},
		"a_build_that_did_not_succeed": {
			built: sessionpkg.BuiltRevision{Revision: 2, BuildStatus: sessionpkg.DraftBuildFailed}, players: 1,
			expected: sessionpkg.PreviewOpening{Outcome: sessionpkg.PreviewBuildNotReady, BuildStatus: sessionpkg.DraftBuildFailed},
		},
		"more_players_than_the_manifest_allows": {
			built:    sessionpkg.BuiltRevision{Revision: 2, Ready: true, Manifest: sessionpkg.RuntimeManifest{MinPlayers: 2, MaxPlayers: 4}},
			players:  5,
			expected: sessionpkg.PreviewOpening{Outcome: sessionpkg.PreviewInvalidPlayers, MinPlayers: 2, MaxPlayers: 4},
		},
		"fewer_players_than_the_manifest_allows": {
			built:    sessionpkg.BuiltRevision{Revision: 2, Ready: true, Manifest: sessionpkg.RuntimeManifest{MinPlayers: 2, MaxPlayers: 4}},
			players:  1,
			expected: sessionpkg.PreviewOpening{Outcome: sessionpkg.PreviewInvalidPlayers, MinPlayers: 2, MaxPlayers: 4},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			builds := NewMockbuiltRevisions(ctrl)
			builds.EXPECT().LoadBuiltRevision(ctx, testVersionUUID, int64(0)).Return(tc.built, tc.err)
			m := &Manager{openPreviewRepo: NewMockopenPreviewRepo(ctrl), builds: builds}

			opening, err := m.OpenPreview(ctx, testVersionUUID, testOwnerUUID, 0, tc.players)

			require.NoError(t, err)
			require.Equal(t, tc.expected, opening)
		})
	}

	t.Run("a_failing_build_read_is_an_error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		builds := NewMockbuiltRevisions(ctrl)
		builds.EXPECT().LoadBuiltRevision(ctx, testVersionUUID, int64(0)).Return(sessionpkg.BuiltRevision{}, errors.New("database down"))
		m := &Manager{builds: builds}
		_, err := m.OpenPreview(ctx, testVersionUUID, testOwnerUUID, 0, 1)
		require.Error(t, err)
	})
}
