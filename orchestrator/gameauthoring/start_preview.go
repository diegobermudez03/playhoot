package gameauthoring

import (
	"context"

	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/play/runtime"
)

//go:generate mockgen -package=gameauthoring -destination=start_preview_mock_test.go . previewStarter

// previewStarter is Play's previews: playing a build of the version being
// built with synthetic participants.
type previewStarter interface {
	StartPreview(ctx context.Context, versionUUID string, ownerUUID string, revision int64, players int) (runtime.StartPreviewResult, error)
}

// PreviewStart is starting a preview of a game's version being built:
// whether Game Management let the user edit it (Access) and, only when it
// did, what Play decided (Result).
type PreviewStart struct {
	Access game.EditableVersionOutcome
	Result runtime.StartPreviewResult
}

// StartPreview starts a preview of revision (0: the current one) of the
// version of gameUUID being built with players synthetic participants, when
// Game Management lets userUUID edit it; the game's earlier preview ends.
func (m *Manager) StartPreview(ctx context.Context, userUUID string, gameUUID string, revision int64, players int) (PreviewStart, error) {
	defer logging.Step(ctx, "orchestrator.start_preview").Close()
	access, result, err := editVersion(ctx, m, userUUID, gameUUID, func(versionUUID string) (runtime.StartPreviewResult, error) {
		return m.previews.StartPreview(ctx, versionUUID, userUUID, revision, players)
	})
	if err != nil || access != game.EditableVersionFound {
		return PreviewStart{Access: access}, err
	}
	logging.LogFields(ctx, logging.Field("preview_outcome", string(result.Outcome)), logging.Field("preview_uuid", result.PreviewUUID))
	return PreviewStart{Access: access, Result: result}, nil
}
