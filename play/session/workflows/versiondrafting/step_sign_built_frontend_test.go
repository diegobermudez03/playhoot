package versiondrafting

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/build"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/draftsources"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/workflows/versiondrafting/internal/repo"
)

func TestSignBuiltFrontend(t *testing.T) {
	ctx := context.Background()
	versionUUID := "16fd2706-8baf-433b-82eb-8c7fada847da"
	expiresAt := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	version := &internalrepo.Version{ID: 9, UUID: versionUUID, Status: "DRAFT"}
	clientKey, clientSHA, clientSize, ratio := "game-versions/builds/client", "c1", int64(10), "16:9"
	succeeded := &internalrepo.BuiltArtifacts{Revision: 3, Status: string(build.Succeeded), ClientObjectKey: &clientKey, ClientSHA256: &clientSHA, ClientSize: &clientSize, AspectRatio: &ratio}
	project := `{"format":"playhoot.project.v1","assets":{"cardBack":{"type":"image","description":"back"}}}`

	setup := func(t *testing.T) (*Manager, *MocksignBuiltFrontendRepo, *MockassetLinkSigner) {
		ctrl := gomock.NewController(t)
		repo, signer := NewMocksignBuiltFrontendRepo(ctrl), NewMockassetLinkSigner(ctrl)
		return &Manager{signBuiltFrontendRepo: repo, assetLinks: signer, dbServicer: nilDB{}}, repo, signer
	}

	t.Run("signs_the_frontend_and_the_declared_bound_assets", func(t *testing.T) {
		m, repo, signer := setup(t)
		repo.EXPECT().FindVersion(ctx, gomock.Any(), versionUUID).Return(version, nil)
		repo.EXPECT().FindBuiltArtifacts(ctx, gomock.Any(), versionUUID, int64(3)).Return(succeeded, nil)
		repo.EXPECT().FindRevision(ctx, gomock.Any(), uint(9), int64(3)).Return(draftsources.Revision{
			Files:  map[string]draftsources.File{draftsources.ProjectFile: {SHA256: "p1"}},
			Assets: map[string]draftsources.AssetBinding{"cardBack": {SHA256: "a1", Size: 5, MIMEType: "image/png"}, "gone": {SHA256: "a2", Size: 6, MIMEType: "image/png"}},
		}, true, nil)
		repo.EXPECT().FindSourceContents(ctx, gomock.Any(), []string{"p1"}).Return(map[string]string{"p1": project}, nil)
		repo.EXPECT().FindAssetObjectKeys(ctx, gomock.Any(), gomock.Any()).Return(map[string]string{"a1": "game-versions/assets/a1", "a2": "game-versions/assets/a2"}, nil)
		signer.EXPECT().PresignGet(ctx, clientKey, expiresAt).Return("https://signed/client", nil)
		signer.EXPECT().PresignGet(ctx, "game-versions/assets/a1", expiresAt).Return("https://signed/a1", nil)

		access, err := m.SignBuiltFrontend(ctx, versionUUID, 3, expiresAt)

		require.NoError(t, err)
		require.Equal(t, sessionpkg.FrontendAccess{
			URL: "https://signed/client", SHA256: "c1", ExpiresAt: expiresAt,
			Viewport: sessionpkg.Viewport{AspectRatio: "16:9", Width: 1920, Height: 1080},
			Assets:   []sessionpkg.FrontendAsset{{Key: "cardBack", Type: "image", MIMEType: "image/png", SHA256: "a1", Size: 5, URL: "https://signed/a1"}},
		}, access, "an asset no longer declared is not delivered")
	})

	t.Run("a_build_that_did_not_succeed_is_not_signed", func(t *testing.T) {
		m, repo, _ := setup(t)
		repo.EXPECT().FindVersion(ctx, gomock.Any(), versionUUID).Return(version, nil)
		repo.EXPECT().FindBuiltArtifacts(ctx, gomock.Any(), versionUUID, int64(3)).Return(&internalrepo.BuiltArtifacts{Revision: 3, Status: string(build.Failed)}, nil)
		_, err := m.SignBuiltFrontend(ctx, versionUUID, 3, expiresAt)
		require.ErrorIs(t, err, sessionpkg.ErrDraftBuildNotReady)
	})

	t.Run("a_revision_without_a_build", func(t *testing.T) {
		m, repo, _ := setup(t)
		repo.EXPECT().FindVersion(ctx, gomock.Any(), versionUUID).Return(version, nil)
		repo.EXPECT().FindBuiltArtifacts(ctx, gomock.Any(), versionUUID, int64(3)).Return(nil, nil)
		_, err := m.SignBuiltFrontend(ctx, versionUUID, 3, expiresAt)
		require.ErrorIs(t, err, sessionpkg.ErrDraftBuildNotFound)
	})
}
