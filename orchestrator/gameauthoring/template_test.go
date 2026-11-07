package gameauthoring

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/game"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

const (
	testTemplateGame    = "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
	testTemplateVersion = "6ba7b811-9dad-11d1-80b4-00c04fd430c8"
)

func TestCreateGameFromATemplate(t *testing.T) {
	ctx := context.Background()
	found := game.Template{Outcome: game.TemplateFound, GameUUID: testTemplateGame, VersionUUID: testTemplateVersion}
	source := &game.TemplateSource{GameUUID: testTemplateGame, VersionUUID: testTemplateVersion}

	t.Run("copies_the_template_s_published_version", func(t *testing.T) {
		manager, deps := newTestManager(t)
		gomock.InOrder(
			deps.templates.EXPECT().FindTemplate(ctx, testTemplateGame).Return(found, nil),
			deps.games.EXPECT().CreateGame(ctx, testUserUUID, "Quiz", testKey, source).Return(game.CreatedGame{GameUUID: testGameUUID, EditionVersionUUID: testVersionUUID}, nil),
			deps.drafts.EXPECT().CreateDraftVersion(ctx, testGameUUID, testVersionUUID, sessionpkg.DraftStart{FromVersionUUID: testTemplateVersion, TemplateGameUUID: testTemplateGame}).
				Return(sessionpkg.DraftVersion{Outcome: sessionpkg.DraftCreated, Revision: 1}, nil),
		)

		created, err := manager.CreateGame(ctx, testUserUUID, "Quiz", testKey, testTemplateGame)

		require.NoError(t, err)
		require.Equal(t, CreatedGame{GameUUID: testGameUUID, VersionUUID: testVersionUUID, Revision: 1}, created)
	})

	t.Run("a_game_that_is_not_a_template_creates_nothing", func(t *testing.T) {
		manager, deps := newTestManager(t)
		deps.templates.EXPECT().FindTemplate(ctx, testTemplateGame).Return(game.Template{Outcome: game.TemplateNotFound}, nil)

		_, err := manager.CreateGame(ctx, testUserUUID, "Quiz", testKey, testTemplateGame)

		require.ErrorIs(t, err, ErrTemplateNotFound)
	})
}

func TestARepairStartsFromTheSameTemplate(t *testing.T) {
	ctx := context.Background()
	manager, deps := newTestManager(t)
	editable := game.EditableVersion{Outcome: game.EditableVersionFound, VersionUUID: testVersionUUID, Template: &game.TemplateSource{GameUUID: testTemplateGame, VersionUUID: testTemplateVersion}}
	gomock.InOrder(
		deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(editable, nil),
		deps.drafts.EXPECT().CreateDraftVersion(ctx, testGameUUID, testVersionUUID, sessionpkg.DraftStart{FromVersionUUID: testTemplateVersion, TemplateGameUUID: testTemplateGame}).
			Return(sessionpkg.DraftVersion{Outcome: sessionpkg.DraftCreated, Revision: 1}, nil),
	)

	outcome, err := manager.EnsureDraft(ctx, testUserUUID, testGameUUID)

	require.NoError(t, err)
	require.Equal(t, game.EditableVersionFound, outcome)
}

func TestUseTemplate(t *testing.T) {
	ctx := context.Background()
	found := game.Template{Outcome: game.TemplateFound, GameUUID: testTemplateGame, VersionUUID: testTemplateVersion}
	editable := game.EditableVersion{Outcome: game.EditableVersionFound, VersionUUID: testVersionUUID}
	template := sessionpkg.DraftTemplate{GameUUID: testTemplateGame, VersionUUID: testTemplateVersion}

	t.Run("switches_the_draft_to_the_template", func(t *testing.T) {
		manager, deps := newTestManager(t)
		applied := sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeApplied, Revision: 4}
		gomock.InOrder(
			deps.templates.EXPECT().FindTemplate(ctx, testTemplateGame).Return(found, nil),
			deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(editable, nil),
			deps.drafts.EXPECT().ApplyTemplate(ctx, testVersionUUID, template, int64(3), testKey, "mcp").Return(applied, nil),
		)

		edit, err := manager.UseTemplate(ctx, testUserUUID, testGameUUID, testTemplateGame, 3, testKey, "mcp")

		require.NoError(t, err)
		require.Equal(t, DraftEdit{Access: game.EditableVersionFound, Result: applied}, edit)
	})

	t.Run("a_template_play_cannot_copy_is_not_found", func(t *testing.T) {
		manager, deps := newTestManager(t)
		gomock.InOrder(
			deps.templates.EXPECT().FindTemplate(ctx, testTemplateGame).Return(found, nil),
			deps.editions.EXPECT().FindEditableVersion(ctx, testGameUUID, testUserUUID).Return(editable, nil),
			deps.drafts.EXPECT().ApplyTemplate(ctx, testVersionUUID, template, int64(3), testKey, "mcp").Return(sessionpkg.DraftChangeResult{}, sessionpkg.ErrDraftTemplateNotFound),
		)

		_, err := manager.UseTemplate(ctx, testUserUUID, testGameUUID, testTemplateGame, 3, testKey, "mcp")

		require.ErrorIs(t, err, ErrTemplateNotFound)
	})
}
