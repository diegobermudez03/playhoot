package versiondrafting

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/draftsources"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/gameversion"
	"github.com/diegobermudez03/playhoot/play/session/workflows/versiondrafting/internal/changeattempt"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/workflows/versiondrafting/internal/repo"
	"github.com/diegobermudez03/playhoot/utils"
)

//go:generate mockgen -package=versiondrafting -destination=step_remove_asset_mock_test.go . removeAssetRepo

// removeAssetRepo is RemoveAsset's own narrow persistence contract.
type removeAssetRepo interface {
	FindVersion(ctx context.Context, tx *gorm.DB, versionUUID string) (*internalrepo.Version, error)
	ClaimChangeAttempt(ctx context.Context, tx *gorm.DB, versionID uint, kind string, idempotencyKey string, requestSHA256 string, baseRevision int64, origin string) (uint, *internalrepo.ChangeAttempt, error)
	FindRevision(ctx context.Context, tx *gorm.DB, versionID uint, number int64) (draftsources.Revision, bool, error)
	AdvanceRevision(ctx context.Context, tx *gorm.DB, versionID uint, status string, base int64) (bool, error)
	CreateRevision(ctx context.Context, tx *gorm.DB, versionID uint, number int64, revision draftsources.Revision, source string) error
	QueueBuild(ctx context.Context, tx *gorm.DB, versionID uint, revision int64, buildUUID string, queued string, superseded string) (int, error)
	CompleteChangeAttempt(ctx context.Context, tx *gorm.DB, attemptID uint, outcome string, revision *int64, diagnostics []draftsources.Diagnostic) error
}

// removeRequest is what makes two removals the same request.
type removeRequest struct {
	Kind string `json:"kind"`
	Key  string `json:"key"`
}

// RemoveAsset unbinds key's content from the DRAFT versionUUID in a new
// revision made on the current one; the stored content is kept, so an
// earlier revision can still be restored. Recorded like any attempt.
func (m *Manager) RemoveAsset(ctx context.Context, versionUUID string, key string, idempotencyKey string, origin string) (sessionpkg.DraftAssetResult, error) {
	defer logging.Step(ctx, "versiondrafting.remove_asset").Close()

	parsed, err := uuid.Parse(versionUUID)
	if err != nil {
		return sessionpkg.DraftAssetResult{}, fmt.Errorf("version uuid is not a valid UUID")
	}
	if idempotencyKey == "" {
		return sessionpkg.DraftAssetResult{}, fmt.Errorf("idempotency key is required")
	}
	if !changeattempt.ValidOrigin(origin) {
		return sessionpkg.DraftAssetResult{}, fmt.Errorf("unknown change origin %q", origin)
	}
	fingerprint, err := changeattempt.Fingerprint(removeRequest{Kind: string(sessionpkg.DraftAttemptAssetRemoval), Key: key})
	if err != nil {
		return sessionpkg.DraftAssetResult{}, err
	}

	done, err := utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (committed[sessionpkg.DraftAssetResult], error) {
		return m.removeAssetInTx(ctx, tx, parsed.String(), key, idempotencyKey, fingerprint, origin)
	})
	if err != nil {
		return sessionpkg.DraftAssetResult{}, err
	}
	countSuperseded(ctx, done.superseded)
	result := done.result
	countAssetEdit(ctx, result)
	if result.Outcome == sessionpkg.DraftAssetAttached || result.Outcome == sessionpkg.DraftAssetRemoved {
		if result.Build, err = m.builtRevision(ctx, parsed.String(), result.Revision, result.Replayed); err != nil {
			return sessionpkg.DraftAssetResult{}, err
		}
	}
	return result, nil
}

// removeAssetInTx is RemoveAsset's work inside its transaction.
func (m *Manager) removeAssetInTx(ctx context.Context, tx *gorm.DB, versionUUID string, key string, idempotencyKey string, fingerprint string, origin string) (committed[sessionpkg.DraftAssetResult], error) {
	version, err := m.removeAssetRepo.FindVersion(ctx, tx, versionUUID)
	if err != nil {
		return committed[sessionpkg.DraftAssetResult]{}, err
	}
	if version == nil {
		return committed[sessionpkg.DraftAssetResult]{}, fmt.Errorf("version %s: %w", versionUUID, sessionpkg.ErrDraftNotFound)
	}
	var current int64
	if version.CurrentRevision != nil {
		current = *version.CurrentRevision
	}
	attemptID, existing, err := m.removeAssetRepo.ClaimChangeAttempt(ctx, tx, version.ID, string(sessionpkg.DraftAttemptAssetRemoval), idempotencyKey, fingerprint, current, origin)
	if err != nil {
		return committed[sessionpkg.DraftAssetResult]{}, err
	}
	if existing != nil {
		replayed, err := changeattempt.ReplayAsset(ctx, existing, fingerprint)
		return committed[sessionpkg.DraftAssetResult]{result: replayed}, err
	}

	decided, err := m.decideRemove(ctx, tx, version, key)
	if err != nil {
		return committed[sessionpkg.DraftAssetResult]{}, err
	}
	var superseded int
	if decided.Outcome == sessionpkg.DraftAssetRemoved {
		if superseded, err = m.queueBuild(ctx, tx, m.removeAssetRepo, version.ID, decided.Revision); err != nil {
			return committed[sessionpkg.DraftAssetResult]{}, err
		}
	}
	revision := decided.Revision
	if err := m.removeAssetRepo.CompleteChangeAttempt(ctx, tx, attemptID, string(decided.Outcome), &revision, nil); err != nil {
		return committed[sessionpkg.DraftAssetResult]{}, err
	}
	return committed[sessionpkg.DraftAssetResult]{result: decided, superseded: superseded}, nil
}

// decideRemove decides the removal against the version's current revision
// and, when the key is bound, writes the new revision without it.
func (m *Manager) decideRemove(ctx context.Context, tx *gorm.DB, version *internalrepo.Version, key string) (sessionpkg.DraftAssetResult, error) {
	for race := 0; race < maxRevisionRaces; race++ {
		var current int64
		if version.CurrentRevision != nil {
			current = *version.CurrentRevision
		}
		if gameversion.Status(version.Status) != gameversion.Draft || version.CurrentRevision == nil {
			return sessionpkg.DraftAssetResult{Outcome: sessionpkg.DraftAssetNotEditable, Revision: current}, nil
		}
		revision, found, err := m.removeAssetRepo.FindRevision(ctx, tx, version.ID, current)
		if err != nil {
			return sessionpkg.DraftAssetResult{}, err
		}
		if !found {
			monitoring.Alert(ctx, "play draft version is missing its current revision")
			return sessionpkg.DraftAssetResult{}, fmt.Errorf("version %s has no revision %d", version.UUID, current)
		}
		if _, bound := revision.Assets[key]; !bound {
			return sessionpkg.DraftAssetResult{Outcome: sessionpkg.DraftAssetUnchanged, Revision: current}, nil
		}
		advanced, err := m.removeAssetRepo.AdvanceRevision(ctx, tx, version.ID, string(gameversion.Draft), current)
		if err != nil {
			return sessionpkg.DraftAssetResult{}, err
		}
		if !advanced {
			// A change of the sources committed meanwhile; decide on top of it.
			version, err = m.removeAssetRepo.FindVersion(ctx, tx, version.UUID)
			if err != nil {
				return sessionpkg.DraftAssetResult{}, err
			}
			continue
		}
		assets := make(map[string]draftsources.AssetBinding, len(revision.Assets))
		for other, binding := range revision.Assets {
			if other != key {
				assets[other] = binding
			}
		}
		next := current + 1
		if err := m.removeAssetRepo.CreateRevision(ctx, tx, version.ID, next, draftsources.Revision{Files: revision.Files, Assets: assets}, assetRevisionSource); err != nil {
			return sessionpkg.DraftAssetResult{}, err
		}
		return sessionpkg.DraftAssetResult{Outcome: sessionpkg.DraftAssetRemoved, Revision: next}, nil
	}
	return sessionpkg.DraftAssetResult{}, fmt.Errorf("version %s kept changing while removing an asset", version.UUID)
}
