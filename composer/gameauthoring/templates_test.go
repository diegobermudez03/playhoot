package gameauthoring

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/discovery"
	"github.com/diegobermudez03/playhoot/game"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

const (
	testTemplateGame    = "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
	testTemplateVersion = "6ba7b811-9dad-11d1-80b4-00c04fd430c8"
)

func TestBrowseAndSearchTemplates(t *testing.T) {
	ctx := context.Background()
	manager, deps := newTestManager(t)
	featured := []discovery.TemplateEntry{{GameUUID: testTemplateGame, Name: "Quiz", Featured: true}}
	categories := []discovery.TemplateCategory{{Name: "trivia", Templates: 1}}
	deps.catalog.EXPECT().Featured(ctx).Return(featured, nil)
	deps.catalog.EXPECT().Categories(ctx).Return(categories, nil)
	query := discovery.TemplateQuery{Text: "quiz", Players: 4, Page: 1}
	deps.catalog.EXPECT().Search(ctx, query).Return(discovery.TemplatePage{Templates: featured}, nil)

	browse, err := manager.BrowseTemplates(ctx)
	require.NoError(t, err)
	require.Equal(t, TemplateBrowse{Featured: featured, Categories: categories}, browse)
	page, err := manager.SearchTemplates(ctx, query)
	require.NoError(t, err)
	require.Equal(t, featured, page.Templates)
}

func TestInspectTemplate(t *testing.T) {
	ctx := context.Background()
	found := game.Template{Outcome: game.TemplateFound, GameUUID: testTemplateGame, VersionUUID: testTemplateVersion, Name: "Quiz"}
	brief := `{"format":"playhoot.brief.v1"}`
	code := "export default 1"
	snapshot := sessionpkg.DraftSnapshot{Revision: 9, Files: []sessionpkg.DraftFile{
		{Path: "brief.json", SHA256: "b", Size: int64(len(brief)), Content: &brief},
		{Path: "server/game.ts", SHA256: "s", Size: int64(len(code)), Content: &code},
	}}

	t.Run("its_card_brief_manifest_and_files_never_its_code", func(t *testing.T) {
		manager, deps := newTestManager(t)
		deps.templates.EXPECT().FindTemplate(ctx, testTemplateGame).Return(found, nil)
		deps.drafts.EXPECT().ReadDraft(ctx, testTemplateVersion, int64(0), true).Return(snapshot, nil)
		deps.drafts.EXPECT().ReadBuild(ctx, testTemplateVersion, int64(9), gomock.Any()).Return(sessionpkg.DraftBuild{Manifest: []byte(`{"players":{"min":1,"max":30}}`)}, nil)

		read, err := manager.InspectTemplate(ctx, testTemplateGame)

		require.NoError(t, err)
		require.True(t, read.Found)
		require.Equal(t, found, read.Template)
		require.Equal(t, int64(9), read.Revision)
		require.Equal(t, brief, *read.Brief)
		require.JSONEq(t, `{"players":{"min":1,"max":30}}`, string(read.Manifest))
		for _, file := range read.Files {
			require.Nil(t, file.Content, file.Path)
		}
	})

	t.Run("one_file_of_it_and_the_missing_ones", func(t *testing.T) {
		manager, deps := newTestManager(t)
		deps.templates.EXPECT().FindTemplate(ctx, testTemplateGame).Return(found, nil)
		deps.drafts.EXPECT().ReadDraft(ctx, testTemplateVersion, int64(0), true).Return(snapshot, nil)

		read, err := manager.ReadTemplateFiles(ctx, testTemplateGame, []string{"server/game.ts", "client/x.tsx"})

		require.NoError(t, err)
		require.Len(t, read.Files, 1)
		require.Equal(t, code, *read.Files[0].Content)
		require.Equal(t, []string{"client/x.tsx"}, read.Missing)
	})

	t.Run("a_game_that_is_not_a_template_reads_nothing", func(t *testing.T) {
		manager, deps := newTestManager(t)
		deps.templates.EXPECT().FindTemplate(ctx, testTemplateGame).Return(game.Template{Outcome: game.TemplateNotFound}, nil)

		read, err := manager.InspectTemplate(ctx, testTemplateGame)

		require.NoError(t, err)
		require.False(t, read.Found)
	})
}
