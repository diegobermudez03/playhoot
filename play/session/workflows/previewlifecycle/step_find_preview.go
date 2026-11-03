package previewlifecycle

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/workflows/previewlifecycle/internal/repo"
)

//go:generate mockgen -package=previewlifecycle -destination=step_find_preview_mock_test.go . findPreviewRepo

// findPreviewRepo is FindPreview's own narrow persistence contract.
type findPreviewRepo interface {
	FindPreview(ctx context.Context, tx *gorm.DB, previewUUID string) (*internalrepo.Preview, error)
}

// FindPreview returns previewUUID as recorded, or
// sessionpkg.ErrPreviewNotFound.
func (m *Manager) FindPreview(ctx context.Context, previewUUID string) (sessionpkg.Preview, error) {
	defer logging.Step(ctx, "previewlifecycle.find_preview").Close()

	parsed, err := uuid.Parse(previewUUID)
	if err != nil {
		return sessionpkg.Preview{}, sessionpkg.ErrPreviewNotFound
	}
	stored, err := m.findPreviewRepo.FindPreview(ctx, m.dbServicer.GetDB(), parsed.String())
	if err != nil {
		return sessionpkg.Preview{}, err
	}
	if stored == nil {
		return sessionpkg.Preview{}, sessionpkg.ErrPreviewNotFound
	}
	preview := sessionpkg.Preview{
		UUID: stored.UUID, OwnerUserUUID: stored.OwnerUserUUID, Revision: stored.Revision,
		Participants: participantIDs(stored.Participants), CreatedAt: stored.CreatedAt, ExpiresAt: stored.ExpiresAt,
	}
	if stored.EndedAt != nil {
		preview.Ended = true
		if stored.EndReason == nil || !endReasons[sessionpkg.PreviewEndReason(*stored.EndReason)] {
			monitoring.Alert(ctx, "play preview ended with an unknown reason")
			return sessionpkg.Preview{}, fmt.Errorf("preview %s ended with an unknown reason", previewUUID)
		}
		preview.EndReason = sessionpkg.PreviewEndReason(*stored.EndReason)
	}
	return preview, nil
}
