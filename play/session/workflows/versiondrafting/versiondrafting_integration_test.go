package versiondrafting

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/draftsources"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/gameversion"
	"github.com/diegobermudez03/playhoot/play/session/internal/objectstore"
	"github.com/diegobermudez03/playhoot/play/session/internal/testdb"
)

// Integration tests run the drafting workflow end to end against the shared
// disposable database. Every case uses its own random Game and version UUIDs.

func TestDraftingIntegration(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	m := New(db, objectstore.NewFake(), nil)
	ctx := context.Background()

	newDraft := func(t *testing.T) (string, string) {
		t.Helper()
		gameUUID, versionUUID := uuid.NewString(), uuid.NewString()
		created, err := m.CreateDraftVersion(ctx, gameUUID, versionUUID, "")
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftVersion{Outcome: sessionpkg.DraftCreated, GameUUID: gameUUID, VersionUUID: versionUUID, Revision: 1}, created)
		return gameUUID, versionUUID
	}
	write := func(base int64, key string, path string, content string) sessionpkg.DraftChange {
		return sessionpkg.DraftChange{BaseRevision: base, Files: []sessionpkg.DraftFileContent{{Path: path, Content: content}}, IdempotencyKey: key, Origin: sessionpkg.DraftOriginWeb}
	}

	t.Run("every_revision_is_queued_and_only_the_newest_waits", func(t *testing.T) {
		_, versionUUID := newDraft(t)
		first, err := m.ReadBuild(ctx, versionUUID, 1, 0)
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftBuildQueued, first.Status, "the starter revision is queued with the draft")

		for i, content := range []string{"a", "b", "c"} {
			applied, err := m.ApplyChanges(ctx, versionUUID, write(int64(i+1), "coalesce-"+content, "notes.md", content))
			require.NoError(t, err)
			withQueuedBuild(t, applied)
		}
		for revision, status := range map[int64]sessionpkg.DraftBuildStatus{1: sessionpkg.DraftBuildSuperseded, 2: sessionpkg.DraftBuildSuperseded, 3: sessionpkg.DraftBuildSuperseded, 4: sessionpkg.DraftBuildQueued} {
			build, err := m.ReadBuild(ctx, versionUUID, revision, 0)
			require.NoError(t, err)
			require.Equal(t, status, build.Status, "revision %d", revision)
		}
		snapshot, err := m.ReadDraft(ctx, versionUUID, 0, false)
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftBuildQueued, snapshot.Build.Status)
		require.Nil(t, snapshot.LastSuccessfulBuild)

		began := time.Now()
		waited, err := m.ReadBuild(ctx, versionUUID, 0, time.Second)
		require.NoError(t, err)
		require.Equal(t, int64(4), waited.Revision)
		require.GreaterOrEqual(t, time.Since(began), time.Second, "a build that does not end is waited for until the wait passes")

		_, err = m.ReadBuild(ctx, versionUUID, 9, 0)
		require.ErrorIs(t, err, sessionpkg.ErrDraftBuildNotFound)
	})

	t.Run("a_new_draft_holds_the_starter_template", func(t *testing.T) {
		gameUUID, versionUUID := newDraft(t)

		again, err := m.CreateDraftVersion(ctx, gameUUID, versionUUID, "")
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftAlreadyCreated, again.Outcome)

		other, err := m.CreateDraftVersion(ctx, gameUUID, uuid.NewString(), "")
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftVersion{Outcome: sessionpkg.DraftAnotherOpen, GameUUID: gameUUID, VersionUUID: versionUUID, Revision: 1}, other)

		_, err = m.CreateDraftVersion(ctx, uuid.NewString(), versionUUID, "")
		require.ErrorIs(t, err, sessionpkg.ErrDraftVersionConflict)

		snapshot, err := m.ReadDraft(ctx, versionUUID, 0, true)
		require.NoError(t, err)
		require.Equal(t, gameUUID, snapshot.GameUUID)
		require.Equal(t, "DRAFT", snapshot.Status)
		require.Equal(t, int64(1), snapshot.Revision)
		template := draftsources.Template()
		require.Len(t, snapshot.Files, len(template))
		for i, file := range snapshot.Files {
			require.Equal(t, template[i].Path, file.Path)
			require.Equal(t, template[i].Content, *file.Content)
		}

		var version struct {
			Manifest      *string
			ScriptProfile string
		}
		require.NoError(t, db.Raw(`SELECT manifest, script_profile FROM play_game_versions WHERE uuid = ?`, versionUUID).Scan(&version).Error)
		require.Nil(t, version.Manifest)
		require.Equal(t, "sdk-v1", version.ScriptProfile)
	})

	t.Run("changes_conflicts_invalid_changes_and_retries", func(t *testing.T) {
		_, versionUUID := newDraft(t)

		applied, err := m.ApplyChanges(ctx, versionUUID, write(1, "k1", "shared/rules.ts", "export const rounds = 5;\n"))
		require.NoError(t, err)
		require.NotNil(t, applied.Changes, "a write answers its revision's files")
		require.Equal(t, int64(1), applied.Changes.Since)
		require.Equal(t, int64(2), applied.Changes.Revision)
		require.Equal(t, []string{"shared/rules.ts"}, applied.Changes.Added)
		require.Empty(t, applied.Changes.Modified)
		require.Empty(t, applied.Changes.Deleted)
		require.Len(t, applied.Changes.Files, len(draftsources.Template())+1)
		for _, file := range applied.Changes.Files {
			require.Nil(t, file.Content, "a write never answers contents")
			require.NotEmpty(t, file.SHA256)
		}
		require.Equal(t, sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeApplied, Revision: 2}, withQueuedBuild(t, applied))

		replayed, err := m.ApplyChanges(ctx, versionUUID, write(1, "k1", "shared/rules.ts", "export const rounds = 5;\n"))
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeApplied, Revision: 2, Replayed: true}, withQueuedBuild(t, replayed))

		_, err = m.ApplyChanges(ctx, versionUUID, write(1, "k1", "shared/rules.ts", "other"))
		require.ErrorIs(t, err, sessionpkg.ErrIdempotencyConflict)

		conflict, err := m.ApplyChanges(ctx, versionUUID, write(1, "k2", "shared/rules.ts", "late"))
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeConflict, Revision: 2}, conflict)

		invalid, err := m.ApplyChanges(ctx, versionUUID, sessionpkg.DraftChange{BaseRevision: 2, Delete: []string{"server/game.ts"}, IdempotencyKey: "k3", Origin: sessionpkg.DraftOriginMCP})
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftChangeInvalid, invalid.Outcome)
		require.Equal(t, draftsources.CodeRequiredFile, invalid.Diagnostics[0].Code)

		snapshot, err := m.ReadDraft(ctx, versionUUID, 0, false)
		require.NoError(t, err)
		require.Equal(t, int64(2), snapshot.CurrentRevision)
		require.Nil(t, snapshot.Files[0].Content)
		outcomes := make([]string, len(snapshot.Attempts))
		for i, attempt := range snapshot.Attempts {
			outcomes[i] = attempt.Outcome
			require.Equal(t, sessionpkg.DraftAttemptChange, attempt.Kind)
		}
		require.Equal(t, []string{string(sessionpkg.DraftChangeInvalid), string(sessionpkg.DraftChangeConflict), string(sessionpkg.DraftChangeApplied)}, outcomes)
		require.Equal(t, sessionpkg.DraftOriginMCP, snapshot.Attempts[0].Origin)
		require.Nil(t, snapshot.Attempts[1].Revision, "a conflict shows no revision")
		require.Equal(t, int64(2), *snapshot.Attempts[2].Revision)

		first, err := m.ReadDraft(ctx, versionUUID, 1, false)
		require.NoError(t, err)
		require.Len(t, first.Files, len(snapshot.Files)-1)
		_, err = m.ReadDraft(ctx, versionUUID, 3, false)
		require.ErrorIs(t, err, sessionpkg.ErrDraftRevisionNotFound)
	})

	t.Run("edits_change_existing_files_on_their_stored_content", func(t *testing.T) {
		_, versionUUID := newDraft(t)
		applied, err := m.ApplyChanges(ctx, versionUUID, write(1, "e0", "shared/rules.ts", "export const rounds = 5;\nexport const lives = 5;\n"))
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftChangeApplied, applied.Outcome)

		edit := func(key string, edits ...sessionpkg.DraftFileEdit) sessionpkg.DraftChange {
			return sessionpkg.DraftChange{BaseRevision: 2, Edits: edits, IdempotencyKey: key, Origin: sessionpkg.DraftOriginMCP}
		}
		ambiguous, err := m.ApplyChanges(ctx, versionUUID, edit("e1", sessionpkg.DraftFileEdit{Path: "shared/rules.ts", Find: "= 5", Replace: "= 3"}))
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftChangeInvalid, ambiguous.Outcome)
		require.Equal(t, draftsources.CodeEditAmbiguous, ambiguous.Diagnostics[0].Code)
		require.Equal(t, 0, *ambiguous.Diagnostics[0].Edit)

		edited, err := m.ApplyChanges(ctx, versionUUID, edit("e2", sessionpkg.DraftFileEdit{Path: "shared/rules.ts", Find: "rounds = 5", Replace: "rounds = 3"}))
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftChangeApplied, edited.Outcome)
		require.Equal(t, []string{"shared/rules.ts"}, edited.Changes.Modified)
		require.Len(t, edited.Diffs, 1)
		require.Contains(t, edited.Diffs[0].Diff, "+export const rounds = 3;")

		read, err := m.ReadDraft(ctx, versionUUID, 0, true)
		require.NoError(t, err)
		for _, file := range read.Files {
			if file.Path == "shared/rules.ts" {
				require.Equal(t, "export const rounds = 3;\nexport const lives = 5;\n", *file.Content, "the whole edited file is stored")
			}
		}

		replayed, err := m.ApplyChanges(ctx, versionUUID, edit("e2", sessionpkg.DraftFileEdit{Path: "shared/rules.ts", Find: "rounds = 5", Replace: "rounds = 3"}))
		require.NoError(t, err)
		require.True(t, replayed.Replayed)
		require.Nil(t, replayed.Diffs, "a replay answers no diffs")
		_, err = m.ApplyChanges(ctx, versionUUID, edit("e2", sessionpkg.DraftFileEdit{Path: "shared/rules.ts", Find: "rounds = 5", Replace: "rounds = 4"}))
		require.ErrorIs(t, err, sessionpkg.ErrIdempotencyConflict, "the edits are part of the fingerprint")
	})

	t.Run("a_change_or_restore_that_changes_nothing_creates_no_revision", func(t *testing.T) {
		_, versionUUID := newDraft(t)
		const rules = "export const rounds = 5;\n"
		applied, err := m.ApplyChanges(ctx, versionUUID, write(1, "a", "shared/rules.ts", rules))
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftChangeApplied, applied.Outcome)

		same, err := m.ApplyChanges(ctx, versionUUID, write(2, "b", "shared/rules.ts", rules))
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeUnchanged, Revision: 2}, keptUnchanged(t, same))
		replayed, err := m.ApplyChanges(ctx, versionUUID, write(2, "b", "shared/rules.ts", rules))
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeUnchanged, Revision: 2, Replayed: true}, keptUnchanged(t, replayed))

		restoredCurrent, err := m.RestoreRevision(ctx, versionUUID, 2, 2, "c", sessionpkg.DraftOriginWeb)
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeUnchanged, Revision: 2}, keptUnchanged(t, restoredCurrent))

		revisions, err := m.ListRevisions(ctx, versionUUID, 0, 0)
		require.NoError(t, err)
		require.Len(t, revisions, 2)
		snapshot, err := m.ReadDraft(ctx, versionUUID, 0, false)
		require.NoError(t, err)
		require.Equal(t, string(sessionpkg.DraftChangeUnchanged), snapshot.Attempts[0].Outcome)
	})

	t.Run("only_one_of_two_changes_over_the_same_base_applies", func(t *testing.T) {
		_, versionUUID := newDraft(t)
		var wg sync.WaitGroup
		results := make([]sessionpkg.DraftChangeResult, 2)
		for i := range results {
			wg.Add(1)
			go func() {
				defer wg.Done()
				result, err := m.ApplyChanges(ctx, versionUUID, write(1, uuid.NewString(), "shared/race.ts", uuid.NewString()))
				require.NoError(t, err)
				results[i] = result
			}()
		}
		wg.Wait()
		outcomes := []sessionpkg.DraftChangeOutcome{results[0].Outcome, results[1].Outcome}
		require.ElementsMatch(t, []sessionpkg.DraftChangeOutcome{sessionpkg.DraftChangeApplied, sessionpkg.DraftChangeConflict}, outcomes)
	})

	t.Run("restore_makes_a_new_revision_with_the_old_files", func(t *testing.T) {
		_, versionUUID := newDraft(t)
		_, err := m.ApplyChanges(ctx, versionUUID, write(1, "a", "notes.md", "changed"))
		require.NoError(t, err)

		restored, err := m.RestoreRevision(ctx, versionUUID, 1, 2, "r1", sessionpkg.DraftOriginAPI)
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeApplied, Revision: 3}, withQueuedBuild(t, restored))

		missing, err := m.RestoreRevision(ctx, versionUUID, 9, 3, "r2", sessionpkg.DraftOriginAPI)
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftChangeInvalid, missing.Outcome)

		one, err := m.ReadDraft(ctx, versionUUID, 1, false)
		require.NoError(t, err)
		three, err := m.ReadDraft(ctx, versionUUID, 3, false)
		require.NoError(t, err)
		require.Equal(t, one.Files, three.Files)

		revisions, err := m.ListRevisions(ctx, versionUUID, 0, 0)
		require.NoError(t, err)
		sources := make([]string, len(revisions))
		for i, revision := range revisions {
			sources[i] = revision.Source
			require.Equal(t, int64(len(revisions)-i), revision.Number, "newest first")
		}
		require.Equal(t, []string{"restore", "web", "template"}, sources)
		require.Equal(t, len(draftsources.Template()), revisions[2].Files)
		older, err := m.ListRevisions(ctx, versionUUID, 3, 1)
		require.NoError(t, err)
		require.Len(t, older, 1, "a page holds at most limit")
		require.Equal(t, int64(2), older[0].Number, "before excludes itself")
	})

	t.Run("a_version_no_longer_a_draft_is_not_editable", func(t *testing.T) {
		_, versionUUID := newDraft(t)
		require.NoError(t, db.Exec(`UPDATE play_game_versions SET status = ? WHERE uuid = ?`, string(gameversion.Executable), versionUUID).Error)

		result, err := m.ApplyChanges(ctx, versionUUID, write(1, "k", "notes.md", "x"))
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeNotEditable, Revision: 1}, result)
	})

	t.Run("unknown_version", func(t *testing.T) {
		_, err := m.ReadDraft(ctx, uuid.NewString(), 0, false)
		require.ErrorIs(t, err, sessionpkg.ErrDraftNotFound)
		_, err = m.ListRevisions(ctx, uuid.NewString(), 0, 0)
		require.ErrorIs(t, err, sessionpkg.ErrDraftNotFound)
		_, err = m.ApplyChanges(ctx, uuid.NewString(), write(1, "k", "notes.md", "x"))
		require.ErrorIs(t, err, sessionpkg.ErrDraftNotFound)
	})
}

// withQueuedBuild checks a change that made a revision carries its build,
// still queued since no worker runs here, and returns it without the build.
func withQueuedBuild(t *testing.T, result sessionpkg.DraftChangeResult) sessionpkg.DraftChangeResult {
	t.Helper()
	require.NotNil(t, result.Build)
	require.Equal(t, sessionpkg.DraftBuildQueued, result.Build.Status)
	require.Equal(t, result.Revision, result.Build.Revision)
	result.Build = nil
	require.NotNil(t, result.Changes)
	require.Equal(t, result.Revision, result.Changes.Revision)
	result.Changes = nil
	return result
}

// keptUnchanged checks that an unchanged write answers its revision's files
// with nothing changed, and returns the result without them.
func keptUnchanged(t *testing.T, result sessionpkg.DraftChangeResult) sessionpkg.DraftChangeResult {
	t.Helper()
	require.NotNil(t, result.Changes)
	require.Equal(t, result.Revision, result.Changes.Since)
	require.Empty(t, result.Changes.Modified)
	result.Changes = nil
	return result
}
