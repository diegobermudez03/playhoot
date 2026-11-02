package versiondrafting

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/play/runtime/sandbox/scriptcheck"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/draftsources"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/gameversion"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/workflows/versiondrafting/internal/repo"
	"github.com/diegobermudez03/playhoot/utils"
)

//go:generate mockgen -package=versiondrafting -destination=step_create_draft_version_mock_test.go . createDraftVersionRepo

// createDraftVersionRepo is CreateDraftVersion's own narrow persistence
// contract.
type createDraftVersionRepo interface {
	LockGame(ctx context.Context, tx *gorm.DB, gameUUID string) (uint, error)
	FindVersion(ctx context.Context, tx *gorm.DB, versionUUID string) (*internalrepo.Version, error)
	FindVersionOfGameInStatus(ctx context.Context, tx *gorm.DB, gameID uint, status string) (*internalrepo.Version, error)
	CreateVersion(ctx context.Context, tx *gorm.DB, gameID uint, versionUUID string, status string, scriptProfile string, revision int64) (uint, error)
	StoreSourceContents(ctx context.Context, tx *gorm.DB, contents map[string]string) error
	CreateRevision(ctx context.Context, tx *gorm.DB, versionID uint, number int64, files map[string]draftsources.File, source string) error
}

// templateSource is the revision source of a DRAFT's starter revision.
const templateSource = "template"

var (
	createDraftOutcomeLabel = metrics.NewLabel("outcome", "created", "already_created", "another_draft_open")

	draftVersionsCreatedTotal = metrics.NewCounter(
		"play_draft_versions_created_total",
		"Completed DRAFT version creations, by outcome; already_created counts retries of the same version.",
		createDraftOutcomeLabel,
	)
)

// CreateDraftVersion gives gameUUID the DRAFT version versionUUID, which the
// caller chooses, at revision 1 with the starter sources. It is idempotent by
// versionUUID; a versionUUID already naming another game's version, or one
// no longer a DRAFT, is sessionpkg.ErrDraftVersionConflict.
func (m *Manager) CreateDraftVersion(ctx context.Context, gameUUID string, versionUUID string) (sessionpkg.DraftVersion, error) {
	defer logging.Step(ctx, "versiondrafting.create_draft_version").Close()

	parsedGame, err := uuid.Parse(gameUUID)
	if err != nil {
		return sessionpkg.DraftVersion{}, fmt.Errorf("game uuid is not a valid UUID")
	}
	parsedVersion, err := uuid.Parse(versionUUID)
	if err != nil {
		return sessionpkg.DraftVersion{}, fmt.Errorf("version uuid is not a valid UUID")
	}
	created, err := utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (sessionpkg.DraftVersion, error) {
		return m.createDraftVersionInTx(ctx, tx, parsedGame.String(), parsedVersion.String())
	})
	if err != nil {
		return sessionpkg.DraftVersion{}, err
	}
	draftVersionsCreatedTotal.Inc(ctx, createDraftOutcomeLabel.Value(string(created.Outcome)))
	logging.LogFields(ctx, logging.Field("draft_outcome", string(created.Outcome)))
	return created, nil
}

// createDraftVersionInTx is CreateDraftVersion's work inside its
// transaction, with normalized UUIDs.
func (m *Manager) createDraftVersionInTx(ctx context.Context, tx *gorm.DB, gameUUID string, versionUUID string) (sessionpkg.DraftVersion, error) {
	gameID, err := m.createDraftVersionRepo.LockGame(ctx, tx, gameUUID)
	if err != nil {
		return sessionpkg.DraftVersion{}, err
	}
	existing, err := m.createDraftVersionRepo.FindVersion(ctx, tx, versionUUID)
	if err != nil {
		return sessionpkg.DraftVersion{}, err
	}
	if existing != nil {
		if existing.GameUUID != gameUUID || gameversion.Status(existing.Status) != gameversion.Draft {
			return sessionpkg.DraftVersion{}, fmt.Errorf("version %s: %w", versionUUID, sessionpkg.ErrDraftVersionConflict)
		}
		revision, err := draftRevision(ctx, existing)
		if err != nil {
			return sessionpkg.DraftVersion{}, err
		}
		return sessionpkg.DraftVersion{Outcome: sessionpkg.DraftAlreadyCreated, GameUUID: gameUUID, VersionUUID: versionUUID, Revision: revision}, nil
	}

	open, err := m.createDraftVersionRepo.FindVersionOfGameInStatus(ctx, tx, gameID, string(gameversion.Draft))
	if err != nil {
		return sessionpkg.DraftVersion{}, err
	}
	if open != nil {
		// A DRAFT written by hand rather than by this workflow has no
		// revision; it still blocks another one.
		var revision int64
		if open.CurrentRevision != nil {
			revision = *open.CurrentRevision
		}
		return sessionpkg.DraftVersion{Outcome: sessionpkg.DraftAnotherOpen, GameUUID: gameUUID, VersionUUID: open.UUID, Revision: revision}, nil
	}

	starter := draftsources.Apply(nil, draftsources.Template(), nil)
	if len(starter.Diagnostics) > 0 {
		return sessionpkg.DraftVersion{}, fmt.Errorf("the starter template breaks the source rules: %v", starter.Diagnostics)
	}
	versionID, err := m.createDraftVersionRepo.CreateVersion(ctx, tx, gameID, versionUUID, string(gameversion.Draft), string(scriptcheck.ProfileSDKV1), 1)
	if err != nil {
		return sessionpkg.DraftVersion{}, err
	}
	if err := m.createDraftVersionRepo.StoreSourceContents(ctx, tx, starter.Contents); err != nil {
		return sessionpkg.DraftVersion{}, err
	}
	if err := m.createDraftVersionRepo.CreateRevision(ctx, tx, versionID, 1, starter.Files, templateSource); err != nil {
		return sessionpkg.DraftVersion{}, err
	}
	return sessionpkg.DraftVersion{Outcome: sessionpkg.DraftCreated, GameUUID: gameUUID, VersionUUID: versionUUID, Revision: 1}, nil
}

// draftRevision is a DRAFT's current revision; every DRAFT this workflow
// created has one, so a DRAFT without it is broken data.
func draftRevision(ctx context.Context, version *internalrepo.Version) (int64, error) {
	if version.CurrentRevision == nil {
		monitoring.Alert(ctx, "play draft version has no current revision")
		return 0, fmt.Errorf("draft version %s has no current revision", version.UUID)
	}
	return *version.CurrentRevision, nil
}
