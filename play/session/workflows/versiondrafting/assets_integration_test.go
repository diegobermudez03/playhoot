package versiondrafting

import (
	"bytes"
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/objectstore"
	"github.com/diegobermudez03/playhoot/play/session/internal/testdb"
)

// pngBytes is the smallest content the sniffer reads as a PNG.
var pngBytes = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01")

// wavBytes is the smallest content the sniffer reads as a WAV.
var wavBytes = []byte("RIFF\x24\x00\x00\x00WAVEfmt \x10\x00\x00\x00")

const declaringProject = `{"format":"playhoot.project.v1","name":"Memory","aspectRatio":"16:9","assets":{` +
	`"cardBack":{"type":"image","description":"Card back, 512x768"},` +
	`"flip":{"type":"audio","description":"Flip sound","optional":true}}}`

func TestDraftAssetsIntegration(t *testing.T) {
	db := testdb.OpenSessionDB(t)
	store := objectstore.NewFake()
	m := New(db, store, nil)
	ctx := context.Background()

	declaringDraft := func(t *testing.T) string {
		t.Helper()
		versionUUID := uuid.NewString()
		_, err := m.CreateDraftVersion(ctx, uuid.NewString(), versionUUID)
		require.NoError(t, err)
		changed, err := m.ApplyChanges(ctx, versionUUID, sessionpkg.DraftChange{
			BaseRevision: 1, Files: []sessionpkg.DraftFileContent{{Path: "game.project.json", Content: declaringProject}},
			IdempotencyKey: "declare", Origin: sessionpkg.DraftOriginWeb,
		})
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftChangeApplied, changed.Outcome)
		return versionUUID
	}
	upload := func(versionUUID string, key string, content []byte, idempotencyKey string) (sessionpkg.DraftAssetResult, error) {
		return m.UploadAsset(ctx, versionUUID, key, bytes.NewReader(content), idempotencyKey, sessionpkg.DraftOriginWeb)
	}

	t.Run("an uploaded asset is checked, stored and bound in a new revision", func(t *testing.T) {
		versionUUID := declaringDraft(t)

		snapshot, err := m.ReadDraft(ctx, versionUUID, 0, false)
		require.NoError(t, err)
		require.Len(t, snapshot.Assets, 2)
		require.Equal(t, sessionpkg.DraftAsset{Key: "cardBack", Type: "image", Description: "Card back, 512x768", Status: sessionpkg.DraftAssetMissing}, snapshot.Assets[0])
		require.True(t, snapshot.Assets[1].Optional)

		attached, err := upload(versionUUID, "cardBack", pngBytes, "attach")
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftAssetAttached, attached.Outcome)
		require.Equal(t, int64(3), attached.Revision)
		require.Equal(t, "image/png", attached.Asset.MIMEType)
		stored, err := store.Get(ctx, assetObjectPrefix+"/"+attached.Asset.SHA256)
		require.NoError(t, err)
		require.Equal(t, pngBytes, stored)
		var registered int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM play_asset_objects WHERE sha256 = ?`, attached.Asset.SHA256).Scan(&registered).Error)
		require.Equal(t, int64(1), registered, "every stored object is registered")

		replayed, err := upload(versionUUID, "cardBack", pngBytes, "attach")
		require.NoError(t, err)
		require.True(t, replayed.Replayed)
		require.Equal(t, attached.Revision, replayed.Revision)
		require.Equal(t, attached.Asset.SHA256, replayed.Asset.SHA256)
		_, err = upload(versionUUID, "cardBack", append([]byte(nil), append(pngBytes, 0)...), "attach")
		require.ErrorIs(t, err, sessionpkg.ErrIdempotencyConflict, "the same key with other content")

		again, err := upload(versionUUID, "cardBack", pngBytes, "attach-again")
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftAssetUnchanged, again.Outcome, "the same content bound again changes nothing")

		snapshot, err = m.ReadDraft(ctx, versionUUID, 0, false)
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftAssetReady, snapshot.Assets[0].Status)
		require.Contains(t, snapshot.Assets[0].URL, assetObjectPrefix+"/"+attached.Asset.SHA256)
		require.False(t, snapshot.Assets[0].URLExpiresAt.IsZero())
		require.Equal(t, sessionpkg.DraftAttemptAssetUpload, snapshot.Attempts[0].Kind)

		// A change of the sources keeps the binding.
		changed, err := m.ApplyChanges(ctx, versionUUID, sessionpkg.DraftChange{
			BaseRevision: 3, Files: []sessionpkg.DraftFileContent{{Path: "notes.md", Content: "notes"}},
			IdempotencyKey: "notes", Origin: sessionpkg.DraftOriginWeb,
		})
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftChangeApplied, changed.Outcome)
		snapshot, err = m.ReadDraft(ctx, versionUUID, 0, false)
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftAssetReady, snapshot.Assets[0].Status)

		removed, err := m.RemoveAsset(ctx, versionUUID, "cardBack", "remove", sessionpkg.DraftOriginWeb)
		require.NoError(t, err)
		require.NotNil(t, removed.Build, "the new revision's build is queued with it")
		require.Equal(t, sessionpkg.DraftBuildQueued, removed.Build.Status)
		removed.Build = nil
		require.Equal(t, sessionpkg.DraftAssetResult{Outcome: sessionpkg.DraftAssetRemoved, Revision: 5}, removed)
		notBound, err := m.RemoveAsset(ctx, versionUUID, "cardBack", "remove-again", sessionpkg.DraftOriginWeb)
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftAssetUnchanged, notBound.Outcome)

		restored, err := m.RestoreRevision(ctx, versionUUID, 4, 5, "restore", sessionpkg.DraftOriginWeb)
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftChangeApplied, restored.Outcome)
		snapshot, err = m.ReadDraft(ctx, versionUUID, 0, false)
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftAssetReady, snapshot.Assets[0].Status, "restoring brings the binding back")
		sameAgain, err := m.RestoreRevision(ctx, versionUUID, 4, 6, "restore-same", sessionpkg.DraftOriginWeb)
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftChangeUnchanged, sameAgain.Outcome)
	})

	t.Run("uploads are refused for what their bytes are, not what they claim", func(t *testing.T) {
		versionUUID := declaringDraft(t)

		refused, err := upload(versionUUID, "cardBack", wavBytes, "wrong")
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftAssetInvalid, refused.Outcome)
		require.Equal(t, int64(2), refused.Revision)
		require.Equal(t, "assets/cardBack", refused.Diagnostics[0].Path)
		require.Equal(t, codeWrongContentType, refused.Diagnostics[0].Code)
		_, err = store.Get(ctx, assetObjectPrefix+"/"+objectstore.HashHex(wavBytes))
		require.ErrorIs(t, err, objectstore.ErrNotFound, "refused content is never stored")

		empty, err := upload(versionUUID, "cardBack", nil, "empty")
		require.NoError(t, err)
		require.Equal(t, codeAssetEmpty, empty.Diagnostics[0].Code)

		large := make([]byte, 5<<20+1)
		copy(large, pngBytes)
		tooLarge, err := upload(versionUUID, "cardBack", large, "large")
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftAssetTooLarge, tooLarge.Outcome)
		require.Equal(t, codeAssetTooLarge, tooLarge.Diagnostics[0].Code)

		undeclared, err := upload(versionUUID, "music", pngBytes, "undeclared")
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftAssetNotDeclared, undeclared.Outcome)
		retried, err := upload(versionUUID, "music", pngBytes, "undeclared")
		require.NoError(t, err)
		require.False(t, retried.Replayed, "a refusal before reading keeps the key free")

		snapshot, err := m.ReadDraft(ctx, versionUUID, 0, false)
		require.NoError(t, err)
		require.Equal(t, int64(2), snapshot.CurrentRevision, "nothing refused made a revision")
		require.Equal(t, string(sessionpkg.DraftAssetNotDeclared), snapshot.Attempts[0].Outcome)
		require.Equal(t, string(sessionpkg.DraftAssetTooLarge), snapshot.Attempts[2].Outcome)
	})

	t.Run("an upload to a version Play does not have reads nothing", func(t *testing.T) {
		content := bytes.NewReader(pngBytes)
		_, err := m.UploadAsset(ctx, uuid.NewString(), "cardBack", content, "missing", sessionpkg.DraftOriginWeb)
		require.ErrorIs(t, err, sessionpkg.ErrDraftNotFound)
		require.Equal(t, len(pngBytes), content.Len(), "the content is still unread, so it can be sent again")
	})

	t.Run("an invalid declaration is refused with diagnostics", func(t *testing.T) {
		versionUUID := uuid.NewString()
		_, err := m.CreateDraftVersion(ctx, uuid.NewString(), versionUUID)
		require.NoError(t, err)
		result, err := m.ApplyChanges(ctx, versionUUID, sessionpkg.DraftChange{
			BaseRevision: 1, Files: []sessionpkg.DraftFileContent{{Path: "game.project.json", Content: `{"assets":{"cardBack":{"type":"model","description":"x"}}}`}},
			IdempotencyKey: "bad", Origin: sessionpkg.DraftOriginWeb,
		})
		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftChangeInvalid, result.Outcome)
		require.Equal(t, "game.project.json", result.Diagnostics[0].Path)
	})
}
