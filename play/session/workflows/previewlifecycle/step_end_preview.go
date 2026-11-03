package previewlifecycle

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

//go:generate mockgen -package=previewlifecycle -destination=step_end_preview_mock_test.go . endPreviewRepo

// endPreviewRepo is EndPreview's own narrow persistence contract.
type endPreviewRepo interface {
	EndPreview(ctx context.Context, tx *gorm.DB, previewUUID string, reason string, endedAt time.Time) (bool, error)
}

// endReasons are the reasons a preview may end with.
var endReasons = map[sessionpkg.PreviewEndReason]bool{
	sessionpkg.PreviewEndFinished: true, sessionpkg.PreviewEndIdle: true, sessionpkg.PreviewEndEndedByOwner: true,
	sessionpkg.PreviewEndReplaced: true, sessionpkg.PreviewEndScriptFailure: true, sessionpkg.PreviewEndUnavailable: true,
}

var (
	endReasonLabel = metrics.NewLabel("reason",
		string(sessionpkg.PreviewEndFinished), string(sessionpkg.PreviewEndIdle), string(sessionpkg.PreviewEndEndedByOwner),
		string(sessionpkg.PreviewEndReplaced), string(sessionpkg.PreviewEndScriptFailure), string(sessionpkg.PreviewEndUnavailable),
	)

	previewsEnded = metrics.NewCounter(
		"play_previews_ended_total",
		"Previews ended, by reason; a preview a newer one replaced is counted when it is opened.",
		endReasonLabel,
	)
)

// EndPreview records that previewUUID ended for reason and reports whether it
// was still running; ending an ended preview changes nothing.
func (m *Manager) EndPreview(ctx context.Context, previewUUID string, reason sessionpkg.PreviewEndReason) (bool, error) {
	defer logging.Step(ctx, "previewlifecycle.end_preview").Close()

	parsed, err := uuid.Parse(previewUUID)
	if err != nil {
		return false, fmt.Errorf("preview uuid is not a valid UUID")
	}
	if !endReasons[reason] {
		return false, fmt.Errorf("unknown preview end reason %q", reason)
	}
	ended, err := m.endPreviewRepo.EndPreview(ctx, m.dbServicer.GetDB(), parsed.String(), string(reason), m.now().UTC())
	if err != nil {
		return false, err
	}
	if ended {
		previewsEnded.Inc(ctx, endReasonLabel.Value(string(reason)))
	}
	logging.LogFields(ctx, logging.Field("preview_end_reason", string(reason)), logging.Field("preview_ended_now", ended))
	return ended, nil
}
