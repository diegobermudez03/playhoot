package frontendaccess

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/sessionlifetime"
	"github.com/diegobermudez03/playhoot/play/session/internal/objectstore"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/usecases/frontendaccess/internal/repo"
)

func TestFrontendAccess(t *testing.T) {
	type dependencies struct {
		frontends *MockfrontendFinder
		batches   *MocksignedURLStore
		signer    *MockurlSigner
	}
	type test struct {
		sessionUUID string

		expected    sessionpkg.FrontendAccess
		expectedErr require.ErrorAssertionFunc
	}

	const sessionUUID = "c1d2e3f4-0000-4000-8000-000000000001"
	const versionID = int64(7)
	ctx := context.Background()
	signedAt := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	now := signedAt.Add(400 * time.Millisecond)
	sessionExpiresAt := now.Add(3 * time.Hour)
	signedUntil := signedAt.Add(batchTTL)
	id, executable, draft := versionID, "EXECUTABLE", "DRAFT"
	key, sha, landscape, unoffered := "game-versions/frontend-packages/9f86d081", "9f86d081", "16:9", "21:9"
	shipKey, thrustKey := "game-versions/assets/aa", "game-versions/assets/bb"
	viewport := sessionpkg.Viewport{AspectRatio: "16:9", Width: 1920, Height: 1080}

	frontend := &internalrepo.Frontend{SessionExpiresAt: sessionExpiresAt, GameVersionID: &id, Status: &executable, ObjectKey: &key, SHA256: &sha, AspectRatio: &landscape}
	assets := []internalrepo.Asset{
		{Key: "ship", Type: "image", MIMEType: "image/png", ObjectKey: &shipKey, SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Size: 175},
		{Key: "thrust", Type: "audio", MIMEType: "audio/wav", ObjectKey: &thrustKey, SHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Size: 4844},
		{Key: "unused", Type: "image", MIMEType: "image/png", SHA256: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", Size: 10},
	}
	current := &internalrepo.SignedURLs{
		FrontendURL: "https://storage.example/f?sig=current",
		AssetURLs:   map[string]string{"ship": "https://storage.example/ship?sig=current", "thrust": "https://storage.example/thrust?sig=current"},
		ExpiresAt:   sessionExpiresAt.Add(batchExpiryMargin + time.Second),
	}
	reused := sessionpkg.FrontendAccess{
		URL: current.FrontendURL, SHA256: sha, ExpiresAt: current.ExpiresAt, Viewport: viewport,
		Assets: []sessionpkg.FrontendAsset{
			{Key: "ship", Type: "image", MIMEType: "image/png", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Size: 175, URL: "https://storage.example/ship?sig=current"},
			{Key: "thrust", Type: "audio", MIMEType: "audio/wav", SHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Size: 4844, URL: "https://storage.example/thrust?sig=current"},
		},
	}
	signed := sessionpkg.FrontendAccess{
		URL: "https://storage.example/f?sig=new", SHA256: sha, ExpiresAt: signedUntil, Viewport: viewport,
		Assets: []sessionpkg.FrontendAsset{
			{Key: "ship", Type: "image", MIMEType: "image/png", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Size: 175, URL: "https://storage.example/ship?sig=new"},
			{Key: "thrust", Type: "audio", MIMEType: "audio/wav", SHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Size: 4844, URL: "https://storage.example/thrust?sig=new"},
		},
	}
	newBatch := internalrepo.SignedURLs{
		FrontendURL: signed.URL,
		AssetURLs:   map[string]string{"ship": "https://storage.example/ship?sig=new", "thrust": "https://storage.example/thrust?sig=new"},
		ExpiresAt:   signedUntil,
	}
	expectSigning := func(deps dependencies) {
		deps.signer.EXPECT().PresignGet(ctx, key, signedUntil).Return(signed.URL, nil)
		deps.signer.EXPECT().PresignGet(ctx, shipKey, signedUntil).Return("https://storage.example/ship?sig=new", nil)
		deps.signer.EXPECT().PresignGet(ctx, thrustKey, signedUntil).Return("https://storage.example/thrust?sig=new", nil)
	}
	foundVersion := func(deps dependencies, batch *internalrepo.SignedURLs) {
		deps.frontends.EXPECT().FindFrontend(ctx, sessionUUID).Return(frontend, nil)
		deps.frontends.EXPECT().FindAssets(ctx, versionID).Return(assets, nil)
		deps.batches.EXPECT().FindSignedURLs(ctx, versionID).Return(batch, nil)
	}
	versionWith := func(change func(f *internalrepo.Frontend)) *internalrepo.Frontend {
		changed := *frontend
		change(&changed)
		return &changed
	}

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"the_current_batch_outliving_the_session_is_reused": func(t *testing.T, deps dependencies) test {
			foundVersion(deps, current)
			return test{sessionUUID: sessionUUID, expected: reused}
		},
		"without_a_batch_the_whole_version_is_signed_and_kept": func(t *testing.T, deps dependencies) test {
			foundVersion(deps, nil)
			expectSigning(deps)
			deps.batches.EXPECT().SaveSignedURLs(ctx, versionID, newBatch, signedAt).Return(nil)
			return test{sessionUUID: sessionUUID, expected: signed}
		},
		"a_batch_expiring_within_the_margin_after_the_session_is_signed_again": func(t *testing.T, deps dependencies) test {
			expiring := *current
			expiring.ExpiresAt = sessionExpiresAt.Add(batchExpiryMargin)
			foundVersion(deps, &expiring)
			expectSigning(deps)
			deps.batches.EXPECT().SaveSignedURLs(ctx, versionID, newBatch, signedAt).Return(nil)
			return test{sessionUUID: sessionUUID, expected: signed}
		},
		"a_batch_missing_an_asset_is_signed_again": func(t *testing.T, deps dependencies) test {
			partial := *current
			partial.AssetURLs = map[string]string{"ship": "https://storage.example/ship?sig=current"}
			foundVersion(deps, &partial)
			expectSigning(deps)
			deps.batches.EXPECT().SaveSignedURLs(ctx, versionID, newBatch, signedAt).Return(nil)
			return test{sessionUUID: sessionUUID, expected: signed}
		},
		"a_version_without_assets_has_none": func(t *testing.T, deps dependencies) test {
			deps.frontends.EXPECT().FindFrontend(ctx, sessionUUID).Return(frontend, nil)
			deps.frontends.EXPECT().FindAssets(ctx, versionID).Return(nil, nil)
			deps.batches.EXPECT().FindSignedURLs(ctx, versionID).Return(nil, nil)
			deps.signer.EXPECT().PresignGet(ctx, key, signedUntil).Return(signed.URL, nil)
			deps.batches.EXPECT().SaveSignedURLs(ctx, versionID, internalrepo.SignedURLs{FrontendURL: signed.URL, AssetURLs: map[string]string{}, ExpiresAt: signedUntil}, signedAt).Return(nil)
			return test{sessionUUID: sessionUUID, expected: sessionpkg.FrontendAccess{URL: signed.URL, SHA256: sha, ExpiresAt: signedUntil, Viewport: viewport, Assets: []sessionpkg.FrontendAsset{}}}
		},
		"failing_to_keep_the_batch_still_grants_it": func(t *testing.T, deps dependencies) test {
			foundVersion(deps, nil)
			expectSigning(deps)
			deps.batches.EXPECT().SaveSignedURLs(ctx, versionID, newBatch, signedAt).Return(errors.New("database down"))
			return test{sessionUUID: sessionUUID, expected: signed}
		},
		"signing_the_frontend_fails": func(t *testing.T, deps dependencies) test {
			foundVersion(deps, nil)
			deps.signer.EXPECT().PresignGet(ctx, key, signedUntil).Return("", objectstore.ErrNotConfigured)
			return test{sessionUUID: sessionUUID, expectedErr: require.Error}
		},
		"signing_an_asset_fails": func(t *testing.T, deps dependencies) test {
			foundVersion(deps, nil)
			deps.signer.EXPECT().PresignGet(ctx, key, signedUntil).Return(signed.URL, nil)
			deps.signer.EXPECT().PresignGet(ctx, shipKey, signedUntil).Return("", errors.New("signing service down"))
			return test{sessionUUID: sessionUUID, expectedErr: require.Error}
		},
		"invalid_session_uuid": func(t *testing.T, deps dependencies) test {
			return test{sessionUUID: "not-a-uuid", expectedErr: isSessionNotFound}
		},
		"unknown_session": func(t *testing.T, deps dependencies) test {
			deps.frontends.EXPECT().FindFrontend(ctx, sessionUUID).Return(nil, nil)
			return test{sessionUUID: sessionUUID, expectedErr: isSessionNotFound}
		},
		"pinned_version_missing_is_broken_data": func(t *testing.T, deps dependencies) test {
			deps.frontends.EXPECT().FindFrontend(ctx, sessionUUID).Return(&internalrepo.Frontend{SessionExpiresAt: sessionExpiresAt}, nil)
			return test{sessionUUID: sessionUUID, expectedErr: require.Error}
		},
		"draft_version_is_not_granted": func(t *testing.T, deps dependencies) test {
			deps.frontends.EXPECT().FindFrontend(ctx, sessionUUID).Return(versionWith(func(f *internalrepo.Frontend) { f.Status = &draft }), nil)
			return test{sessionUUID: sessionUUID, expectedErr: require.Error}
		},
		"executable_version_without_frontend_is_broken_data": func(t *testing.T, deps dependencies) test {
			deps.frontends.EXPECT().FindFrontend(ctx, sessionUUID).Return(versionWith(func(f *internalrepo.Frontend) { f.ObjectKey, f.SHA256 = nil, nil }), nil)
			return test{sessionUUID: sessionUUID, expectedErr: require.Error}
		},
		"executable_version_without_aspect_ratio_is_broken_data": func(t *testing.T, deps dependencies) test {
			deps.frontends.EXPECT().FindFrontend(ctx, sessionUUID).Return(versionWith(func(f *internalrepo.Frontend) { f.AspectRatio = nil }), nil)
			return test{sessionUUID: sessionUUID, expectedErr: require.Error}
		},
		"executable_version_with_an_aspect_ratio_not_offered_is_broken_data": func(t *testing.T, deps dependencies) test {
			deps.frontends.EXPECT().FindFrontend(ctx, sessionUUID).Return(versionWith(func(f *internalrepo.Frontend) { f.AspectRatio = &unoffered }), nil)
			return test{sessionUUID: sessionUUID, expectedErr: require.Error}
		},
		"assets_over_the_total_limit_are_broken_data": func(t *testing.T, deps dependencies) test {
			video := func(key string) internalrepo.Asset {
				objectKey := "game-versions/assets/" + key
				return internalrepo.Asset{Key: key, Type: "video", MIMEType: "video/mp4", ObjectKey: &objectKey, SHA256: key, Size: 50 << 20}
			}
			deps.frontends.EXPECT().FindFrontend(ctx, sessionUUID).Return(frontend, nil)
			deps.frontends.EXPECT().FindAssets(ctx, versionID).Return([]internalrepo.Asset{video("a"), video("b"), video("c")}, nil)
			return test{sessionUUID: sessionUUID, expectedErr: require.Error}
		},
		"finding_the_session_fails": func(t *testing.T, deps dependencies) test {
			deps.frontends.EXPECT().FindFrontend(ctx, sessionUUID).Return(nil, errors.New("database down"))
			return test{sessionUUID: sessionUUID, expectedErr: require.Error}
		},
		"finding_the_assets_fails": func(t *testing.T, deps dependencies) test {
			deps.frontends.EXPECT().FindFrontend(ctx, sessionUUID).Return(frontend, nil)
			deps.frontends.EXPECT().FindAssets(ctx, versionID).Return(nil, errors.New("database down"))
			return test{sessionUUID: sessionUUID, expectedErr: require.Error}
		},
		"finding_the_batch_fails": func(t *testing.T, deps dependencies) test {
			deps.frontends.EXPECT().FindFrontend(ctx, sessionUUID).Return(frontend, nil)
			deps.frontends.EXPECT().FindAssets(ctx, versionID).Return(assets, nil)
			deps.batches.EXPECT().FindSignedURLs(ctx, versionID).Return(nil, errors.New("database down"))
			return test{sessionUUID: sessionUUID, expectedErr: require.Error}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			deps := dependencies{frontends: NewMockfrontendFinder(ctrl), batches: NewMocksignedURLStore(ctrl), signer: NewMockurlSigner(ctrl)}
			tc := setup(t, deps)
			service := &Service{frontends: deps.frontends, batches: deps.batches, signer: deps.signer, now: func() time.Time { return now }}

			access, err := service.FrontendAccess(ctx, tc.sessionUUID)

			if tc.expectedErr != nil {
				tc.expectedErr(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, access)
		})
	}
}

func isSessionNotFound(t require.TestingT, err error, _ ...any) {
	require.ErrorIs(t, err, sessionpkg.ErrSessionNotFound)
}

// TestBatchOutlivesEverySession holds the rule that makes a reused batch
// safe: a batch signed while any Session is alive outlives it by the margin,
// and Cloud Storage still accepts signing it.
func TestBatchOutlivesEverySession(t *testing.T) {
	require.Greater(t, batchTTL, sessionlifetime.Max+batchExpiryMargin)
	require.Less(t, batchTTL, objectstore.MaxPresignTTL)
}
