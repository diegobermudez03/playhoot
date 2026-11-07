package gameauthoring

import (
	"context"
	"fmt"

	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

var (
	draftRepairOutcomeLabel = metrics.NewLabel("outcome", "repaired", "already_present", "failed")

	draftRepairsTotal = metrics.NewCounter(
		"orchestrator_game_authoring_draft_repairs_total",
		"Versions being built that an operation found missing in Play and created again, by outcome.",
		draftRepairOutcomeLabel,
	)
)

// EnsureDraft creates in Play the version of gameUUID being built when it is
// missing there, as when its creation stopped after Game Management created
// the game. It answers Game Management's edit decision for userUUID.
func (m *Manager) EnsureDraft(ctx context.Context, userUUID string, gameUUID string) (game.EditableVersionOutcome, error) {
	defer logging.Step(ctx, "orchestrator.ensure_draft").Close()

	editable, err := m.editableVersion(ctx, gameUUID, userUUID)
	if err != nil {
		return "", err
	}
	logging.LogFields(ctx, logging.Field("edit_access", string(editable.Outcome)))
	if editable.Outcome != game.EditableVersionFound {
		return editable.Outcome, nil
	}
	if err := m.repairDraft(ctx, gameUUID, editable); err != nil {
		return "", err
	}
	return editable.Outcome, nil
}

// repairDraft creates versionUUID in Play for gameUUID, which is idempotent
// by version uuid, so it needs no request key of its own.
func (m *Manager) repairDraft(ctx context.Context, gameUUID string, editable game.EditableVersion) error {
	created, err := m.createDraft(ctx, gameUUID, editable.VersionUUID, draftStart(editable))
	if err != nil {
		draftRepairsTotal.Inc(ctx, draftRepairOutcomeLabel.Value("failed"))
		return err
	}
	outcome := "repaired"
	if created.Outcome == sessionpkg.DraftAlreadyCreated {
		outcome = "already_present"
	}
	draftRepairsTotal.Inc(ctx, draftRepairOutcomeLabel.Value(outcome))
	logging.LogFields(ctx, logging.Field("draft_repair", outcome))
	return nil
}

// createDraft creates versionUUID in Play for gameUUID. Another version being
// built for the same game is broken data: Game Management has one open
// edition per game, and it names this version.
func (m *Manager) createDraft(ctx context.Context, gameUUID string, versionUUID string, start sessionpkg.DraftStart) (sessionpkg.DraftVersion, error) {
	draft, err := m.drafts.CreateDraftVersion(ctx, gameUUID, versionUUID, start)
	if err != nil {
		return sessionpkg.DraftVersion{}, fmt.Errorf("creating the version being built: %s", err)
	}
	switch draft.Outcome {
	case sessionpkg.DraftCreated, sessionpkg.DraftAlreadyCreated:
		return draft, nil
	case sessionpkg.DraftAnotherOpen:
		monitoring.Alert(ctx, "play has another version being built than game management's open edition")
		return sessionpkg.DraftVersion{}, fmt.Errorf("game %s already has version %s being built, not %s", gameUUID, draft.VersionUUID, versionUUID)
	default:
		monitoring.Alert(ctx, "play answered an unknown draft creation outcome")
		return sessionpkg.DraftVersion{}, fmt.Errorf("unknown draft creation outcome %q", draft.Outcome)
	}
}
