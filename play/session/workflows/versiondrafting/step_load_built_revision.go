package versiondrafting

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/build"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/manifest"
	"github.com/diegobermudez03/playhoot/play/session/internal/objectstore"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/workflows/versiondrafting/internal/repo"
)

//go:generate mockgen -package=versiondrafting -destination=step_load_built_revision_mock_test.go . loadBuiltRevisionRepo,scriptLoader

// loadBuiltRevisionRepo is LoadBuiltRevision's own narrow persistence
// contract.
type loadBuiltRevisionRepo interface {
	FindVersion(ctx context.Context, tx *gorm.DB, versionUUID string) (*internalrepo.Version, error)
	FindBuiltArtifacts(ctx context.Context, tx *gorm.DB, versionUUID string, revision int64) (*internalrepo.BuiltArtifacts, error)
}

// scriptLoader reads a build's backend script, verified against its
// locator.
type scriptLoader interface {
	Load(ctx context.Context, locator objectstore.Locator) ([]byte, error)
}

// LoadBuiltRevision returns the build of revision (0: the current one) of
// versionUUID as running it needs: Ready, with its backend script, profile
// and manifest, only when the build SUCCEEDED. A revision without a build is
// sessionpkg.ErrDraftBuildNotFound.
func (m *Manager) LoadBuiltRevision(ctx context.Context, versionUUID string, revision int64) (sessionpkg.BuiltRevision, error) {
	defer logging.Step(ctx, "versiondrafting.load_built_revision").Close()

	parsed, err := uuid.Parse(versionUUID)
	if err != nil {
		return sessionpkg.BuiltRevision{}, fmt.Errorf("version uuid is not a valid UUID")
	}
	if revision < 0 {
		return sessionpkg.BuiltRevision{}, fmt.Errorf("revision must not be negative")
	}
	db := m.dbServicer.GetDB()
	version, err := m.loadBuiltRevisionRepo.FindVersion(ctx, db, parsed.String())
	if err != nil {
		return sessionpkg.BuiltRevision{}, err
	}
	if version == nil {
		return sessionpkg.BuiltRevision{}, fmt.Errorf("version %s: %w", versionUUID, sessionpkg.ErrDraftNotFound)
	}
	if revision == 0 && version.CurrentRevision != nil {
		revision = *version.CurrentRevision
	}
	built, err := m.loadBuiltRevisionRepo.FindBuiltArtifacts(ctx, db, version.UUID, revision)
	if err != nil {
		return sessionpkg.BuiltRevision{}, err
	}
	if built == nil {
		return sessionpkg.BuiltRevision{}, fmt.Errorf("version %s revision %d: %w", versionUUID, revision, sessionpkg.ErrDraftBuildNotFound)
	}
	logging.LogFields(ctx, logging.Field("build_status", built.Status), logging.Field("revision", revision))
	loaded := sessionpkg.BuiltRevision{Revision: built.Revision, BuildStatus: sessionpkg.DraftBuildStatus(built.Status)}
	if build.Status(built.Status) != build.Succeeded {
		return loaded, nil
	}

	if built.ServerObjectKey == nil || built.ServerSHA256 == nil || built.ServerSize == nil {
		monitoring.Alert(ctx, "play draft build succeeded without its backend script")
		return sessionpkg.BuiltRevision{}, fmt.Errorf("version %s revision %d: a succeeded build has no backend script", versionUUID, revision)
	}
	parsedManifest, err := manifest.Parse(built.Manifest)
	if err != nil {
		monitoring.Alert(ctx, "play draft build succeeded without a valid runtime manifest")
		return sessionpkg.BuiltRevision{}, fmt.Errorf("version %s revision %d: %w", versionUUID, revision, err)
	}
	script, err := m.scripts.Load(ctx, objectstore.Locator{ObjectKey: *built.ServerObjectKey, SHA256: *built.ServerSHA256, Size: *built.ServerSize})
	if err != nil {
		return sessionpkg.BuiltRevision{}, fmt.Errorf("loading the build's backend script: %w", err)
	}
	loaded.Ready = true
	loaded.Script = script
	loaded.ScriptProfile = built.ScriptProfile
	loaded.ToolchainVersion = built.ToolchainVersion
	loaded.Manifest = parsedManifest.Runtime()
	return loaded, nil
}
