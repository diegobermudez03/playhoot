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
	CreateRevision(ctx context.Context, tx *gorm.DB, versionID uint, number int64, revision draftsources.Revision, provenance internalrepo.Provenance) error
	FindRevision(ctx context.Context, tx *gorm.DB, versionID uint, number int64) (draftsources.Revision, bool, error)
	QueueBuild(ctx context.Context, tx *gorm.DB, versionID uint, revision int64, buildUUID string, queued string, superseded string) (int, error)
}

var (
	createDraftOutcomeLabel = metrics.NewLabel("outcome", "created", "already_created", "another_draft_open")

	draftVersionsCreatedTotal = metrics.NewCounter(
		"play_draft_versions_created_total",
		"Completed DRAFT version creations, by outcome; already_created counts retries of the same version.",
		createDraftOutcomeLabel,
	)
)

// CreateDraftVersion gives gameUUID the DRAFT versionUUID at revision 1: the
// starter sources, or a copy of what start names (an earlier published
// version of the game, or a template's). Idempotent by versionUUID; one
// naming another game's version or no longer a DRAFT is
// sessionpkg.ErrDraftVersionConflict.
func (m *Manager) CreateDraftVersion(ctx context.Context, gameUUID string, versionUUID string, start sessionpkg.DraftStart) (sessionpkg.DraftVersion, error) {
	defer logging.Step(ctx, "versiondrafting.create_draft_version").Close()

	parsedGame, err := uuid.Parse(gameUUID)
	if err != nil {
		return sessionpkg.DraftVersion{}, fmt.Errorf("game uuid is not a valid UUID")
	}
	parsedVersion, err := uuid.Parse(versionUUID)
	if err != nil {
		return sessionpkg.DraftVersion{}, fmt.Errorf("version uuid is not a valid UUID")
	}
	var from sessionpkg.DraftStart
	if start.FromVersionUUID != "" {
		parsedFrom, err := uuid.Parse(start.FromVersionUUID)
		if err != nil {
			return sessionpkg.DraftVersion{}, fmt.Errorf("from version uuid is not a valid UUID")
		}
		from.FromVersionUUID = parsedFrom.String()
	}
	if start.TemplateGameUUID != "" {
		parsedTemplate, err := uuid.Parse(start.TemplateGameUUID)
		if err != nil || from.FromVersionUUID == "" {
			return sessionpkg.DraftVersion{}, fmt.Errorf("a template start names a valid template game and its version")
		}
		from.TemplateGameUUID = parsedTemplate.String()
	}
	done, err := utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (committed[sessionpkg.DraftVersion], error) {
		return m.createDraftVersionInTx(ctx, tx, parsedGame.String(), parsedVersion.String(), from)
	})
	if err != nil {
		return sessionpkg.DraftVersion{}, err
	}
	countSuperseded(ctx, done.superseded)
	created := done.result
	draftVersionsCreatedTotal.Inc(ctx, createDraftOutcomeLabel.Value(string(created.Outcome)))
	if created.Outcome == sessionpkg.DraftCreated {
		m.buildSignal.Notify()
	}
	logging.LogFields(ctx, logging.Field("draft_outcome", string(created.Outcome)))
	return created, nil
}

// createDraftVersionInTx is CreateDraftVersion's work inside its
// transaction, with normalized UUIDs.
func (m *Manager) createDraftVersionInTx(ctx context.Context, tx *gorm.DB, gameUUID string, versionUUID string, from sessionpkg.DraftStart) (committed[sessionpkg.DraftVersion], error) {
	gameID, err := m.createDraftVersionRepo.LockGame(ctx, tx, gameUUID)
	if err != nil {
		return committed[sessionpkg.DraftVersion]{}, err
	}
	existing, err := m.createDraftVersionRepo.FindVersion(ctx, tx, versionUUID)
	if err != nil {
		return committed[sessionpkg.DraftVersion]{}, err
	}
	if existing != nil {
		if existing.GameUUID != gameUUID || gameversion.Status(existing.Status) != gameversion.Draft {
			return committed[sessionpkg.DraftVersion]{}, fmt.Errorf("version %s: %w", versionUUID, sessionpkg.ErrDraftVersionConflict)
		}
		revision, err := draftRevision(ctx, existing)
		if err != nil {
			return committed[sessionpkg.DraftVersion]{}, err
		}
		return committed[sessionpkg.DraftVersion]{result: sessionpkg.DraftVersion{Outcome: sessionpkg.DraftAlreadyCreated, GameUUID: gameUUID, VersionUUID: versionUUID, Revision: revision}}, nil
	}

	open, err := m.createDraftVersionRepo.FindVersionOfGameInStatus(ctx, tx, gameID, string(gameversion.Draft))
	if err != nil {
		return committed[sessionpkg.DraftVersion]{}, err
	}
	if open != nil {
		// A DRAFT written by hand rather than by this workflow has no
		// revision; it still blocks another one.
		var revision int64
		if open.CurrentRevision != nil {
			revision = *open.CurrentRevision
		}
		return committed[sessionpkg.DraftVersion]{result: sessionpkg.DraftVersion{Outcome: sessionpkg.DraftAnotherOpen, GameUUID: gameUUID, VersionUUID: open.UUID, Revision: revision}}, nil
	}

	first, err := m.firstRevision(ctx, tx, gameUUID, from)
	if err != nil {
		return committed[sessionpkg.DraftVersion]{}, err
	}
	versionID, err := m.createDraftVersionRepo.CreateVersion(ctx, tx, gameID, versionUUID, string(gameversion.Draft), string(scriptcheck.ProfileSDKV1), 1)
	if err != nil {
		return committed[sessionpkg.DraftVersion]{}, err
	}
	if len(first.contents) > 0 {
		if err := m.createDraftVersionRepo.StoreSourceContents(ctx, tx, first.contents); err != nil {
			return committed[sessionpkg.DraftVersion]{}, err
		}
	}
	if err := m.createDraftVersionRepo.CreateRevision(ctx, tx, versionID, 1, first.revision, first.provenance); err != nil {
		return committed[sessionpkg.DraftVersion]{}, err
	}
	superseded, err := m.queueBuild(ctx, tx, m.createDraftVersionRepo, versionID, 1)
	if err != nil {
		return committed[sessionpkg.DraftVersion]{}, err
	}
	return committed[sessionpkg.DraftVersion]{result: sessionpkg.DraftVersion{Outcome: sessionpkg.DraftCreated, GameUUID: gameUUID, VersionUUID: versionUUID, Revision: 1}, superseded: superseded}, nil
}

// draftRevision is a DRAFT's current revision; every DRAFT this workflow
// created has one, so a DRAFT without it is broken data.
// startingRevision is a new DRAFT's revision 1, its provenance, and the
// source contents still to store.
type startingRevision struct {
	revision   draftsources.Revision
	provenance internalrepo.Provenance
	contents   map[string]string
}

// firstRevision is a new DRAFT's revision 1: the starter sources, or the
// files and assets of the current revision of the version start names, whose
// contents are already stored.
func (m *Manager) firstRevision(ctx context.Context, tx *gorm.DB, gameUUID string, start sessionpkg.DraftStart) (startingRevision, error) {
	fromVersionUUID := start.FromVersionUUID
	if fromVersionUUID == "" {
		starter := draftsources.Apply(nil, draftsources.Template(), nil)
		if len(starter.Diagnostics) > 0 {
			return startingRevision{}, fmt.Errorf("the starter template breaks the source rules: %v", starter.Diagnostics)
		}
		// The built-in starter is no game's version: it has no From.
		provenance := internalrepo.Provenance{Kind: sessionpkg.DraftRevisionTemplate, Actor: sessionpkg.DraftActorPlatform, SDK: string(scriptcheck.ProfileSDKV1)}
		return startingRevision{revision: draftsources.Revision{Files: starter.Files}, provenance: provenance, contents: starter.Contents}, nil
	}
	// An edition copies its own game's version; a template, the template's.
	sourceGame, kind := gameUUID, sessionpkg.DraftRevisionEdition
	if start.TemplateGameUUID != "" {
		sourceGame, kind = start.TemplateGameUUID, sessionpkg.DraftRevisionTemplate
	}
	from, err := m.createDraftVersionRepo.FindVersion(ctx, tx, fromVersionUUID)
	if err != nil {
		return startingRevision{}, err
	}
	if from == nil || from.GameUUID != sourceGame || gameversion.Status(from.Status) != gameversion.Executable || from.CurrentRevision == nil {
		if start.TemplateGameUUID != "" {
			return startingRevision{}, fmt.Errorf("version %s of template %s: %w", fromVersionUUID, sourceGame, sessionpkg.ErrDraftTemplateNotFound)
		}
		return startingRevision{}, fmt.Errorf("version %s is not an executable version of game %s with sources to copy", fromVersionUUID, gameUUID)
	}
	copied, found, err := m.createDraftVersionRepo.FindRevision(ctx, tx, from.ID, *from.CurrentRevision)
	if err != nil {
		return startingRevision{}, err
	}
	if !found {
		monitoring.Alert(ctx, "play executable version is missing the revision it was frozen at")
		return startingRevision{}, fmt.Errorf("version %s has no revision %d", fromVersionUUID, *from.CurrentRevision)
	}
	provenance := internalrepo.Provenance{
		Kind: kind, Actor: sessionpkg.DraftActorPlatform, SDK: string(scriptcheck.ProfileSDKV1),
		FromGameUUID: sourceGame, FromVersionUUID: from.UUID, FromRevision: from.CurrentRevision,
	}
	return startingRevision{revision: copied, provenance: provenance}, nil
}

func draftRevision(ctx context.Context, version *internalrepo.Version) (int64, error) {
	if version.CurrentRevision == nil {
		monitoring.Alert(ctx, "play draft version has no current revision")
		return 0, fmt.Errorf("draft version %s has no current revision", version.UUID)
	}
	return *version.CurrentRevision, nil
}
