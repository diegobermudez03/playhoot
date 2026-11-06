package versiondrafting

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"testing/iotest"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gorm.io/gorm"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/draftsources"
	"github.com/diegobermudez03/playhoot/play/session/internal/objectstore"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/workflows/versiondrafting/internal/repo"
)

const (
	assetVersionUUID = "0f8fad5b-d9cb-469f-a165-70867728950e"
	assetVersionID   = uint(21)
)

func assetDraft(status string, current int64) *internalrepo.Version {
	return &internalrepo.Version{ID: assetVersionID, UUID: assetVersionUUID, GameUUID: "game", Status: status, ScriptProfile: "sdk-v1", CurrentRevision: &current}
}

// declaringRevision is a revision whose project declares cardBack as an
// image, with the project file's content.
func declaringRevision() (draftsources.Revision, map[string]string) {
	project := `{"assets":{"cardBack":{"type":"image","description":"back"}}}`
	hash := draftsources.SHA256Hex(project)
	revision := draftsources.Revision{Files: map[string]draftsources.File{draftsources.ProjectFile: {SHA256: hash, Size: int64(len(project))}}}
	return revision, map[string]string{hash: project}
}

func TestLookUpUpload(t *testing.T) {
	ctx := context.Background()
	tx := &gorm.DB{}

	tests := map[string]struct {
		version  *internalrepo.Version
		key      string
		existing *internalrepo.ChangeAttempt
		refusal  sessionpkg.DraftAssetOutcome
	}{
		"declared":     {version: assetDraft("DRAFT", 2), key: "cardBack"},
		"not_declared": {version: assetDraft("DRAFT", 2), key: "music", refusal: sessionpkg.DraftAssetNotDeclared},
		"not_a_draft":  {version: assetDraft("EXECUTABLE", 2), key: "cardBack", refusal: sessionpkg.DraftAssetNotEditable},
		"a_retry_is_found_with_its_attempt": {
			version: assetDraft("DRAFT", 2), key: "cardBack", existing: &internalrepo.ChangeAttempt{ID: 4},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := NewMockuploadAssetRepo(ctrl)
			m := &Manager{uploadAssetRepo: repo}
			repo.EXPECT().FindVersion(ctx, tx, assetVersionUUID).Return(tc.version, nil)
			repo.EXPECT().FindChangeAttempt(ctx, tx, assetVersionID, "key").Return(tc.existing, nil)
			if tc.refusal != sessionpkg.DraftAssetNotEditable {
				revision, contents := declaringRevision()
				repo.EXPECT().FindRevision(ctx, tx, assetVersionID, int64(2)).Return(revision, true, nil)
				repo.EXPECT().FindSourceContents(ctx, tx, gomock.Any()).Return(contents, nil)
			}

			got, err := m.lookUpUpload(ctx, tx, assetVersionUUID, tc.key, "key")

			require.NoError(t, err)
			require.Equal(t, tc.refusal, got.refusal)
			require.Equal(t, tc.existing, got.existing)
			require.Equal(t, tc.refusal == "", got.declaration != nil)
		})
	}

	t.Run("unknown_version", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		repo := NewMockuploadAssetRepo(ctrl)
		repo.EXPECT().FindVersion(ctx, tx, assetVersionUUID).Return(nil, nil)
		_, err := (&Manager{uploadAssetRepo: repo}).lookUpUpload(ctx, tx, assetVersionUUID, "cardBack", "key")
		require.ErrorIs(t, err, sessionpkg.ErrDraftNotFound)
	})
}

func TestReceive(t *testing.T) {
	image := &draftsources.AssetDeclaration{Type: "image", Description: "back"}
	tests := map[string]struct {
		content  []byte
		limit    int64
		expected sessionpkg.DraftAssetOutcome
		code     string
	}{
		"valid":                   {content: pngBytes, limit: 64},
		"exactly_at_the_limit":    {content: pngBytes, limit: int64(len(pngBytes))},
		"one_byte_past_the_limit": {content: pngBytes, limit: int64(len(pngBytes)) - 1, expected: sessionpkg.DraftAssetTooLarge, code: codeAssetTooLarge},
		"empty":                   {content: nil, limit: 64, expected: sessionpkg.DraftAssetInvalid, code: codeAssetEmpty},
		"audio_declared_as_image": {content: wavBytes, limit: 64, expected: sessionpkg.DraftAssetInvalid, code: codeWrongContentType},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := receive("cardBack", image, bytes.NewReader(tc.content), tc.limit)

			require.NoError(t, err)
			require.Equal(t, tc.expected, got.refusal)
			if tc.code != "" {
				require.Equal(t, tc.code, got.diagnostics[0].Code)
				require.Equal(t, "assets/cardBack", got.diagnostics[0].Path)
				return
			}
			locator := objectstore.LocatorFor(assetObjectPrefix, pngBytes)
			require.Equal(t, draftsources.AssetBinding{SHA256: locator.SHA256, Size: locator.Size, MIMEType: "image/png"}, got.binding)
			require.Equal(t, pngBytes, got.content)
		})
	}

	t.Run("a_failing_reader_is_an_error", func(t *testing.T) {
		_, err := receive("cardBack", image, iotest.ErrReader(errors.New("connection reset")), 64)
		require.Error(t, err)
	})
}

func TestTakeUploadSlot(t *testing.T) {
	m := &Manager{uploadSlots: make(chan struct{}, 1), now: time.Now}
	release, err := m.takeUploadSlot(context.Background())
	require.NoError(t, err)

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = m.takeUploadSlot(cancelled)
	require.ErrorIs(t, err, context.Canceled, "a full instance waits only as long as the request")

	release()
	again, err := m.takeUploadSlot(context.Background())
	require.NoError(t, err, "a released slot is taken again")
	again()
}

func TestRemoveAssetInTx(t *testing.T) {
	ctx := context.Background()
	tx := &gorm.DB{}
	const fingerprint = "fingerprint"
	bound := draftsources.Revision{Assets: map[string]draftsources.AssetBinding{"cardBack": {SHA256: "p", Size: 3}}}
	two := int64(2)
	applied := "removed"

	t.Run("unbinds_the_key_in_a_new_revision", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		repo := NewMockremoveAssetRepo(ctrl)
		repo.EXPECT().FindVersion(ctx, tx, assetVersionUUID).Return(assetDraft("DRAFT", 2), nil)
		repo.EXPECT().ClaimChangeAttempt(ctx, tx, assetVersionID, "asset_removal", "key", fingerprint, int64(2), "web").Return(uint(7), nil, nil)
		repo.EXPECT().FindRevision(ctx, tx, assetVersionID, int64(2)).Return(bound, true, nil)
		repo.EXPECT().AdvanceRevision(ctx, tx, assetVersionID, "DRAFT", int64(2)).Return(true, nil)
		repo.EXPECT().CreateRevision(ctx, tx, assetVersionID, int64(3), draftsources.Revision{Assets: map[string]draftsources.AssetBinding{}}, internalrepo.Provenance{Kind: "asset", Origin: "web", Actor: "person", ParentRevision: &two, SDK: "sdk-v1"}).Return(nil)
		repo.EXPECT().QueueBuild(ctx, tx, assetVersionID, int64(3), "build-uuid", "QUEUED", "SUPERSEDED").Return(0, nil)
		three := int64(3)
		repo.EXPECT().CompleteChangeAttempt(ctx, tx, uint(7), "removed", &three, nil).Return(nil)

		got, err := (&Manager{removeAssetRepo: repo, newUUID: func() string { return "build-uuid" }}).removeAssetInTx(ctx, tx, assetVersionUUID, "cardBack", "key", fingerprint, "web")

		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftAssetResult{Outcome: sessionpkg.DraftAssetRemoved, Revision: 3}, got.result)
	})

	t.Run("a_retry_answers_the_first_decision", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		repo := NewMockremoveAssetRepo(ctrl)
		repo.EXPECT().FindVersion(ctx, tx, assetVersionUUID).Return(assetDraft("DRAFT", 3), nil)
		repo.EXPECT().ClaimChangeAttempt(ctx, tx, assetVersionID, "asset_removal", "key", fingerprint, int64(3), "web").
			Return(uint(0), &internalrepo.ChangeAttempt{ID: 7, RequestSHA256: fingerprint, Outcome: &applied, Revision: &two}, nil)

		got, err := (&Manager{removeAssetRepo: repo, newUUID: func() string { return "build-uuid" }}).removeAssetInTx(ctx, tx, assetVersionUUID, "cardBack", "key", fingerprint, "web")

		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftAssetResult{Outcome: sessionpkg.DraftAssetRemoved, Revision: 2, Replayed: true}, got.result)
		require.Zero(t, got.superseded, "a replay queues nothing")
	})

	t.Run("not_a_draft", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		repo := NewMockremoveAssetRepo(ctrl)
		repo.EXPECT().FindVersion(ctx, tx, assetVersionUUID).Return(assetDraft("EXECUTABLE", 2), nil)
		repo.EXPECT().ClaimChangeAttempt(ctx, tx, assetVersionID, "asset_removal", "key", fingerprint, int64(2), "web").Return(uint(7), nil, nil)
		repo.EXPECT().CompleteChangeAttempt(ctx, tx, uint(7), "not_editable", &two, nil).Return(nil)

		got, err := (&Manager{removeAssetRepo: repo, newUUID: func() string { return "build-uuid" }}).removeAssetInTx(ctx, tx, assetVersionUUID, "cardBack", "key", fingerprint, "web")

		require.NoError(t, err)
		require.Equal(t, sessionpkg.DraftAssetNotEditable, got.result.Outcome)
	})
}
