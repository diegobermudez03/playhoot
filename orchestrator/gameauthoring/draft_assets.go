package gameauthoring

import (
	"context"
	"io"

	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/logging"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

// DraftAssetEdit is an asset attached to or removed from a game's version
// being built: Game Management's decision (Access) and, only when it let the
// user edit, what Play decided (Result).
type DraftAssetEdit struct {
	Access game.EditableVersionOutcome
	Result sessionpkg.DraftAssetResult
}

// UploadDraftAsset sends content as the asset key of the version of
// gameUUID being built, when Game Management lets userUUID edit it; Play
// reads content only once it has the version, so a repair never loses it.
// Errors: sessionpkg.ErrIdempotencyConflict, sessionpkg.ErrUploadsBusy.
func (m *Manager) UploadDraftAsset(ctx context.Context, userUUID string, gameUUID string, key string, content io.Reader, idempotencyKey string, origin string) (DraftAssetEdit, error) {
	defer logging.Step(ctx, "orchestrator.upload_draft_asset").Close()
	return m.editDraftAsset(ctx, userUUID, gameUUID, func(versionUUID string) (sessionpkg.DraftAssetResult, error) {
		return m.drafts.UploadAsset(ctx, versionUUID, key, content, idempotencyKey, origin)
	})
}

// RemoveDraftAsset unbinds the asset key from the version of gameUUID being
// built, when Game Management lets userUUID edit it.
func (m *Manager) RemoveDraftAsset(ctx context.Context, userUUID string, gameUUID string, key string, idempotencyKey string, origin string) (DraftAssetEdit, error) {
	defer logging.Step(ctx, "orchestrator.remove_draft_asset").Close()
	return m.editDraftAsset(ctx, userUUID, gameUUID, func(versionUUID string) (sessionpkg.DraftAssetResult, error) {
		return m.drafts.RemoveAsset(ctx, versionUUID, key, idempotencyKey, origin)
	})
}

// editDraftAsset runs edit on the version Game Management lets userUUID
// edit, repairing a version Play does not have yet.
func (m *Manager) editDraftAsset(ctx context.Context, userUUID string, gameUUID string, edit func(versionUUID string) (sessionpkg.DraftAssetResult, error)) (DraftAssetEdit, error) {
	access, result, err := editVersion(ctx, m, userUUID, gameUUID, edit)
	if err != nil || access != game.EditableVersionFound {
		return DraftAssetEdit{Access: access}, err
	}
	logging.LogFields(ctx, logging.Field("asset_outcome", string(result.Outcome)), logging.Field("revision", result.Revision))
	return DraftAssetEdit{Access: access, Result: result}, nil
}
