package versiondrafting

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/draftsources"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/gameversion"
	"github.com/diegobermudez03/playhoot/play/session/internal/testdb"
)

// Integration tests run the drafting workflow end to end against the shared
// disposable database. Every case uses its own random Game and version UUIDs.

func TestDraftingIntegration(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	m := New(db)
	ctx := context.Background()

	newDraft := func(t *testing.T) (string, string) {
		t.Helper()
		gameUUID, versionUUID := uuid.NewString(), uuid.NewString()
		created, err := m.CreateDraftVersion(ctx, gameUUID, versionUUID)
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftVersion{Outcome: sessionpkg.DraftCreated, GameUUID: gameUUID, VersionUUID: versionUUID, Revision: 1}, created)
		return gameUUID, versionUUID
	}
	write := func(base int64, key string, path string, content string) sessionpkg.DraftChange {
		return sessionpkg.DraftChange{BaseRevision: base, Files: []sessionpkg.DraftFileContent{{Path: path, Content: content}}, IdempotencyKey: key, Origin: sessionpkg.DraftOriginWeb}
	}

	t.Run("a_new_draft_holds_the_starter_template", func(t *testing.T) {
		gameUUID, versionUUID := newDraft(t)

		again, err := m.CreateDraftVersion(ctx, gameUUID, versionUUID)
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftAlreadyCreated, again.Outcome)

		other, err := m.CreateDraftVersion(ctx, gameUUID, uuid.NewString())
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftVersion{Outcome: sessionpkg.DraftAnotherOpen, GameUUID: gameUUID, VersionUUID: versionUUID, Revision: 1}, other)

		_, err = m.CreateDraftVersion(ctx, uuid.NewString(), versionUUID)
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
		require.Equal(t, sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeApplied, Revision: 2}, applied)

		replayed, err := m.ApplyChanges(ctx, versionUUID, write(1, "k1", "shared/rules.ts", "export const rounds = 5;\n"))
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeApplied, Revision: 2, Replayed: true}, replayed)

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
		outcomes := make([]sessionpkg.DraftChangeOutcome, len(snapshot.Attempts))
		for i, attempt := range snapshot.Attempts {
			outcomes[i] = attempt.Outcome
		}
		require.Equal(t, []sessionpkg.DraftChangeOutcome{sessionpkg.DraftChangeInvalid, sessionpkg.DraftChangeConflict, sessionpkg.DraftChangeApplied}, outcomes)
		require.Equal(t, sessionpkg.DraftOriginMCP, snapshot.Attempts[0].Origin)
		require.Nil(t, snapshot.Attempts[1].Revision, "a conflict shows no revision")
		require.Equal(t, int64(2), *snapshot.Attempts[2].Revision)

		first, err := m.ReadDraft(ctx, versionUUID, 1, false)
		require.NoError(t, err)
		require.Len(t, first.Files, len(snapshot.Files)-1)
		_, err = m.ReadDraft(ctx, versionUUID, 3, false)
		require.ErrorIs(t, err, sessionpkg.ErrDraftRevisionNotFound)
	})

	t.Run("a_change_or_restore_that_changes_nothing_creates_no_revision", func(t *testing.T) {
		_, versionUUID := newDraft(t)
		const rules = "export const rounds = 5;\n"
		applied, err := m.ApplyChanges(ctx, versionUUID, write(1, "a", "shared/rules.ts", rules))
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftChangeApplied, applied.Outcome)

		same, err := m.ApplyChanges(ctx, versionUUID, write(2, "b", "shared/rules.ts", rules))
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeUnchanged, Revision: 2}, same)
		replayed, err := m.ApplyChanges(ctx, versionUUID, write(2, "b", "shared/rules.ts", rules))
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeUnchanged, Revision: 2, Replayed: true}, replayed)

		restoredCurrent, err := m.RestoreRevision(ctx, versionUUID, 2, 2, "c", sessionpkg.DraftOriginWeb)
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeUnchanged, Revision: 2}, restoredCurrent)

		revisions, err := m.ListRevisions(ctx, versionUUID)
		require.NoError(t, err)
		require.Len(t, revisions, 2)
		snapshot, err := m.ReadDraft(ctx, versionUUID, 0, false)
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftChangeUnchanged, snapshot.Attempts[0].Outcome)
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
		require.Equal(t, sessionpkg.DraftChangeResult{Outcome: sessionpkg.DraftChangeApplied, Revision: 3}, restored)

		missing, err := m.RestoreRevision(ctx, versionUUID, 9, 3, "r2", sessionpkg.DraftOriginAPI)
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftChangeInvalid, missing.Outcome)

		one, err := m.ReadDraft(ctx, versionUUID, 1, false)
		require.NoError(t, err)
		three, err := m.ReadDraft(ctx, versionUUID, 3, false)
		require.NoError(t, err)
		require.Equal(t, one.Files, three.Files)

		revisions, err := m.ListRevisions(ctx, versionUUID)
		require.NoError(t, err)
		sources := make([]string, len(revisions))
		for i, revision := range revisions {
			sources[i] = revision.Source
			require.Equal(t, int64(i+1), revision.Number)
		}
		require.Equal(t, []string{"template", "web", "restore"}, sources)
		require.Equal(t, len(draftsources.Template()), revisions[0].Files)
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
		_, err = m.ListRevisions(ctx, uuid.NewString())
		require.ErrorIs(t, err, sessionpkg.ErrDraftNotFound)
		_, err = m.ApplyChanges(ctx, uuid.NewString(), write(1, "k", "notes.md", "x"))
		require.ErrorIs(t, err, sessionpkg.ErrDraftNotFound)
	})
}
