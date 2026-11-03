package gameauthoring

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/game"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

func TestDraftAssets(t *testing.T) {
	ctx := context.Background()
	found := game.EditableVersion{Outcome: game.EditableVersionFound, VersionUUID: testVersionUUID}

	t.Run("the_owner_uploads_an_asset", func(t *testing.T) {
		m, deps := newTestManager(t)
		content := strings.NewReader("png bytes")
		attached := sessionpkg.DraftAssetResult{Outcome: sessionpkg.DraftAssetAttached, Revision: 2}
		deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(found, nil)
		deps.drafts.EXPECT().UploadAsset(ctx, testVersionUUID, "cardBack", content, testKey, sessionpkg.DraftOriginWeb).Return(attached, nil)

		got, err := m.UploadDraftAsset(ctx, testUserUUID, testGameUUID, "cardBack", content, testKey, sessionpkg.DraftOriginWeb)

		require.NoError(t, err)
		require.Equal(t, DraftAssetEdit{Access: game.EditableVersionFound, Result: attached}, got)
	})

	t.Run("someone_else_never_reaches_play", func(t *testing.T) {
		m, deps := newTestManager(t)
		deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(game.EditableVersion{Outcome: game.EditableVersionNotFound}, nil)

		got, err := m.UploadDraftAsset(ctx, testUserUUID, testGameUUID, "cardBack", strings.NewReader("x"), testKey, sessionpkg.DraftOriginWeb)

		require.NoError(t, err)
		require.Equal(t, DraftAssetEdit{Access: game.EditableVersionNotFound}, got)
	})

	t.Run("uploading_to_a_version_play_does_not_have_yet_repairs_it", func(t *testing.T) {
		m, deps := newTestManager(t)
		content := strings.NewReader("png bytes")
		attached := sessionpkg.DraftAssetResult{Outcome: sessionpkg.DraftAssetAttached, Revision: 2}
		deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(found, nil)
		gomock.InOrder(
			deps.drafts.EXPECT().UploadAsset(ctx, testVersionUUID, "cardBack", content, testKey, sessionpkg.DraftOriginWeb).Return(sessionpkg.DraftAssetResult{}, sessionpkg.ErrDraftNotFound),
			deps.drafts.EXPECT().CreateDraftVersion(ctx, testGameUUID, testVersionUUID, "").Return(sessionpkg.DraftVersion{Outcome: sessionpkg.DraftCreated, GameUUID: testGameUUID, VersionUUID: testVersionUUID, Revision: 1}, nil),
			deps.drafts.EXPECT().UploadAsset(ctx, testVersionUUID, "cardBack", content, testKey, sessionpkg.DraftOriginWeb).Return(attached, nil),
		)

		got, err := m.UploadDraftAsset(ctx, testUserUUID, testGameUUID, "cardBack", content, testKey, sessionpkg.DraftOriginWeb)

		require.NoError(t, err)
		require.Equal(t, DraftAssetEdit{Access: game.EditableVersionFound, Result: attached}, got)
	})

	t.Run("a_full_instance_is_told_as_is", func(t *testing.T) {
		m, deps := newTestManager(t)
		deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(found, nil)
		deps.drafts.EXPECT().UploadAsset(ctx, testVersionUUID, "cardBack", gomock.Any(), testKey, sessionpkg.DraftOriginWeb).Return(sessionpkg.DraftAssetResult{}, sessionpkg.ErrUploadsBusy)

		_, err := m.UploadDraftAsset(ctx, testUserUUID, testGameUUID, "cardBack", strings.NewReader("x"), testKey, sessionpkg.DraftOriginWeb)

		require.ErrorIs(t, err, sessionpkg.ErrUploadsBusy)
	})

	t.Run("a_reused_key_is_an_idempotency_conflict", func(t *testing.T) {
		m, deps := newTestManager(t)
		deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(found, nil)
		deps.drafts.EXPECT().RemoveAsset(ctx, testVersionUUID, "cardBack", testKey, sessionpkg.DraftOriginWeb).Return(sessionpkg.DraftAssetResult{}, sessionpkg.ErrIdempotencyConflict)

		_, err := m.RemoveDraftAsset(ctx, testUserUUID, testGameUUID, "cardBack", testKey, sessionpkg.DraftOriginWeb)

		require.ErrorIs(t, err, sessionpkg.ErrIdempotencyConflict)
	})
}
