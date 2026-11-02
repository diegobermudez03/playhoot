package gameauthoring

import (
	"context"
	"errors"
	"fmt"

	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/logging"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

// DraftEdit is a change to a game's version being built: whether Game
// Management let the user edit it (Access) and, only when it did, what Play
// decided (Result).
type DraftEdit struct {
	Access game.EditableVersionOutcome
	Result sessionpkg.DraftChangeResult
}

// ApplyDraftChanges applies change to the version of gameUUID being built,
// when Game Management lets userUUID edit it. A key reused with another
// change returns sessionpkg.ErrIdempotencyConflict.
func (m *Manager) ApplyDraftChanges(ctx context.Context, userUUID string, gameUUID string, change sessionpkg.DraftChange) (DraftEdit, error) {
	defer logging.Step(ctx, "orchestrator.apply_draft_changes").Close()
	return m.editDraft(ctx, userUUID, gameUUID, func(versionUUID string) (sessionpkg.DraftChangeResult, error) {
		return m.drafts.ApplyChanges(ctx, versionUUID, change)
	})
}

// RestoreDraftRevision makes an earlier revision of the version of gameUUID
// being built its newest one, when Game Management lets userUUID edit it.
func (m *Manager) RestoreDraftRevision(ctx context.Context, userUUID string, gameUUID string, revision int64, baseRevision int64, idempotencyKey string, origin string) (DraftEdit, error) {
	defer logging.Step(ctx, "orchestrator.restore_draft_revision").Close()
	return m.editDraft(ctx, userUUID, gameUUID, func(versionUUID string) (sessionpkg.DraftChangeResult, error) {
		return m.drafts.RestoreRevision(ctx, versionUUID, revision, baseRevision, idempotencyKey, origin)
	})
}

// editDraft runs edit on the version Game Management lets userUUID edit. A
// version Play does not have yet is created, and edit runs once more.
func (m *Manager) editDraft(ctx context.Context, userUUID string, gameUUID string, edit func(versionUUID string) (sessionpkg.DraftChangeResult, error)) (DraftEdit, error) {
	editable, err := m.editableVersion(ctx, gameUUID, userUUID)
	if err != nil {
		return DraftEdit{}, err
	}
	logging.LogFields(ctx, logging.Field("edit_access", string(editable.Outcome)))
	if editable.Outcome != game.EditableVersionFound {
		return DraftEdit{Access: editable.Outcome}, nil
	}

	result, err := edit(editable.VersionUUID)
	if errors.Is(err, sessionpkg.ErrDraftNotFound) {
		if err := m.repairDraft(ctx, gameUUID, editable.VersionUUID); err != nil {
			return DraftEdit{}, err
		}
		result, err = edit(editable.VersionUUID)
	}
	if err != nil {
		if errors.Is(err, sessionpkg.ErrIdempotencyConflict) {
			return DraftEdit{}, err
		}
		return DraftEdit{}, fmt.Errorf("changing the version being built: %s", err)
	}
	logging.LogFields(ctx, logging.Field("draft_outcome", string(result.Outcome)), logging.Field("revision", result.Revision))
	return DraftEdit{Access: editable.Outcome, Result: result}, nil
}
