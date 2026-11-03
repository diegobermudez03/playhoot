package previewlifecycle

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/workflows/previewlifecycle/internal/repo"
	"github.com/diegobermudez03/playhoot/utils"
)

//go:generate mockgen -package=previewlifecycle -destination=step_open_preview_mock_test.go . openPreviewRepo,builtRevisions

// openPreviewRepo is OpenPreview's own narrow persistence contract.
type openPreviewRepo interface {
	LockVersion(ctx context.Context, tx *gorm.DB, versionUUID string) (*uint, error)
	FindRunningPreview(ctx context.Context, tx *gorm.DB, versionID uint) (*string, error)
	EndPreview(ctx context.Context, tx *gorm.DB, previewUUID string, reason string, endedAt time.Time) (bool, error)
	CreatePreview(ctx context.Context, tx *gorm.DB, preview internalrepo.NewPreview) error
}

var (
	openOutcomeLabel = metrics.NewLabel("outcome", string(sessionpkg.PreviewOpened), string(sessionpkg.PreviewBuildNotReady), string(sessionpkg.PreviewInvalidPlayers))

	previewsOpened = metrics.NewCounter(
		"play_previews_opened_total",
		"Requests to open a preview of a draft build, by outcome.",
		openOutcomeLabel,
	)
)

// OpenPreview opens a preview of revision (0: the current one) of the DRAFT
// versionUUID for ownerUUID with players synthetic participants, ending the
// version's running preview as replaced. The revision's build must have
// SUCCEEDED and the player count fit its manifest.
func (m *Manager) OpenPreview(ctx context.Context, versionUUID string, ownerUUID string, revision int64, players int) (sessionpkg.PreviewOpening, error) {
	defer logging.Step(ctx, "previewlifecycle.open_preview").Close()

	parsedVersion, err := uuid.Parse(versionUUID)
	if err != nil {
		return sessionpkg.PreviewOpening{}, fmt.Errorf("version uuid is not a valid UUID")
	}
	parsedOwner, err := uuid.Parse(ownerUUID)
	if err != nil {
		return sessionpkg.PreviewOpening{}, fmt.Errorf("owner uuid is not a valid UUID")
	}
	if revision < 0 {
		return sessionpkg.PreviewOpening{}, fmt.Errorf("revision must not be negative")
	}

	built, err := m.builds.LoadBuiltRevision(ctx, parsedVersion.String(), revision)
	if errors.Is(err, sessionpkg.ErrDraftBuildNotFound) {
		return m.decided(ctx, sessionpkg.PreviewOpening{Outcome: sessionpkg.PreviewBuildNotReady}), nil
	}
	if err != nil {
		return sessionpkg.PreviewOpening{}, err
	}
	if !built.Ready {
		return m.decided(ctx, sessionpkg.PreviewOpening{Outcome: sessionpkg.PreviewBuildNotReady, BuildStatus: built.BuildStatus}), nil
	}
	if players < built.Manifest.MinPlayers || players > built.Manifest.MaxPlayers {
		return m.decided(ctx, sessionpkg.PreviewOpening{
			Outcome: sessionpkg.PreviewInvalidPlayers, MinPlayers: built.Manifest.MinPlayers, MaxPlayers: built.Manifest.MaxPlayers,
		}), nil
	}

	createdAt := m.now().UTC().Truncate(time.Microsecond)
	expiresAt := createdAt.Add(maxLifetime)
	frontend, err := m.builds.SignBuiltFrontend(ctx, parsedVersion.String(), built.Revision, expiresAt.Add(frontendExpiryMargin))
	if err != nil {
		return sessionpkg.PreviewOpening{}, err
	}
	opening := sessionpkg.PreviewOpening{
		Outcome:         sessionpkg.PreviewOpened,
		PreviewUUID:     m.newUUID(),
		Revision:        built.Revision,
		Participants:    participantIDs(players),
		GameVersionUUID: parsedVersion.String(),
		Script:          built.Script,
		ScriptProfile:   built.ScriptProfile,
		Manifest:        built.Manifest,
		Frontend:        frontend,
		ExpiresAt:       expiresAt,
	}
	replaced, err := utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (string, error) {
		return m.recordOpening(ctx, tx, opening, parsedOwner.String(), createdAt)
	})
	if err != nil {
		return sessionpkg.PreviewOpening{}, err
	}
	opening.Replaced = replaced
	logging.LogFields(ctx, logging.Field("preview_uuid", opening.PreviewUUID), logging.Field("replaced_preview_uuid", replaced))
	return m.decided(ctx, opening), nil
}

// recordOpening creates the preview, ending the version's running one, and
// returns that one's uuid ("" when none ran).
func (m *Manager) recordOpening(ctx context.Context, tx *gorm.DB, opening sessionpkg.PreviewOpening, ownerUUID string, createdAt time.Time) (string, error) {
	versionID, err := m.openPreviewRepo.LockVersion(ctx, tx, opening.GameVersionUUID)
	if err != nil {
		return "", err
	}
	if versionID == nil {
		return "", fmt.Errorf("version %s: %w", opening.GameVersionUUID, sessionpkg.ErrDraftNotFound)
	}
	running, err := m.openPreviewRepo.FindRunningPreview(ctx, tx, *versionID)
	if err != nil {
		return "", err
	}
	replaced := ""
	if running != nil {
		if _, err := m.openPreviewRepo.EndPreview(ctx, tx, *running, string(sessionpkg.PreviewEndReplaced), createdAt); err != nil {
			return "", err
		}
		replaced = *running
	}
	err = m.openPreviewRepo.CreatePreview(ctx, tx, internalrepo.NewPreview{
		UUID: opening.PreviewUUID, GameVersionID: *versionID, Revision: opening.Revision, OwnerUserUUID: ownerUUID,
		Participants: len(opening.Participants), CreatedAt: createdAt, ExpiresAt: opening.ExpiresAt,
	})
	if err != nil {
		return "", err
	}
	return replaced, nil
}

// decided counts and returns opening.
func (m *Manager) decided(ctx context.Context, opening sessionpkg.PreviewOpening) sessionpkg.PreviewOpening {
	previewsOpened.Inc(ctx, openOutcomeLabel.Value(string(opening.Outcome)))
	logging.LogFields(ctx, logging.Field("preview_open_outcome", string(opening.Outcome)))
	return opening
}
