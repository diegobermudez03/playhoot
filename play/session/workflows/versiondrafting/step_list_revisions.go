package versiondrafting

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/logging"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/workflows/versiondrafting/internal/repo"
	"github.com/diegobermudez03/playhoot/utils"
)

//go:generate mockgen -package=versiondrafting -destination=step_list_revisions_mock_test.go . listRevisionsRepo

// listRevisionsRepo is ListRevisions' own narrow persistence contract.
type listRevisionsRepo interface {
	FindVersion(ctx context.Context, tx *gorm.DB, versionUUID string) (*internalrepo.Version, error)
	ListRevisions(ctx context.Context, tx *gorm.DB, versionID uint, before int64, limit int) ([]internalrepo.Revision, error)
}

// ListRevisions returns versionUUID's revisions numbered below before (from
// the current one when before is 0), newest first, at most limit (at most
// sessionpkg.MaxDraftRevisionsPage, which 0 also means).
func (m *Manager) ListRevisions(ctx context.Context, versionUUID string, before int64, limit int) ([]sessionpkg.DraftRevision, error) {
	defer logging.Step(ctx, "versiondrafting.list_revisions").Close()

	parsed, err := uuid.Parse(versionUUID)
	if err != nil {
		return nil, fmt.Errorf("version uuid is not a valid UUID")
	}
	if before < 0 || limit < 0 {
		return nil, fmt.Errorf("before and limit must not be negative")
	}
	if limit == 0 || limit > sessionpkg.MaxDraftRevisionsPage {
		limit = sessionpkg.MaxDraftRevisionsPage
	}
	return utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) ([]sessionpkg.DraftRevision, error) {
		return m.listRevisionsInTx(ctx, tx, parsed.String(), before, limit)
	})
}

// listRevisionsInTx is ListRevisions' work inside its transaction.
func (m *Manager) listRevisionsInTx(ctx context.Context, tx *gorm.DB, versionUUID string, before int64, limit int) ([]sessionpkg.DraftRevision, error) {
	version, err := m.listRevisionsRepo.FindVersion(ctx, tx, versionUUID)
	if err != nil {
		return nil, err
	}
	if version == nil {
		return nil, fmt.Errorf("version %s: %w", versionUUID, sessionpkg.ErrDraftNotFound)
	}
	revisions, err := m.listRevisionsRepo.ListRevisions(ctx, tx, version.ID, before, limit)
	if err != nil {
		return nil, err
	}
	result := make([]sessionpkg.DraftRevision, len(revisions))
	for i, revision := range revisions {
		result[i] = publicRevision(revision)
	}
	return result, nil
}
