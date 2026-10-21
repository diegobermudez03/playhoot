package frontendaccess

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/objectstore"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/usecases/frontendaccess/internal/repo"
)

func TestFrontendAccess(t *testing.T) {
	type dependencies struct {
		frontends *MockfrontendFinder
		signer    *MockurlSigner
	}
	type test struct {
		gameVersionUUID string

		expected  sessionpkg.FrontendAccess
		expectErr bool
	}

	const versionUUID = "c1d2e3f4-0000-4000-8000-000000000001"
	ctx := context.Background()
	key, sha, landscape, unoffered := "game-versions/frontend-packages/9f86d081", "9f86d081", "16:9", "21:9"
	expiresAt := time.Date(2026, 10, 17, 12, 15, 0, 0, time.UTC)

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"executable_version_is_granted_a_signed_url": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.frontends.EXPECT().FindFrontend(ctx, versionUUID).Return(&internalrepo.Frontend{Status: "EXECUTABLE", ObjectKey: &key, SHA256: &sha, AspectRatio: &landscape}, nil),
				deps.signer.EXPECT().PresignGet(ctx, key, urlTTL).Return(objectstore.SignedURL{URL: "https://storage.example/f?sig=1", ExpiresAt: expiresAt}, nil),
			)
			return test{
				gameVersionUUID: versionUUID,
				expected: sessionpkg.FrontendAccess{
					URL:       "https://storage.example/f?sig=1",
					SHA256:    sha,
					ExpiresAt: expiresAt,
					Viewport:  sessionpkg.Viewport{AspectRatio: "16:9", Width: 1920, Height: 1080},
				},
			}
		},
		"invalid_version_uuid": func(t *testing.T, deps dependencies) test {
			return test{gameVersionUUID: "not-a-uuid", expectErr: true}
		},
		"unknown_version": func(t *testing.T, deps dependencies) test {
			deps.frontends.EXPECT().FindFrontend(ctx, versionUUID).Return(nil, nil)
			return test{gameVersionUUID: versionUUID, expectErr: true}
		},
		"draft_version_is_not_granted": func(t *testing.T, deps dependencies) test {
			deps.frontends.EXPECT().FindFrontend(ctx, versionUUID).Return(&internalrepo.Frontend{Status: "DRAFT", ObjectKey: &key, SHA256: &sha}, nil)
			return test{gameVersionUUID: versionUUID, expectErr: true}
		},
		"executable_version_without_frontend_is_broken_data": func(t *testing.T, deps dependencies) test {
			deps.frontends.EXPECT().FindFrontend(ctx, versionUUID).Return(&internalrepo.Frontend{Status: "EXECUTABLE"}, nil)
			return test{gameVersionUUID: versionUUID, expectErr: true}
		},
		"executable_version_without_aspect_ratio_is_broken_data": func(t *testing.T, deps dependencies) test {
			deps.frontends.EXPECT().FindFrontend(ctx, versionUUID).Return(&internalrepo.Frontend{Status: "EXECUTABLE", ObjectKey: &key, SHA256: &sha}, nil)
			return test{gameVersionUUID: versionUUID, expectErr: true}
		},
		"executable_version_with_an_aspect_ratio_not_offered_is_broken_data": func(t *testing.T, deps dependencies) test {
			deps.frontends.EXPECT().FindFrontend(ctx, versionUUID).Return(&internalrepo.Frontend{Status: "EXECUTABLE", ObjectKey: &key, SHA256: &sha, AspectRatio: &unoffered}, nil)
			return test{gameVersionUUID: versionUUID, expectErr: true}
		},
		"signing_fails": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.frontends.EXPECT().FindFrontend(ctx, versionUUID).Return(&internalrepo.Frontend{Status: "EXECUTABLE", ObjectKey: &key, SHA256: &sha, AspectRatio: &landscape}, nil),
				deps.signer.EXPECT().PresignGet(ctx, key, urlTTL).Return(objectstore.SignedURL{}, objectstore.ErrNotConfigured),
			)
			return test{gameVersionUUID: versionUUID, expectErr: true}
		},
		"finding_the_version_fails": func(t *testing.T, deps dependencies) test {
			deps.frontends.EXPECT().FindFrontend(ctx, versionUUID).Return(nil, errors.New("database down"))
			return test{gameVersionUUID: versionUUID, expectErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			deps := dependencies{frontends: NewMockfrontendFinder(ctrl), signer: NewMockurlSigner(ctrl)}
			tc := setup(t, deps)
			service := &Service{frontends: deps.frontends, signer: deps.signer, ttl: urlTTL}

			access, err := service.FrontendAccess(ctx, tc.gameVersionUUID)

			if tc.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, access)
		})
	}
}
