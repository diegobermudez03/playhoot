package versiondrafting

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/build"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/draftsources"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/gameversion"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/workflows/versiondrafting/internal/repo"
)

//go:generate mockgen -package=versiondrafting -destination=step_sign_built_frontend_mock_test.go . signBuiltFrontendRepo

// signBuiltFrontendRepo is SignBuiltFrontend's own narrow persistence
// contract.
type signBuiltFrontendRepo interface {
	FindVersion(ctx context.Context, tx *gorm.DB, versionUUID string) (*internalrepo.Version, error)
	FindBuiltArtifacts(ctx context.Context, tx *gorm.DB, versionUUID string, revision int64) (*internalrepo.BuiltArtifacts, error)
	FindRevision(ctx context.Context, tx *gorm.DB, versionID uint, number int64) (draftsources.Revision, bool, error)
	FindSourceContents(ctx context.Context, tx *gorm.DB, hashes []string) (map[string]string, error)
	FindAssetObjectKeys(ctx context.Context, tx *gorm.DB, hashes []string) (map[string]string, error)
}

// SignBuiltFrontend signs, until expiresAt, the frontend of revision's
// SUCCEEDED build of versionUUID and the assets the revision declares and
// binds, for a browser to download them directly: their bytes never pass
// through the backend.
func (m *Manager) SignBuiltFrontend(ctx context.Context, versionUUID string, revision int64, expiresAt time.Time) (sessionpkg.FrontendAccess, error) {
	defer logging.Step(ctx, "versiondrafting.sign_built_frontend").Close()

	parsed, err := uuid.Parse(versionUUID)
	if err != nil {
		return sessionpkg.FrontendAccess{}, fmt.Errorf("version uuid is not a valid UUID")
	}
	db := m.dbServicer.GetDB()
	version, err := m.signBuiltFrontendRepo.FindVersion(ctx, db, parsed.String())
	if err != nil {
		return sessionpkg.FrontendAccess{}, err
	}
	if version == nil {
		return sessionpkg.FrontendAccess{}, fmt.Errorf("version %s: %w", versionUUID, sessionpkg.ErrDraftNotFound)
	}
	built, err := m.signBuiltFrontendRepo.FindBuiltArtifacts(ctx, db, version.UUID, revision)
	if err != nil {
		return sessionpkg.FrontendAccess{}, err
	}
	if built == nil {
		return sessionpkg.FrontendAccess{}, fmt.Errorf("version %s revision %d: %w", versionUUID, revision, sessionpkg.ErrDraftBuildNotFound)
	}
	if build.Status(built.Status) != build.Succeeded {
		return sessionpkg.FrontendAccess{}, fmt.Errorf("version %s revision %d is %s: %w", versionUUID, revision, built.Status, sessionpkg.ErrDraftBuildNotReady)
	}
	if built.ClientObjectKey == nil || built.ClientSHA256 == nil {
		monitoring.Alert(ctx, "play draft build succeeded without its frontend")
		return sessionpkg.FrontendAccess{}, fmt.Errorf("version %s revision %d: a succeeded build has no frontend", versionUUID, revision)
	}
	viewport, ok := builtViewport(built.AspectRatio)
	if !ok {
		monitoring.Alert(ctx, "play draft build succeeded without an offered aspect ratio")
		return sessionpkg.FrontendAccess{}, fmt.Errorf("version %s revision %d: a succeeded build has no offered aspect ratio", versionUUID, revision)
	}

	assets, err := m.boundAssets(ctx, db, version, revision)
	if err != nil {
		return sessionpkg.FrontendAccess{}, err
	}
	url, err := m.assetLinks.PresignGet(ctx, *built.ClientObjectKey, expiresAt)
	if err != nil {
		return sessionpkg.FrontendAccess{}, fmt.Errorf("signing the build's frontend: %w", err)
	}
	access := sessionpkg.FrontendAccess{URL: url, SHA256: *built.ClientSHA256, ExpiresAt: expiresAt.UTC(), Viewport: viewport, Assets: []sessionpkg.FrontendAsset{}}
	for _, asset := range assets {
		link, err := m.assetLinks.PresignGet(ctx, asset.objectKey, expiresAt)
		if err != nil {
			return sessionpkg.FrontendAccess{}, fmt.Errorf("signing asset %s: %w", asset.Key, err)
		}
		asset.URL = link
		access.Assets = append(access.Assets, asset.FrontendAsset)
	}
	return access, nil
}

// signedAsset is a bound asset and the object it is stored at.
type signedAsset struct {
	sessionpkg.FrontendAsset
	objectKey string
}

// boundAssets are the assets revision both declares and binds, by key; one
// bound but no longer declared is not delivered, as the game cannot use it.
func (m *Manager) boundAssets(ctx context.Context, db *gorm.DB, version *internalrepo.Version, revision int64) ([]signedAsset, error) {
	read, found, err := m.signBuiltFrontendRepo.FindRevision(ctx, db, version.ID, revision)
	if err != nil {
		return nil, err
	}
	if !found {
		monitoring.Alert(ctx, "play draft build has no revision")
		return nil, fmt.Errorf("version %s has no revision %d", version.UUID, revision)
	}
	declared, err := declarationsOf(ctx, db, m.signBuiltFrontendRepo, read)
	if err != nil {
		return nil, err
	}
	hashes := make([]string, 0, len(read.Assets))
	for _, binding := range read.Assets {
		hashes = append(hashes, binding.SHA256)
	}
	keys, err := m.signBuiltFrontendRepo.FindAssetObjectKeys(ctx, db, hashes)
	if err != nil {
		return nil, err
	}
	assets := make([]signedAsset, 0, len(read.Assets))
	for key, binding := range read.Assets {
		declaration, isDeclared := declared[key]
		if !isDeclared {
			continue
		}
		objectKey, stored := keys[binding.SHA256]
		if !stored {
			monitoring.Alert(ctx, "play draft revision binds an asset that is not registered")
			return nil, fmt.Errorf("version %s revision %d: asset %s is not registered", version.UUID, revision, key)
		}
		assets = append(assets, signedAsset{
			FrontendAsset: sessionpkg.FrontendAsset{Key: key, Type: string(declaration.Type), MIMEType: binding.MIMEType, SHA256: binding.SHA256, Size: binding.Size},
			objectKey:     objectKey,
		})
	}
	sort.Slice(assets, func(i, j int) bool { return assets[i].Key < assets[j].Key })
	return assets, nil
}

// builtViewport is the logical viewport of a build's aspect ratio.
func builtViewport(aspectRatio *string) (sessionpkg.Viewport, bool) {
	if aspectRatio == nil {
		return sessionpkg.Viewport{}, false
	}
	resolution, ok := gameversion.AspectRatio(*aspectRatio).Resolution()
	if !ok {
		return sessionpkg.Viewport{}, false
	}
	return sessionpkg.Viewport{AspectRatio: *aspectRatio, Width: resolution.Width, Height: resolution.Height}, true
}
