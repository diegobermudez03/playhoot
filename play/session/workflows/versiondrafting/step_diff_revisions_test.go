package versiondrafting

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gorm.io/gorm"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/draftsources"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/workflows/versiondrafting/internal/repo"
)

func TestDiffRevisionsInTx(t *testing.T) {
	const versionUUID = "0f8fad5b-d9cb-469f-a165-70867728950e"
	ctx := context.Background()
	tx := &gorm.DB{}
	current := int64(3)
	version := &internalrepo.Version{ID: 9, UUID: versionUUID, Status: "DRAFT", CurrentRevision: &current}
	file := func(content string) draftsources.File {
		return draftsources.File{SHA256: draftsources.SHA256Hex(content), Size: int64(len(content))}
	}
	one := map[string]draftsources.File{"notes.md": file("a\n"), "shared/old.ts": file("old\n"), "server/game.ts": file("same\n")}
	three := map[string]draftsources.File{"notes.md": file("b\n"), "shared/new.ts": file("new\n"), "server/game.ts": file("same\n")}
	contents := map[string]string{}
	for _, content := range []string{"a\n", "b\n", "old\n", "new\n"} {
		contents[draftsources.SHA256Hex(content)] = content
	}

	t.Run("lists_each_differing_file_with_its_diff", func(t *testing.T) {
		repo := NewMockdiffRevisionsRepo(gomock.NewController(t))
		repo.EXPECT().FindVersion(ctx, tx, versionUUID).Return(version, nil)
		repo.EXPECT().FindRevision(ctx, tx, uint(9), int64(1)).Return(draftsources.Revision{Files: one}, true, nil)
		repo.EXPECT().FindRevision(ctx, tx, uint(9), int64(3)).Return(draftsources.Revision{Files: three}, true, nil)
		repo.EXPECT().FindSourceContents(ctx, tx, gomock.Len(4)).Return(contents, nil)
		m := &Manager{diffRevisionsRepo: repo}

		diff, err := m.diffRevisionsInTx(ctx, tx, versionUUID, 1, 0, "")

		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftRevisionDiff{From: 1, To: 3, Files: []sessionpkg.DraftRevisionFileDiff{
			{Path: "notes.md", Status: "modified", Diff: draftsources.Diff("notes.md", "a\n", "b\n").Diff},
			{Path: "shared/new.ts", Status: "added", Diff: draftsources.Diff("shared/new.ts", "", "new\n").Diff},
			{Path: "shared/old.ts", Status: "deleted", Diff: draftsources.Diff("shared/old.ts", "old\n", "").Diff},
		}}, diff)
	})

	t.Run("only_the_path_asked_for", func(t *testing.T) {
		repo := NewMockdiffRevisionsRepo(gomock.NewController(t))
		repo.EXPECT().FindVersion(ctx, tx, versionUUID).Return(version, nil)
		repo.EXPECT().FindRevision(ctx, tx, uint(9), int64(3)).Return(draftsources.Revision{Files: three}, true, nil)
		repo.EXPECT().FindRevision(ctx, tx, uint(9), int64(1)).Return(draftsources.Revision{Files: one}, true, nil)
		repo.EXPECT().FindSourceContents(ctx, tx, []string{draftsources.SHA256Hex("b\n"), draftsources.SHA256Hex("a\n")}).Return(contents, nil)
		m := &Manager{diffRevisionsRepo: repo}

		diff, err := m.diffRevisionsInTx(ctx, tx, versionUUID, 3, 1, "notes.md")

		require.NoError(t, err)
		require.Equal(t, []sessionpkg.DraftRevisionFileDiff{{Path: "notes.md", Status: "modified", Diff: draftsources.Diff("notes.md", "b\n", "a\n").Diff}}, diff.Files, "either order: backwards undoes")
	})

	t.Run("from_the_empty_project_and_past_the_budget", func(t *testing.T) {
		big := map[string]draftsources.File{}
		stored := map[string]string{}
		for _, name := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"} {
			content := strings.Repeat(name+" line\n", 2000)
			big["shared/"+name+".ts"] = file(content)
			stored[draftsources.SHA256Hex(content)] = content
		}
		repo := NewMockdiffRevisionsRepo(gomock.NewController(t))
		repo.EXPECT().FindVersion(ctx, tx, versionUUID).Return(version, nil)
		repo.EXPECT().FindRevision(ctx, tx, uint(9), int64(2)).Return(draftsources.Revision{Files: big}, true, nil)
		repo.EXPECT().FindSourceContents(ctx, tx, gomock.Len(10)).Return(stored, nil)
		m := &Manager{diffRevisionsRepo: repo}

		diff, err := m.diffRevisionsInTx(ctx, tx, versionUUID, 0, 2, "")

		require.NoError(t, err)
		require.Len(t, diff.Files, 10)
		total := 0
		for i, entry := range diff.Files {
			require.Equal(t, "added", entry.Status)
			total += len(entry.Diff)
			require.Equal(t, i >= 8, entry.Omitted, "eight 8 KiB diffs fill the 64 KiB budget")
		}
		require.LessOrEqual(t, total, sessionpkg.MaxDraftDiffBytes)
	})

	t.Run("a_revision_past_the_current_one", func(t *testing.T) {
		repo := NewMockdiffRevisionsRepo(gomock.NewController(t))
		repo.EXPECT().FindVersion(ctx, tx, versionUUID).Return(version, nil)
		m := &Manager{diffRevisionsRepo: repo}

		_, err := m.diffRevisionsInTx(ctx, tx, versionUUID, 4, 0, "")

		require.ErrorIs(t, err, sessionpkg.ErrDraftRevisionNotFound)
	})
}
