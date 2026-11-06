package versiondrafting

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/play/internal/business/inputrequirements"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/build"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/gameversion"
	"github.com/diegobermudez03/playhoot/play/session/internal/objectstore"
	"github.com/diegobermudez03/playhoot/play/session/internal/testdb"
)

// succeedBuild makes the build of revision of versionUUID a succeeded one
// with stored artifacts, as the build worker would.
func succeedBuild(t *testing.T, db *gorm.DB, store *objectstore.Fake, versionUUID string, revision int64) {
	t.Helper()
	ctx := context.Background()
	server, err := objectstore.PutContent(ctx, store, "game-versions/backend-scripts", []byte("function start(){} // "+versionUUID), "text/javascript")
	require.NoError(t, err)
	client, err := objectstore.PutContent(ctx, store, "game-versions/frontend-packages", []byte("<html>"+versionUUID+"</html>"), "text/html")
	require.NoError(t, err)
	require.NoError(t, db.Exec(`
		UPDATE play_game_version_builds SET status = ?,
			server_object_key = ?, server_sha256 = ?, server_size = ?,
			client_object_key = ?, client_sha256 = ?, client_size = ?,
			manifest = ?, aspect_ratio = ?, languages = ?, uses_keyboard = ?, uses_pointer_hover = ?, uses_pointer_press = ?, finished_at = ?
		WHERE revision = ? AND game_version_id = (SELECT id FROM play_game_versions WHERE uuid = ?)
	`, string(build.Succeeded), server.ObjectKey, server.SHA256, server.Size, client.ObjectKey, client.SHA256, client.Size,
		`{"schema_version":1,"players":{"min":1,"max":4}}`, string(gameversion.AspectRatio16x9), `["es","en"]`, false, false, true, time.Now(), revision, versionUUID).Error)
}

func TestPublishingIntegration(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	store := objectstore.NewFake()
	m := New(db, store, nil)
	ctx := context.Background()

	declaringDraft := func(t *testing.T, gameUUID string) string {
		t.Helper()
		versionUUID := uuid.NewString()
		_, err := m.CreateDraftVersion(ctx, gameUUID, versionUUID, "")
		require.NoError(t, err)
		changed, err := m.ApplyChanges(ctx, versionUUID, sessionpkg.DraftChange{
			BaseRevision: 1, Files: []sessionpkg.DraftFileContent{{Path: "game.project.json", Content: declaringProject}},
			IdempotencyKey: "declare", Origin: sessionpkg.DraftOriginWeb,
		})
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftChangeApplied, changed.Outcome)
		return versionUUID
	}
	upload := func(t *testing.T, versionUUID string, content []byte, key string) int64 {
		t.Helper()
		attached, err := m.UploadAsset(ctx, versionUUID, "cardBack", bytes.NewReader(content), key, sessionpkg.DraftOriginWeb)
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftAssetAttached, attached.Outcome)
		return attached.Revision
	}

	t.Run("a revision without a succeeded build or its required assets is not ready", func(t *testing.T) {
		versionUUID := declaringDraft(t, uuid.NewString())
		readiness, err := m.CheckPublication(ctx, versionUUID)
		require.NoError(t, err)
		require.Equal(t, sessionpkg.PublishReadiness{Outcome: sessionpkg.PublishNotReady, Revision: 2, Blockers: []sessionpkg.PublishBlocker{
			{Code: sessionpkg.PublishBuildNotSucceeded, BuildStatus: sessionpkg.DraftBuildStatus(build.Queued)},
			{Code: sessionpkg.PublishMissingRequiredAssets, Keys: []string{"cardBack"}},
		}}, readiness)
	})

	t.Run("freezing makes the version executable, current and immutable", func(t *testing.T) {
		gameUUID := uuid.NewString()
		versionUUID := declaringDraft(t, gameUUID)
		revision := upload(t, versionUUID, pngBytes, "png")
		succeedBuild(t, db, store, versionUUID, revision)

		readiness, err := m.CheckPublication(ctx, versionUUID)
		require.NoError(t, err)
		require.Equal(t, sessionpkg.PublishReadiness{Outcome: sessionpkg.PublishReady, Revision: revision}, readiness)

		stale, err := m.FreezeVersion(ctx, gameUUID, versionUUID, revision-1)
		require.NoError(t, err)
		require.Equal(t, sessionpkg.FreezeNotReady, stale.Outcome)
		require.Equal(t, []sessionpkg.PublishBlocker{{Code: sessionpkg.PublishRevisionChanged}}, stale.Blockers)

		frozen, err := m.FreezeVersion(ctx, gameUUID, versionUUID, revision)
		require.NoError(t, err)
		require.Equal(t, sessionpkg.FrozenVersion{Outcome: sessionpkg.FreezeFrozen, Revision: revision, Languages: []string{"es", "en"}}, frozen)
		again, err := m.FreezeVersion(ctx, gameUUID, versionUUID, revision)
		require.NoError(t, err)
		require.Equal(t, sessionpkg.FreezeAlreadyFrozen, again.Outcome)

		var row struct {
			Status      string
			Current     bool
			Assets      int
			Devices     string
			AspectRatio string
		}
		require.NoError(t, db.Raw(`
			SELECT v.status, g.current_version_id = v.id AS current, v.aspect_ratio,
				(SELECT COUNT(*) FROM play_game_version_assets a WHERE a.game_version_id = v.id) AS assets,
				(SELECT devices FROM play_game_version_input_requirements r WHERE r.game_version_id = v.id) AS devices
			FROM play_game_versions v JOIN play_games g ON g.id = v.game_id WHERE v.uuid = ?
		`, versionUUID).Scan(&row).Error)
		require.Equal(t, string(gameversion.Executable), row.Status)
		require.True(t, row.Current)
		require.Equal(t, 1, row.Assets)
		require.Equal(t, string(gameversion.AspectRatio16x9), row.AspectRatio)
		// The build's client only presses: it plays on desktop and mobile.
		require.Equal(t, string(inputrequirements.DevicesBoth), row.Devices)

		changed, err := m.ApplyChanges(ctx, versionUUID, sessionpkg.DraftChange{
			BaseRevision: revision, Files: []sessionpkg.DraftFileContent{{Path: "notes.md", Content: "late"}},
			IdempotencyKey: "late", Origin: sessionpkg.DraftOriginWeb,
		})
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftChangeNotEditable, changed.Outcome)

		t.Run("a new edition starts from the frozen sources and assets", func(t *testing.T) {
			nextUUID := uuid.NewString()
			created, err := m.CreateDraftVersion(ctx, gameUUID, nextUUID, versionUUID)
			require.NoError(t, err)
			require.Equal(t, sessionpkg.DraftCreated, created.Outcome)
			published, err := m.ReadDraft(ctx, versionUUID, 0, false)
			require.NoError(t, err)
			next, err := m.ReadDraft(ctx, nextUUID, 1, false)
			require.NoError(t, err)
			require.Equal(t, published.Files, next.Files)
			require.Equal(t, published.Assets[0].SHA256, next.Assets[0].SHA256)
			revisions, err := m.ListRevisions(ctx, nextUUID, 0, 0)
			require.NoError(t, err)
			first := revisions[0]
			require.Equal(t, sessionpkg.DraftRevisionEdition, first.Kind)
			require.Equal(t, sessionpkg.DraftActorPlatform, first.Actor)
			require.Nil(t, first.Parent)
			require.Equal(t, &sessionpkg.DraftRevisionFrom{GameUUID: gameUUID, VersionUUID: versionUUID, Revision: &revision}, first.From, "a new edition records the published version it was copied from")
			require.Equal(t, "sdk-v1", first.SDK)
		})
	})

	t.Run("cleanup deletes only what nothing uses anywhere", func(t *testing.T) {
		gameUUID := uuid.NewString()
		versionUUID := declaringDraft(t, gameUUID)
		replaced := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR-replaced")
		upload(t, versionUUID, replaced, "first")
		kept := upload(t, versionUUID, pngBytes, "second")
		succeedBuild(t, db, store, versionUUID, kept)
		_, err := m.FreezeVersion(ctx, gameUUID, versionUUID, kept)
		require.NoError(t, err)

		// Another game's DRAFT binds the replaced content too, in a revision.
		sharing := declaringDraft(t, uuid.NewString())
		upload(t, sharing, replaced, "shared")

		require.NoError(t, db.Exec(`UPDATE play_asset_objects SET registered_at = ?`, time.Now().Add(-2*cleanupSettle)).Error)
		_, err = m.CleanUnusedObjects(ctx)
		require.NoError(t, err)
		replacedKey := objectstore.LocatorFor(assetObjectPrefix, replaced).ObjectKey
		require.True(t, store.Has(replacedKey), "a DRAFT elsewhere still binds it")
		require.True(t, store.Has(objectstore.LocatorFor(assetObjectPrefix, pngBytes).ObjectKey), "the executable version is delivered with it")

		require.NoError(t, db.Exec(`
			DELETE FROM play_game_version_revisions WHERE game_version_id = (SELECT id FROM play_game_versions WHERE uuid = ?)
		`, sharing).Error)
		cleaned, err := m.CleanUnusedObjects(ctx)
		require.NoError(t, err)
		require.GreaterOrEqual(t, cleaned.Assets, 1)
		require.False(t, store.Has(replacedKey), "nothing uses it any more")
		var deleted int
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM play_asset_objects WHERE object_key = ? AND deleted_at IS NOT NULL`, replacedKey).Scan(&deleted).Error)
		require.Equal(t, 1, deleted, "its record is kept with its deletion time")

		// Its content uploaded again is registered and stored again.
		again := declaringDraft(t, uuid.NewString())
		upload(t, again, replaced, "again")
		require.True(t, store.Has(replacedKey))
	})
}
