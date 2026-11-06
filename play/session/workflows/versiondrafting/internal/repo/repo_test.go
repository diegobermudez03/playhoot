package repo

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/play/runtime/sandbox/scriptcheck"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/draftsources"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/gameversion"
	"github.com/diegobermudez03/playhoot/play/session/internal/testdb"
)

// seedDraft inserts a game with a DRAFT version at revision 1.
func seedDraft(t *testing.T, repo *Repo, db *gorm.DB) (string, uint) {
	t.Helper()
	ctx := context.Background()
	gameID, err := repo.LockGame(ctx, db, uuid.NewString())
	require.NoError(t, err)
	versionUUID := uuid.NewString()
	versionID, err := repo.CreateVersion(ctx, db, gameID, versionUUID, string(gameversion.Draft), string(scriptcheck.ProfileSDKV1), 1)
	require.NoError(t, err)
	return versionUUID, versionID
}

func TestVersions(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	repo := New(db)
	ctx := context.Background()

	t.Run("lock_game_inserts_once", func(t *testing.T) {
		gameUUID := uuid.NewString()
		first, err := repo.LockGame(ctx, db, gameUUID)
		require.NoError(t, err)
		second, err := repo.LockGame(ctx, db, gameUUID)
		require.NoError(t, err)
		require.Equal(t, first, second)
	})

	t.Run("find_version_and_the_games_draft", func(t *testing.T) {
		versionUUID, versionID := seedDraft(t, repo, db)
		version, err := repo.FindVersion(ctx, db, versionUUID)
		require.NoError(t, err)
		require.Equal(t, versionID, version.ID)
		require.Equal(t, "DRAFT", version.Status)
		require.Equal(t, int64(1), *version.CurrentRevision)

		var gameID uint
		require.NoError(t, db.Raw(`SELECT game_id FROM play_game_versions WHERE id = ?`, versionID).Scan(&gameID).Error)
		draft, err := repo.FindVersionOfGameInStatus(ctx, db, gameID, string(gameversion.Draft))
		require.NoError(t, err)
		require.Equal(t, versionUUID, draft.UUID)

		missing, err := repo.FindVersion(ctx, db, uuid.NewString())
		require.NoError(t, err)
		require.Nil(t, missing)
	})

	t.Run("advance_revision_only_from_its_base", func(t *testing.T) {
		_, versionID := seedDraft(t, repo, db)
		advanced, err := repo.AdvanceRevision(ctx, db, versionID, string(gameversion.Draft), 1)
		require.NoError(t, err)
		require.True(t, advanced)
		stale, err := repo.AdvanceRevision(ctx, db, versionID, string(gameversion.Draft), 1)
		require.NoError(t, err)
		require.False(t, stale)
		var current int64
		require.NoError(t, db.Raw(`SELECT current_revision FROM play_game_versions WHERE id = ?`, versionID).Scan(&current).Error)
		require.Equal(t, int64(2), current)
	})
}

func TestRevisionsAndContents(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	repo := New(db)
	ctx := context.Background()
	_, versionID := seedDraft(t, repo, db)

	files := map[string]draftsources.File{"notes.md": {SHA256: draftsources.SHA256Hex("hi"), Size: 2}}
	assets := map[string]draftsources.AssetBinding{"cardBack": {SHA256: draftsources.SHA256Hex("png"), Size: 3, MIMEType: "image/png"}}
	require.NoError(t, repo.CreateRevision(ctx, db, versionID, 1, draftsources.Revision{Files: files, Assets: assets}, Provenance{Kind: "template", Actor: "platform", SDK: "sdk-v1"}))
	found, ok, err := repo.FindRevision(ctx, db, versionID, 1)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, draftsources.Revision{Files: files, Assets: assets}, found)
	_, ok, err = repo.FindRevision(ctx, db, versionID, 2)
	require.NoError(t, err)
	require.False(t, ok)

	one, seven := int64(1), int64(7)
	copied := Provenance{
		Kind: "restore", Origin: "mcp", Actor: "ai", ParentRevision: &one, RestoredFrom: &one,
		FromGameUUID: "7c9e6679-7425-40de-944b-e07fc1f90ae7", FromVersionUUID: "0f8fad5b-d9cb-469f-a165-70867728950e", FromRevision: &seven, SDK: "sdk-v1",
	}
	require.NoError(t, repo.CreateRevision(ctx, db, versionID, 2, draftsources.Revision{Files: files}, copied))

	revisions, err := repo.ListRevisions(ctx, db, versionID, 0, 10)
	require.NoError(t, err)
	require.Len(t, revisions, 2)
	require.Equal(t, Revision{Number: 2, Provenance: copied, Files: 1, CreatedAt: revisions[0].CreatedAt}, revisions[0], "every provenance field round-trips")
	require.Equal(t, Revision{Number: 1, Provenance: Provenance{Kind: "template", Actor: "platform", SDK: "sdk-v1"}, Files: 1, CreatedAt: revisions[1].CreatedAt}, revisions[1], "absent fields read back empty")

	contents := map[string]string{draftsources.SHA256Hex("hi"): "hi"}
	require.NoError(t, repo.StoreSourceContents(ctx, db, contents))
	require.NoError(t, repo.StoreSourceContents(ctx, db, contents), "storing the same content again changes nothing")
	stored, err := repo.FindSourceContents(ctx, db, []string{draftsources.SHA256Hex("hi"), "absent"})
	require.NoError(t, err)
	require.Equal(t, contents, stored)
}

func TestChangeAttempts(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	repo := New(db)
	ctx := context.Background()
	_, versionID := seedDraft(t, repo, db)

	claimedID, existing, err := repo.ClaimChangeAttempt(ctx, db, versionID, "asset_upload", "key", "sha", 1, "web")
	require.NoError(t, err)
	require.NotZero(t, claimedID)
	require.Nil(t, existing)

	diagnostics := []draftsources.Diagnostic{{Path: "x", Code: "invalid_path", Message: "m"}}
	revision := int64(1)
	require.NoError(t, repo.CompleteChangeAttempt(ctx, db, claimedID, "invalid", &revision, diagnostics))
	require.Error(t, repo.CompleteChangeAttempt(ctx, db, claimedID, "applied", &revision, nil), "an attempt is completed once")

	againID, again, err := repo.ClaimChangeAttempt(ctx, db, versionID, "change", "key", "other", 1, "api")
	require.NoError(t, err)
	require.Zero(t, againID)
	require.Equal(t, "sha", again.RequestSHA256)
	require.Equal(t, "asset_upload", again.Kind, "the claim keeps the kind it was made with")
	require.Equal(t, "invalid", *again.Outcome)
	require.Equal(t, diagnostics, again.Diagnostics)

	open, _, err := repo.ClaimChangeAttempt(ctx, db, versionID, "change", "open", "sha", 1, "web")
	require.NoError(t, err)
	recent, err := repo.RecentChangeAttempts(ctx, db, versionID, 10)
	require.NoError(t, err)
	require.Len(t, recent, 1, "an attempt still open is not listed")
	require.Equal(t, claimedID, recent[0].ID)
	require.NotZero(t, open)

	found, err := repo.FindChangeAttempt(ctx, db, versionID, "key")
	require.NoError(t, err)
	require.Equal(t, claimedID, found.ID)
	missing, err := repo.FindChangeAttempt(ctx, db, versionID, "never")
	require.NoError(t, err)
	require.Nil(t, missing)
}

func TestAssetObjects(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	repo := New(db)
	ctx := context.Background()

	hash := draftsources.SHA256Hex("png-" + uuid.NewString())
	require.NoError(t, repo.RegisterAssetObject(ctx, db, hash, 3, "image/png", "game-versions/assets/"+hash, time.Now()))
	require.NoError(t, repo.RegisterAssetObject(ctx, db, hash, 3, "image/png", "game-versions/assets/"+hash, time.Now()), "registering the same content again changes nothing")
	keys, err := repo.FindAssetObjectKeys(ctx, db, []string{hash, "absent"})
	require.NoError(t, err)
	require.Equal(t, map[string]string{hash: "game-versions/assets/" + hash}, keys)
}
