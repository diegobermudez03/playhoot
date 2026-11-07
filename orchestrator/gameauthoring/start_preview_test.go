package gameauthoring

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/play/runtime"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

func TestStartPreview(t *testing.T) {
	ctx := context.Background()
	found := game.EditableVersion{Outcome: game.EditableVersionFound, VersionUUID: testVersionUUID}

	t.Run("starts_a_preview_of_the_version_being_built", func(t *testing.T) {
		m, deps := newTestManager(t)
		deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(found, nil)
		started := runtime.StartPreviewResult{Outcome: runtime.PreviewStarted, PreviewUUID: "p-1", Revision: 3, Participants: []string{"P1", "P2"}}
		deps.previews.EXPECT().StartPreview(ctx, testVersionUUID, testUserUUID, int64(3), 2).Return(started, nil)

		result, err := m.StartPreview(ctx, testUserUUID, testGameUUID, 3, 2)

		require.NoError(t, err)
		require.Equal(t, PreviewStart{Access: game.EditableVersionFound, Result: started}, result)
	})

	t.Run("someone_who_may_not_edit_starts_nothing", func(t *testing.T) {
		m, deps := newTestManager(t)
		deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(game.EditableVersion{Outcome: game.EditableVersionNotFound}, nil)

		result, err := m.StartPreview(ctx, testUserUUID, testGameUUID, 0, 2)

		require.NoError(t, err)
		require.Equal(t, game.EditableVersionNotFound, result.Access)
	})

	t.Run("a_draft_play_lacks_is_created_and_the_start_tried_again", func(t *testing.T) {
		m, deps := newTestManager(t)
		deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(found, nil).AnyTimes()
		gomock.InOrder(
			deps.previews.EXPECT().StartPreview(ctx, testVersionUUID, testUserUUID, int64(0), 1).Return(runtime.StartPreviewResult{}, sessionpkg.ErrDraftNotFound),
			deps.drafts.EXPECT().CreateDraftVersion(ctx, testGameUUID, testVersionUUID, sessionpkg.DraftStart{}).Return(sessionpkg.DraftVersion{Outcome: sessionpkg.DraftCreated, GameUUID: testGameUUID, VersionUUID: testVersionUUID, Revision: 1}, nil),
			deps.previews.EXPECT().StartPreview(ctx, testVersionUUID, testUserUUID, int64(0), 1).Return(runtime.StartPreviewResult{Outcome: runtime.PreviewBuildNotReady}, nil),
		)

		result, err := m.StartPreview(ctx, testUserUUID, testGameUUID, 0, 1)

		require.NoError(t, err)
		require.Equal(t, runtime.PreviewBuildNotReady, result.Result.Outcome)
	})

	t.Run("a_failure_is_an_error", func(t *testing.T) {
		m, deps := newTestManager(t)
		deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(found, nil)
		deps.previews.EXPECT().StartPreview(ctx, testVersionUUID, testUserUUID, int64(0), 1).Return(runtime.StartPreviewResult{}, errors.New("sandbox down"))

		_, err := m.StartPreview(ctx, testUserUUID, testGameUUID, 0, 1)

		require.Error(t, err)
	})
}
