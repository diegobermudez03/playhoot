package versiondrafting

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/gameversion"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/workflows/versiondrafting/internal/repo"
	"github.com/diegobermudez03/playhoot/utils"
)

//go:generate mockgen -package=versiondrafting -destination=step_clean_unused_objects_mock_test.go . cleanUnusedObjectsRepo,objectDeleter

// cleanUnusedObjectsRepo is CleanUnusedObjects' own narrow persistence
// contract.
type cleanUnusedObjectsRepo interface {
	FindUnusedAssetObjects(ctx context.Context, tx *gorm.DB, cutoff time.Time, status string, limit int) ([]internalrepo.StoredObject, error)
	LockUnusedAssetObject(ctx context.Context, tx *gorm.DB, objectID uint, cutoff time.Time, status string) (*internalrepo.StoredObject, error)
	MarkAssetObjectDeleted(ctx context.Context, tx *gorm.DB, objectID uint, at time.Time) error
	FindUnusedArtifactObjects(ctx context.Context, tx *gorm.DB, cutoff time.Time, status string, limit int) ([]internalrepo.StoredObject, error)
	LockUnusedArtifactObject(ctx context.Context, tx *gorm.DB, objectID uint, cutoff time.Time, status string) (*internalrepo.StoredObject, error)
	MarkArtifactObjectDeleted(ctx context.Context, tx *gorm.DB, objectID uint, at time.Time) error
}

// objectDeleter removes stored content.
type objectDeleter interface {
	Delete(ctx context.Context, key string) error
}

const (
	// cleanupBatch bounds the objects of each kind one cleanup deletes.
	cleanupBatch = 200
	// cleanupSettle is how long after its last registration an object may be
	// taken for unused: an upload or a build may be storing it right now,
	// before any reference to it is committed.
	cleanupSettle = time.Hour
)

var (
	cleanedKindLabel = metrics.NewLabel("kind", "asset", "artifact")

	unusedObjectsDeletedTotal = metrics.NewCounter(
		"play_unused_objects_deleted_total",
		"Stored objects deleted because no executable version and no DRAFT used them any more, by kind.",
		cleanedKindLabel,
	)
	unusedObjectCleanupFailuresTotal = metrics.NewCounter(
		"play_unused_object_cleanup_failures_total",
		"Stored objects a cleanup failed to delete, left for the next cleanup, by kind.",
		cleanedKindLabel,
	)
)

// CleanUnusedObjects deletes a bounded batch of asset and build artifact
// content no executable version and no DRAFT anywhere uses (content is shared
// across games), keeping each record with its deletion time; a failure
// leaves the object for the next cleanup.
func (m *Manager) CleanUnusedObjects(ctx context.Context) (sessionpkg.CleanedObjects, error) {
	defer logging.Step(ctx, "versiondrafting.clean_unused_objects").Close()

	cutoff := m.now().Add(-cleanupSettle)
	draft := string(gameversion.Draft)
	db := m.dbServicer.GetDB()
	var cleaned sessionpkg.CleanedObjects

	assets, err := m.cleanUnusedObjectsRepo.FindUnusedAssetObjects(ctx, db, cutoff, draft, cleanupBatch)
	if err != nil {
		return cleaned, err
	}
	for _, object := range assets {
		deleted, err := m.deleteUnused(ctx, object.ID, func(tx *gorm.DB) (*internalrepo.StoredObject, error) {
			return m.cleanUnusedObjectsRepo.LockUnusedAssetObject(ctx, tx, object.ID, cutoff, draft)
		}, m.cleanUnusedObjectsRepo.MarkAssetObjectDeleted)
		if err != nil {
			unusedObjectCleanupFailuresTotal.Inc(ctx, cleanedKindLabel.Value("asset"))
			logging.LogError(ctx, err)
			continue
		}
		if deleted {
			cleaned.Assets++
			unusedObjectsDeletedTotal.Inc(ctx, cleanedKindLabel.Value("asset"))
		}
	}

	artifacts, err := m.cleanUnusedObjectsRepo.FindUnusedArtifactObjects(ctx, db, cutoff, draft, cleanupBatch)
	if err != nil {
		return cleaned, err
	}
	for _, object := range artifacts {
		deleted, err := m.deleteUnused(ctx, object.ID, func(tx *gorm.DB) (*internalrepo.StoredObject, error) {
			return m.cleanUnusedObjectsRepo.LockUnusedArtifactObject(ctx, tx, object.ID, cutoff, draft)
		}, m.cleanUnusedObjectsRepo.MarkArtifactObjectDeleted)
		if err != nil {
			unusedObjectCleanupFailuresTotal.Inc(ctx, cleanedKindLabel.Value("artifact"))
			logging.LogError(ctx, err)
			continue
		}
		if deleted {
			cleaned.Artifacts++
			unusedObjectsDeletedTotal.Inc(ctx, cleanedKindLabel.Value("artifact"))
		}
	}
	logging.LogFields(ctx, logging.Field("assets_deleted", cleaned.Assets), logging.Field("artifacts_deleted", cleaned.Artifacts))
	return cleaned, nil
}

// deleteUnused deletes one object if it is still unused once locked, which
// keeps a concurrent registration of the same content waiting until the
// deletion is recorded; it reports whether it deleted it.
func (m *Manager) deleteUnused(ctx context.Context, objectID uint, lock func(tx *gorm.DB) (*internalrepo.StoredObject, error), mark func(ctx context.Context, tx *gorm.DB, objectID uint, at time.Time) error) (bool, error) {
	return utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (bool, error) {
		object, err := lock(tx)
		if err != nil || object == nil {
			return false, err
		}
		if err := m.objectDeleter.Delete(ctx, object.ObjectKey); err != nil {
			return false, err
		}
		return true, mark(ctx, tx, objectID, m.now().UTC().Truncate(time.Microsecond))
	})
}
