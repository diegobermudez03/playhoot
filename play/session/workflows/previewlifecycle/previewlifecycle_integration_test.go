package previewlifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/objectstore"
	"github.com/diegobermudez03/playhoot/play/session/internal/testdb"
	"github.com/diegobermudez03/playhoot/play/session/workflows/versiondrafting"
)

// readyBuilds answers every revision as SUCCEEDED for one or two players.
type readyBuilds struct{}

func (readyBuilds) LoadBuiltRevision(_ context.Context, _ string, revision int64) (sessionpkg.BuiltRevision, error) {
	if revision == 0 {
		revision = 1
	}
	return sessionpkg.BuiltRevision{
		Revision: revision, BuildStatus: sessionpkg.DraftBuildSucceeded, Ready: true, Script: []byte("script"), ScriptProfile: "sdk-v1",
		Manifest: sessionpkg.RuntimeManifest{MinPlayers: 1, MaxPlayers: 2},
	}, nil
}

func (readyBuilds) SignBuiltFrontend(_ context.Context, _ string, _ int64, expiresAt time.Time) (sessionpkg.FrontendAccess, error) {
	return sessionpkg.FrontendAccess{URL: "https://storage/client.html", ExpiresAt: expiresAt}, nil
}

// Integration tests run previews against the shared disposable database,
// each case with its own draft version.
func TestPreviewLifecycleIntegration(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	drafts := versiondrafting.New(db, objectstore.NewFake(), nil)
	m := New(db, readyBuilds{})
	ctx := context.Background()

	newDraft := func(t *testing.T) string {
		t.Helper()
		versionUUID := uuid.NewString()
		_, err := drafts.CreateDraftVersion(ctx, uuid.NewString(), versionUUID, sessionpkg.DraftStart{})
		require.NoError(t, err)
		return versionUUID
	}

	t.Run("opening_records_the_preview_and_replaces_the_running_one", func(t *testing.T) {
		versionUUID := newDraft(t)
		first, err := m.OpenPreview(ctx, versionUUID, testOwnerUUID, 0, 2)
		require.NoError(t, err)
		require.Equal(t, sessionpkg.PreviewOpened, first.Outcome)
		require.Equal(t, []string{"P1", "P2"}, first.Participants)
		require.Empty(t, first.Replaced)
		require.Equal(t, []byte("script"), first.Script)
		require.Equal(t, "https://storage/client.html", first.Frontend.URL)

		found, err := m.FindPreview(ctx, first.PreviewUUID)
		require.NoError(t, err)
		require.Equal(t, testOwnerUUID, found.OwnerUserUUID)
		require.Equal(t, []string{"P1", "P2"}, found.Participants)
		require.False(t, found.Ended)

		second, err := m.OpenPreview(ctx, versionUUID, testOwnerUUID, 1, 1)
		require.NoError(t, err)
		require.Equal(t, first.PreviewUUID, second.Replaced)
		replaced, err := m.FindPreview(ctx, first.PreviewUUID)
		require.NoError(t, err)
		require.Equal(t, sessionpkg.Preview{
			UUID: first.PreviewUUID, OwnerUserUUID: testOwnerUUID, Revision: 1, Participants: []string{"P1", "P2"},
			CreatedAt: replaced.CreatedAt, ExpiresAt: replaced.ExpiresAt, Ended: true, EndReason: sessionpkg.PreviewEndReplaced,
		}, replaced)
	})

	t.Run("a_preview_ends_once", func(t *testing.T) {
		versionUUID := newDraft(t)
		opened, err := m.OpenPreview(ctx, versionUUID, testOwnerUUID, 0, 1)
		require.NoError(t, err)

		ended, err := m.EndPreview(ctx, opened.PreviewUUID, sessionpkg.PreviewEndFinished)
		require.NoError(t, err)
		require.True(t, ended)
		again, err := m.EndPreview(ctx, opened.PreviewUUID, sessionpkg.PreviewEndIdle)
		require.NoError(t, err)
		require.False(t, again, "an ended preview keeps why it ended")
		found, err := m.FindPreview(ctx, opened.PreviewUUID)
		require.NoError(t, err)
		require.Equal(t, sessionpkg.PreviewEndFinished, found.EndReason)

		_, err = m.EndPreview(ctx, opened.PreviewUUID, "exploded")
		require.Error(t, err)
	})

	t.Run("unknown_previews_and_versions", func(t *testing.T) {
		_, err := m.FindPreview(ctx, uuid.NewString())
		require.ErrorIs(t, err, sessionpkg.ErrPreviewNotFound)
		_, err = m.FindPreview(ctx, "not-a-uuid")
		require.ErrorIs(t, err, sessionpkg.ErrPreviewNotFound)
		_, err = m.OpenPreview(ctx, uuid.NewString(), testOwnerUUID, 0, 1)
		require.ErrorIs(t, err, sessionpkg.ErrDraftNotFound)
	})
}
